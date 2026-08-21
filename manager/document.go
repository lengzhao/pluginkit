package manager

import (
	"encoding/json"
	"fmt"

	"gopkg.in/yaml.v3"
)

// Document 是 UI 维护的内联实例树，导出时只生成单个 root 顶层的 YAML。
type Document struct {
	RootID string     `json:"rootId"`
	Plugin PluginNode `json:"plugin"`
}

// PluginNode 对应一个内联插件实例。
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
	return validateTree(d.Plugin)
}

// ToGraph 转为 build.Build 接受的 root 实例图。
func (d Document) ToGraph() map[string]any {
	return map[string]any{
		d.RootID: d.Plugin.toAny(),
	}
}

// ToYAML 导出内联 root 配置。
func (d Document) ToYAML() ([]byte, error) {
	return yaml.Marshal(d.ToGraph())
}

// FromYAML 从单 root 内联 YAML 导入 Document。
func FromYAML(data []byte) (Document, error) {
	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return Document{}, err
	}
	if len(raw) == 0 {
		return Document{}, fmt.Errorf("empty yaml")
	}
	if len(raw) != 1 {
		return Document{}, fmt.Errorf("inline manager expects exactly one top-level instance id")
	}
	for rootID, value := range raw {
		node, err := parsePluginNode(value)
		if err != nil {
			return Document{}, fmt.Errorf("%s: %w", rootID, err)
		}
		return Document{RootID: rootID, Plugin: node}, nil
	}
	return Document{}, fmt.Errorf("empty yaml")
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
	case []any:
		items := make([]any, 0, len(v))
		for _, item := range v {
			node, err := decodeDepNode(item)
			if err != nil {
				return nil, err
			}
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
		return nil, fmt.Errorf("reference deps are not supported in inline manager")
	case []any:
		items := make([]any, 0, len(v))
		for i, item := range v {
			if id, ok := item.(string); ok {
				return nil, fmt.Errorf("reference deps are not supported in inline manager (index %d: %q)", i, id)
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
			return PluginNode{}, fmt.Errorf("dep must be a plugin object")
		}
		var node PluginNode
		if err := json.Unmarshal(data, &node); err != nil {
			return PluginNode{}, fmt.Errorf("dep must be a plugin object")
		}
		if node.Use == "" {
			return PluginNode{}, fmt.Errorf("dep.use is required")
		}
		return node, nil
	}
}

func decodeDepList(raw any) ([]PluginNode, error) {
	switch v := raw.(type) {
	case []any:
		items := make([]PluginNode, 0, len(v))
		for _, item := range v {
			node, err := decodeDepNode(item)
			if err != nil {
				return nil, err
			}
			items = append(items, node)
		}
		return items, nil
	case []PluginNode:
		return v, nil
	default:
		data, err := json.Marshal(raw)
		if err != nil {
			return nil, fmt.Errorf("dep list must be an array")
		}
		var items []PluginNode
		if err := json.Unmarshal(data, &items); err != nil {
			return nil, fmt.Errorf("dep list must be an array")
		}
		return items, nil
	}
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
