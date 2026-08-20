package pluginkit

import (
	"reflect"
	"testing"
)

type sampleCfg struct {
	Name string
}

type sampleDeps struct {
	N int
}

type samplePlugin struct{}

func TestParseSpec_ValidShapes(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		fn   any
		cfg  bool
		deps bool
	}{
		{"no args", func() (*samplePlugin, error) { return &samplePlugin{}, nil }, false, false},
		{"cfg", func(sampleCfg) (*samplePlugin, error) { return &samplePlugin{}, nil }, true, false},
		{"cfg ptr", func(*sampleCfg) (*samplePlugin, error) { return &samplePlugin{}, nil }, true, false},
		{"cfg deps", func(sampleCfg, sampleDeps) (*samplePlugin, error) { return &samplePlugin{}, nil }, true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			spec, err := parseSpec("k", tc.fn)
			if err != nil {
				t.Fatalf("parseSpec: %v", err)
			}
			if (spec.ConfigType != nil) != tc.cfg {
				t.Fatalf("ConfigType nil=%v want cfg=%v", spec.ConfigType == nil, tc.cfg)
			}
			if (spec.DepsType != nil) != tc.deps {
				t.Fatalf("DepsType nil=%v want deps=%v", spec.DepsType == nil, tc.deps)
			}
			if spec.ReturnType != reflect.TypeOf((*samplePlugin)(nil)) {
				t.Fatalf("ReturnType=%v", spec.ReturnType)
			}
			if spec.Constructor() == nil {
				t.Fatal("Constructor is nil")
			}
		})
	}
}

func TestParseSpec_InvalidShapes(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		fn   any
	}{
		{"nil", nil},
		{"not func", "x"},
		{"no error", func() *samplePlugin { return nil }},
		{"only error", func() error { return nil }},
		{"three args", func(sampleCfg, sampleDeps, int) (*samplePlugin, error) { return nil, nil }},
		{"non struct cfg", func(string) (*samplePlugin, error) { return nil, nil }},
		{"variadic", func(...sampleCfg) (*samplePlugin, error) { return nil, nil }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := parseSpec("k", tc.fn); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}
