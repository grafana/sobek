package sobek

import "fmt"

// ESMConfig holds all ESM-related configuration for module resolution. Attach it
// to a Runtime once via AttachESM, then use EvaluateModule to run modules —
// the resolver is baked in so it cannot be omitted or mismatched.
//
// The old API (SetImportModuleDynamically, SetGetImportMetaProperties,
// SetFinalImportMeta) is mutually exclusive with ESMConfig: once an ESMConfig
// is attached, those setters panic.
//
// # Basic Usage: filesystem source files + custom Go modules
//
// The resolver passed to WithHostResolveImportedModule is the single dispatch
// point for all imports. It receives the specifier string and must return a
// ModuleRecord — either a SourceTextModuleRecord (parsed from JS source) or
// any type that implements the ModuleRecord interface directly in Go.
//
//	// goMathModule is a custom Go module that exports an "add" function.
//	// Implement ModuleRecord (and optionally CyclicModuleRecord) to expose
//	// Go functionality to JavaScript.
//	type goMathModule struct{}
//	// ... implement GetExportedNames, ResolveExport, Link, Evaluate
//
//	var resolve sobek.HostResolveImportedModuleFunc
//	resolve = func(_ interface{}, specifier string) (sobek.ModuleRecord, error) {
//		// Dispatch custom Go modules by specifier prefix.
//		if specifier == "custom:math" {
//			return &goMathModule{}, nil
//		}
//		// Load everything else as JS source from the filesystem.
//		src, err := os.ReadFile(specifier)
//		if err != nil {
//			return nil, err
//		}
//		return sobek.ParseModule(specifier, string(src), resolve)
//	}
//
//	vm := sobek.New()
//	config := sobek.NewESMConfig().
//		WithHostResolveImportedModule(resolve).
//		WithImportModuleDynamically(func(referrer interface{}, specifier sobek.Value, pcap interface{}) {
//			m, err := resolve(referrer, specifier.String())
//			vm.FinishLoadingImportModule(referrer, specifier, pcap, m, err)
//		}).
//		WithGetImportMetaProperties(func(m sobek.ModuleRecord) []sobek.MetaProperty {
//			// Provide import.meta.url. You need your own bookkeeping to map
//			// a ModuleRecord back to its path (e.g. a reverse cache populated
//			// inside the resolve function above).
//			return []sobek.MetaProperty{
//				{Key: "url", Value: vm.ToValue("file:///path/to/module.js")},
//			}
//		})
//	vm.AttachESM(config)
//
//	entry, _ := resolve(nil, "main.js")
//	_ = entry.Link()
//	mp := config.EvaluateModule(entry.(*sobek.SourceTextModuleRecord))
//	if err := mp.Err(); err != nil {
//		log.Fatal(err)
//	}
//
// # Implementing a Go module
//
// To expose Go values or functions to JavaScript, implement the ModuleRecord
// interface. The minimal approach is two types: a record (shared, stateless)
// and an instance (per-runtime, holds a *Runtime to create JS values):
//
//	type myRecord struct{}
//
//	func (m *myRecord) GetExportedNames(cb func([]string), _ ...sobek.ModuleRecord) bool {
//		cb([]string{"hello"})
//		return true
//	}
//	func (m *myRecord) ResolveExport(name string, _ ...sobek.ResolveSetElement) (*sobek.ResolvedBinding, bool) {
//		if name == "hello" {
//			return &sobek.ResolvedBinding{Module: m, BindingName: "hello"}, false
//		}
//		return nil, false
//	}
//	func (m *myRecord) Link() error { return nil }
//	func (m *myRecord) Evaluate(rt *sobek.Runtime) *sobek.Promise {
//		p, resolve, _ := rt.NewPromise()
//		resolve(&myInstance{rt: rt})
//		return p
//	}
//
//	type myInstance struct{ rt *sobek.Runtime }
//
//	func (i *myInstance) GetBindingValue(name string) sobek.Value {
//		if name == "hello" {
//			return i.rt.ToValue(func(call sobek.FunctionCall) sobek.Value {
//				return i.rt.ToValue("Hello, " + call.Argument(0).String() + "!")
//			})
//		}
//		return sobek.Undefined()
//	}
//
// # One Config Per Runtime
//
// An ESMConfig can only be attached to one Runtime. Attaching the same config
// to a second Runtime will panic.
type ESMConfig struct {
	runtime *Runtime

	hostResolveImportedModule HostResolveImportedModuleFunc
	importModuleDynamically   ImportModuleDynamicallyCallback
	getImportMetaProperties   func(ModuleRecord) []MetaProperty
	finalizeImportMeta        func(*Object, ModuleRecord)
}

// NewESMConfig creates a new ESMConfig with nil defaults.
func NewESMConfig() *ESMConfig {
	return &ESMConfig{}
}

// Runtime returns the Runtime this config is attached to, or nil if not attached.
func (c *ESMConfig) Runtime() *Runtime {
	return c.runtime
}

// WithHostResolveImportedModule sets the function that resolves a module specifier
// to a ModuleRecord. This is the central dispatch point for all imports: return a
// SourceTextModuleRecord for JS source files, or any ModuleRecord implementation
// for custom Go modules. The same function is used by EvaluateModule for static
// imports and by the WithImportModuleDynamically callback for dynamic import().
func (c *ESMConfig) WithHostResolveImportedModule(fn HostResolveImportedModuleFunc) *ESMConfig {
	c.hostResolveImportedModule = fn
	return c
}

// WithImportModuleDynamically sets the callback for dynamic import().
func (c *ESMConfig) WithImportModuleDynamically(callback ImportModuleDynamicallyCallback) *ESMConfig {
	c.importModuleDynamically = callback
	return c
}

// WithGetImportMetaProperties sets the function that provides import.meta properties.
func (c *ESMConfig) WithGetImportMetaProperties(fn func(ModuleRecord) []MetaProperty) *ESMConfig {
	c.getImportMetaProperties = fn
	return c
}

// WithFinalizeImportMeta sets the function that finalizes the import.meta object.
func (c *ESMConfig) WithFinalizeImportMeta(fn func(*Object, ModuleRecord)) *ESMConfig {
	c.finalizeImportMeta = fn
	return c
}

// EvaluateModule evaluates the given module record using this config's resolver.
// The config must be attached to a Runtime (via AttachESM) and the module must
// already be linked. WithHostResolveImportedModule must be set if the module
// graph requires resolution during evaluation (i.e. any dynamic imports or
// async module traversal).
func (c *ESMConfig) EvaluateModule(m CyclicModuleRecord) *ModulePromise {
	if c.runtime == nil {
		panic("ESMConfig is not attached to a Runtime; call AttachESM first")
	}
	return &ModulePromise{promise: c.runtime.CyclicModuleRecordEvaluate(m, c.hostResolveImportedModule)}
}

// ModulePromise wraps the Promise returned by module evaluation and provides
// typed, ergonomic access to the result without requiring manual type assertions.
type ModulePromise struct {
	promise *Promise
}

// Promise returns the underlying Promise, for use with promise handlers or APIs
// that expect a raw Promise (e.g. performPromiseThen).
func (p *ModulePromise) Promise() *Promise {
	return p.promise
}

// State returns the current state of the evaluation promise.
func (p *ModulePromise) State() PromiseState {
	return p.promise.State()
}

// Err returns the rejection error if the promise is in the rejected state, or nil
// if it is fulfilled or still pending. For async modules, check State() first to
// confirm the promise has settled before relying on Err().
func (p *ModulePromise) Err() error {
	if p.promise.State() != PromiseStateRejected {
		return nil
	}
	result := p.promise.Result()
	if err, ok := result.Export().(error); ok {
		return err
	}
	return fmt.Errorf("module evaluation failed: %s", result.String())
}
