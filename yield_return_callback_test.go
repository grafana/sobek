package sobek

import (
	"testing"
)

// mockTimerCallback simulates K6's event loop timer callback mechanism.
// It runs a callback after a delay, using Sobek's interrupt mechanism
// to execute JavaScript code from a "foreign" context.
type mockEventLoop struct {
	vm      *Runtime
	pending []func()
}

func newMockEventLoop(vm *Runtime) *mockEventLoop {
	return &mockEventLoop{vm: vm}
}

func (el *mockEventLoop) setTimeout(call FunctionCall) Value {
	callback, ok := AssertFunction(call.Argument(0))
	if !ok {
		panic(el.vm.NewTypeError("setTimeout: callback is not a function"))
	}

	// Queue the callback
	el.pending = append(el.pending, func() {
		_, _ = callback(Undefined())
	})

	return Undefined()
}

func (el *mockEventLoop) runPending() {
	for len(el.pending) > 0 {
		cb := el.pending[0]
		el.pending = el.pending[1:]
		cb()
	}
}

// TestGeneratorReturnFromEventLoop tests generator.return() being called
// from within an event loop callback (like K6's timers/WebSocket handlers).
// This more closely simulates the actual K6 failure scenario.
func TestGeneratorReturnFromEventLoop(t *testing.T) {
	vm := New()
	eventLoop := newMockEventLoop(vm)

	// Expose setTimeout to JavaScript
	vm.Set("setTimeout", eventLoop.setTimeout)

	// Create generator and schedule return() to be called from timer callback
	_, err := vm.RunString(`
		var gen;
		var returnResult;
		var nextResult;
		var error;
		
		function* myGen() {
			try {
				yield "working";
			} finally {
				yield "cleanup";
			}
		}
		
		gen = myGen();
		gen.next(); // Start generator
		
		// Schedule return() to be called from "timer callback"
		setTimeout(function() {
			try {
				returnResult = gen.return("cancelled");
			} catch (e) {
				error = e;
			}
		});
	`)
	if err != nil {
		t.Fatalf("Setup failed: %v", err)
	}

	// Run the event loop - this executes the setTimeout callback
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("PANIC during event loop: %v\n"+
					"This indicates the callback context bug", r)
			}
		}()
		eventLoop.runPending()
	}()

	// Check if return worked
	errorVal, _ := vm.RunString(`error`)
	if !IsUndefined(errorVal) {
		t.Fatalf("gen.return() threw error: %v", errorVal)
	}

	returnResult, _ := vm.RunString(`returnResult`)
	if IsUndefined(returnResult) {
		t.Fatal("returnResult is undefined - gen.return() didn't complete")
	}

	resultObj := returnResult.ToObject(vm)
	value := resultObj.Get("value").String()
	done := resultObj.Get("done").ToBoolean()

	if value != "cleanup" || done != false {
		t.Errorf("returnResult = {value: %q, done: %v}, want {value: 'cleanup', done: false}",
			value, done)
	}

	// Now continue the generator from another timer callback
	_, err = vm.RunString(`
		setTimeout(function() {
			try {
				nextResult = gen.next();
			} catch (e) {
				error = e;
			}
		});
	`)
	if err != nil {
		t.Fatalf("Failed to schedule next(): %v", err)
	}

	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("PANIC during second event loop iteration: %v\n"+
					"This is the callback context bug - vm.sp corrupted after completeReturnYield", r)
			}
		}()
		eventLoop.runPending()
	}()

	errorVal, _ = vm.RunString(`error`)
	if !IsUndefined(errorVal) {
		t.Fatalf("gen.next() threw error: %v", errorVal)
	}

	nextResult, _ := vm.RunString(`nextResult`)
	if IsUndefined(nextResult) {
		t.Fatal("nextResult is undefined")
	}

	resultObj = nextResult.ToObject(vm)
	value = resultObj.Get("value").String()
	done = resultObj.Get("done").ToBoolean()

	if value != "cancelled" || done != true {
		t.Errorf("nextResult = {value: %q, done: %v}, want {value: 'cancelled', done: true}",
			value, done)
	}
}

// TestGeneratorReturnFromPromiseHandler tests generator.return() being called
// from within a Promise resolution handler, which is what happens when K6
// runs Effection operations that clean up on scope exit.
func TestGeneratorReturnFromPromiseHandler(t *testing.T) {
	vm := New()

	_, err := vm.RunString(`
		var gen;
		var results = [];
		var finalResult = null;
		
		function* myGen() {
			try {
				yield "working";
			} finally {
				yield "cleanup";
				results.push("cleanup-done");
			}
		}
		
		gen = myGen();
		gen.next(); // Start generator
		
		// Call gen.return() from within a Promise.then() handler
		// This simulates the K6 event loop Promise handling
		Promise.resolve().then(function() {
			results.push("in-promise-handler");
			var r1 = gen.return("cancelled");
			results.push("r1:" + r1.value + ":" + r1.done);
			
			// Continue after yield in finally - still in promise handler
			var r2 = gen.next();
			results.push("r2:" + r2.value + ":" + r2.done);
			
			finalResult = "done";
		});
	`)
	if err != nil {
		t.Fatalf("Setup failed: %v", err)
	}

	// Need to let the promise handler run - use a microtask drain
	// In Sobek, promise handlers run via the job queue which needs to be processed
	vm.RunString(`null`) // Trigger job queue processing

	results, _ := vm.RunString(`results.join(", ")`)
	expected := "in-promise-handler, r1:cleanup:false, cleanup-done, r2:cancelled:true"
	if results.String() != expected {
		t.Errorf("Results = %q, want %q", results.String(), expected)
	}

	finalResult, _ := vm.RunString(`finalResult`)
	if finalResult.String() != "done" {
		t.Error("Promise handler did not complete")
	}
}

// TestGeneratorReturnFromAsyncFunction tests generator.return() being called
// from within an async function, which is how K6 VU iterations work.
// The async function creates a different VM stack context.
func TestGeneratorReturnFromAsyncFunction(t *testing.T) {
	vm := New()

	// Track panics in promise rejection handlers
	var panicMsg interface{}
	vm.Set("reportPanic", func(call FunctionCall) Value {
		panicMsg = call.Argument(0).Export()
		return Undefined()
	})

	_, err := vm.RunString(`
		var gen;
		var results = [];
		var done = false;
		var error = null;
		
		function* myGen() {
			try {
				yield "working";
			} finally {
				yield "cleanup";
				results.push("cleanup-done");
			}
		}
		
		async function runTest() {
			gen = myGen();
			gen.next(); // Start generator, suspended at "working"
			
			// Call return from async context
			var r1 = gen.return("cancelled");
			results.push("r1:" + r1.value + ":" + r1.done);
			
			// Continue after yield in finally
			var r2 = gen.next();
			results.push("r2:" + r2.value + ":" + r2.done);
			
			done = true;
			return "completed";
		}
		
		runTest().catch(function(e) {
			error = e;
		});
	`)
	if err != nil {
		t.Fatalf("Setup failed: %v", err)
	}

	// The async function should have started
	if panicMsg != nil {
		t.Fatalf("Panic occurred: %v", panicMsg)
	}

	// Check results
	errorVal, _ := vm.RunString(`error`)
	if !IsUndefined(errorVal) && !IsNull(errorVal) {
		t.Fatalf("Async function error: %v", errorVal)
	}

	doneVal, _ := vm.RunString(`done`)
	if !doneVal.ToBoolean() {
		// Async function didn't complete - might have panicked
		results, _ := vm.RunString(`results.join(", ")`)
		t.Fatalf("Async function did not complete. Results so far: %s", results.String())
	}

	results, _ := vm.RunString(`results.join(", ")`)
	// cleanup-done happens during the gen.next() call before we capture r2
	expected := "r1:cleanup:false, cleanup-done, r2:cancelled:true"
	if results.String() != expected {
		t.Errorf("Results = %q, want %q", results.String(), expected)
	}
}

// TestGeneratorReturnFromCallbackContext verifies that generator.return()
// works correctly when invoked from a Go callback context where the VM
// stack state might be at base level (sb=0 or sb=1).
//
// This tests a bug where completeReturnYield() used vm.sb from the callback
// context instead of the generator's own suspended frame state, causing
// vm.sp to become negative and crashing on subsequent resume().
//
// The scenario:
// 1. Generator created and started in main context
// 2. Generator yields and suspends
// 3. Go code (simulating a callback like K6's timer/WebSocket handler) calls gen.return()
// 4. Generator has yield in finally block, so it should suspend during cleanup
// 5. Go code calls gen.next() to continue
// 6. This should not panic with "slice bounds out of range [-1:]"
func TestGeneratorReturnFromCallbackContext(t *testing.T) {
	vm := New()

	// Create and start a generator with yield in finally
	_, err := vm.RunString(`
		var gen;
		var results = [];
		
		function* myGen() {
			try {
				yield "working";
				return "done";
			} finally {
				yield "cleanup";
				results.push("cleanup-done");
			}
		}
		
		gen = myGen();
		var startResult = gen.next(); // Start it, now suspended at "working"
		
		if (startResult.value !== "working" || startResult.done !== false) {
			throw new Error("Setup failed: expected {value: 'working', done: false}, got " + 
				JSON.stringify(startResult));
		}
	`)
	if err != nil {
		t.Fatalf("Setup failed: %v", err)
	}

	// Get the generator object
	genVal, err := vm.RunString(`gen`)
	if err != nil {
		t.Fatalf("Failed to get generator: %v", err)
	}
	genObj := genVal.ToObject(vm)

	// Get generator.return as a callable function
	returnMethod := genObj.Get("return")
	returnFn, ok := AssertFunction(returnMethod)
	if !ok {
		t.Fatal("gen.return is not a function")
	}

	// Call gen.return() from Go context - this simulates being called
	// from a callback context (like K6's event loop) with minimal stack frame.
	// This is the key part that triggers the bug.
	var returnResult Value
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("PANIC during gen.return(): %v\n"+
					"This indicates the callback context bug - vm.sp became negative", r)
			}
		}()
		returnResult, err = returnFn(genVal, vm.ToValue("cancelled"))
	}()
	if err != nil {
		t.Fatalf("gen.return() failed: %v", err)
	}

	// Should yield "cleanup" from finally, not complete immediately
	returnResultObj := returnResult.ToObject(vm)
	returnValue := returnResultObj.Get("value").String()
	returnDone := returnResultObj.Get("done").ToBoolean()

	if returnValue != "cleanup" || returnDone != false {
		t.Errorf("gen.return() result: got {value: %q, done: %v}, want {value: 'cleanup', done: false}",
			returnValue, returnDone)
	}

	// Now call gen.next() to continue after the yield in finally
	// This is where the original bug would panic during resume()
	nextMethod := genObj.Get("next")
	nextFn, ok := AssertFunction(nextMethod)
	if !ok {
		t.Fatal("gen.next is not a function")
	}

	var nextResult Value
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("PANIC during gen.next() after return: %v\n"+
					"This indicates the callback context bug - vm.sp was corrupted", r)
			}
		}()
		nextResult, err = nextFn(genVal)
	}()
	if err != nil {
		t.Fatalf("gen.next() after return failed: %v", err)
	}

	// Should now be done with the return value
	nextResultObj := nextResult.ToObject(vm)
	nextValue := nextResultObj.Get("value").String()
	nextDone := nextResultObj.Get("done").ToBoolean()

	if nextValue != "cancelled" || nextDone != true {
		t.Errorf("gen.next() after return: got {value: %q, done: %v}, want {value: 'cancelled', done: true}",
			nextValue, nextDone)
	}

	// Verify cleanup completed
	results, err := vm.RunString(`results`)
	if err != nil {
		t.Fatalf("Failed to get results: %v", err)
	}
	resultsObj := results.ToObject(vm)
	length := resultsObj.Get("length").ToInteger()
	if length != 1 {
		t.Errorf("results.length = %d, want 1", length)
	}
	if length > 0 {
		firstResult := resultsObj.Get("0").String()
		if firstResult != "cleanup-done" {
			t.Errorf("results[0] = %q, want 'cleanup-done'", firstResult)
		}
	}
}

// TestGeneratorReturnFromCallbackWithDelegation tests the callback context
// bug with yield* delegation chains.
func TestGeneratorReturnFromCallbackWithDelegation(t *testing.T) {
	vm := New()

	_, err := vm.RunString(`
		var gen;
		var cleanupOrder = [];
		
		function* inner() {
			try {
				yield "inner-work";
			} finally {
				yield "inner-cleanup";
				cleanupOrder.push("inner");
			}
		}
		
		function* outer() {
			try {
				yield* inner();
			} finally {
				yield "outer-cleanup";
				cleanupOrder.push("outer");
			}
		}
		
		gen = outer();
		gen.next(); // Suspended at "inner-work"
	`)
	if err != nil {
		t.Fatalf("Setup failed: %v", err)
	}

	genVal, _ := vm.RunString(`gen`)
	genObj := genVal.ToObject(vm)

	returnFn, _ := AssertFunction(genObj.Get("return"))
	nextFn, _ := AssertFunction(genObj.Get("next"))

	// Call return from Go context
	var result Value
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("PANIC during gen.return(): %v", r)
			}
		}()
		result, err = returnFn(genVal, vm.ToValue("cancelled"))
	}()
	if err != nil {
		t.Fatalf("gen.return() failed: %v", err)
	}

	// Should yield "inner-cleanup" first
	resultObj := result.ToObject(vm)
	if got := resultObj.Get("value").String(); got != "inner-cleanup" {
		t.Errorf("first return result value = %q, want 'inner-cleanup'", got)
	}

	// Continue to outer cleanup
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("PANIC during gen.next() #1: %v", r)
			}
		}()
		result, err = nextFn(genVal)
	}()
	if err != nil {
		t.Fatalf("gen.next() #1 failed: %v", err)
	}

	resultObj = result.ToObject(vm)
	if got := resultObj.Get("value").String(); got != "outer-cleanup" {
		t.Errorf("second result value = %q, want 'outer-cleanup'", got)
	}

	// Continue to completion
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("PANIC during gen.next() #2: %v", r)
			}
		}()
		result, err = nextFn(genVal)
	}()
	if err != nil {
		t.Fatalf("gen.next() #2 failed: %v", err)
	}

	resultObj = result.ToObject(vm)
	if got := resultObj.Get("value").String(); got != "cancelled" {
		t.Errorf("final result value = %q, want 'cancelled'", got)
	}
	if !resultObj.Get("done").ToBoolean() {
		t.Error("final result done = false, want true")
	}

	// Verify cleanup order
	order, _ := vm.RunString(`cleanupOrder.join(",")`)
	if got := order.String(); got != "inner,outer" {
		t.Errorf("cleanup order = %q, want 'inner,outer'", got)
	}
}

// TestGeneratorReturnFromCallbackNestedFinally tests multiple levels
// of try/finally with yields during return from callback context.
func TestGeneratorReturnFromCallbackNestedFinally(t *testing.T) {
	vm := New()

	_, err := vm.RunString(`
		var gen;
		var steps = [];
		
		function* nestedCleanup() {
			try {
				try {
					yield "work";
				} finally {
					yield "inner-cleanup";
					steps.push("inner");
				}
			} finally {
				yield "outer-cleanup";
				steps.push("outer");
			}
		}
		
		gen = nestedCleanup();
		gen.next(); // Suspended at "work"
	`)
	if err != nil {
		t.Fatalf("Setup failed: %v", err)
	}

	genVal, _ := vm.RunString(`gen`)
	genObj := genVal.ToObject(vm)

	returnFn, _ := AssertFunction(genObj.Get("return"))
	nextFn, _ := AssertFunction(genObj.Get("next"))

	values := []string{}

	// Call return from Go context
	var result Value
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("PANIC during gen.return(): %v", r)
			}
		}()
		result, err = returnFn(genVal, vm.ToValue("cancelled"))
	}()
	if err != nil {
		t.Fatalf("gen.return() failed: %v", err)
	}
	values = append(values, result.ToObject(vm).Get("value").String())

	// Continue through all yields
	for i := 0; i < 3; i++ {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("PANIC during gen.next() #%d: %v", i+1, r)
				}
			}()
			result, err = nextFn(genVal)
		}()
		if err != nil {
			t.Fatalf("gen.next() #%d failed: %v", i+1, err)
		}
		resultObj := result.ToObject(vm)
		values = append(values, resultObj.Get("value").String())
		if resultObj.Get("done").ToBoolean() {
			break
		}
	}

	// Expected sequence: inner-cleanup, outer-cleanup, cancelled
	expected := []string{"inner-cleanup", "outer-cleanup", "cancelled"}
	if len(values) != len(expected) {
		t.Errorf("got %d values, want %d: %v", len(values), len(expected), values)
	} else {
		for i, want := range expected {
			if values[i] != want {
				t.Errorf("values[%d] = %q, want %q", i, values[i], want)
			}
		}
	}

	// Verify cleanup order
	steps, _ := vm.RunString(`steps.join(",")`)
	if got := steps.String(); got != "inner,outer" {
		t.Errorf("steps = %q, want 'inner,outer'", got)
	}
}
