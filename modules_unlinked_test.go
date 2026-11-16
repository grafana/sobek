package sobek

import (
	"strings"
	"testing"
)

func TestEvaluateUnlinkedSourceModule(t *testing.T) {
	for _, source := range []string{"", "globalThis.executed = true", "export const value = 42", "await 1; globalThis.executed = true", "import 'dep.js'"} {
		t.Run(source, func(t *testing.T) {
			m, err := ParseModule("unlinked.js", source, func(interface{}, string) (ModuleRecord, error) {
				t.Fatal("unlinked evaluation must not resolve imports")
				return nil, nil
			})
			if err != nil {
				t.Fatal(err)
			}
			rt := New()
			p := m.Evaluate(rt)
			if p.State() != PromiseStateRejected {
				t.Fatalf("state = %v, want rejected", p.State())
			}
			if !strings.Contains(p.Result().String(), "linked") {
				t.Fatalf("unhelpful rejection: %v", p.Result())
			}
			if rt.GetModuleInstance(m) != nil {
				t.Fatal("failed instantiation registered a module instance")
			}
			if got := rt.Get("executed"); got != nil && !IsUndefined(got) {
				t.Fatal("unlinked body executed")
			}
			if again := m.Evaluate(rt); again != p {
				t.Fatal("evaluation did not preserve cached rejection")
			}
			v, err := rt.RunString("6 * 7")
			if err != nil || v.ToInteger() != 42 {
				t.Fatalf("runtime unusable after rejection: %v, %v", v, err)
			}
		})
	}
}

func TestInstantiateUnlinkedSourceModule(t *testing.T) {
	m, err := ParseModule("unlinked.js", "export const value = 42", nil)
	if err != nil {
		t.Fatal(err)
	}
	rt := New()
	instance, err := m.Instantiate(rt)
	if instance != nil || err == nil || !strings.Contains(err.Error(), "linked") {
		t.Fatalf("Instantiate = %v, %v", instance, err)
	}
	if rt.GetModuleInstance(m) != nil {
		t.Fatal("failed instantiation registered a module instance")
	}
}

func TestEvaluateLinkedSourceModuleAfterUnlinkedAttempt(t *testing.T) {
	m, err := ParseModule("linked.js", "globalThis.executed = 42; export const value = 42", nil)
	if err != nil {
		t.Fatal(err)
	}
	rejectedRuntime := New()
	rejected := m.Evaluate(rejectedRuntime)
	if rejected.State() != PromiseStateRejected {
		t.Fatalf("state = %v", rejected.State())
	}
	if err := m.Link(); err != nil {
		t.Fatal(err)
	}
	// Evaluation promises are cached per runtime, including rejected evaluations.
	if again := m.Evaluate(rejectedRuntime); again != rejected || again.State() != PromiseStateRejected {
		t.Fatal("cached rejection changed")
	}
	for i := 0; i < 2; i++ {
		rt := New()
		p := m.Evaluate(rt)
		if p.State() != PromiseStateFulfilled {
			t.Fatalf("linked state = %v: %v", p.State(), p.Result())
		}
		if got := rt.Get("executed").ToInteger(); got != 42 {
			t.Fatalf("body result = %d", got)
		}
	}
}

func TestEvaluateLinkedSourceModule(t *testing.T) {
	for _, source := range []string{"globalThis.executed = 42", "await 1; globalThis.executed = 42"} {
		t.Run(source, func(t *testing.T) {
			m, err := ParseModule("linked.js", source, nil)
			if err != nil {
				t.Fatal(err)
			}
			if err := m.Link(); err != nil {
				t.Fatal(err)
			}
			rt := New()
			p := m.Evaluate(rt)
			if p.State() != PromiseStateFulfilled {
				t.Fatalf("state = %v: %v", p.State(), p.Result())
			}
			if rt.Get("executed").ToInteger() != 42 {
				t.Fatal("linked body did not execute")
			}
		})
	}
}
