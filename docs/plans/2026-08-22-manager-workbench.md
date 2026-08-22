# Manager 工作台 Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 把 manager 改成「装配树 + 检查器」工作台：后端拥有图语义（path、edit op、view 投影、分层诊断），前端只渲染返回结果。

**Architecture:** 浏览器持有 `Document`。`POST /api/edit` 接收 `{document, op}`，应用操作后投影 `view`，并跑 structure + `build.ValidatePlan`。`POST /api/build` 才调用宿主 `ValidateBuild`。UI 按 `view` 画树和检查器，诊断按 path 钉节点。嵌入静态资源不变。

**Tech Stack:** Go, `net/http`, `testing`, `httptest`, embed 的 HTML/CSS/JS。规格：`docs/2026-08-21-web-manager.md`。

**规格要点（实现时对照）：**

- 新建 root 的 `deps` 必须是空对象，不要写入 `use: ""` 占位。
- `hoist` 只接受内联节点；`remove` 解除引用后默认删除孤儿 shared。
- 删除旧路由：`/api/describe`、`/api/compatible`、`/api/template`、`/api/export`、`/api/import`、`/api/validate`。
- 保留 `GET /api/catalog`。
- 不引入新 JS 框架、不引入服务端会话。

测试插件用 `sync.Once` 注册一次（`manager` 包测无法调用 `resetRegistry`）。

---

### Task 1: Path 寻址

**Files:**
- Create: `manager/path.go`
- Create: `manager/edit_test.go`（本任务只写 path 相关测试；后续任务往同一文件加测试）

**Step 1: Write the failing test**

在 `manager/edit_test.go` 写入测试插件与 path 测试：

```go
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
```

**Step 2: Run test to verify it fails**

Run: `go test ./manager -run TestResolvePathRootAndNested -count=1`

Expected: FAIL，`resolvePath` 未定义。

**Step 3: Write minimal implementation**

在 `manager/path.go` 实现：

```go
package manager

import (
	"fmt"
	"strconv"
	"strings"
)

type resolved struct {
	Parent *PluginNode
	Ext    string
	Index  int // 非列表为 -1；列表项为下标
	Node   *PluginNode
	RefID  string
}

func resolvePath(doc Document, path string) (resolved, error) {
	// 解析 root / root.deps.x / root.deps.x[i] / shared.id / shared.id.deps...
	// 未知 path 返回 error
}
```

规则：

- `root` → `Node` 指向 `doc.Plugin`。
- `shared.<id>` → `Node` 指向 `doc.Shared[id]`。
- `*.deps.name` 单值：值为 `PluginNode` 则填 `Node`；值为 `string` 则填 `RefID`；缺省则 `Node`/`RefID` 皆空（空槽）。
- `*.deps.name[i]`：列表项，同样区分节点与引用。
- `Index` 非列表为 `-1`。

**Step 4: Run the tests and make sure they pass**

Run: `go test ./manager -run TestResolvePathRootAndNested -count=1`

Expected: PASS。

**Step 5: Commit**

```bash
git add manager/path.go manager/edit_test.go
git commit -m "$(cat <<'EOF'
feat(manager): add document path addressing

EOF
)"
```

---

### Task 2: structure 诊断

**Files:**
- Create: `manager/diagnostic.go`
- Modify: `manager/validate.go`
- Modify: `manager/edit_test.go`

**Step 1: Write the failing test**

```go
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

func hasDiag(diags []Diagnostic, path, code string) bool {
	for _, d := range diags {
		if d.Path == path && d.Code == code {
			return true
		}
	}
	return false
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./manager -run 'TestStructureDiagnostics' -count=1`

Expected: FAIL，`Diagnostic` / `structureDiagnostics` 未定义。

**Step 3: Write minimal implementation**

`manager/diagnostic.go`：

```go
type Diagnostic struct {
	Path     string `json:"path"`
	Severity string `json:"severity"`
	Stage    string `json:"stage"`
	Code     string `json:"code"`
	Message  string `json:"message"`
}
```

`structureDiagnostics` 沿用 `validateTree` 的规则，但产出 `[]Diagnostic` 而不是第一个 error：

- 必填扩展点缺失 → `path` 为槽 path，`code=missing_dep`，`stage=structure`。
- 未知引用 → `unknown_ref`。
- 未知 kind → `unknown_kind`。
- 返回类型不满足扩展点 → `incompatible`。
- 可选 `hook` 未填不报错。
- slice 缺省视为空列表；`tools` 对 agent 是必填字段但 slice 可为空——对照 `validate.go`：当前实现把缺失 key 当缺失。保持与现有 `validateTree` 一致：deps 中没有该 key 才 `missing_dep`；空数组算已填。因此 agent 新建应在测试里对 `tools` 给出空数组，或对缺失 key 报 `missing_dep`。**实现与 `validateTree` 对齐：没有 key 就是 missing。** 空数组 `[]` 不算 missing。

可把 `validate.go` 的 `validateTree` 改成基于 `structureDiagnostics`：有 error 级诊断则 `Validate()` 失败。不要两套规则。

plan 诊断：`planDiagnostics(doc)` 调用现有 `validatePlan`，若 `errors.As` 到 `*build.Error`，用 `ID` 映射 path（`ID==RootID` → `root`；`shared[ID]` 存在 → `shared.ID`；否则用 `ID` 原文）。映射不到则 `path=root`，`code=plan`，`stage=plan`。

本任务可以只做 structure；plan 映射放在同文件，并加一个未知 kind 的 plan 测试（可选）。至少导出：

```go
func collectDiagnostics(doc Document) []Diagnostic {
    diags := structureDiagnostics(doc)
    diags = append(diags, planDiagnostics(doc)...)
    return diags
}
```

structure 已失败时，plan 仍可跑；重复信息不强制去重。

**Step 4: Run the tests and make sure they pass**

Run: `go test ./manager -count=1`

Expected: PASS（含原有 YAML roundtrip）。

**Step 5: Commit**

```bash
git add manager/diagnostic.go manager/validate.go manager/edit_test.go
git commit -m "$(cat <<'EOF'
feat(manager): emit path-scoped structure diagnostics

EOF
)"
```

---

### Task 3: view 投影

**Files:**
- Create: `manager/view.go`
- Modify: `manager/edit_test.go`

**Step 1: Write the failing test**

```go
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
```

补上 `slotByName` / `contains` 测试辅助函数。

**Step 2: Run test to verify it fails**

Run: `go test ./manager -run 'TestProjectView' -count=1`

Expected: FAIL。

**Step 3: Write minimal implementation**

`View` / `ViewNode` / `ViewSlot` / `ViewField` / `RefCandidate` 的 JSON 字段名与规格一致。

`projectView`：

- 从 root 递归：每个 kind 用 `pluginkit.Describe` 列 extensions。
- 槽 `path` = `parentPath+".deps."+name` 或带 `[i]`。
- 空槽 `status=empty`，`kinds=CompatibleKinds(ext.Type)`；`refs` 为 shared 中 kind 落在 `kinds` 里的 `{id, kind}`。
- 内联子节点 `role=inline`，继续投影 slots。
- 引用 `role=ref`，不展开子树；`kind` 从 shared/root 解析。
- `view.Shared` 对每个 shared id 投影为 `role=shared` 的节点（含子槽），供检查器跳转。
- config 字段做成 `ViewField{Name,Type,Value}`。

**Step 4: Run the tests and make sure they pass**

Run: `go test ./manager -count=1`

Expected: PASS。

**Step 5: Commit**

```bash
git add manager/view.go manager/edit_test.go
git commit -m "$(cat <<'EOF'
feat(manager): project assembly view for the workbench UI

EOF
)"
```

---

### Task 4: 应用 edit 操作

**Files:**
- Create: `manager/edit.go`
- Modify: `manager/edit_test.go`

**Step 1: Write the failing tests**

```go
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
	if out.RootID != "agent" || out.Plugin.Deps["llm"].(PluginNode).Use != "openai" {
		t.Fatalf("out=%#v", out)
	}
}

func TestApplyUnknownPathIsError(t *testing.T) {
	ensureTestPlugins(t)
	_, err := apply(Document{RootID: "agent", Plugin: PluginNode{Use: "agent"}}, Operation{
		Type: "attach", Path: "root.deps.nope", Kind: "openai",
	})
	if err == nil {
		t.Fatal("expected error")
	}
}
```

注意 `Deps["llm"].(PluginNode)` 在 JSON roundtrip 后可能是 `map[string]any`。`apply` 内部保持 `PluginNode`；`importYAML` 走现有 `FromYAML`。若类型断言失败，用已有 `decodeDepNode`。

**Step 2: Run test to verify it fails**

Run: `go test ./manager -run 'TestApply' -count=1`

Expected: FAIL。

**Step 3: Write minimal implementation**

```go
type Operation struct {
	Type         string         `json:"type"`
	Path         string         `json:"path,omitempty"`
	Kind         string         `json:"kind,omitempty"`
	RefID        string         `json:"refId,omitempty"`
	ID           string         `json:"id,omitempty"`
	Config       map[string]any `json:"config,omitempty"`
	YAML         string         `json:"yaml,omitempty"`
	DeleteOrphan *bool          `json:"deleteOrphan,omitempty"`
}

func apply(doc Document, op Operation) (Document, error)
```

行为：

- `validate`：原样返回（深拷贝）。
- `newRoot`：`Describe(kind)`，`config` 用 `Template()` 的 config 部分（若有），`deps={}`，`RootID` 默认 `kind`，`Shared={}`。
- `setRootId`：冲突则 error。
- `attach`：用 template 建内联节点（deps 仍空）；列表 append；单值替换。
- `attachRef`：`refId` 必须存在于 shared（或等于 rootId）。
- `remove`：删节点/引用；`deleteOrphan` 默认 true，引用计数为 0 则删 shared。
- `setConfig`：写到该节点。
- `hoist`：非 inline（root / ref / 空槽）则 error；`id` 空、冲突、等于 rootId 则 error；把节点移入 shared，原处写 string id。
- `importYAML`：`FromYAML`。
- 未知 type：error。

`apply` 必须深拷贝输入，失败时调用方仍持有旧文档。

**Step 4: Run the tests and make sure they pass**

Run: `go test ./manager -count=1`

Expected: PASS。

**Step 5: Commit**

```bash
git add manager/edit.go manager/edit_test.go manager/path.go
git commit -m "$(cat <<'EOF'
feat(manager): apply workbench edit operations

EOF
)"
```

---

### Task 5: HTTP /api/edit 与 /api/build

**Files:**
- Modify: `manager/api.go`
- Create: `manager/api_test.go`

**Step 1: Write the failing test**

```go
func TestEditAPIAttachReturnsViewAndDiagnostics(t *testing.T) {
	ensureTestPlugins(t)
	handler, err := New(Options{})
	if err != nil {
		t.Fatal(err)
	}
	body := `{"document":{"rootId":"","plugin":{"use":""}},"op":{"type":"newRoot","kind":"agent"}}`
	req := httptest.NewRequest(http.MethodPost, "/api/edit", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.Bytes())
	}
	var resp editResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Document.Plugin.Use != "agent" || resp.View.Root == nil {
		t.Fatalf("resp=%#v", resp)
	}
	if !hasDiag(resp.Diagnostics, "root.deps.llm", "missing_dep") {
		t.Fatalf("diags=%#v", resp.Diagnostics)
	}
	if resp.YAML == "" {
		t.Fatal("expected yaml")
	}
}

func TestBuildAPIDoesNotMutate(t *testing.T) {
	ensureTestPlugins(t)
	called := false
	handler, err := New(Options{
		ValidateBuild: func(ctx context.Context, doc Document) error {
			called = true
			return fmt.Errorf("boom")
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	doc := Document{
		RootID: "agent",
		Plugin: PluginNode{
			Use: "agent",
			Deps: map[string]any{
				"llm":   PluginNode{Use: "openai", Config: map[string]any{"model": "m"}},
				"tools": []any{PluginNode{Use: "shell"}},
			},
		},
	}
	raw, _ := json.Marshal(map[string]any{"document": doc})
	req := httptest.NewRequest(http.MethodPost, "/api/build", bytes.NewReader(raw))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.Bytes())
	}
	if !called {
		t.Fatal("expected ValidateBuild")
	}
	var resp editResponse
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if !hasDiag(resp.Diagnostics, "root", "build") && !hasBuildDiag(resp.Diagnostics) {
		t.Fatalf("diags=%#v", resp.Diagnostics)
	}
}

func TestRemovedRoutesGone(t *testing.T) {
	handler, err := New(Options{})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/validate", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != 404 {
		t.Fatalf("status=%d", rec.Code)
	}
}
```

`hasBuildDiag`：任意 `stage=="build"` 或 `code=="build"`。

**Step 2: Run test to verify it fails**

Run: `go test ./manager -run 'TestEditAPI|TestBuildAPI|TestRemovedRoutes' -count=1`

Expected: FAIL（404 或旧 validate 仍 200）。

**Step 3: Write minimal implementation**

`registerRoutes` 只保留：

```go
mux.HandleFunc("GET /api/catalog", s.handleCatalog)
mux.HandleFunc("POST /api/edit", s.handleEdit)
mux.HandleFunc("POST /api/build", s.handleBuild)
```

删除 describe/compatible/template/export/import/validate handler。

```go
type editRequest struct {
	Document Document  `json:"document"`
	Op       Operation `json:"op"`
}

type editResponse struct {
	Document     Document     `json:"document"`
	View         View         `json:"view"`
	Diagnostics  []Diagnostic `json:"diagnostics"`
	YAML         string       `json:"yaml"`
}
```

`handleEdit`：`apply` 失败 → 400 `{"error":"..."}`；成功则 `ToYAML` + `projectView` + `collectDiagnostics` → 200。

`handleBuild`：不 `apply`；`collectDiagnostics`；若存在 error 级 structure/plan，不调用 `ValidateBuild`；否则调用并追加 `stage=build` 诊断（失败 `code=build`，`path=root` 或从 `*build.Error` 映射）。始终 200（除非 JSON 坏了 400）。未配置 `ValidateBuild` 且无结构/plan 错误 → 空 build 诊断（或一条 info，不要）。规格：plan 通过即成功，`diagnostics` 可无 build 条。测试里要断言 `called`，把 `ValidateBuild` 配成返回 error。

**Step 4: Run the tests and make sure they pass**

Run: `go test ./manager -count=1`

Expected: PASS。

**Step 5: Commit**

```bash
git add manager/api.go manager/api_test.go
git commit -m "$(cat <<'EOF'
feat(manager): replace CRUD endpoints with edit and build APIs

EOF
)"
```

---

### Task 6: 工作台 HTML/CSS

**Files:**
- Modify: `manager/ui/index.html`
- Modify: `manager/ui/style.css`

**Step 1: Replace the layout**

`index.html` 改为三块：顶栏、`#tree`、`#inspector`。去掉目录侧栏、共享区、树/扁平切换、常驻 YAML 列、config/shared 专用 dialog。保留导入 YAML `<dialog>`。检查器内放 `#inspectorBody` 与可折叠 `#yamlPreview`。

顶栏元素 id：`rootId`、`rootKind`、`issueCount`、`btnImport`、`btnExport`、`btnBuild`、`btnNew`。

**Step 2: Restyle for tree + inspector**

CSS：顶栏 + 主区（树可滚动）+ 右栏检查器（约 360px）。空槽、引用芯片、节点错误点、检查器表单。删除 `.sidebar` / `.shared-section` / `.view-toggle` 等旧规则。

本任务无自动测试。用浏览器看静态结构即可（JS 下一任务才接 API）。

**Step 3: Commit**

```bash
git add manager/ui/index.html manager/ui/style.css
git commit -m "$(cat <<'EOF'
feat(manager): switch UI shell to tree and inspector layout

EOF
)"
```

---

### Task 7: 前端只渲染 edit 响应

**Files:**
- Modify: `manager/ui/app.js`

**Step 1: Rewrite app.js**

状态：`{ catalog, document, view, diagnostics, selectedPath, yaml }`。

启动：`GET /api/catalog` 填 `rootKind`；`POST /api/edit` `op=newRoot`（catalog 有 `agent` 则用 agent，否则第一个 kind）。

```js
async function edit(op) {
  const data = await api("/api/edit", {
    method: "POST",
    body: JSON.stringify({ document: state.document, op }),
  });
  state.document = data.document;
  state.view = data.view;
  state.diagnostics = data.diagnostics || [];
  state.yaml = data.yaml || "";
  render();
}
```

禁止：本地改 `deps`、`inlineSingleUseShared`、调用 `/api/compatible` `/api/template` `/api/validate` `/api/export` `/api/import`。

渲染：

- 按 `view.root.slots` 递归画卡片；`role=ref` 画芯片；`status=empty` 画空槽。
- 诊断按 path 前缀或精确匹配给节点加点；顶栏 `issueCount` 可点击选中第一条 path。
- 选中空槽：检查器列出 `kinds` 与 `refs`，点击 `attach` / `attachRef`。
- 选中 inline：config 表单（input 防抖 300ms `setConfig`）、提取为共享（prompt id 后 `hoist`）、删除 `remove`。
- 选中 ref：跳到 `shared.<id>`、解除 `remove`。
- 选中 shared：检查器画该节点 slots（来自 `view.shared`）。
- 导入：`op=importYAML`。导出：把 `state.yaml` 写入剪贴板。
- 试装配：`POST /api/build`，用返回的 diagnostics 覆盖展示，不改 document。
- 换 root kind / 新建：文档有内容则 `confirm`，然后 `newRoot`。

**Step 2: Manual check against demo plugins**

Run: `go run ./examples/manager`

在 http://localhost:8080：

1. 新建 agent → llm/tools 空槽标红。
2. 为 llm 选 openai，填 model，tools 加 read-file。
3. 提取 openai 为共享 `llm`，树变 `→ llm`。
4. 解除引用后共享定义消失。
5. 导入规格里的 workflow YAML，树与引用正确。
6. 试装配：demo 的 agent/workflow 能通过。

**Step 3: Commit**

```bash
git add manager/ui/app.js manager/ui/index.html manager/ui/style.css
git commit -m "$(cat <<'EOF'
feat(manager): drive the workbench from edit API responses

EOF
)"
```

---

### Task 8: 文档与包注释对齐

**Files:**
- Modify: `manager/doc.go`
- Modify: `docs/2026-08-21-web-manager.md`（若实现时有 API 细节差异，改文档而不是让代码偏离规格）
- Modify: `docs/plans/2026-08-22-manager-redesign-design.md`（仅当决策变更）

**Step 1: Update package comment**

`manager/doc.go` 改为说明工作台、`/api/edit`、`/api/build`，不要再写旧的导入/校验按钮流。

**Step 2: Mark spec implemented**

把 `docs/2026-08-21-web-manager.md` 状态改为 `已实现`。若实现时字段名有不可避免的调整，同步改该文档。

**Step 3: Run full tests**

Run: `go test ./...`

Expected: PASS。

**Step 4: Commit**

```bash
git add manager/doc.go docs/2026-08-21-web-manager.md
git commit -m "$(cat <<'EOF'
docs(manager): align package comment with the workbench spec

EOF
)"
```

---

## 完成标准

- `go test ./...` 通过。
- UI 不再持有图语义，刷新后除内存中的当前文档外无服务端会话。
- 导出 YAML 仍能被 `build.Build` 消费（demo `ValidateBuild` 可证明）。
