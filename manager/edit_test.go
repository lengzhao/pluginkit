package manager

import (
	"strings"
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

type blob struct{}

func newBlob() (blob, error) { return blob{}, nil }

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
		pluginkit.Register("blob", newBlob)
	})
}

func TestResolvePathRootAndNested(t *testing.T) {
	ensureTestPlugins(t)
	doc := Document{
		RootID: "agent",
		Plugin: PluginNode{
			Use: "agent",
			Deps: map[string]any{
				"llm": PluginNode{
					Use:    "openai",
					Config: map[string]any{"model": "m"},
					Deps: map[string]any{
						"hook": PluginNode{Use: "shell"},
					},
				},
				"tools": []any{
					PluginNode{Use: "read-file"},
					"shared-tool",
				},
			},
		},
		Shared: map[string]PluginNode{
			"shared-tool": {Use: "shell"},
			"save": {Use: "save-step", Deps: map[string]any{
				"store": PluginNode{Use: "sqlite-store"},
			}},
		},
	}

	root, err := resolvePath(&doc, "root")
	if err != nil || root.Node != &doc.Plugin || root.Node.Use != "agent" || root.Index != -1 {
		t.Fatalf("root=%#v err=%v", root, err)
	}

	llm, err := resolvePath(&doc, "root.deps.llm")
	if err != nil {
		t.Fatalf("llm err=%v", err)
	}
	assertSlot(t, llm, &doc.Plugin, "llm", -1)
	if llm.Node == nil || llm.Node.Use != "openai" || llm.RefID != "" {
		t.Fatalf("llm=%#v", llm)
	}

	tools, err := resolvePath(&doc, "root.deps.tools")
	if err != nil {
		t.Fatalf("tools slot err=%v", err)
	}
	assertSlot(t, tools, &doc.Plugin, "tools", -1)
	if tools.Node != nil || tools.RefID != "" {
		t.Fatalf("list slot should have empty Node/RefID, got %#v", tools)
	}

	item0, err := resolvePath(&doc, "root.deps.tools[0]")
	if err != nil {
		t.Fatalf("tools[0] err=%v", err)
	}
	assertSlot(t, item0, &doc.Plugin, "tools", 0)
	if item0.Node == nil || item0.Node.Use != "read-file" || item0.RefID != "" {
		t.Fatalf("tools[0]=%#v", item0)
	}

	item1, err := resolvePath(&doc, "root.deps.tools[1]")
	if err != nil {
		t.Fatalf("tools[1] err=%v", err)
	}
	assertSlot(t, item1, &doc.Plugin, "tools", 1)
	if item1.RefID != "shared-tool" || item1.Node != nil {
		t.Fatalf("tools[1]=%#v", item1)
	}

	sh, err := resolvePath(&doc, "shared.shared-tool")
	if err != nil || sh.Node == nil || sh.Node.Use != "shell" || sh.Index != -1 {
		t.Fatalf("shared=%#v err=%v", sh, err)
	}

	nested, err := resolvePath(&doc, "root.deps.llm.deps.hook")
	if err != nil {
		t.Fatalf("nested err=%v", err)
	}
	if nested.Parent == nil || nested.Ext != "hook" || nested.Index != -1 {
		t.Fatalf("nested slot=%#v", nested)
	}
	if nested.Node == nil || nested.Node.Use != "shell" {
		t.Fatalf("nested=%#v", nested)
	}

	store, err := resolvePath(&doc, "shared.save.deps.store")
	if err != nil {
		t.Fatalf("shared deps err=%v", err)
	}
	if store.Parent == nil || store.Parent.Use != "save-step" || store.Ext != "store" || store.Index != -1 {
		t.Fatalf("store slot=%#v", store)
	}
	if store.Node == nil || store.Node.Use != "sqlite-store" {
		t.Fatalf("store=%#v", store)
	}

	empty, err := resolvePath(&doc, "root.deps.missing")
	if err != nil {
		t.Fatalf("missing key should be empty slot, err=%v", err)
	}
	assertSlot(t, empty, &doc.Plugin, "missing", -1)
	if empty.Node != nil || empty.RefID != "" {
		t.Fatalf("empty slot=%#v", empty)
	}

	doc.Plugin.Deps["empty-tools"] = []any{}
	listSlot, err := resolvePath(&doc, "root.deps.empty-tools")
	if err != nil {
		t.Fatalf("empty list err=%v", err)
	}
	assertSlot(t, listSlot, &doc.Plugin, "empty-tools", -1)
	if listSlot.Node != nil || listSlot.RefID != "" {
		t.Fatalf("empty list slot=%#v", listSlot)
	}

	for _, path := range []string{"", "foo", "root.foo", "shared.no-such", "root.deps.tools[5]"} {
		if _, err := resolvePath(&doc, path); err == nil {
			t.Fatalf("expected unknown path %q", path)
		}
	}
	if _, err := resolvePath(&doc, "root.deps.tools[1].deps.x"); err == nil {
		t.Fatal("expected unknown path through ref")
	}
}

func TestResolvePathWriteBack(t *testing.T) {
	ensureTestPlugins(t)
	doc := Document{
		RootID: "agent",
		Plugin: PluginNode{Use: "agent"},
		Shared: map[string]PluginNode{
			"save": {Use: "save-step"},
		},
	}

	root, err := resolvePath(&doc, "root")
	if err != nil || root.Node != &doc.Plugin {
		t.Fatalf("root node must alias document plugin, got %#v err=%v", root, err)
	}
	root.Node.Use = "agent-2"
	if doc.Plugin.Use != "agent-2" {
		t.Fatalf("root node write did not persist, use=%q", doc.Plugin.Use)
	}

	slot, err := resolvePath(&doc, "root.deps.llm")
	if err != nil {
		t.Fatal(err)
	}
	assertSlot(t, slot, &doc.Plugin, "llm", -1)
	if slot.Parent.Deps == nil {
		slot.Parent.Deps = map[string]any{}
	}
	slot.Parent.Deps[slot.Ext] = PluginNode{Use: "openai"}
	got, ok := doc.Plugin.Deps["llm"].(PluginNode)
	if !ok || got.Use != "openai" {
		t.Fatalf("parent deps write did not persist: %#v", doc.Plugin.Deps["llm"])
	}

	doc.Plugin.Deps["tools"] = []any{}
	listSlot, err := resolvePath(&doc, "root.deps.tools")
	if err != nil {
		t.Fatal(err)
	}
	items := listSlot.Parent.Deps[listSlot.Ext].([]any)
	items = append(items, PluginNode{Use: "read-file"})
	listSlot.Parent.Deps[listSlot.Ext] = items
	gotList, _ := doc.Plugin.Deps["tools"].([]any)
	if len(gotList) != 1 {
		t.Fatalf("list append did not persist: %#v", doc.Plugin.Deps["tools"])
	}

	sharedSlot, err := resolvePath(&doc, "shared.save.deps.store")
	if err != nil {
		t.Fatal(err)
	}
	if sharedSlot.Parent == nil || sharedSlot.Ext != "store" || sharedSlot.Index != -1 {
		t.Fatalf("shared empty slot=%#v", sharedSlot)
	}
	if sharedSlot.Parent.Deps == nil {
		sharedSlot.Parent.Deps = map[string]any{}
	}
	sharedSlot.Parent.Deps[sharedSlot.Ext] = PluginNode{Use: "sqlite-store"}
	gotStore, ok := doc.Shared["save"].Deps["store"].(PluginNode)
	if !ok || gotStore.Use != "sqlite-store" {
		t.Fatalf("shared parent deps write did not persist: %#v", doc.Shared["save"])
	}
}

func assertSlot(t *testing.T, got resolved, parent *PluginNode, ext string, index int) {
	t.Helper()
	if got.Parent != parent || got.Ext != ext || got.Index != index {
		t.Fatalf("got parent=%p ext=%q index=%d, want parent=%p ext=%q index=%d (resolved=%#v)",
			got.Parent, got.Ext, got.Index, parent, ext, index, got)
	}
}

func TestStructureDiagnosticsMissingRequiredDep(t *testing.T) {
	ensureTestPlugins(t)
	doc := Document{
		RootID: "agent",
		Plugin: PluginNode{Use: "agent", Deps: map[string]any{}},
	}
	diags := structureDiagnostics(doc)
	if !hasDiag(diags, "root.deps.llm", "missing_dep") {
		t.Fatalf("diags=%#v", diags)
	}
	if !hasDiag(diags, "root.deps.tools", "missing_dep") {
		t.Fatalf("expected missing tools slot, diags=%#v", diags)
	}
}

func TestStructureDiagnosticsUnknownRef(t *testing.T) {
	ensureTestPlugins(t)
	doc := Document{
		RootID: "agent",
		Plugin: PluginNode{
			Use: "agent",
			Deps: map[string]any{
				"llm":   "missing",
				"tools": []any{},
			},
		},
	}
	diags := structureDiagnostics(doc)
	if !hasDiag(diags, "root.deps.llm", "unknown_ref") {
		t.Fatalf("diags=%#v", diags)
	}
}

func TestStructureDiagnosticsUnknownKind(t *testing.T) {
	ensureTestPlugins(t)
	doc := Document{
		RootID: "agent",
		Plugin: PluginNode{Use: "no-such"},
	}
	diags := structureDiagnostics(doc)
	if !hasDiag(diags, "root", "unknown_kind") {
		t.Fatalf("diags=%#v", diags)
	}
}

func TestStructureDiagnosticsIncompatible(t *testing.T) {
	ensureTestPlugins(t)
	doc := Document{
		RootID: "agent",
		Plugin: PluginNode{
			Use: "agent",
			Deps: map[string]any{
				"llm":   PluginNode{Use: "blob"},
				"tools": []any{},
			},
		},
	}
	diags := structureDiagnostics(doc)
	if !hasDiag(diags, "root.deps.llm", "incompatible") {
		t.Fatalf("diags=%#v", diags)
	}
}

func TestStructureDiagnosticsOptionalHookUnset(t *testing.T) {
	ensureTestPlugins(t)
	doc := Document{
		RootID: "agent",
		Plugin: PluginNode{
			Use: "agent",
			Deps: map[string]any{
				"llm":   PluginNode{Use: "openai"},
				"tools": []any{},
			},
		},
	}
	diags := structureDiagnostics(doc)
	if hasDiag(diags, "root.deps.hook", "missing_dep") {
		t.Fatalf("optional hook must not be missing_dep, diags=%#v", diags)
	}
	if hasDiag(diags, "root.deps.tools", "missing_dep") {
		t.Fatalf("empty tools slice must not be missing_dep, diags=%#v", diags)
	}
}

func TestValidateFailsOnStructureErrors(t *testing.T) {
	ensureTestPlugins(t)
	doc := Document{
		RootID: "agent",
		Plugin: PluginNode{Use: "agent", Deps: map[string]any{}},
	}
	err := doc.Validate()
	if err == nil {
		t.Fatal("expected Validate to fail on error-level structure diagnostics")
	}
	if !strings.Contains(err.Error(), "root.deps.llm") {
		t.Fatalf("Validate should surface structure diagnostic path, err=%v", err)
	}
}

func TestCollectDiagnosticsIncludesPlan(t *testing.T) {
	ensureTestPlugins(t)
	doc := Document{
		RootID: "agent",
		Plugin: PluginNode{Use: "no-such"},
	}
	diags := collectDiagnostics(doc)
	if !hasDiag(diags, "root", "unknown_kind") {
		t.Fatalf("expected structure unknown_kind, diags=%#v", diags)
	}
	if !hasDiag(diags, "root", "plan") {
		t.Fatalf("expected plan diagnostic even after structure failure, diags=%#v", diags)
	}
}

func TestPlanDiagnosticsMapsSharedID(t *testing.T) {
	ensureTestPlugins(t)
	doc := Document{
		RootID: "agent",
		Plugin: PluginNode{
			Use: "agent",
			Deps: map[string]any{
				"llm":   "llm-shared",
				"tools": []any{},
			},
		},
		Shared: map[string]PluginNode{
			"llm-shared": {Use: "no-such"},
		},
	}
	diags := planDiagnostics(doc)
	if !hasDiag(diags, "shared.llm-shared", "plan") {
		t.Fatalf("expected plan path mapped from instance id, diags=%#v", diags)
	}
}

func hasDiag(diags []Diagnostic, path, code string) bool {
	for _, d := range diags {
		if d.Path == path && d.Code == code {
			return true
		}
	}
	return false
}
