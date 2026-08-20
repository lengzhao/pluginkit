package pluginkit

import "testing"

func TestRegisterLookup(t *testing.T) {
	resetRegistry()
	t.Cleanup(resetRegistry)

	if err := register("alpha", func() (*samplePlugin, error) { return &samplePlugin{}, nil }); err != nil {
		t.Fatal(err)
	}
	spec, ok := Lookup("alpha")
	if !ok || spec.Kind != "alpha" {
		t.Fatalf("Lookup alpha: ok=%v spec=%+v", ok, spec)
	}
}

func TestRegisterErrors(t *testing.T) {
	resetRegistry()
	t.Cleanup(resetRegistry)

	if err := register("", func() (*samplePlugin, error) { return nil, nil }); err == nil {
		t.Fatal("empty kind")
	}
	if err := register("dup", "not a func"); err == nil {
		t.Fatal("non func")
	}
	if err := register("ok", func() (*samplePlugin, error) { return nil, nil }); err != nil {
		t.Fatal(err)
	}
	if err := register("ok", func() (*samplePlugin, error) { return nil, nil }); err == nil {
		t.Fatal("duplicate kind")
	}
}

func TestRegisterPanics(t *testing.T) {
	resetRegistry()
	t.Cleanup(resetRegistry)

	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	Register("", func() (*samplePlugin, error) { return nil, nil })
}
