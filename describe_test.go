package pluginkit

import (
	"reflect"
	"testing"
)

type LLM interface {
	Model() string
}

type Tool interface {
	Name() string
}

type Store interface {
	Get(key string) string
}

type describeCfg struct {
	Model string `json:"model"`
	Root  string `json:"root"`
}

type describeDeps struct {
	LLM        LLM    `json:"llm"`
	Tools      []Tool `json:"tools"`
	Logger     LLM    `json:"logger,omitempty"`
	Skip       string `json:"-"`
	unexported string
}

type describePlugin struct{}

func TestDescribe(t *testing.T) {
	resetRegistry()
	t.Cleanup(resetRegistry)

	kind := t.Name() + "/plugin"
	if err := register(kind, func(describeCfg, describeDeps) (*describePlugin, error) {
		return &describePlugin{}, nil
	}); err != nil {
		t.Fatal(err)
	}

	desc, ok := Describe(kind)
	if !ok {
		t.Fatal("Describe returned ok=false")
	}
	if desc.Kind != kind {
		t.Fatalf("Kind=%q want %q", desc.Kind, kind)
	}
	if desc.ReturnType != reflect.TypeOf((*describePlugin)(nil)) {
		t.Fatalf("ReturnType=%v", desc.ReturnType)
	}

	wantConfig := []FieldDescription{
		{Name: "model", GoName: "Model", Type: reflect.TypeOf(""), List: false, Optional: false},
		{Name: "root", GoName: "Root", Type: reflect.TypeOf(""), List: false, Optional: false},
	}
	if !reflect.DeepEqual(desc.Config, wantConfig) {
		t.Fatalf("Config=%#v want %#v", desc.Config, wantConfig)
	}

	llmType := reflect.TypeOf((*LLM)(nil)).Elem()
	toolType := reflect.TypeOf((*Tool)(nil)).Elem()
	wantExtensions := []FieldDescription{
		{Name: "llm", GoName: "LLM", Type: llmType, List: false, Optional: false},
		{Name: "tools", GoName: "Tools", Type: toolType, List: true, Optional: false},
		{Name: "logger", GoName: "Logger", Type: llmType, List: false, Optional: true},
	}
	if !reflect.DeepEqual(desc.Extensions, wantExtensions) {
		t.Fatalf("Extensions=%#v want %#v", desc.Extensions, wantExtensions)
	}
}

func TestDescribeUnknownKind(t *testing.T) {
	resetRegistry()
	t.Cleanup(resetRegistry)

	_, ok := Describe("no-such-kind")
	if ok {
		t.Fatal("expected ok=false for unknown kind")
	}
}

func TestDescribeNoConfigOrDeps(t *testing.T) {
	resetRegistry()
	t.Cleanup(resetRegistry)

	kind := t.Name() + "/plugin"
	if err := register(kind, func() (*describePlugin, error) {
		return &describePlugin{}, nil
	}); err != nil {
		t.Fatal(err)
	}

	desc, ok := Describe(kind)
	if !ok {
		t.Fatal("Describe returned ok=false")
	}
	if len(desc.Config) != 0 {
		t.Fatalf("Config=%#v want empty", desc.Config)
	}
	if len(desc.Extensions) != 0 {
		t.Fatalf("Extensions=%#v want empty", desc.Extensions)
	}
}

func TestPluginDescriptionTemplate(t *testing.T) {
	resetRegistry()
	t.Cleanup(resetRegistry)

	kind := t.Name() + "/plugin"
	if err := register(kind, func(describeCfg, describeDeps) (*describePlugin, error) {
		return &describePlugin{}, nil
	}); err != nil {
		t.Fatal(err)
	}

	desc, ok := Describe(kind)
	if !ok {
		t.Fatal("Describe returned ok=false")
	}
	tmpl := desc.Template()
	if tmpl["use"] != kind {
		t.Fatalf("use=%v want %q", tmpl["use"], kind)
	}

	cfg, ok := tmpl["config"].(map[string]any)
	if !ok {
		t.Fatalf("config=%#v", tmpl["config"])
	}
	if cfg["model"] != "" || cfg["root"] != "" {
		t.Fatalf("config=%#v want empty string placeholders", cfg)
	}

	deps, ok := tmpl["deps"].(map[string]any)
	if !ok {
		t.Fatalf("deps=%#v", tmpl["deps"])
	}
	if _, ok := deps["logger"]; ok {
		t.Fatal("optional dep logger should be omitted")
	}

	llm, ok := deps["llm"].(map[string]any)
	if !ok || llm["use"] != "" {
		t.Fatalf("deps.llm=%#v want use placeholder", deps["llm"])
	}

	tools, ok := deps["tools"].([]any)
	if !ok || len(tools) != 1 {
		t.Fatalf("deps.tools=%#v want one placeholder item", deps["tools"])
	}
	first, ok := tools[0].(map[string]any)
	if !ok || first["use"] != "" {
		t.Fatalf("deps.tools[0]=%#v want use placeholder", tools[0])
	}
}

type describeDefaultCfg struct {
	Model   string `json:"model"`
	Timeout int    `json:"timeout"`
}

func (c *describeDefaultCfg) SetDefaults() {
	if c.Model == "" {
		c.Model = "gpt-5.5"
	}
	if c.Timeout == 0 {
		c.Timeout = 30
	}
}

func TestPluginDescriptionTemplateDefaults(t *testing.T) {
	resetRegistry()
	t.Cleanup(resetRegistry)

	kind := t.Name() + "/plugin"
	if err := register(kind, func(cfg describeDefaultCfg) (*describePlugin, error) {
		return &describePlugin{}, nil
	}); err != nil {
		t.Fatal(err)
	}

	desc, ok := Describe(kind)
	if !ok {
		t.Fatal("Describe returned ok=false")
	}
	tmpl := desc.Template()
	cfg, ok := tmpl["config"].(map[string]any)
	if !ok {
		t.Fatalf("config=%#v", tmpl["config"])
	}
	if cfg["model"] != "gpt-5.5" {
		t.Fatalf("model=%v want gpt-5.5", cfg["model"])
	}
	if cfg["timeout"] != 30 {
		t.Fatalf("timeout=%v want 30", cfg["timeout"])
	}
}

func TestPluginDescriptionConfigDefaults(t *testing.T) {
	resetRegistry()
	t.Cleanup(resetRegistry)

	withDefaults := t.Name() + "/with"
	if err := register(withDefaults, func(cfg describeDefaultCfg) (*describePlugin, error) {
		return &describePlugin{}, nil
	}); err != nil {
		t.Fatal(err)
	}
	desc, ok := Describe(withDefaults)
	if !ok {
		t.Fatal("Describe returned ok=false")
	}
	defaults := desc.ConfigDefaults()
	if defaults["model"] != "gpt-5.5" || defaults["timeout"] != 30 {
		t.Fatalf("ConfigDefaults=%#v", defaults)
	}

	withoutDefaults := t.Name() + "/without"
	if err := register(withoutDefaults, func(cfg describeCfg) (*describePlugin, error) {
		return &describePlugin{}, nil
	}); err != nil {
		t.Fatal(err)
	}
	desc2, ok := Describe(withoutDefaults)
	if !ok {
		t.Fatal("Describe returned ok=false")
	}
	if got := desc2.ConfigDefaults(); got != nil {
		t.Fatalf("ConfigDefaults=%#v want nil", got)
	}
}

func TestPluginDescriptionTemplateNoConfigOrDeps(t *testing.T) {
	resetRegistry()
	t.Cleanup(resetRegistry)

	kind := t.Name() + "/plugin"
	if err := register(kind, func() (*describePlugin, error) {
		return &describePlugin{}, nil
	}); err != nil {
		t.Fatal(err)
	}

	desc, ok := Describe(kind)
	if !ok {
		t.Fatal("Describe returned ok=false")
	}
	tmpl := desc.Template()
	if tmpl["use"] != kind {
		t.Fatalf("use=%v want %q", tmpl["use"], kind)
	}
	if _, ok := tmpl["config"]; ok {
		t.Fatalf("config=%#v want omitted", tmpl["config"])
	}
	if _, ok := tmpl["deps"]; ok {
		t.Fatalf("deps=%#v want omitted", tmpl["deps"])
	}
}
