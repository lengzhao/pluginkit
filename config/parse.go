package config

import (
	"fmt"
	"sort"
)

// Parse 把 plugins 配置树识别为字段列表。
//
// value 可以是单个对象或数组。map 的 key 顺序不稳定，返回的字段按 Name 排序。
// 缺省 id：单值字段使用字段名；多值字段使用 `{字段名}[{下标}]`。
func Parse(plugins map[string]any) ([]Field, error) {
	if len(plugins) == 0 {
		return nil, nil
	}
	names := make([]string, 0, len(plugins))
	for name := range plugins {
		names = append(names, name)
	}
	sort.Strings(names)

	out := make([]Field, 0, len(names))
	for _, name := range names {
		field, err := parseField(name, plugins[name])
		if err != nil {
			return nil, err
		}
		out = append(out, field)
	}
	return out, nil
}

func parseField(name string, raw any) (Field, error) {
	if raw == nil {
		return Field{}, fmt.Errorf("config field %q: value is nil", name)
	}

	var field Field
	switch v := raw.(type) {
	case []PluginUse:
		field = Field{Name: name, List: true, Uses: append([]PluginUse(nil), v...)}
	case []any:
		uses := make([]PluginUse, 0, len(v))
		for i, item := range v {
			use, err := parsePluginUse(item)
			if err != nil {
				return Field{}, fmt.Errorf("config field %q[%d]: %w", name, i, err)
			}
			uses = append(uses, use)
		}
		field = Field{Name: name, List: true, Uses: uses}
	default:
		use, err := parsePluginUse(raw)
		if err != nil {
			return Field{}, fmt.Errorf("config field %q: %w", name, err)
		}
		field = Field{Name: name, List: false, Uses: []PluginUse{use}}
	}
	if err := finalizeField(&field); err != nil {
		return Field{}, err
	}
	return field, nil
}

func parsePluginUse(raw any) (PluginUse, error) {
	switch v := raw.(type) {
	case PluginUse:
		return v, nil
	case *PluginUse:
		if v == nil {
			return PluginUse{}, fmt.Errorf("plugin use is nil")
		}
		return *v, nil
	default:
		m, ok := asStringMap(raw)
		if !ok {
			return PluginUse{}, fmt.Errorf("plugin use must be an object, got %T", raw)
		}
		return pluginUseFromMap(m)
	}
}

func pluginUseFromMap(m map[string]any) (PluginUse, error) {
	use := PluginUse{}
	if v, ok := m["id"]; ok && v != nil {
		s, ok := v.(string)
		if !ok {
			return PluginUse{}, fmt.Errorf("id must be a string")
		}
		use.ID = s
	}
	if v, ok := m["use"]; ok && v != nil {
		s, ok := v.(string)
		if !ok {
			return PluginUse{}, fmt.Errorf("use must be a string")
		}
		use.Use = s
	}
	if v, ok := m["config"]; ok && v != nil {
		cm, ok := asStringMap(v)
		if !ok {
			return PluginUse{}, fmt.Errorf("config must be an object")
		}
		use.Config = cm
	}
	if v, ok := m["deps"]; ok && v != nil {
		dm, ok := asStringMap(v)
		if !ok {
			return PluginUse{}, fmt.Errorf("deps must be an object")
		}
		use.Deps = dm
	}
	return use, nil
}

func finalizeField(field *Field) error {
	for i := range field.Uses {
		if field.Uses[i].Use == "" {
			return fmt.Errorf("config field %q: plugin use is required", field.Name)
		}
		if field.Uses[i].ID != "" {
			continue
		}
		if !field.List {
			field.Uses[i].ID = field.Name
			continue
		}
		field.Uses[i].ID = fmt.Sprintf("%s[%d]", field.Name, i)
	}
	return nil
}

func asStringMap(v any) (map[string]any, bool) {
	switch m := v.(type) {
	case map[string]any:
		return m, true
	case map[any]any:
		out := make(map[string]any, len(m))
		for k, val := range m {
			ks, ok := k.(string)
			if !ok {
				return nil, false
			}
			out[ks] = val
		}
		return out, true
	default:
		return nil, false
	}
}
