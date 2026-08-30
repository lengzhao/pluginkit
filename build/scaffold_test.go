package build

import (
	"context"
	"strings"
	"testing"

	"github.com/lengzhao/pluginkit"
)

func registerScaffoldFixtures(t *testing.T) (openaiKind, claudeKind, readKind, shellKind, storeKind, httpKind, saveKind string) {
	t.Helper()
	openaiKind = kind(t, "openai")
	claudeKind = kind(t, "claude")
	readKind = kind(t, "read-file")
	shellKind = kind(t, "shell")
	storeKind = kind(t, "sqlite-store")
	httpKind = kind(t, "http-step")
	saveKind = kind(t, "save-step")

	pluginkit.Register(openaiKind, func(cfg openaiCfg) (*openai, error) {
		return &openai{model: cfg.Model}, nil
	})
	pluginkit.Register(claudeKind, func(cfg openaiCfg) (*openai, error) {
		return &openai{model: cfg.Model}, nil
	})
	pluginkit.Register(readKind, func(cfg readFileCfg) (*readFile, error) {
		return &readFile{root: cfg.Root}, nil
	})
	pluginkit.Register(shellKind, func() (*shell, error) { return &shell{}, nil })
	pluginkit.Register(storeKind, func() (*sqliteStore, error) { return &sqliteStore{}, nil })
	pluginkit.Register(httpKind, func() (*httpStep, error) { return &httpStep{}, nil })
	pluginkit.Register(saveKind, func(_ struct{}, deps saveDeps) (*saveStep, error) {
		return &saveStep{store: deps.Store}, nil
	})
	return
}

func TestScaffoldAgentPlugins(t *testing.T) {
	openaiKind, claudeKind, readKind, shellKind, _, _, _ := registerScaffoldFixtures(t)
	opts := ScaffoldOptions{Whitelist: []string{openaiKind, claudeKind, readKind, shellKind}}

	cfg, err := Scaffold(&agentPlugins{}, opts)
	if err != nil {
		t.Fatal(err)
	}

	llm, ok := cfg["llm"].(map[string]any)
	if !ok {
		t.Fatalf("llm=%T", cfg["llm"])
	}
	if llm["use"] != claudeKind || llm["id"] != claudeKind {
		t.Fatalf("llm=%v want kind %q", llm, claudeKind)
	}
	_ = openaiKind

	tools, ok := cfg["tools"].([]any)
	if !ok || len(tools) != 2 {
		t.Fatalf("tools=%v", cfg["tools"])
	}
	first, _ := tools[0].(map[string]any)
	second, _ := tools[1].(map[string]any)
	if first["use"] != readKind || second["use"] != shellKind {
		t.Fatalf("tools=%v", tools)
	}
	if first["id"] != readKind || second["id"] != shellKind {
		t.Fatalf("tool ids=%v %v", first["id"], second["id"])
	}

	var plugins agentPlugins
	if _, err := BuildInto(context.Background(), cfg, &plugins); err != nil {
		t.Fatal(err)
	}
	if plugins.LLM == nil || len(plugins.Tools) != 2 {
		t.Fatalf("plugins=%+v", plugins)
	}
}

func TestScaffoldWorkflowPlugins(t *testing.T) {
	_, _, _, _, storeKind, httpKind, saveKind := registerScaffoldFixtures(t)
	opts := ScaffoldOptions{Whitelist: []string{storeKind, httpKind, saveKind}}

	cfg, err := Scaffold(&workflowPlugins{}, opts)
	if err != nil {
		t.Fatal(err)
	}

	store, ok := cfg["store"].(map[string]any)
	if !ok || store["use"] != storeKind || store["id"] != storeKind {
		t.Fatalf("store=%v", cfg["store"])
	}

	steps, ok := cfg["steps"].([]any)
	if !ok || len(steps) != 2 {
		t.Fatalf("steps=%v", cfg["steps"])
	}
	save, ok := steps[1].(map[string]any)
	if !ok || save["use"] != saveKind {
		t.Fatalf("save step=%v", steps[1])
	}
	deps, ok := save["deps"].(map[string]any)
	if !ok || deps["store"] != storeKind {
		t.Fatalf("save deps=%v want store ref %q", deps, storeKind)
	}
	_ = httpKind

	var plugins workflowPlugins
	if _, err := BuildInto(context.Background(), cfg, &plugins); err != nil {
		t.Fatal(err)
	}
	if plugins.Store == nil || len(plugins.Steps) != 2 || plugins.Steps[1].Run() != "save:x" {
		t.Fatalf("plugins=%+v", plugins)
	}
}

func TestScaffoldYAMLComment(t *testing.T) {
	openaiKind, claudeKind, readKind, shellKind, _, _, _ := registerScaffoldFixtures(t)
	opts := ScaffoldOptions{Whitelist: []string{openaiKind, claudeKind, readKind, shellKind}}

	raw, err := ScaffoldYAML(&agentPlugins{}, opts)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if !strings.Contains(text, "alternatives:") || !strings.Contains(text, claudeKind) {
		t.Fatalf("yaml missing alternatives comment:\n%s", text)
	}
	if !strings.Contains(text, openaiKind) {
		t.Fatalf("yaml missing default kind:\n%s", text)
	}
}

func TestScaffoldWhitelist(t *testing.T) {
	openaiKind, claudeKind, readKind, shellKind, _, _, _ := registerScaffoldFixtures(t)

	cfg, err := Scaffold(&agentPlugins{}, ScaffoldOptions{Whitelist: []string{claudeKind, shellKind}})
	if err != nil {
		t.Fatal(err)
	}
	llm, _ := cfg["llm"].(map[string]any)
	if llm["use"] != claudeKind {
		t.Fatalf("llm=%v want %q", llm, claudeKind)
	}
	tools, _ := cfg["tools"].([]any)
	if len(tools) != 1 {
		t.Fatalf("tools=%v", tools)
	}
	tool, _ := tools[0].(map[string]any)
	if tool["use"] != shellKind {
		t.Fatalf("tool=%v", tool)
	}
	_ = openaiKind
	_ = readKind
}

func TestScaffoldBlacklist(t *testing.T) {
	openaiKind, claudeKind, readKind, shellKind, _, _, _ := registerScaffoldFixtures(t)
	opts := ScaffoldOptions{
		Whitelist: []string{openaiKind, claudeKind, readKind, shellKind},
		Blacklist:   []string{openaiKind, readKind},
	}

	cfg, err := Scaffold(&agentPlugins{}, opts)
	if err != nil {
		t.Fatal(err)
	}
	llm, _ := cfg["llm"].(map[string]any)
	if llm["use"] != claudeKind {
		t.Fatalf("llm=%v want %q", llm, claudeKind)
	}
	tools, _ := cfg["tools"].([]any)
	if len(tools) != 1 {
		t.Fatalf("tools=%v", tools)
	}
	tool, _ := tools[0].(map[string]any)
	if tool["use"] != shellKind {
		t.Fatalf("tool=%v", tool)
	}
}

type scaffoldLocalLLM interface {
	LocalModel() string
}

type scaffoldLocalLLMImpl struct{}

func (scaffoldLocalLLMImpl) LocalModel() string { return "x" }

func TestScaffoldMissingRequiredField(t *testing.T) {
	kindName := kind(t, "missing")
	pluginkit.Register(kindName, func() (*shell, error) { return &shell{}, nil })

	type plugins struct {
		LLM scaffoldLocalLLM `json:"llm"`
	}
	_, err := Scaffold(&plugins{}, ScaffoldOptions{Whitelist: []string{kindName}})
	if err == nil || !strings.Contains(err.Error(), `no compatible plugin for field "llm"`) {
		t.Fatalf("err=%v", err)
	}
}

func TestScaffoldAllocateDuplicateKindID(t *testing.T) {
	dupKind := kind(t, "dup")
	pluginkit.Register(dupKind, func() (*shell, error) { return &shell{}, nil })

	type plugins struct {
		First  Tool `json:"first"`
		Second Tool `json:"second"`
	}
	cfg, err := Scaffold(&plugins{}, ScaffoldOptions{Whitelist: []string{dupKind}})
	if err != nil {
		t.Fatal(err)
	}
	first, _ := cfg["first"].(map[string]any)
	second, _ := cfg["second"].(map[string]any)
	if first["id"] != dupKind {
		t.Fatalf("first id=%v", first["id"])
	}
	if second["id"] != dupKind+"-2" {
		t.Fatalf("second id=%v", second["id"])
	}
}
