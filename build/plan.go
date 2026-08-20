package build

import (
	"fmt"
	"reflect"
	"sort"

	"github.com/lengzhao/pluginkit"
	"github.com/lengzhao/pluginkit/config"
)

type plan struct {
	steps    []*node
	bindings []binding
}

type binding struct {
	target targetField
	uses   []config.PluginUse
}

func compilePlan(parsed []config.Field, fields []targetField) (*plan, error) {
	known := map[string]targetField{}
	for _, f := range fields {
		known[f.jsonName] = f
	}

	byName := map[string]config.Field{}
	var nodes []*node
	var bindings []binding
	seenID := map[string]*node{}
	idx := 0
	for _, pf := range parsed {
		tf, ok := known[pf.Name]
		if !ok {
			id, use := firstUse(pf)
			return nil, assembleErr(pf.Name, use, id, StageResolve, fmt.Errorf("target has no field with json name %q", pf.Name))
		}
		if pf.List != tf.slice {
			id, use := firstUse(pf)
			if tf.slice {
				return nil, assembleErr(pf.Name, use, id, StageResolve, fmt.Errorf("field is a slice, config must be a list"))
			}
			return nil, assembleErr(pf.Name, use, id, StageResolve, fmt.Errorf("field is not a slice, config must be a single object"))
		}

		byName[pf.Name] = pf
		for _, use := range pf.Uses {
			spec, ok := pluginkit.Lookup(use.Use)
			if !ok {
				return nil, assembleErr(pf.Name, use.Use, use.ID, StageResolve, fmt.Errorf("unknown plugin kind %q", use.Use))
			}
			if err := staticTypeCheck(tf.elemType, spec.ReturnType, use); err != nil {
				return nil, assembleErr(pf.Name, use.Use, use.ID, StageTypeCheck, err)
			}
			n := &node{index: idx, field: pf.Name, list: pf.List, target: tf, want: tf.elemType, use: use, spec: spec}
			idx++
			if prev, exists := seenID[use.ID]; exists {
				return nil, assembleErr(pf.Name, use.Use, use.ID, StageResolve, fmt.Errorf("duplicate instance id, also used by field %q", prev.field))
			}
			seenID[use.ID] = n
			nodes = append(nodes, n)
		}
	}

	for _, tf := range fields {
		pf, ok := byName[tf.jsonName]
		if !ok && !tf.optional && !tf.slice {
			return nil, assembleErr(tf.jsonName, "", "", StageResolve, fmt.Errorf("missing plugin config"))
		}
		if ok {
			pf.Uses = append([]config.PluginUse(nil), pf.Uses...)
		}
		bind := binding{target: tf}
		if ok {
			bind.uses = pf.Uses
		}
		bindings = append(bindings, bind)
	}

	order, err := topoSort(nodes)
	if err != nil {
		return nil, err
	}
	return &plan{steps: order, bindings: bindings}, nil
}

func compileRootPlan(rootID string, want reflect.Type, graph map[string]config.PluginUse) (*plan, error) {
	if rootID == "" {
		return nil, assembleErr("", "", "", StageResolve, fmt.Errorf("root id is required"))
	}
	seen := map[string]bool{}
	var nodes []*node

	var visit func(string) error
	visit = func(id string) error {
		if seen[id] {
			return nil
		}
		use, ok := graph[id]
		if !ok {
			return nil
		}
		seen[id] = true

		spec, ok := pluginkit.Lookup(use.Use)
		if !ok {
			return assembleErr(id, use.Use, use.ID, StageResolve, fmt.Errorf("unknown plugin kind %q", use.Use))
		}
		nodeWant := reflect.Type(nil)
		if id == rootID {
			nodeWant = want
			if err := staticTypeCheck(want, spec.ReturnType, use); err != nil {
				return assembleErr(id, use.Use, use.ID, StageTypeCheck, err)
			}
		}
		nodes = append(nodes, &node{index: len(nodes), field: id, want: nodeWant, use: use, spec: spec})

		depIDs, err := collectDepIDs(use.Deps)
		if err != nil {
			return assembleErr(id, use.Use, use.ID, StageDeps, err)
		}
		sort.Strings(depIDs)
		for _, depID := range depIDs {
			if err := visit(depID); err != nil {
				return err
			}
		}
		return nil
	}

	if _, ok := graph[rootID]; !ok {
		return nil, assembleErr(rootID, "", rootID, StageResolve, fmt.Errorf("unknown root instance %q", rootID))
	}
	if err := visit(rootID); err != nil {
		return nil, err
	}
	order, err := topoSort(nodes)
	if err != nil {
		return nil, err
	}
	return &plan{steps: order}, nil
}

func parseGraph(raw map[string]any) (map[string]config.PluginUse, error) {
	parsed, err := config.Parse(raw)
	if err != nil {
		return nil, assembleErr("", "", "", StageResolve, err)
	}
	graph := make(map[string]config.PluginUse, len(parsed))
	var addUse func(path string, use config.PluginUse) error
	addUse = func(path string, use config.PluginUse) error {
		if use.ID == "" {
			use.ID = path
		}
		deps, err := normalizeInlineDeps(use.ID, use.Deps, addUse)
		if err != nil {
			return assembleErr(path, use.Use, use.ID, StageDeps, err)
		}
		use.Deps = deps
		if _, exists := graph[use.ID]; exists {
			return assembleErr(path, use.Use, use.ID, StageResolve, fmt.Errorf("duplicate instance id %q", use.ID))
		}
		graph[use.ID] = use
		return nil
	}
	for _, field := range parsed {
		if field.List {
			id, use := firstUse(field)
			return nil, assembleErr(field.Name, use, id, StageResolve, fmt.Errorf("root graph entry must be a single plugin object"))
		}
		use := field.Uses[0]
		use.ID = field.Name
		if err := addUse(field.Name, use); err != nil {
			return nil, err
		}
	}
	return graph, nil
}

func normalizeInlineDeps(parentID string, deps map[string]any, addUse func(path string, use config.PluginUse) error) (map[string]any, error) {
	if len(deps) == 0 {
		return nil, nil
	}
	out := make(map[string]any, len(deps))
	for name, raw := range deps {
		val, err := normalizeInlineDepValue(parentID, name, raw, addUse)
		if err != nil {
			return nil, fmt.Errorf("deps.%s: %w", name, err)
		}
		out[name] = val
	}
	return out, nil
}

func normalizeInlineDepValue(parentID, name string, raw any, addUse func(path string, use config.PluginUse) error) (any, error) {
	switch v := raw.(type) {
	case string:
		if v == "" {
			return nil, fmt.Errorf("empty instance id")
		}
		return v, nil
	case []string:
		for _, id := range v {
			if id == "" {
				return nil, fmt.Errorf("empty instance id")
			}
		}
		return append([]string(nil), v...), nil
	case []config.PluginUse:
		ids := make([]string, 0, len(v))
		for i, item := range v {
			id, err := addInlineDepUse(fmt.Sprintf("%s.%s[%d]", parentID, name, i), item, addUse)
			if err != nil {
				return nil, err
			}
			ids = append(ids, id)
		}
		return ids, nil
	case []any:
		ids := make([]string, 0, len(v))
		for i, item := range v {
			id, err := normalizeInlineDepItem(fmt.Sprintf("%s.%s[%d]", parentID, name, i), item, addUse)
			if err != nil {
				return nil, err
			}
			ids = append(ids, id)
		}
		return ids, nil
	default:
		if use, ok, err := parseInlineUse(raw); ok || err != nil {
			if err != nil {
				return nil, err
			}
			return addInlineDepUse(parentID+"."+name, use, addUse)
		}
		return nil, fmt.Errorf("must be a string, plugin object, or list of strings/plugin objects, got %T", raw)
	}
}

func normalizeInlineDepItem(path string, raw any, addUse func(path string, use config.PluginUse) error) (string, error) {
	if id, ok := raw.(string); ok {
		if id == "" {
			return "", fmt.Errorf("empty instance id")
		}
		return id, nil
	}
	use, ok, err := parseInlineUse(raw)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("instance id or plugin object is required")
	}
	return addInlineDepUse(path, use, addUse)
}

func addInlineDepUse(path string, use config.PluginUse, addUse func(path string, use config.PluginUse) error) (string, error) {
	if use.Use == "" {
		return "", fmt.Errorf("plugin use is required")
	}
	id := use.ID
	if id == "" {
		id = path
	}
	use.ID = id
	if err := addUse(path, use); err != nil {
		return "", err
	}
	return id, nil
}

func parseInlineUse(raw any) (config.PluginUse, bool, error) {
	switch v := raw.(type) {
	case config.PluginUse:
		return v, true, nil
	case *config.PluginUse:
		if v == nil {
			return config.PluginUse{}, true, fmt.Errorf("plugin use is nil")
		}
		return *v, true, nil
	default:
		m, ok := stringMap(raw)
		if !ok {
			return config.PluginUse{}, false, nil
		}
		use := config.PluginUse{}
		if v, ok := m["id"]; ok && v != nil {
			s, ok := v.(string)
			if !ok {
				return config.PluginUse{}, true, fmt.Errorf("id must be a string")
			}
			use.ID = s
		}
		if v, ok := m["use"]; ok && v != nil {
			s, ok := v.(string)
			if !ok {
				return config.PluginUse{}, true, fmt.Errorf("use must be a string")
			}
			use.Use = s
		}
		if v, ok := m["config"]; ok && v != nil {
			cm, ok := stringMap(v)
			if !ok {
				return config.PluginUse{}, true, fmt.Errorf("config must be an object")
			}
			use.Config = cm
		}
		if v, ok := m["deps"]; ok && v != nil {
			dm, ok := stringMap(v)
			if !ok {
				return config.PluginUse{}, true, fmt.Errorf("deps must be an object")
			}
			use.Deps = dm
		}
		return use, true, nil
	}
}

func stringMap(raw any) (map[string]any, bool) {
	switch m := raw.(type) {
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
