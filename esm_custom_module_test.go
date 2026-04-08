package sobek_test

// This file demonstrates the recommended pattern for using ESMConfig with:
//   - source text modules loaded from a filesystem (or in-memory FS)
//   - custom modules implemented in Go
//
// The two types are distinguished by specifier prefix: paths starting with
// "custom:" are resolved to Go modules; everything else is loaded as JS source.

import (
	"fmt"
	"io/fs"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/grafana/sobek"
)

// goMathModule implements sobek.ModuleRecord in Go.
// It exports a single function: add(a, b) number.
type goMathModule struct{}

func (m *goMathModule) GetExportedNames(callback func([]string), _ ...sobek.ModuleRecord) bool {
	callback([]string{"add"})
	return true
}

func (m *goMathModule) ResolveExport(name string, _ ...sobek.ResolveSetElement) (*sobek.ResolvedBinding, bool) {
	if name == "add" {
		return &sobek.ResolvedBinding{Module: m, BindingName: "add"}, false
	}
	return nil, false
}

func (m *goMathModule) Link() error { return nil }

func (m *goMathModule) Evaluate(rt *sobek.Runtime) *sobek.Promise {
	p, resolve, _ := rt.NewPromise()
	resolve(&goMathInstance{rt: rt})
	return p
}

// goMathInstance is the live module instance; GetBindingValue is called by
// the VM whenever JS code reads an export.
type goMathInstance struct {
	rt *sobek.Runtime
}

func (i *goMathInstance) GetBindingValue(name string) sobek.Value {
	if name == "add" {
		return i.rt.ToValue(func(call sobek.FunctionCall) sobek.Value {
			a := call.Argument(0).ToFloat()
			b := call.Argument(1).ToFloat()
			return i.rt.ToValue(a + b)
		})
	}
	return sobek.Undefined()
}

// fsResolver resolves module specifiers to ModuleRecords.
//
//   - Specifiers starting with "custom:" are handled as custom Go modules.
//   - Everything else is treated as a path inside the provided fs.FS.
//
// It caches parsed modules so each file is only parsed once.
type fsResolver struct {
	mu           sync.Mutex
	fsys         fs.FS
	cache        map[string]sobek.ModuleRecord
	reverseCache map[sobek.ModuleRecord]string // module → specifier, for import.meta.url
	customs      map[string]sobek.ModuleRecord
}

func newFSResolver(fsys fs.FS, customs map[string]sobek.ModuleRecord) *fsResolver {
	return &fsResolver{
		fsys:         fsys,
		cache:        make(map[string]sobek.ModuleRecord),
		reverseCache: make(map[sobek.ModuleRecord]string),
		customs:      customs,
	}
}

func (r *fsResolver) resolve(referrer interface{}, specifier string) (sobek.ModuleRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Custom Go modules are matched by their registered specifier.
	if m, ok := r.customs[specifier]; ok {
		return m, nil
	}

	// Everything else is a source file. Resolve relative to the referrer's path.
	if cached, ok := r.cache[specifier]; ok {
		return cached, nil
	}

	src, err := fs.ReadFile(r.fsys, specifier)
	if err != nil {
		return nil, fmt.Errorf("cannot load module %q: %w", specifier, err)
	}

	m, err := sobek.ParseModule(specifier, string(src), r.resolve)
	if err != nil {
		return nil, fmt.Errorf("parse error in %q: %w", specifier, err)
	}

	r.cache[specifier] = m
	r.reverseCache[m] = specifier
	return m, nil
}

func (r *fsResolver) urlForModule(m sobek.ModuleRecord) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if spec, ok := r.reverseCache[m]; ok {
		return "file:///" + spec
	}
	return ""
}

func TestCustomGoModuleWithFilesystem(t *testing.T) {
	t.Parallel()

	// In-memory filesystem stands in for os.DirFS or embed.FS in real code.
	fsys := fstest.MapFS{
		"main.js": {Data: []byte(`
			import { greet } from "greet.js";
			import { add }   from "custom:math";

			const msg = greet("World");
			const sum = add(3, 4);

			if (msg !== "Hello, World!") throw "unexpected greeting: " + msg;
			if (sum !== 7)              throw "unexpected sum: " + sum;

			globalThis.result = msg + " 3+4=" + sum;
		`)},
		"greet.js": {Data: []byte(`
			export function greet(name) {
				return "Hello, " + name + "!";
			}
		`)},
	}

	resolver := newFSResolver(fsys, map[string]sobek.ModuleRecord{
		"custom:math": &goMathModule{},
	})

	vm := sobek.New()
	config := sobek.NewESMConfig().
		WithHostResolveImportedModule(resolver.resolve).
		WithImportModuleDynamically(func(referrer interface{}, specifier sobek.Value, pcap interface{}) {
			// Synchronous dynamic import: fine for non-async event-loop scenarios.
			m, err := resolver.resolve(referrer, specifier.String())
			vm.FinishLoadingImportModule(referrer, specifier, pcap, m, err)
		}).
		WithGetImportMetaProperties(func(m sobek.ModuleRecord) []sobek.MetaProperty {
			return []sobek.MetaProperty{
				{Key: "url", Value: vm.ToValue(resolver.urlForModule(m))},
			}
		})
	vm.AttachESM(config)

	entry, err := resolver.resolve(nil, "main.js")
	if err != nil {
		t.Fatal(err)
	}
	if err := entry.Link(); err != nil {
		t.Fatal(err)
	}

	mp := config.EvaluateModule(entry.(*sobek.SourceTextModuleRecord))
	if err := mp.Err(); err != nil {
		t.Fatal(err)
	}

	if got := vm.GlobalObject().Get("result").String(); got != "Hello, World! 3+4=7" {
		t.Fatalf("unexpected result: %q", got)
	}
}
