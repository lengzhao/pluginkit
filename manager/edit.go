package manager

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/lengzhao/pluginkit"
)

// Operation 是工作台的一次编辑命令。
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

func apply(doc Document, op Operation) (Document, error) {
	switch op.Type {
	case "validate":
		return cloneDocument(doc)
	case "newRoot":
		return applyNewRoot(op)
	case "setRootId":
		out, err := cloneDocument(doc)
		if err != nil {
			return Document{}, err
		}
		return applySetRootID(out, op.ID)
	case "attach":
		out, err := cloneDocument(doc)
		if err != nil {
			return Document{}, err
		}
		return out, applyAttach(&out, op.Path, op.Kind)
	case "attachRef":
		out, err := cloneDocument(doc)
		if err != nil {
			return Document{}, err
		}
		return out, applyAttachRef(&out, op.Path, op.RefID)
	case "remove":
		out, err := cloneDocument(doc)
		if err != nil {
			return Document{}, err
		}
		deleteOrphan := true
		if op.DeleteOrphan != nil {
			deleteOrphan = *op.DeleteOrphan
		}
		return out, applyRemove(&out, op.Path, deleteOrphan)
	case "setConfig":
		out, err := cloneDocument(doc)
		if err != nil {
			return Document{}, err
		}
		return out, applySetConfig(&out, op.Path, op.Config)
	case "hoist":
		out, err := cloneDocument(doc)
		if err != nil {
			return Document{}, err
		}
		return out, applyHoist(&out, op.Path, op.ID)
	case "importYAML":
		return FromYAML([]byte(op.YAML))
	default:
		return Document{}, fmt.Errorf("unknown operation type %q", op.Type)
	}
}

func cloneDocument(doc Document) (Document, error) {
	data, err := json.Marshal(doc)
	if err != nil {
		return Document{}, err
	}
	var out Document
	if err := json.Unmarshal(data, &out); err != nil {
		return Document{}, err
	}
	return out, nil
}

func applyNewRoot(op Operation) (Document, error) {
	if op.Kind == "" {
		return Document{}, fmt.Errorf("kind is required")
	}
	if _, ok := pluginkit.Describe(op.Kind); !ok {
		return Document{}, fmt.Errorf("unknown kind %q", op.Kind)
	}
	rootID := op.ID
	if rootID == "" {
		rootID = op.Kind
	}
	node, err := templateNode(op.Kind)
	if err != nil {
		return Document{}, err
	}
	return Document{
		RootID: rootID,
		Plugin: node,
		Shared: map[string]PluginNode{},
	}, nil
}

func applySetRootID(doc Document, id string) (Document, error) {
	if id == "" {
		return Document{}, fmt.Errorf("id is required")
	}
	if doc.Shared == nil {
		doc.Shared = map[string]PluginNode{}
	}
	if _, ok := doc.Shared[id]; ok {
		return Document{}, fmt.Errorf("shared instance id %q conflicts with rootId", id)
	}
	doc.RootID = id
	return doc, nil
}

func templateNode(kind string) (PluginNode, error) {
	desc, ok := pluginkit.Describe(kind)
	if !ok {
		return PluginNode{}, fmt.Errorf("unknown kind %q", kind)
	}
	tmpl := desc.Template()
	node := PluginNode{Use: kind, Deps: map[string]any{}}
	if cfg, ok := tmpl["config"].(map[string]any); ok {
		node.Config = cloneMap(cfg)
	}
	return node, nil
}

func applyAttach(doc *Document, path, kind string) error {
	if kind == "" {
		return fmt.Errorf("kind is required")
	}
	if _, ok := pluginkit.Describe(kind); !ok {
		return fmt.Errorf("unknown kind %q", kind)
	}
	child, err := templateNode(kind)
	if err != nil {
		return err
	}
	return writeAtPath(doc, path, child)
}

func applyAttachRef(doc *Document, path, refID string) error {
	if refID == "" {
		return fmt.Errorf("refId is required")
	}
	if refID == doc.RootID {
		return writeAtPath(doc, path, refID)
	}
	if doc.Shared == nil {
		return fmt.Errorf("unknown instance reference %q", refID)
	}
	if _, ok := doc.Shared[refID]; !ok {
		return fmt.Errorf("unknown instance reference %q", refID)
	}
	return writeAtPath(doc, path, refID)
}

func writeAtPath(doc *Document, path string, value any) error {
	loc, err := resolvePath(doc, path)
	if err != nil {
		return err
	}
	if loc.Parent == nil {
		return fmt.Errorf("cannot attach at %q", path)
	}
	if loc.Parent.Deps == nil {
		loc.Parent.Deps = map[string]any{}
	}
	if loc.Index >= 0 {
		items, err := decodeDepList(loc.Parent.Deps[loc.Ext])
		if err != nil {
			return err
		}
		if loc.Index >= len(items) {
			return fmt.Errorf("unknown path %q", path)
		}
		items[loc.Index] = value
		loc.Parent.Deps[loc.Ext] = items
		return nil
	}
	if loc.Node != nil || loc.RefID != "" {
		loc.Parent.Deps[loc.Ext] = value
		return nil
	}
	raw, ok := loc.Parent.Deps[loc.Ext]
	if ok && isDepList(raw) {
		items, err := decodeDepList(raw)
		if err != nil {
			return err
		}
		items = append(items, value)
		loc.Parent.Deps[loc.Ext] = items
		return nil
	}
	loc.Parent.Deps[loc.Ext] = value
	return nil
}

func applyRemove(doc *Document, path string, deleteOrphan bool) error {
	loc, err := resolvePath(doc, path)
	if err != nil {
		return err
	}
	var removedRef string
	if loc.Parent == nil {
		return fmt.Errorf("cannot remove %q", path)
	}
	if loc.Index >= 0 {
		items, err := decodeDepList(loc.Parent.Deps[loc.Ext])
		if err != nil {
			return err
		}
		if loc.Index >= len(items) {
			return fmt.Errorf("unknown path %q", path)
		}
		if id, ok := items[loc.Index].(string); ok {
			removedRef = id
		}
		items = append(items[:loc.Index], items[loc.Index+1:]...)
		if len(items) == 0 {
			delete(loc.Parent.Deps, loc.Ext)
		} else {
			loc.Parent.Deps[loc.Ext] = items
		}
	} else if loc.RefID != "" {
		removedRef = loc.RefID
		delete(loc.Parent.Deps, loc.Ext)
	} else if loc.Node != nil {
		delete(loc.Parent.Deps, loc.Ext)
	} else {
		return fmt.Errorf("cannot remove empty slot %q", path)
	}
	if deleteOrphan && removedRef != "" && removedRef != doc.RootID {
		if countRefIDs(*doc)[removedRef] == 0 {
			delete(doc.Shared, removedRef)
		}
	}
	if len(doc.Shared) == 0 {
		doc.Shared = map[string]PluginNode{}
	}
	return nil
}

func applySetConfig(doc *Document, path string, cfg map[string]any) error {
	loc, err := resolvePath(doc, path)
	if err != nil {
		return err
	}
	if loc.Node == nil {
		return fmt.Errorf("path %q is not a plugin node", path)
	}
	if cfg == nil {
		loc.Node.Config = nil
	} else {
		loc.Node.Config = cloneMap(cfg)
	}
	if path == "root" {
		doc.Plugin = *loc.Node
		return nil
	}
	if strings.HasPrefix(path, "shared.") {
		id := strings.TrimPrefix(path, "shared.")
		if i := strings.Index(id, ".deps."); i >= 0 {
			id = id[:i]
		}
		if path == "shared."+id {
			doc.Shared[id] = *loc.Node
			return nil
		}
	}
	if loc.Parent == nil {
		return fmt.Errorf("cannot set config at %q", path)
	}
	if loc.Index >= 0 {
		items, err := decodeDepList(loc.Parent.Deps[loc.Ext])
		if err != nil {
			return err
		}
		items[loc.Index] = *loc.Node
		loc.Parent.Deps[loc.Ext] = items
		return nil
	}
	loc.Parent.Deps[loc.Ext] = *loc.Node
	return nil
}

func applyHoist(doc *Document, path, id string) error {
	if id == "" {
		return fmt.Errorf("id is required")
	}
	if id == doc.RootID {
		return fmt.Errorf("shared instance id %q conflicts with rootId", id)
	}
	if doc.Shared == nil {
		doc.Shared = map[string]PluginNode{}
	}
	if _, ok := doc.Shared[id]; ok {
		return fmt.Errorf("shared instance %q already exists", id)
	}
	loc, err := resolvePath(doc, path)
	if err != nil {
		return err
	}
	if loc.Parent == nil || loc.Node == nil || loc.RefID != "" {
		return fmt.Errorf("path %q is not an inline node", path)
	}
	if path == "root" {
		return fmt.Errorf("cannot hoist root")
	}
	node := *loc.Node
	if node.Deps == nil {
		node.Deps = map[string]any{}
	}
	doc.Shared[id] = node
	if loc.Index >= 0 {
		items, err := decodeDepList(loc.Parent.Deps[loc.Ext])
		if err != nil {
			return err
		}
		items[loc.Index] = id
		loc.Parent.Deps[loc.Ext] = items
	} else {
		loc.Parent.Deps[loc.Ext] = id
	}
	return nil
}
