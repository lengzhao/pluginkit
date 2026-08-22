package manager

import (
	"sync"
	"testing"

	"github.com/lengzhao/pluginkit"
)

type llm interface{ Model() string }
type tool interface{ Name() string }
type store interface{ Name() string }
type step interface{ Run() string }
type workflow interface{ Run() }
type agent interface {
	LLM() llm
	Tools() []tool
}

type agentDeps struct {
	LLM   llm    `json:"llm"`
	Tools []tool `json:"tools"`
	Hook  any    `json:"hook,omitempty"`
}

type openaiCfg struct {
	Model string `json:"model"`
}

func newAgent(_ struct{}, deps agentDeps) (agent, error) { return stubAgent{}, nil }
func newOpenAI(cfg openaiCfg) (llm, error)               { return stubLLM{cfg.Model}, nil }
func newReadFile() (tool, error)                         { return stubTool{}, nil }
func newShell() (tool, error)                            { return stubTool{}, nil }
func newStore() (store, error)                           { return stubStore{}, nil }
func newHTTP() (step, error)                             { return stubStep{}, nil }
func newSave(_ struct{}, deps struct {
	Store store `json:"store"`
}) (step, error) {
	return stubStep{}, nil
}
func newWorkflow(_ struct{}, deps struct {
	Steps []step `json:"steps"`
}) (workflow, error) {
	return stubWF{}, nil
}

type stubAgent struct{}
type stubLLM struct{ model string }
type stubTool struct{}
type stubStore struct{}
type stubStep struct{}
type stubWF struct{}

func (stubAgent) LLM() llm      { return stubLLM{} }
func (stubAgent) Tools() []tool { return nil }
func (s stubLLM) Model() string { return s.model }
func (stubTool) Name() string   { return "t" }
func (stubStore) Name() string  { return "s" }
func (stubStep) Run() string    { return "r" }
func (stubWF) Run()             {}

var registerTestPluginsOnce sync.Once

func ensureTestPlugins(t *testing.T) {
	t.Helper()
	registerTestPluginsOnce.Do(func() {
		pluginkit.Register("agent", newAgent)
		pluginkit.Register("openai", newOpenAI)
		pluginkit.Register("read-file", newReadFile)
		pluginkit.Register("shell", newShell)
		pluginkit.Register("sqlite-store", newStore)
		pluginkit.Register("http-step", newHTTP)
		pluginkit.Register("save-step", newSave)
		pluginkit.Register("sequential-workflow", newWorkflow)
	})
}

func TestResolvePathRootAndNested(t *testing.T) {
	ensureTestPlugins(t)
	doc := Document{
		RootID: "agent",
		Plugin: PluginNode{
			Use: "agent",
			Deps: map[string]any{
				"llm": PluginNode{Use: "openai", Config: map[string]any{"model": "m"}},
				"tools": []any{
					PluginNode{Use: "read-file"},
					"shared-tool",
				},
			},
		},
		Shared: map[string]PluginNode{
			"shared-tool": {Use: "shell"},
		},
	}

	root, err := resolvePath(doc, "root")
	if err != nil || root.Node == nil || root.Node.Use != "agent" {
		t.Fatalf("root=%#v err=%v", root, err)
	}
	llm, err := resolvePath(doc, "root.deps.llm")
	if err != nil || llm.Node == nil || llm.Node.Use != "openai" {
		t.Fatalf("llm=%#v err=%v", llm, err)
	}
	item, err := resolvePath(doc, "root.deps.tools[1]")
	if err != nil || item.RefID != "shared-tool" {
		t.Fatalf("tools[1]=%#v err=%v", item, err)
	}
	sh, err := resolvePath(doc, "shared.shared-tool")
	if err != nil || sh.Node == nil || sh.Node.Use != "shell" {
		t.Fatalf("shared=%#v err=%v", sh, err)
	}
	if _, err := resolvePath(doc, "root.deps.missing"); err == nil {
		t.Fatal("expected unknown path")
	}
}
