package manager

import (
	"fmt"
	"reflect"

	"github.com/lengzhao/pluginkit"
)

// View 是装配树给前端的投影：只渲染 view，不解析 deps 联合类型。
type View struct {
	Root   *ViewNode  `json:"root"`
	Shared []ViewNode `json:"shared,omitempty"`
}

// ViewNode 是装配树上的一个节点：内联子树、引用芯片、或共享定义。
type ViewNode struct {
	Path     string      `json:"path"`
	Role     string      `json:"role"`
	Kind     string      `json:"kind,omitempty"`
	RefID    string      `json:"refId,omitempty"`
	RefCount int         `json:"refCount,omitempty"`
	Config   []ViewField `json:"config,omitempty"`
	Slots    []ViewSlot  `json:"slots,omitempty"`
}

// ViewSlot 是插件扩展点：空槽带候选，已填槽带 items。
type ViewSlot struct {
	Name     string          `json:"name"`
	Path     string          `json:"path"`
	Status   string          `json:"status"`
	List     bool            `json:"list,omitempty"`
	Optional bool            `json:"optional,omitempty"`
	Type     string          `json:"type,omitempty"`
	Kinds    []KindCandidate `json:"kinds,omitempty"`
	Refs     []RefCandidate  `json:"refs,omitempty"`
	Items    []ViewNode      `json:"items,omitempty"`
}

// ViewField 是检查器里的一个 config 字段。
// Kind 是前端渲染控件的类型归类：string / number / bool / json（复杂类型兜底）。
// Default 是插件 Defaulter 给出的默认值，未实现 Defaulter 时为 nil。
type ViewField struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Kind     string `json:"kind"`
	Optional bool   `json:"optional,omitempty"`
	Value    any    `json:"value"`
	Default  any    `json:"default,omitempty"`
}

// RefCandidate 是空槽可引用的已有共享实例。
type RefCandidate struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
}

// KindCandidate 是空槽可新建的内联插件候选。
type KindCandidate struct {
	Kind       string `json:"kind"`
	ReturnType string `json:"returnType"`
	Exact      bool   `json:"exact"`
}

func projectView(doc Document) View {
	counts := countRefIDs(doc)
	resolve := doc.instanceResolver()
	view := View{
		Root: projectNode(doc, "root", "inline", doc.Plugin, "", counts, resolve),
	}
	for _, id := range sortedKeys(doc.Shared) {
		node := projectNode(doc, "shared."+id, "shared", doc.Shared[id], id, counts, resolve)
		view.Shared = append(view.Shared, *node)
	}
	return view
}

func projectNode(doc Document, path, role string, node PluginNode, id string, counts map[string]int, resolve instanceResolver) *ViewNode {
	out := &ViewNode{
		Path: path,
		Role: role,
		Kind: node.Use,
	}
	if role == "shared" {
		out.RefCount = counts[id]
	}
	desc, ok := pluginkit.Describe(node.Use)
	if !ok {
		return out
	}
	out.Config = projectConfig(desc, node.Config)
	out.Slots = projectSlots(doc, path, node, desc.Extensions, counts, resolve)
	return out
}

func projectConfig(desc pluginkit.PluginDescription, cfg map[string]any) []ViewField {
	fields := desc.Config
	if len(fields) == 0 {
		return nil
	}
	defaults := desc.ConfigDefaults()
	out := make([]ViewField, 0, len(fields))
	for _, field := range fields {
		var value any
		if cfg != nil {
			value = cfg[field.Name]
		}
		out = append(out, ViewField{
			Name:     field.Name,
			Type:     pluginkit.FormatType(field.Type),
			Kind:     fieldKind(field),
			Optional: field.Optional,
			Value:    value,
			Default:  defaults[field.Name],
		})
	}
	return out
}

// fieldKind 把字段类型归类为前端控件类型：string / number / bool / json。
// 多值字段（List）一律归为 json，由 JSON 编辑器兜底。
func fieldKind(field pluginkit.FieldDescription) string {
	if field.List {
		return "json"
	}
	t := field.Type
	if t == nil {
		return "json"
	}
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	switch t.Kind() {
	case reflect.String:
		return "string"
	case reflect.Bool:
		return "bool"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return "number"
	default:
		return "json"
	}
}

func projectSlots(doc Document, parentPath string, node PluginNode, exts []pluginkit.FieldDescription, counts map[string]int, resolve instanceResolver) []ViewSlot {
	if len(exts) == 0 {
		return nil
	}
	slots := make([]ViewSlot, 0, len(exts))
	for _, ext := range exts {
		slotPath := parentPath + ".deps." + ext.Name
		candidates := pluginkit.KindCandidates(ext.Type)
		kindNames := kindNamesFromCandidates(candidates)
		slot := ViewSlot{
			Name:     ext.Name,
			Path:     slotPath,
			Status:   "empty",
			List:     ext.List,
			Optional: ext.Optional,
			Type:     pluginkit.FormatType(ext.Type),
			Kinds:    toViewKindCandidates(candidates),
			Refs:     slotRefs(doc, kindNames),
		}
		raw, ok := node.Deps[ext.Name]
		if !ok {
			slots = append(slots, slot)
			continue
		}
		if ext.List {
			items, err := decodeDepList(raw)
			if err != nil || len(items) == 0 {
				slots = append(slots, slot)
				continue
			}
			slot.Status = "filled"
			slot.Items = make([]ViewNode, 0, len(items))
			for i, item := range items {
				itemPath := fmt.Sprintf("%s[%d]", slotPath, i)
				slot.Items = append(slot.Items, projectDepItem(doc, itemPath, item, counts, resolve))
			}
			slots = append(slots, slot)
			continue
		}
		if child, ok := projectFilledSingle(doc, slotPath, raw, counts, resolve); ok {
			slot.Status = "filled"
			slot.Items = []ViewNode{child}
		}
		slots = append(slots, slot)
	}
	return slots
}

func projectFilledSingle(doc Document, path string, raw any, counts map[string]int, resolve instanceResolver) (ViewNode, bool) {
	if id, ok := raw.(string); ok {
		if id == "" {
			return ViewNode{}, false
		}
		return projectRef(path, id, counts, resolve), true
	}
	child, err := decodeDepNode(raw)
	if err != nil {
		return ViewNode{}, false
	}
	return *projectNode(doc, path, "inline", child, "", counts, resolve), true
}

func projectDepItem(doc Document, path string, raw any, counts map[string]int, resolve instanceResolver) ViewNode {
	if id, ok := raw.(string); ok {
		return projectRef(path, id, counts, resolve)
	}
	child, err := decodeDepNode(raw)
	if err != nil {
		return ViewNode{Path: path, Role: "inline"}
	}
	return *projectNode(doc, path, "inline", child, "", counts, resolve)
}

func projectRef(path, id string, counts map[string]int, resolve instanceResolver) ViewNode {
	kind := ""
	if target, ok := resolve(id); ok {
		kind = target.Use
	}
	return ViewNode{
		Path:     path,
		Role:     "ref",
		Kind:     kind,
		RefID:    id,
		RefCount: counts[id],
	}
}

func slotRefs(doc Document, kinds []string) []RefCandidate {
	allow := make(map[string]bool, len(kinds))
	for _, kind := range kinds {
		allow[kind] = true
	}
	var refs []RefCandidate
	for _, id := range sortedKeys(doc.Shared) {
		kind := doc.Shared[id].Use
		if allow[kind] {
			refs = append(refs, RefCandidate{ID: id, Kind: kind})
		}
	}
	return refs
}

func countRefIDs(doc Document) map[string]int {
	counts := map[string]int{}
	addNodeRefs(doc.Plugin, counts)
	for _, id := range sortedKeys(doc.Shared) {
		addNodeRefs(doc.Shared[id], counts)
	}
	return counts
}

func addNodeRefs(node PluginNode, counts map[string]int) {
	for _, name := range sortedDepNames(node.Deps) {
		addDepRefs(node.Deps[name], counts)
	}
}

func addDepRefs(raw any, counts map[string]int) {
	switch v := raw.(type) {
	case string:
		if v != "" {
			counts[v]++
		}
	case []any, []PluginNode:
		items, err := decodeDepList(v)
		if err != nil {
			return
		}
		for _, item := range items {
			addDepRefs(item, counts)
		}
	default:
		child, err := decodeDepNode(raw)
		if err != nil {
			return
		}
		addNodeRefs(child, counts)
	}
}

func toViewKindCandidates(in []pluginkit.KindCandidate) []KindCandidate {
	out := make([]KindCandidate, len(in))
	for i, c := range in {
		out[i] = KindCandidate{
			Kind:       c.Kind,
			ReturnType: c.ReturnType,
			Exact:      c.Exact,
		}
	}
	return out
}

func kindNamesFromCandidates(candidates []pluginkit.KindCandidate) []string {
	names := make([]string, len(candidates))
	for i, c := range candidates {
		names[i] = c.Kind
	}
	return names
}
