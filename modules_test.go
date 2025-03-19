package sobek

import (
	"fmt"
	"sync"
	"testing"
)

func TestSimpleModule(t *testing.T) {
	t.Parallel()

	testCases := map[string]map[string]string{
		"function export": {
			"a.js": `
				import { b } from "dep.js";
				globalThis.s = b()
			`,
			"dep.js": `export function b() { return 5 };`,
		},
		"function export eval": {
			"a.js": `
				import { b } from "dep.js";
				eval("globalThis.s = b()");
			`,
			"dep.js": `export function b() { return 5 };`,
		},
		"var export destructuring": {
			"a.js": `
				import { b } from "dep.js";
				eval("globalThis.s = b()");
			`,
			"dep.js": `
				const a = { b: function() {return 5 } };
				export var { b } = a;
			`,
		},
		"var export destructuring array": {
			"a.js": `
				import { b } from "dep.js";
				eval("globalThis.s = b()");
			`,
			"dep.js": `
				const a = [ function() {return 5 }, 5 ];
				export var [ b, s ] = a;
				if (s != 5) {
					throw "bad s=" + s;
				}
			`,
		},
		"let export": {
			"a.js": `
				import { b } from "dep.js";
				globalThis.s = b()
			`,
			"dep.js": `export let b = function() {return 5 };`,
		},
		"let export eval": {
			"a.js": `
				import { b } from "dep.js";
				eval("globalThis.s = b()");
			`,
			"dep.js": `export let b = function() {return 5 };`,
		},
		"let export destructuring": {
			"a.js": `
				import { b } from "dep.js";
				eval("globalThis.s = b()");
			`,
			"dep.js": `
				const a = { b: function() {return 5 } };
				export let { b } = a;
			`,
		},
		"let export destructuring array": {
			"a.js": `
				import { b } from "dep.js";
				eval("globalThis.s = b()");
			`,
			"dep.js": `
				const a = [ function() {return 5 }, 5 ];
				export let [ b, s ] = a;
				if (s != 5) {
					throw "bad s=" + s;
				}
			`,
		},
		"const export": {
			"a.js": `
				import { b } from "dep.js";
				globalThis.s = b()
			`,
			"dep.js": `export const b = function() { return 5 };`,
		},
		"const export eval": {
			"a.js": `
				import { b } from "dep.js";
				eval("globalThis.s = b()")
			`,
			"dep.js": `export const b = function() { return 5 };`,
		},
		"const export destructuring array": {
			"a.js": `
				import { b } from "dep.js";
				eval("globalThis.s = b()");
			`,
			"dep.js": `
				const a = [ function() {return 5 }, 5 ];
				export const [ b, s ] = a;
				if (s != 5) {
					throw "bad s=" + s;
				}
			`,
		},
		"let export with update": {
			"a.js": `
				import { s, b } from "dep.js";
				s()
				globalThis.s = b()
			`,
			"dep.js": `
				export let b = "something";
				export function s(){
					b = function() {
						return 5;
					};
				}`,
		},
		"let export with update eval": {
			"a.js": `
				import { s, b } from "dep.js";
				s();
				eval("globalThis.s = b();");
			`,
			"dep.js": `
				export let b = "something";
				export function s(){
					b = function() {
						return 5;
					};
				}`,
		},
		"default export": {
			"a.js": `
				import b from "dep.js";
				globalThis.s = b()
			`,
			"dep.js": `export default function() { return 5 };`,
		},
		"default export eval": {
			"a.js": `
				import b from "dep.js";
				eval("globalThis.s = b()");
			`,
			"dep.js": `export default function() { return 5 };`,
		},
		"default loop": {
			"a.js": `
				import b from "a.js";
				export default function() {return 5;};
				globalThis.s = b()
			`,
		},
		"default loop eval": {
			"a.js": `
				import b from "a.js";
				export default function() {return 5;};
				eval("globalThis.s = b()");
			`,
		},
		"default export arrow": {
			"a.js": `
				import b from "dep.js";
				globalThis.s = b();
			`,
			"dep.js": `export default () => {return 5 };`,
		},
		"default export Class usage": {
			"a.js": `
				import b from "dep.js";
				export default class m {
					some() {
						return b();
					}
				}
				let l = new m();
				globalThis.s = l.some();
			`,
			"dep.js": `export default () => {return 5 };`,
		},
		"default export function usage": {
			"a.js": `
				import b from "dep.js";
				export default function some () {
					return b();
				}
				globalThis.s = some();
			`,
			"dep.js": `export default () => {return 5 };`,
		},
		"default export generator usage": {
			"a.js": `
				import b from "dep.js";
				export default function * some () {
					yield b();
				}
				globalThis.s = some().next().value;
			`,
			"dep.js": `export default () => {return 5 };`,
		},
		"default export async function usage": {
			"a.js": `
				import b from "dep.js";
				export default async function some () {
					return b();
				}
				globalThis.s = await some();
			`,
			"dep.js": `export default () => {return 5 };`,
		},
		"default export arrow async": {
			"a.js": `
				import b from "dep.js";
				globalThis.s = await b();
			`,
			"dep.js": `export default async () => {return 5 };`,
		},
		"default export arrow eval": {
			"a.js": `
				import b from "dep.js";
				eval("globalThis.s = b();")
			`,
			"dep.js": `export default () => {return 5 };`,
		},
		"default export with as": {
			"a.js": `
				import b from "dep.js";
				globalThis.s = b()
			`,
			"dep.js": `
				function f() {return 5;};
				export { f as default };
			`,
		},
		"default export with as eval": {
			"a.js": `
				import b from "dep.js";
				eval("globalThis.s = b()")
			`,
			"dep.js": `
				function f() {return 5;};
				export { f as default };
			`,
		},
		"export usage before evaluation as": {
			"a.js": `
				import  "dep.js";
				export function a() { return 5; }
			`,
			"dep.js": `
				import { a } from "a.js";
				globalThis.s = a();
			`,
		},
		"export usage before evaluation as eval": {
			"a.js": `
				import  "dep.js";
				export function a() { return 5; }
			`,
			"dep.js": `
				import { a } from "a.js";
				eval("globalThis.s = a()");
			`,
		},
		"dynamic import": {
			"a.js": `
				import("dep.js").then((imported) => {
					globalThis.s = imported.default();
				});
			`,
			"dep.js": `export default function() { return 5; }`,
		},
		"dynamic import eval": {
			"a.js": `
				import("dep.js").then((imported) => {
					eval("globalThis.s = imported.default()");
				});
			`,
			"dep.js": `export default function() { return 5; }`,
		},
		"dynamic import error": {
			"a.js": `
				do {
					import('dep.js').catch(error => {
						if (error.name == "SyntaxError") {
							globalThis.s = 5;
						}
					});
				} while (false);
			`,
			"dep.js": `import { x } from "0-fixture.js";`,
			"0-fixture.js": `
					export * from "1-fixture.js";
					export * from "2-fixture.js";
			`,
			"1-fixture.js": `export var x`,
			"2-fixture.js": `export var x`,
		},
	}
	for name, cases := range testCases {
		cases := cases
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			fn := runModules(t, cases)

			for i := 0; i < 10; i++ {
				i := i
				t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
					t.Parallel()
					vm := New()
					promise := fn(vm)
					if promise.state != PromiseStateFulfilled {
						t.Fatalf("got %+v", promise.Result().Export())
					}
					v := vm.Get("s")
					if v == nil || v.ToNumber().ToInteger() != 5 {
						t.Fatalf("expected 5 got %s", v)
					}
				})
			}
		})
	}
}

func runModules(t testing.TB, files map[string]string) func(*Runtime) *Promise {
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

	return func(vm *Runtime) *Promise {
		eventLoopQueue := make(chan func(), 2) // the most basic and likely buggy event loop
		vm.SetImportModuleDynamically(func(referencingScriptOrModule interface{}, specifierValue Value, pcap interface{}) {
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
		var promise *Promise
		eventLoopQueue <- func() { promise = m.Evaluate(vm) }

	outer:
		for {
			select {
			case fn := <-eventLoopQueue:
				fn()
			default:
				break outer
			}
		}
		return promise
	}
}

func TestAmbiguousImport(t *testing.T) {
	t.Parallel()
	fn := runModules(t, map[string]string{
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
	promise := fn(New())
	if promise.state != PromiseStateRejected {
		t.Fatalf("expected promise to be rejected %q", promise.state)
	}
	exc := promise.Result().Export().(*Exception)
	expValue := `SyntaxError: The requested module "a.js" contains conflicting star exports for name "x"
	at dep.js:3:5(2)
	at dep.js:1:1(2)
`
	if exc.String() != expValue {
		t.Fatalf("Expected values %q but got %q", expValue, exc.String())
	}
}

func TestImportingUnexported(t *testing.T) {
	t.Parallel()
	fn := runModules(t, map[string]string{
		`a.js`: `
			import "dep.js"
			export let s = 5;
		`,
		`dep.js`: `
			import { s } from "a.js"
			import { x } from "a.js"
		`,
	})
	promise := fn(New())
	if promise.state != PromiseStateRejected {
		t.Fatalf("expected promise to be rejected %q", promise.state)
	}
	exc := promise.Result().Export().(*Exception)
	expValue := `SyntaxError: The requested module "a.js" does not provide an export named "x"
	at dep.js:3:5(2)
	at dep.js:1:1(2)
`
	if exc.String() != expValue {
		t.Fatalf("Expected values %q but got %q", expValue, exc.String())
	}
}

func TestModuleAsyncErrorAndPromiseRejection(t *testing.T) {
	t.Parallel()
	fn := runModules(t, map[string]string{
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
	promise := fn(rt)
	rt.performPromiseThen(promise, rt.ToValue(func() {}), rt.ToValue(func() {}), nil)
	if promise.state != PromiseStateRejected {
		t.Fatalf("expected promise to be rejected %q", promise.state)
	}
	exc := promise.Result().Export().(*Exception)
	expValue := "ReferenceError: something is not defined\n\tat a.js:3:4(6)\n"
	if exc.String() != expValue {
		t.Fatalf("Expected values %q but got %q", expValue, exc.String())
	}

	if len(unhandledRejectedPromises) != 0 {
		t.Fatalf("zero unhandled exceptions were expected but there were some %+v", unhandledRejectedPromises)
	}
}

func TestModuleAsyncInterrupt(t *testing.T) {
	t.Parallel()
	fn := runModules(t, map[string]string{
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
	promise := fn(rt)
	rt.performPromiseThen(promise, rt.ToValue(func() {}), rt.ToValue(func() {}), nil)
	if promise.state != PromiseStateRejected {
		t.Fatalf("expected promise to be rejected %q", promise.state)
	}
	exc := promise.Result().Export().(*InterruptedError)
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
