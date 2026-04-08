package sobek

import (
	"fmt"
	"testing"
)

// ExampleESMConfig demonstrates how to create and attach an ESMConfig.
func ExampleESMConfig() {
	vm := New()
	config := NewESMConfig().
		WithHostResolveImportedModule(func(_ interface{}, specifier string) (ModuleRecord, error) {
			return nil, nil // stub resolver
		}).
		WithImportModuleDynamically(func(_ interface{}, _ Value, _ interface{}) {
			// stub for dynamic import()
		})
	vm.AttachESM(config)
	fmt.Println(vm.ESM() != nil)
	// Output: true
}

func TestESMConfig_Attach(t *testing.T) {
	vm := New()
	config := NewESMConfig()

	if vm.ESM() != nil {
		t.Fatal("expected nil ESM before attach")
	}

	vm.AttachESM(config)
	if vm.ESM() == nil {
		t.Fatal("expected non-nil ESM after attach")
	}
	if vm.ESM() != config {
		t.Fatal("ESM() should return attached config")
	}
	if config.Runtime() != vm {
		t.Fatal("config.Runtime() should return attached runtime")
	}
}

func TestESMConfig_AttachSameConfigIdempotent(t *testing.T) {
	config := NewESMConfig()
	vm := New()

	vm.AttachESM(config)
	vm.AttachESM(config) // same config to same runtime — no-op
	if vm.ESM() != config {
		t.Fatal("expected config to still be attached")
	}
}

func TestESMConfig_AttachToTwoRuntimesPanics(t *testing.T) {
	config := NewESMConfig()
	vm1 := New()
	vm2 := New()

	vm1.AttachESM(config)

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic when attaching same config to second runtime")
		}
	}()
	vm2.AttachESM(config)
}

func TestESMConfig_AttachNilPanics(t *testing.T) {
	vm := New()
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic when attaching nil config")
		}
	}()
	vm.AttachESM(nil)
}

func TestESMConfig_OldAPIPanicsWhenAttached(t *testing.T) {
	vm := New()
	config := NewESMConfig()
	vm.AttachESM(config)

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic when using SetImportModuleDynamically with ESM attached")
		}
	}()
	vm.SetImportModuleDynamically(func(interface{}, Value, interface{}) {})
}

func TestESMConfig_EvaluateModuleNotAttachedPanics(t *testing.T) {
	config := NewESMConfig().
		WithHostResolveImportedModule(func(_ interface{}, _ string) (ModuleRecord, error) {
			return nil, nil
		})
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic when calling EvaluateModule without attached runtime")
		}
	}()
	config.EvaluateModule(nil)
}

func TestESMConfig_EvaluateModuleNoResolverPanics(t *testing.T) {
	vm := New()
	config := NewESMConfig() // no WithHostResolveImportedModule
	vm.AttachESM(config)
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic when calling EvaluateModule without resolver")
		}
	}()
	config.EvaluateModule(nil)
}
