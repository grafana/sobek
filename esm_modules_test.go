package sobek

import (
	"fmt"
	"sync"
	"testing"
)

// runModulesWithESMConfig is like runModules but uses ESMConfig instead of
// SetImportModuleDynamically. Each call to the returned function creates a fresh
// ESMConfig and attaches it to the given vm.
func runModulesWithESMConfig(t testing.TB, files map[string]string) func(*Runtime) *ModulePromise {
	type cacheElement struct {
		m   ModuleRecord
		err error
	}
	mu := sync.Mutex{}
	cache := make(map[string]cacheElement)
	var hostResolveImportedModule func(referencingScriptOrModule interface{}, specifier string) (ModuleRecord, error)
	hostResolveImportedModule = func(_ interface{}, specifier string) (ModuleRecord, error) {
		mu.Lock()
		defer mu.Unlock()
		k, ok := cache[specifier]
		if ok {
			return k.m, k.err
		}

		src, ok := files[specifier]
		if !ok {
			return nil, fmt.Errorf("can't find %q from files", specifier)
		}
		p, err := ParseModule(specifier, src, hostResolveImportedModule)
		if err != nil {
			cache[specifier] = cacheElement{err: err}
			return nil, err
		}
		cache[specifier] = cacheElement{m: p}
		return p, nil
	}

	linked := make(map[ModuleRecord]error)
	linkMu := new(sync.Mutex)
	link := func(m ModuleRecord) error {
		linkMu.Lock()
		defer linkMu.Unlock()
		if err, ok := linked[m]; ok {
			return err
		}
		err := m.Link()
		linked[m] = err
		return err
	}

	m, err := hostResolveImportedModule(nil, "a.js")
	if err != nil {
		t.Fatalf("got error %s", err)
	}
	p := m.(*SourceTextModuleRecord)

	err = link(p)
	if err != nil {
		t.Fatalf("got error %s", err)
	}

	return func(vm *Runtime) *ModulePromise {
		eventLoopQueue := make(chan func(), 2)
		config := NewESMConfig().
			WithHostResolveImportedModule(hostResolveImportedModule).
			WithImportModuleDynamically(func(referencingScriptOrModule interface{}, specifierValue Value, pcap interface{}) {
				specifier := specifierValue.String()
				eventLoopQueue <- func() {
					ex := vm.runWrapped(func() {
						m, err := hostResolveImportedModule(referencingScriptOrModule, specifier)
						vm.FinishLoadingImportModule(referencingScriptOrModule, specifierValue, pcap, m, err)
					})
					if ex != nil {
						vm.FinishLoadingImportModule(referencingScriptOrModule, specifierValue, pcap, nil, ex)
					}
				}
			})
		vm.AttachESM(config)

		var mp *ModulePromise
		eventLoopQueue <- func() { mp = config.EvaluateModule(p) }

	outer:
		for {
			select {
			case fn := <-eventLoopQueue:
				fn()
			default:
				break outer
			}
		}
		return mp
	}
}

func TestSimpleModuleESMConfig(t *testing.T) {
	t.Parallel()

	testCases := map[string]map[string]string{
		"function export": {
			"a.js": `
				import { b } from "dep.js";
				globalThis.s = b()
			`,
			"dep.js": `export function b() { return 5 };`,
		},
		"default export": {
			"a.js": `
				import b from "dep.js";
				globalThis.s = b()
			`,
			"dep.js": `export default function() { return 5 };`,
		},
		"dynamic import": {
			"a.js": `
				import("dep.js").then((imported) => {
					globalThis.s = imported.default();
				});
			`,
			"dep.js": `export default function() { return 5; }`,
		},
	}
	for name, cases := range testCases {
		cases := cases
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			fn := runModulesWithESMConfig(t, cases)
			vm := New()
			mp := fn(vm)
			if err := mp.Err(); err != nil {
				t.Fatalf("got %s", err)
			}
			v := vm.Get("s")
			if v == nil || v.ToNumber().ToInteger() != 5 {
				t.Fatalf("expected 5 got %s", v)
			}
		})
	}
}

func TestAmbiguousImportESMConfig(t *testing.T) {
	t.Parallel()
	fn := runModulesWithESMConfig(t, map[string]string{
		`a.js`: `
			import "dep.js"
			export let s = 5;
			export * from "test1.js"
			export * from "test2.js"
		`,
		`dep.js`: `
			import { s } from "a.js"
			import { x } from "a.js"
		`,
		`test1.js`: `
			export let x = 6
			export let a = 6
		`,
		`test2.js`: `
			export let x = 6
			export let b = 6
		`,
	})
	mp := fn(New())
	if mp.State() != PromiseStateRejected {
		t.Fatalf("expected promise to be rejected, got %q", mp.State())
	}
	exc := mp.Promise().Result().Export().(*Exception)
	expValue := `SyntaxError: The requested module "a.js" contains conflicting star exports for name "x"
	at dep.js:3:5(2)
	at dep.js:1:1(2)
`
	if exc.String() != expValue {
		t.Fatalf("Expected values %q but got %q", expValue, exc.String())
	}
}

func TestImportingUnexportedESMConfig(t *testing.T) {
	t.Parallel()
	fn := runModulesWithESMConfig(t, map[string]string{
		`a.js`: `
			import "dep.js"
			export let s = 5;
		`,
		`dep.js`: `
			import { s } from "a.js"
			import { x } from "a.js"
		`,
	})
	mp := fn(New())
	if mp.State() != PromiseStateRejected {
		t.Fatalf("expected promise to be rejected, got %q", mp.State())
	}
	exc := mp.Promise().Result().Export().(*Exception)
	expValue := `SyntaxError: The requested module "a.js" does not provide an export named "x"
	at dep.js:3:5(2)
	at dep.js:1:1(2)
`
	if exc.String() != expValue {
		t.Fatalf("Expected values %q but got %q", expValue, exc.String())
	}
}

func TestModuleAsyncErrorAndPromiseRejectionESMConfig(t *testing.T) {
	t.Parallel()
	fn := runModulesWithESMConfig(t, map[string]string{
		`a.js`: `
			import "dep.js"
			something;
		`,
		`dep.js`: `
			await 5;
		`,
	})

	rt := New()
	unhandledRejectedPromises := make(map[*Promise]struct{})
	rt.promiseRejectionTracker = func(p *Promise, operation PromiseRejectionOperation) {
		switch operation {
		case PromiseRejectionReject:
			unhandledRejectedPromises[p] = struct{}{}
		case PromiseRejectionHandle:
			delete(unhandledRejectedPromises, p)
		}
	}
	mp := fn(rt)
	rt.performPromiseThen(mp.Promise(), rt.ToValue(func() {}), rt.ToValue(func() {}), nil)
	if mp.State() != PromiseStateRejected {
		t.Fatalf("expected promise to be rejected, got %q", mp.State())
	}
	exc := mp.Promise().Result().Export().(*Exception)
	expValue := "ReferenceError: something is not defined\n\tat a.js:3:4(6)\n"
	if exc.String() != expValue {
		t.Fatalf("Expected values %q but got %q", expValue, exc.String())
	}

	if len(unhandledRejectedPromises) != 0 {
		t.Fatalf("zero unhandled exceptions were expected but there were some %+v", unhandledRejectedPromises)
	}
}

func TestModuleAsyncInterruptESMConfig(t *testing.T) {
	t.Parallel()
	fn := runModulesWithESMConfig(t, map[string]string{
		`a.js`: `
			import { s } from "dep.js"
			s();
		`,
		`dep.js`: `
			await 5;
			export function s() {
				interrupt();
				let buf = "";
				for (let i = 0; i < 10000; i++) {
					buf += "a" + "a";
				}
				badcall();
			}
		`,
	})

	rt := New()
	var shouldntHappen bool
	rt.Set("interrupt", rt.ToValue(func() { rt.Interrupt("the error we want") }))
	rt.Set("badcall", rt.ToValue(func() { shouldntHappen = true }))

	unhandledRejectedPromises := make(map[*Promise]struct{})
	rt.promiseRejectionTracker = func(p *Promise, operation PromiseRejectionOperation) {
		switch operation {
		case PromiseRejectionReject:
			unhandledRejectedPromises[p] = struct{}{}
		case PromiseRejectionHandle:
			delete(unhandledRejectedPromises, p)
		}
	}
	mp := fn(rt)
	rt.performPromiseThen(mp.Promise(), rt.ToValue(func() {}), rt.ToValue(func() {}), nil)
	if mp.State() != PromiseStateRejected {
		t.Fatalf("expected promise to be rejected, got %q", mp.State())
	}
	exc := mp.Promise().Result().Export().(*InterruptedError)
	expValue := "the error we want\n\tat s (dep.js:4:14(3))\n\tat a.js:3:5(10)\n"
	if exc.String() != expValue {
		t.Fatalf("Expected values %q but got %q", expValue, exc.String())
	}

	if len(unhandledRejectedPromises) != 0 {
		t.Fatalf("zero unhandled exceptions were expected but there were some %+v", unhandledRejectedPromises)
	}
	if shouldntHappen {
		t.Fatal("code was supposed to be interrupted but that din't work")
	}
}
