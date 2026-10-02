package manager

import (
	"testing"

	"github.com/lengzhao/pluginkit"
)

type viewCfg struct {
	Model   string         `json:"model"`
	Timeout int            `json:"timeout"`
	Debug   bool           `json:"debug,omitempty"`
	Tags    []string       `json:"tags,omitempty"`
	Extra   map[string]any `json:"extra,omitempty"`
}

func (c *viewCfg) SetDefaults() {
	if c.Timeout == 0 {
		c.Timeout = 30
	}
	if c.Model == "" {
		c.Model = "gpt-5.5"
	}
}

type viewPlugin struct{ cfg viewCfg }

func (p *viewPlugin) Name() string { return "view" }

func TestProjectViewFieldKinds(t *testing.T) {
	ensureTestPlugins(t)
	pluginkit.Register("view-typed", func(cfg viewCfg) (*viewPlugin, error) {
		return &viewPlugin{cfg: cfg}, nil
	})

	doc := Document{
		RootID: "root",
		Plugin: PluginNode{
			Use:    "view-typed",
			Config: map[string]any{"model": "gpt-4", "timeout": 10, "debug": true},
			Deps:   map[string]any{},
		},
		Shared: map[string]PluginNode{},
	}
	view := projectView(doc)
	if view.Root == nil || len(view.Root.Config) != 5 {
		t.Fatalf("config fields = %#v", view.Root.Config)
	}
	byName := map[string]ViewField{}
	for _, f := range view.Root.Config {
		byName[f.Name] = f
	}

	cases := []struct {
		name     string
		kind     string
		optional bool
		value    any
		def      any
	}{
		{"model", "string", false, "gpt-4", "gpt-5.5"},
		{"timeout", "number", false, 10, 30},
		{"debug", "bool", true, true, false},
		{"tags", "json", true, nil, nil},
		{"extra", "json", true, nil, nil},
	}
	for _, c := range cases {
		f, ok := byName[c.name]
		if !ok {
			t.Fatalf("missing field %q", c.name)
		}
		if f.Kind != c.kind {
			t.Errorf("%s kind=%q want %q", c.name, f.Kind, c.kind)
		}
		if f.Optional != c.optional {
			t.Errorf("%s optional=%v want %v", c.name, f.Optional, c.optional)
		}
		if c.value != nil && f.Value != c.value {
			t.Errorf("%s value=%#v want %#v", c.name, f.Value, c.value)
		}
		// c.def 为 nil 表示不检查默认值（零值切片/map 的默认值不做断言）
		if c.def != nil && f.Default != c.def {
			t.Errorf("%s default=%#v want %#v", c.name, f.Default, c.def)
		}
	}
}
