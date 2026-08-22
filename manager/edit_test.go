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

func TestPlanDiagnosticsMapsInlineBuildID(t *testing.T) {
	ensureTestPlugins(t)
	doc := Document{
		RootID: "agent",
		Plugin: PluginNode{
			Use: "agent",
			Deps: map[string]any{
				"llm":   PluginNode{Use: "no-such"},
				"tools": []any{},
			},
		},
	}
	diags := planDiagnostics(doc)
	if !hasDiag(diags, "root.deps.llm", "plan") {
		t.Fatalf("expected inline build id mapped to root.deps.llm, diags=%#v", diags)
	}
	for _, d := range diags {
		if d.Path == "agent.llm" || d.Path == "agent.deps.llm" {
			t.Fatalf("untranslated build instance id %q, diags=%#v", d.Path, diags)
		}
	}
}

func TestPlanErrorPathTranslatesBuildIDs(t *testing.T) {
	doc := Document{
		RootID: "agent",
		Shared: map[string]PluginNode{
			"save": {Use: "save-step"},
		},
	}
	tests := []struct {
		id   string
		path string
		ok   bool
	}{
		{"", "root", false},
		{"agent", "root", true},
		{"save", "shared.save", true},
		{"agent.llm", "root.deps.llm", true},
		{"agent.llm.hook", "root.deps.llm.deps.hook", true},
		{"agent.tools[0]", "root.deps.tools[0]", true},
		{"agent.tools[0].store", "root.deps.tools[0].deps.store", true},
		{"save.store", "shared.save.deps.store", true},
		{"orphan.x", "root", false},
		{"agent.", "root", false},
	}
	for _, tt := range tests {
		got, ok := planErrorPath(doc, tt.id)
		if got != tt.path || ok != tt.ok {
			t.Fatalf("id=%q got (%q, %v) want (%q, %v)", tt.id, got, ok, tt.path, tt.ok)
		}
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

func TestProjectViewEmptyAgentSlots(t *testing.T) {
	ensureTestPlugins(t)
	doc := Document{RootID: "agent", Plugin: PluginNode{Use: "agent"}}
	view := projectView(doc)
	if view.Root == nil || view.Root.Kind != "agent" || view.Root.Role != "inline" {
		t.Fatalf("root=%#v", view.Root)
	}
	llm := slotByName(view.Root, "llm")
	if llm == nil || llm.Status != "empty" || !contains(llm.Kinds, "openai") {
		t.Fatalf("llm slot=%#v", llm)
	}
	if len(llm.Items) != 0 {
		t.Fatalf("empty slot should have no items")
	}
}

func TestProjectViewRefItem(t *testing.T) {
	ensureTestPlugins(t)
	doc := Document{
		RootID: "workflow",
		Plugin: PluginNode{
			Use:  "sequential-workflow",
			Deps: map[string]any{"steps": []any{"fetch"}},
		},
		Shared: map[string]PluginNode{"fetch": {Use: "http-step"}},
	}
	view := projectView(doc)
	steps := slotByName(view.Root, "steps")
	if steps == nil || len(steps.Items) != 1 || steps.Items[0].Role != "ref" || steps.Items[0].RefID != "fetch" {
		t.Fatalf("steps=%#v", steps)
	}
	if len(view.Shared) != 1 || view.Shared[0].Path != "shared.fetch" {
		t.Fatalf("shared=%#v", view.Shared)
	}
}

func TestProjectViewInlineConfigAndSlotRefs(t *testing.T) {
	ensureTestPlugins(t)
	doc := Document{
		RootID: "agent",
		Plugin: PluginNode{
			Use: "agent",
			Deps: map[string]any{
				"llm": PluginNode{
					Use:    "openai",
					Config: map[string]any{"model": "m"},
				},
			},
		},
		Shared: map[string]PluginNode{
			"gpt": {Use: "openai", Config: map[string]any{"model": "x"}},
		},
	}
	view := projectView(doc)
	if view.Root == nil || view.Root.Path != "root" {
		t.Fatalf("root path=%#v", view.Root)
	}
	llm := slotByName(view.Root, "llm")
	if llm == nil || llm.Status != "filled" || llm.Path != "root.deps.llm" {
		t.Fatalf("llm=%#v", llm)
	}
	if len(llm.Items) != 1 || llm.Items[0].Role != "inline" || llm.Items[0].Kind != "openai" || llm.Items[0].Path != "root.deps.llm" {
		t.Fatalf("llm items=%#v", llm.Items)
	}
	if len(llm.Items[0].Config) == 0 || llm.Items[0].Config[0].Name != "model" || llm.Items[0].Config[0].Value != "m" {
		t.Fatalf("config=%#v", llm.Items[0].Config)
	}
	if !hasRef(llm.Refs, "gpt", "openai") {
		t.Fatalf("expected gpt ref candidate, refs=%#v", llm.Refs)
	}
	tools := slotByName(view.Root, "tools")
	if tools == nil || tools.Status != "empty" || tools.Path != "root.deps.tools" {
		t.Fatalf("tools=%#v", tools)
	}
	hook := slotByName(view.Root, "hook")
	if hook == nil || hook.Status != "empty" {
		t.Fatalf("optional hook should still appear as empty slot, hook=%#v", hook)
	}
}

func TestProjectViewSharedHasSlots(t *testing.T) {
	ensureTestPlugins(t)
	doc := Document{
		RootID: "workflow",
		Plugin: PluginNode{
			Use:  "sequential-workflow",
			Deps: map[string]any{"steps": []any{"save"}},
		},
		Shared: map[string]PluginNode{
			"save": {Use: "save-step"},
		},
	}
	view := projectView(doc)
	if len(view.Shared) != 1 || view.Shared[0].Role != "shared" || view.Shared[0].Kind != "save-step" {
		t.Fatalf("shared=%#v", view.Shared)
	}
	store := slotByName(&view.Shared[0], "store")
	if store == nil || store.Status != "empty" || store.Path != "shared.save.deps.store" {
		t.Fatalf("store slot=%#v", store)
	}
	steps := slotByName(view.Root, "steps")
	if steps == nil || len(steps.Items) != 1 {
		t.Fatalf("steps=%#v", steps)
	}
	item := steps.Items[0]
	if item.Role != "ref" || item.Kind != "save-step" || item.RefID != "save" || item.RefCount != 1 {
		t.Fatalf("ref item=%#v", item)
	}
	if item.Path != "root.deps.steps[0]" {
		t.Fatalf("ref path=%q", item.Path)
	}
	if len(item.Slots) != 0 {
		t.Fatalf("ref must not expand subtree, slots=%#v", item.Slots)
	}
}

func slotByName(node *ViewNode, name string) *ViewSlot {
	if node == nil {
		return nil
	}
	for i := range node.Slots {
		if node.Slots[i].Name == name {
			return &node.Slots[i]
		}
	}
	return nil
}

func contains(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func hasRef(refs []RefCandidate, id, kind string) bool {
	for _, ref := range refs {
		if ref.ID == id && ref.Kind == kind {
			return true
		}
	}
	return false
}

func TestApplyNewRootHasEmptyDeps(t *testing.T) {
	ensureTestPlugins(t)
	out, err := apply(Document{}, Operation{Type: "newRoot", Kind: "agent"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Plugin.Use != "agent" || out.RootID != "agent" {
		t.Fatalf("doc=%#v", out)
	}
	if len(out.Plugin.Deps) != 0 {
		t.Fatalf("deps must be empty, got %#v", out.Plugin.Deps)
	}
}

func TestApplyAttachAndHoist(t *testing.T) {
	ensureTestPlugins(t)
	doc, err := apply(Document{}, Operation{Type: "newRoot", Kind: "agent"})
	if err != nil {
		t.Fatal(err)
	}
	doc, err = apply(doc, Operation{Type: "attach", Path: "root.deps.llm", Kind: "openai"})
	if err != nil {
		t.Fatal(err)
	}
	doc, err = apply(doc, Operation{Type: "hoist", Path: "root.deps.llm", ID: "llm"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := doc.Plugin.Deps["llm"].(string); !ok {
		t.Fatalf("expected ref, got %#v", doc.Plugin.Deps["llm"])
	}
	if doc.Shared["llm"].Use != "openai" {
		t.Fatalf("shared=%#v", doc.Shared)
	}
}

func TestApplyRemoveOrphanShared(t *testing.T) {
	ensureTestPlugins(t)
	doc := Document{
		RootID: "agent",
		Plugin: PluginNode{Use: "agent", Deps: map[string]any{"llm": "llm", "tools": []any{}}},
		Shared: map[string]PluginNode{"llm": {Use: "openai"}},
	}
	out, err := apply(doc, Operation{Type: "remove", Path: "root.deps.llm"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := out.Plugin.Deps["llm"]; ok {
		t.Fatal("llm slot should be empty")
	}
	if _, ok := out.Shared["llm"]; ok {
		t.Fatal("orphan shared should be deleted")
	}
}

func TestApplyImportYAML(t *testing.T) {
	ensureTestPlugins(t)
	raw := "agent:\n  use: agent\n  deps:\n    llm:\n      use: openai\n"
	out, err := apply(Document{}, Operation{Type: "importYAML", YAML: raw})
	if err != nil {
		t.Fatal(err)
	}
	if out.RootID != "agent" {
		t.Fatalf("root=%q", out.RootID)
	}
	llm, ok := out.Plugin.Deps["llm"].(PluginNode)
	if !ok {
		if m, ok := out.Plugin.Deps["llm"].(map[string]any); ok {
			if m["use"] != "openai" {
				t.Fatalf("llm=%#v", out.Plugin.Deps["llm"])
			}
			return
		}
		t.Fatalf("llm=%#v", out.Plugin.Deps["llm"])
	}
	if llm.Use != "openai" {
		t.Fatalf("llm=%#v", llm)
	}
}

func TestApplyUnknownPathIsError(t *testing.T) {
	ensureTestPlugins(t)
	_, err := apply(Document{RootID: "agent", Plugin: PluginNode{Use: "agent"}}, Operation{
		Type: "attach", Path: "root.foo", Kind: "openai",
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestApplySetConfigNested(t *testing.T) {
	ensureTestPlugins(t)
	doc, err := apply(Document{}, Operation{Type: "newRoot", Kind: "agent"})
	if err != nil {
		t.Fatal(err)
	}
	doc, err = apply(doc, Operation{Type: "attach", Path: "root.deps.llm", Kind: "openai"})
	if err != nil {
		t.Fatal(err)
	}
	doc, err = apply(doc, Operation{Type: "setConfig", Path: "root.deps.llm", Config: map[string]any{"model": "gpt-5"}})
	if err != nil {
		t.Fatal(err)
	}
	llm, ok := doc.Plugin.Deps["llm"].(PluginNode)
	if !ok {
		t.Fatalf("llm=%#v", doc.Plugin.Deps["llm"])
	}
	if llm.Config["model"] != "gpt-5" {
		t.Fatalf("config=%#v", llm.Config)
	}
}
