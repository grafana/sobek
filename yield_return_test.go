package sobek

import (
	"testing"
)

// TestGeneratorYieldInFinally tests that generators properly handle
// yield statements inside finally blocks when return() is called.
//
// This is a critical feature for implementing structured concurrency
// cleanup patterns where cleanup code may need to suspend (yield)
// while performing async cleanup operations.
//
// Per ECMAScript spec, when generator.return(value) is called:
// 1. If the generator has finally blocks, they must execute
// 2. If a finally block contains a yield, the generator should suspend
// 3. Resuming the generator should continue executing the finally block
// 4. Only after all finally blocks complete should the generator be done
//
// See: https://tc39.es/ecma262/#sec-generator.prototype.return
func TestGeneratorYieldInFinally(t *testing.T) {
	vm := New()

	// Test: yield in finally block during return()
	script := `
		let cleanupYielded = false;
		let cleanupCompleted = false;

		function* withCleanupYield() {
			try {
				yield "working";
				return "done";
			} finally {
				yield "cleanup";  // This yield should suspend the generator
				cleanupYielded = true;
				cleanupCompleted = true;
			}
		}

		const gen = withCleanupYield();
		const result1 = gen.next();  // Should yield "working"

		if (result1.value !== "working" || result1.done !== false) {
			throw new Error("Test setup failed: expected {value: 'working', done: false}");
		}

		const result2 = gen.return("cancelled");  // Should yield "cleanup" from finally

		// Per spec, the generator should suspend at "yield cleanup" in the finally block
		// result2 should be {value: "cleanup", done: false}
		const yieldInFinallyWorks = result2.value === "cleanup" && result2.done === false;

		// If it doesn't work, Sobek likely returns {value: "cancelled", done: true}
		// skipping the yield in finally entirely
		const skipsFinallyYield = result2.value === "cancelled" && result2.done === true;

		({
			yieldInFinallyWorks,
			skipsFinallyYield,
			result2Value: result2.value,
			result2Done: result2.done,
			cleanupYielded,
			cleanupCompleted
		})
	`

	result, err := vm.RunString(script)
	if err != nil {
		t.Fatalf("Script error: %v", err)
	}

	obj := result.ToObject(vm)

	yieldInFinallyWorks := obj.Get("yieldInFinallyWorks").ToBoolean()
	skipsFinallyYield := obj.Get("skipsFinallyYield").ToBoolean()
	result2Value := obj.Get("result2Value").String()
	result2Done := obj.Get("result2Done").ToBoolean()

	if skipsFinallyYield {
		t.Errorf("BUG: Generator skips yield in finally block during return()")
		t.Errorf("  Expected: {value: 'cleanup', done: false}")
		t.Errorf("  Got:      {value: '%s', done: %v}", result2Value, result2Done)
		t.Error("  The finally block's yield should suspend the generator, not be skipped")
	}

	if !yieldInFinallyWorks {
		t.Errorf("yield in finally does not work correctly")
		t.Errorf("  Expected: {value: 'cleanup', done: false}")
		t.Errorf("  Got:      {value: '%s', done: %v}", result2Value, result2Done)
	}
}

// TestGeneratorYieldInFinallyWithDelegation tests yield* delegation
// combined with yield in finally during return(). This is even more
// critical for structured concurrency as cleanup often involves
// delegating to other generators.
func TestGeneratorYieldInFinallyWithDelegation(t *testing.T) {
	vm := New()

	script := `
		let cleanupOrder = [];

		function* inner() {
			try {
				yield "inner-work";
				return "inner-done";
			} finally {
				yield "inner-cleanup";
				cleanupOrder.push("inner");
			}
		}

		function* outer() {
			try {
				const result = yield* inner();
				return result;
			} finally {
				yield "outer-cleanup";
				cleanupOrder.push("outer");
			}
		}

		const gen = outer();
		const r1 = gen.next();  // "inner-work"
		
		if (r1.value !== "inner-work") {
			throw new Error("Setup failed");
		}

		const r2 = gen.return("cancelled");  // Should yield "inner-cleanup"
		
		// Expected behavior per spec:
		// r2 = {value: "inner-cleanup", done: false}
		const worksCorrectly = r2.value === "inner-cleanup" && r2.done === false;
		
		({
			worksCorrectly,
			r2Value: r2.value,
			r2Done: r2.done,
			cleanupOrder: cleanupOrder.join(",")
		})
	`

	result, err := vm.RunString(script)
	if err != nil {
		t.Fatalf("Script error: %v", err)
	}

	obj := result.ToObject(vm)
	worksCorrectly := obj.Get("worksCorrectly").ToBoolean()
	r2Value := obj.Get("r2Value").String()
	r2Done := obj.Get("r2Done").ToBoolean()

	if !worksCorrectly {
		t.Errorf("BUG: yield* delegation with yield in finally fails during return()")
		t.Errorf("  Expected: {value: 'inner-cleanup', done: false}")
		t.Errorf("  Got:      {value: '%s', done: %v}", r2Value, r2Done)
	}
}

func TestGeneratorReturnNestedFinallyYields(t *testing.T) {
	vm := New()

	script := `
		function* nestedCleanup() {
			try {
				try {
					yield "work";
				} finally {
					yield "inner-cleanup";
				}
			} finally {
				yield "outer-cleanup";
			}
		}

		const gen = nestedCleanup();
		const r1 = gen.next();
		const r2 = gen.return("cancelled");
		const r3 = gen.next();
		const r4 = gen.next();

		({
			r1Value: r1.value,
			r1Done: r1.done,
			r2Value: r2.value,
			r2Done: r2.done,
			r3Value: r3.value,
			r3Done: r3.done,
			r4Value: r4.value,
			r4Done: r4.done
		})
	`

	result, err := vm.RunString(script)
	if err != nil {
		t.Fatalf("Script error: %v", err)
	}

	obj := result.ToObject(vm)
	if got := obj.Get("r1Value").String(); got != "work" {
		t.Fatalf("r1.value = %q, want %q", got, "work")
	}
	if got := obj.Get("r2Value").String(); got != "inner-cleanup" {
		t.Fatalf("r2.value = %q, want %q", got, "inner-cleanup")
	}
	if got := obj.Get("r3Value").String(); got != "outer-cleanup" {
		t.Fatalf("r3.value = %q, want %q", got, "outer-cleanup")
	}
	if got := obj.Get("r4Value").String(); got != "cancelled" {
		t.Fatalf("r4.value = %q, want %q", got, "cancelled")
	}
	if obj.Get("r2Done").ToBoolean() || obj.Get("r3Done").ToBoolean() || !obj.Get("r4Done").ToBoolean() {
		t.Fatal("unexpected done flags for nested finally return flow")
	}
}

func TestGeneratorReturnYieldResInFinally(t *testing.T) {
	vm := New()

	script := `
		let cleanupInput;

		function* cleanupNeedsAck() {
			try {
				yield "work";
			} finally {
				cleanupInput = yield "cleanup-1";
				yield "cleanup-2";
			}
		}

		const gen = cleanupNeedsAck();
		const r1 = gen.next();
		const r2 = gen.return("cancelled");
		const r3 = gen.next("ack");
		const r4 = gen.next();

		({
			r1Value: r1.value,
			r2Value: r2.value,
			r2Done: r2.done,
			r3Value: r3.value,
			r3Done: r3.done,
			r4Value: r4.value,
			r4Done: r4.done,
			cleanupInput
		})
	`

	result, err := vm.RunString(script)
	if err != nil {
		t.Fatalf("Script error: %v", err)
	}

	obj := result.ToObject(vm)
	if got := obj.Get("r2Value").String(); got != "cleanup-1" {
		t.Fatalf("r2.value = %q, want %q", got, "cleanup-1")
	}
	if got := obj.Get("r3Value").String(); got != "cleanup-2" {
		t.Fatalf("r3.value = %q, want %q", got, "cleanup-2")
	}
	if got := obj.Get("r4Value").String(); got != "cancelled" {
		t.Fatalf("r4.value = %q, want %q", got, "cancelled")
	}
	if got := obj.Get("cleanupInput").String(); got != "ack" {
		t.Fatalf("cleanupInput = %q, want %q", got, "ack")
	}
	if obj.Get("r2Done").ToBoolean() || obj.Get("r3Done").ToBoolean() || !obj.Get("r4Done").ToBoolean() {
		t.Fatal("unexpected done flags for yieldRes finally return flow")
	}
}

func TestGeneratorReturnFinallyThrowsBeforeYield(t *testing.T) {
	vm := New()

	script := `
		let caught;

		function* genWithThrowingCleanup() {
			try {
				yield "work";
			} finally {
				throw new Error("cleanup-failed");
			}
		}

		const gen = genWithThrowingCleanup();
		gen.next();

		try {
			gen.return("cancelled");
		} catch (e) {
			caught = e.message;
		}

		({ caught })
	`

	result, err := vm.RunString(script)
	if err != nil {
		t.Fatalf("Script error: %v", err)
	}

	obj := result.ToObject(vm)
	if got := obj.Get("caught").String(); got != "cleanup-failed" {
		t.Fatalf("caught = %q, want %q", got, "cleanup-failed")
	}
}

func TestGeneratorReturnFinallyThrowsAfterYield(t *testing.T) {
	vm := New()

	script := `
		let caught;

		function* genWithSuspendingThrowingCleanup() {
			try {
				yield "work";
			} finally {
				yield "cleanup";
				throw new Error("cleanup-failed-after-yield");
			}
		}

		const gen = genWithSuspendingThrowingCleanup();
		const r1 = gen.next();
		const r2 = gen.return("cancelled");
		try {
			gen.next();
		} catch (e) {
			caught = e.message;
		}

		({ r1Value: r1.value, r2Value: r2.value, r2Done: r2.done, caught })
	`

	result, err := vm.RunString(script)
	if err != nil {
		t.Fatalf("Script error: %v", err)
	}

	obj := result.ToObject(vm)
	if got := obj.Get("r1Value").String(); got != "work" {
		t.Fatalf("r1.value = %q, want %q", got, "work")
	}
	if got := obj.Get("r2Value").String(); got != "cleanup" {
		t.Fatalf("r2.value = %q, want %q", got, "cleanup")
	}
	if obj.Get("r2Done").ToBoolean() {
		t.Fatal("r2.done = true, want false")
	}
	if got := obj.Get("caught").String(); got != "cleanup-failed-after-yield" {
		t.Fatalf("caught = %q, want %q", got, "cleanup-failed-after-yield")
	}
}

func TestGeneratorReturnFinallyReturnOverridesValue(t *testing.T) {
	vm := New()

	script := `
		function* genWithOverride() {
			try {
				yield "work";
			} finally {
				return "cleanup-override";
			}
		}

		const gen = genWithOverride();
		gen.next();
		const r = gen.return("cancelled");
		({ value: r.value, done: r.done })
	`

	result, err := vm.RunString(script)
	if err != nil {
		t.Fatalf("Script error: %v", err)
	}

	obj := result.ToObject(vm)
	if got := obj.Get("value").String(); got != "cleanup-override" {
		t.Fatalf("r.value = %q, want %q", got, "cleanup-override")
	}
	if !obj.Get("done").ToBoolean() {
		t.Fatal("r.done = false, want true")
	}
}

func TestGeneratorReturnFinallyYieldStar(t *testing.T) {
	vm := New()

	script := `
		function* delegatedCleanup() {
			yield "cleanup-1";
			yield "cleanup-2";
		}

		function* withYieldStarCleanup() {
			try {
				yield "work";
			} finally {
				yield* delegatedCleanup();
			}
		}

		const gen = withYieldStarCleanup();
		const r1 = gen.next();
		const r2 = gen.return("cancelled");
		const r3 = gen.next();
		const r4 = gen.next();

		({
			r1Value: r1.value,
			r2Value: r2.value,
			r2Done: r2.done,
			r3Value: r3.value,
			r3Done: r3.done,
			r4Value: r4.value,
			r4Done: r4.done
		})
	`

	result, err := vm.RunString(script)
	if err != nil {
		t.Fatalf("Script error: %v", err)
	}

	obj := result.ToObject(vm)
	if got := obj.Get("r1Value").String(); got != "work" {
		t.Fatalf("r1.value = %q, want %q", got, "work")
	}
	if got := obj.Get("r2Value").String(); got != "cleanup-1" {
		t.Fatalf("r2.value = %q, want %q", got, "cleanup-1")
	}
	if obj.Get("r2Done").ToBoolean() {
		t.Fatal("r2.done = true, want false")
	}
	if got := obj.Get("r3Value").String(); got != "cleanup-2" {
		t.Fatalf("r3.value = %q, want %q", got, "cleanup-2")
	}
	if obj.Get("r3Done").ToBoolean() {
		t.Fatal("r3.done = true, want false")
	}
	if got := obj.Get("r4Value").String(); got != "cancelled" {
		t.Fatalf("r4.value = %q, want %q", got, "cancelled")
	}
	if !obj.Get("r4Done").ToBoolean() {
		t.Fatal("r4.done = false, want true")
	}
}

func TestGeneratorReturnBeforeStart(t *testing.T) {
	vm := New()

	script := `
		let entered = false;
		function* neverStarted() {
			entered = true;
			yield "work";
		}

		const gen = neverStarted();
		const r = gen.return("cancelled");
		({ value: r.value, done: r.done, entered })
	`

	result, err := vm.RunString(script)
	if err != nil {
		t.Fatalf("Script error: %v", err)
	}

	obj := result.ToObject(vm)
	if got := obj.Get("value").String(); got != "cancelled" {
		t.Fatalf("r.value = %q, want %q", got, "cancelled")
	}
	if !obj.Get("done").ToBoolean() {
		t.Fatal("r.done = false, want true")
	}
	if obj.Get("entered").ToBoolean() {
		t.Fatal("entered = true, want false")
	}
}
