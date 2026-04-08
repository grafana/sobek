// Sobek is a command-line JavaScript runtime that supports ESM modules.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/grafana/sobek"
)

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: sobek [options] <script.js>\n\n")
		fmt.Fprintf(os.Stderr, "Sobek is a JavaScript runtime with ESM module support.\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	args := flag.Args()
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "Error: no script specified\n")
		flag.Usage()
		os.Exit(1)
	}

	absEntry, err := filepath.Abs(args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	vm := sobek.New()
	resolver := newFSModuleResolver(absEntry)
	eventLoop, config := setupVM(vm, resolver)
	addConsolePolyfill(vm)

	mp, err := loadAndRunEntry(config, resolver, absEntry)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	runEventLoop(vm, mp, eventLoop)
}

// fsModuleResolver resolves ESM modules from the filesystem.
type fsModuleResolver struct {
	mu           sync.Mutex
	absEntry     string
	cache        map[string]cacheEntry
	reverseCache map[sobek.ModuleRecord]string
}

type cacheEntry struct {
	m   sobek.ModuleRecord
	err error
}

func newFSModuleResolver(absEntry string) *fsModuleResolver {
	return &fsModuleResolver{
		absEntry:     absEntry,
		cache:        make(map[string]cacheEntry),
		reverseCache: make(map[sobek.ModuleRecord]string),
	}
}

func (r *fsModuleResolver) resolve(referencingScriptOrModule interface{}, specifier string) (sobek.ModuleRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var baseDir string
	if referencingScriptOrModule == nil {
		baseDir = filepath.Dir(r.absEntry)
	} else {
		referrerPath, ok := r.reverseCache[referencingScriptOrModule.(sobek.ModuleRecord)]
		if !ok {
			return nil, fmt.Errorf("unknown referrer module")
		}
		baseDir = filepath.Dir(referrerPath)
	}

	resolvedPath := r.resolvePath(baseDir, specifier)
	if resolvedPath == "" {
		return nil, fmt.Errorf("invalid path for specifier %q", specifier)
	}

	if entry, ok := r.cache[resolvedPath]; ok {
		return entry.m, entry.err
	}

	data, err := os.ReadFile(resolvedPath)
	if err != nil {
		r.cache[resolvedPath] = cacheEntry{err: err}
		return nil, fmt.Errorf("cannot load module %q: %w", specifier, err)
	}

	module, err := sobek.ParseModule(resolvedPath, string(data), r.resolve)
	if err != nil {
		r.cache[resolvedPath] = cacheEntry{err: err}
		return nil, err
	}

	r.cache[resolvedPath] = cacheEntry{m: module}
	r.reverseCache[module] = resolvedPath
	return module, nil
}

func (r *fsModuleResolver) resolvePath(baseDir, specifier string) string {
	var resolvedPath string
	if filepath.IsAbs(specifier) {
		resolvedPath = filepath.Clean(specifier)
	} else {
		resolvedPath = filepath.Clean(filepath.Join(baseDir, specifier))
	}
	if !filepath.IsAbs(resolvedPath) {
		cwd, _ := os.Getwd()
		resolvedPath = filepath.Join(cwd, resolvedPath)
	}
	absPath, err := filepath.Abs(resolvedPath)
	if err != nil {
		return ""
	}
	if filepath.Ext(absPath) == "" {
		if _, err := os.Stat(absPath + ".js"); err == nil {
			absPath += ".js"
		}
	}
	return absPath
}

func (r *fsModuleResolver) pathForModule(m sobek.ModuleRecord) (string, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	path, ok := r.reverseCache[m]
	return path, ok
}

func setupVM(vm *sobek.Runtime, resolver *fsModuleResolver) (chan func(), *sobek.ESMConfig) {
	eventLoop := make(chan func(), 8)

	config := sobek.NewESMConfig().
		WithHostResolveImportedModule(resolver.resolve).
		WithImportModuleDynamically(func(referrer interface{}, specifier sobek.Value, pcap interface{}) {
			spec := specifier.String()
			eventLoop <- func() {
				ex := vm.Try(func() {
					m, err := resolver.resolve(referrer, spec)
					vm.FinishLoadingImportModule(referrer, specifier, pcap, m, err)
				})
				if ex != nil {
					vm.FinishLoadingImportModule(referrer, specifier, pcap, nil, ex)
				}
			}
		}).
		WithGetImportMetaProperties(func(m sobek.ModuleRecord) []sobek.MetaProperty {
			path, ok := resolver.pathForModule(m)
			if !ok {
				return nil
			}
			url := "file://" + path
			if !strings.HasPrefix(path, "/") {
				url = "file:///" + filepath.ToSlash(path)
			}
			return []sobek.MetaProperty{
				{Key: "url", Value: vm.ToValue(url)},
			}
		})

	vm.AttachESM(config)
	return eventLoop, config
}


func addConsolePolyfill(vm *sobek.Runtime) {
	console := vm.NewObject()
	_ = console.Set("log", vm.ToValue(func(call sobek.FunctionCall) sobek.Value {
		for i, arg := range call.Arguments {
			if i > 0 {
				fmt.Print(" ")
			}
			fmt.Print(arg.String())
		}
		fmt.Println()
		return sobek.Undefined()
	}))
	vm.Set("console", console)
}

func loadAndRunEntry(config *sobek.ESMConfig, resolver *fsModuleResolver, absEntry string) (*sobek.ModulePromise, error) {
	entryModule, err := resolver.resolve(nil, absEntry)
	if err != nil {
		return nil, err
	}

	smr, ok := entryModule.(*sobek.SourceTextModuleRecord)
	if !ok {
		return nil, fmt.Errorf("entry must be a source text module")
	}

	if err := entryModule.Link(); err != nil {
		return nil, fmt.Errorf("linking: %w", err)
	}

	return config.EvaluateModule(smr), nil
}

func runEventLoop(vm *sobek.Runtime, mp *sobek.ModulePromise, eventLoop chan func()) {
	for {
		select {
		case fn := <-eventLoop:
			fn()
		default:
			_, _ = vm.RunString("")
		}

		switch mp.State() {
		case sobek.PromiseStateRejected:
			fmt.Fprintf(os.Stderr, "%s\n", mp.Err())
			os.Exit(1)
		case sobek.PromiseStateFulfilled:
			os.Exit(0)
		case sobek.PromiseStatePending:
			fn := <-eventLoop
			fn()
		}
	}
}
