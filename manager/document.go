package manager

import (
	"encoding/json"
	"fmt"
	"sort"

	"gopkg.in/yaml.v3"
)

// Document 是 UI 维护的实例图：root 内联树 + 可选顶层共享实例。
type Document struct {
	RootID string                `json:"rootId"`
	Plugin PluginNode            `json:"plugin"`
	Shared map[string]PluginNode `json:"shared,omitempty"`
}

// PluginNode 对应一个插件实例（内联或共享）。
type PluginNode struct {
	Use    string         `json:"use"`
	Config map[string]any `json:"config,omitempty"`
	Deps   map[string]any `json:"deps,omitempty"`
}

// Validate 检查文档结构与插件 deps 类型约束。
func (d Document) Validate() error {
	if d.RootID == "" {
		return fmt.Errorf("rootId is required")
	}
	if d.Plugin.Use == "" {
		return fmt.Errorf("plugin.use is required")
	}
	if d.Shared == nil {
		d.Shared = map[string]PluginNode{}
	}
	if _, ok := d.Shared[d.RootID]; ok {
		return fmt.Errorf("shared instance id %q conflicts with rootId", d.RootID)
	}
	resolver := d.instanceResolver()
	if err := validateTree(d.Plugin, resolver); err != nil {
		return fmt.Errorf("%s: %w", d.RootID, err)
	}
	ids := sortedKeys(d.Shared)
	for _, id := range ids {
		if err := validateTree(d.Shared[id], resolver); err != nil {
			return fmt.Errorf("%s: %w", id, err)
		}
	}
	return nil
}

// ToGraph 转为 build.Build 接受的 root 实例图。
func (d Document) ToGraph() map[string]any {
	graph := map[string]any{
		d.RootID: d.Plugin.toAny(),
	}
	if d.Shared == nil {
		return graph
	}
	ids := sortedKeys(d.Shared)
	for _, id := range ids {
		if id == d.RootID {
			continue
		}
		graph[id] = d.Shared[id].toAny()
	}
	return graph
}

// ToYAML 导出 root 实例图（含顶层共享实例与引用）。
func (d Document) ToYAML() ([]byte, error) {
	return yaml.Marshal(d.ToGraph())
}

// FromYAML 从 YAML 导入 Document，支持顶层共享实例与 deps 引用。
func FromYAML(data []byte) (Document, error) {
	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return Document{}, err
	}
	if len(raw) == 0 {
		return Document{}, fmt.Errorf("empty yaml")
	}

	instances := make(map[string]PluginNode, len(raw))
	for id, value := range raw {
		node, err := parsePluginNode(value)
		if err != nil {
			return Document{}, fmt.Errorf("%s: %w", id, err)
		}
		instances[id] = node
	}

	rootID, shared, err := splitRootAndShared(instances)
	if err != nil {
		return Document{}, err
	}
	return Document{
		RootID: rootID,
		Plugin: instances[rootID],
		Shared: shared,
	}, nil
}

func splitRootAndShared(instances map[string]PluginNode) (string, map[string]PluginNode, error) {
	if len(instances) == 1 {
		for id := range instances {
			return id, map[string]PluginNode{}, nil
		}
	}

	referenced := collectReferencedIDs(instances)
	unreferenced := make([]string, 0)
	for id := range instances {
		if !referenced[id] {
			unreferenced = append(unreferenced, id)
		}
	}
	sort.Strings(unreferenced)

	switch len(unreferenced) {
	case 0:
		return "", nil, fmt.Errorf("cannot determine root: all top-level instances are referenced")
	case 1:
		rootID := unreferenced[0]
		shared := make(map[string]PluginNode, len(instances)-1)
		for id, node := range instances {
			if id == rootID {
				continue
			}
			shared[id] = node
		}
		return rootID, shared, nil
	default:
		return "", nil, fmt.Errorf("cannot determine root: multiple unreferenced instances %v", unreferenced)
	}
}

func collectReferencedIDs(instances map[string]PluginNode) map[string]bool {
	referenced := map[string]bool{}
	for _, node := range instances {
		collectRefsInNode(node, referenced)
	}
	return referenced
}

func collectRefsInNode(node PluginNode, out map[string]bool) {
	for _, raw := range node.Deps {
		collectRefsInDep(raw, out)
	}
}

func collectRefsInDep(raw any, out map[string]bool) {
	switch v := raw.(type) {
	case string:
		out[v] = true
	case []any:
		for _, item := range v {
			if id, ok := item.(string); ok {
				out[id] = true
			} else if node, err := decodeDepNode(item); err == nil {
				collectRefsInNode(node, out)
			}
		}
	case []PluginNode:
		for _, node := range v {
			collectRefsInNode(node, out)
		}
	default:
		if node, err := decodeDepNode(raw); err == nil {
			collectRefsInNode(node, out)
		}
	}
}

type instanceResolver func(id string) (PluginNode, bool)

func (d Document) instanceResolver() instanceResolver {
	shared := d.Shared
	if shared == nil {
		shared = map[string]PluginNode{}
	}
	rootID := d.RootID
	root := d.Plugin
	return func(id string) (PluginNode, bool) {
		if id == rootID {
			return root, true
		}
		node, ok := shared[id]
		return node, ok
	}
}

func (n PluginNode) toAny() map[string]any {
	out := map[string]any{"use": n.Use}
	if len(n.Config) > 0 {
		out["config"] = n.Config
	}
	if len(n.Deps) > 0 {
		deps := make(map[string]any, len(n.Deps))
		for name, raw := range n.Deps {
			converted, err := depToAny(raw)
			if err != nil {
				continue
			}
			deps[name] = converted
		}
		if len(deps) > 0 {
			out["deps"] = deps
		}
	}
	return out
}

func depToAny(raw any) (any, error) {
	switch v := raw.(type) {
	case string:
		return v, nil
	case map[string]any:
		if use, _ := v["use"].(string); use != "" {
			child, err := parsePluginNode(v)
			if err != nil {
				return nil, err
			}
			return child.toAny(), nil
		}
		node, err := decodeDepNode(v)
		if err != nil {
			return nil, err
		}
		return node.toAny(), nil
	case PluginNode:
		return v.toAny(), nil
	case []any:
		items := make([]any, 0, len(v))
		for _, item := range v {
			converted, err := depItemToAny(item)
			if err != nil {
				return nil, err
			}
			items = append(items, converted)
		}
		return items, nil
	case []PluginNode:
		items := make([]any, 0, len(v))
		for _, node := range v {
			items = append(items, node.toAny())
		}
		return items, nil
	default:
		node, err := decodeDepNode(raw)
		if err != nil {
			return nil, err
		}
		return node.toAny(), nil
	}
}

func depItemToAny(raw any) (any, error) {
	if id, ok := raw.(string); ok {
		return id, nil
	}
	node, err := decodeDepNode(raw)
	if err != nil {
		return nil, err
	}
	return node.toAny(), nil
}

func parsePluginNode(raw any) (PluginNode, error) {
	m, ok := raw.(map[string]any)
	if !ok {
		return PluginNode{}, fmt.Errorf("plugin must be an object")
	}
	use, _ := m["use"].(string)
	if use == "" {
		return PluginNode{}, fmt.Errorf("plugin.use is required")
	}
	node := PluginNode{Use: use}
	if cfg, ok := m["config"].(map[string]any); ok {
		node.Config = cloneMap(cfg)
	}
	if depsRaw, ok := m["deps"].(map[string]any); ok {
		deps := make(map[string]any, len(depsRaw))
		for name, value := range depsRaw {
			converted, err := parseDepValue(value)
			if err != nil {
				return PluginNode{}, fmt.Errorf("deps.%s: %w", name, err)
			}
			deps[name] = converted
		}
		node.Deps = deps
	}
	return node, nil
}

func parseDepValue(raw any) (any, error) {
	switch v := raw.(type) {
	case string:
		if v == "" {
			return nil, fmt.Errorf("reference id must not be empty")
		}
		return v, nil
	case []any:
		items := make([]any, 0, len(v))
		for i, item := range v {
			if id, ok := item.(string); ok {
				if id == "" {
					return nil, fmt.Errorf("reference id must not be empty (index %d)", i)
				}
				items = append(items, id)
				continue
			}
			node, err := decodeDepNode(item)
			if err != nil {
				return nil, err
			}
			items = append(items, node)
		}
		return items, nil
	default:
		return decodeDepNode(raw)
	}
}

func decodeDepNode(raw any) (PluginNode, error) {
	switch v := raw.(type) {
	case PluginNode:
		return v, nil
	case map[string]any:
		return parsePluginNode(v)
	default:
		data, err := json.Marshal(raw)
		if err != nil {
			return PluginNode{}, fmt.Errorf("dep must be a plugin object or reference id")
		}
		var node PluginNode
		if err := json.Unmarshal(data, &node); err != nil {
			return PluginNode{}, fmt.Errorf("dep must be a plugin object or reference id")
		}
		if node.Use == "" {
			return PluginNode{}, fmt.Errorf("dep.use is required")
		}
		return node, nil
	}
}

func decodeDepList(raw any) ([]any, error) {
	switch v := raw.(type) {
	case []any:
		items := make([]any, 0, len(v))
		for _, item := range v {
			if id, ok := item.(string); ok {
				if id == "" {
					return nil, fmt.Errorf("reference id must not be empty")
				}
				items = append(items, id)
				continue
			}
			node, err := decodeDepNode(item)
			if err != nil {
				return nil, err
			}
			items = append(items, node)
		}
		return items, nil
	case []PluginNode:
		items := make([]any, 0, len(v))
		for _, node := range v {
			items = append(items, node)
		}
		return items, nil
	default:
		data, err := json.Marshal(raw)
		if err != nil {
			return nil, fmt.Errorf("dep list must be an array")
		}
		var items []PluginNode
		if err := json.Unmarshal(data, &items); err != nil {
			return nil, fmt.Errorf("dep list must be an array")
		}
		out := make([]any, 0, len(items))
		for _, node := range items {
			out = append(out, node)
		}
		return out, nil
	}
}

func sortedKeys(m map[string]PluginNode) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func cloneMap(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
