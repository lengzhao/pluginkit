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

type pathSeg struct {
	name  string
	index int
}

func resolvePath(doc *Document, path string) (resolved, error) {
	unknown := func() (resolved, error) {
		return resolved{}, fmt.Errorf("unknown path %q", path)
	}
	if doc == nil {
		return unknown()
	}

	switch {
	case path == "root":
		return resolved{Node: &doc.Plugin, Index: -1}, nil
	case strings.HasPrefix(path, "root."):
		segs, err := parseDepsWalk(strings.TrimPrefix(path, "root"))
		if err != nil {
			return unknown()
		}
		return resolveFrom(&doc.Plugin, segs, path)
	case strings.HasPrefix(path, "shared."):
		rest := strings.TrimPrefix(path, "shared.")
		if rest == "" {
			return unknown()
		}
		id, suffix := rest, ""
		if i := strings.Index(rest, ".deps."); i >= 0 {
			id, suffix = rest[:i], rest[i:]
		}
		if id == "" {
			return unknown()
		}
		parent, err := internShared(doc, id)
		if err != nil {
			return unknown()
		}
		if suffix == "" {
			return resolved{Node: parent, Index: -1}, nil
		}
		segs, err := parseDepsWalk(suffix)
		if err != nil {
			return unknown()
		}
		return resolveFrom(parent, segs, path)
	default:
		return unknown()
	}
}

func resolveFrom(parent *PluginNode, segs []pathSeg, fullPath string) (resolved, error) {
	unknown := func() (resolved, error) {
		return resolved{}, fmt.Errorf("unknown path %q", fullPath)
	}
	current := parent
	for i, seg := range segs {
		last := i == len(segs)-1
		raw, ok := current.Deps[seg.name]
		if !ok {
			if last && seg.index < 0 {
				return resolved{Parent: current, Ext: seg.name, Index: -1}, nil
			}
			return unknown()
		}
		if seg.index >= 0 {
			items, err := decodeDepList(raw)
			if err != nil {
				return unknown()
			}
			if seg.index >= len(items) {
				return unknown()
			}
			item := items[seg.index]
			if last {
				return depResolved(current, seg.name, seg.index, item, fullPath)
			}
			node, err := internListItem(current, seg.name, items, seg.index)
			if err != nil {
				return unknown()
			}
			current = node
			continue
		}
		if last {
			if isDepList(raw) {
				return resolved{Parent: current, Ext: seg.name, Index: -1}, nil
			}
			return depResolved(current, seg.name, -1, raw, fullPath)
		}
		node, err := internChild(current, seg.name)
		if err != nil {
			return unknown()
		}
		current = node
	}
	return unknown()
}

func internShared(doc *Document, id string) (*PluginNode, error) {
	node, ok := doc.Shared[id]
	if !ok {
		return nil, fmt.Errorf("unknown shared %q", id)
	}
	if node.Deps == nil {
		node.Deps = map[string]any{}
	}
	doc.Shared[id] = node
	return &node, nil
}

func internChild(parent *PluginNode, name string) (*PluginNode, error) {
	raw := parent.Deps[name]
	if _, ok := raw.(string); ok {
		return nil, fmt.Errorf("ref")
	}
	if isDepList(raw) {
		return nil, fmt.Errorf("list")
	}
	if p, ok := raw.(*PluginNode); ok {
		if p.Deps == nil {
			p.Deps = map[string]any{}
		}
		return p, nil
	}
	node, err := decodeDepNode(raw)
	if err != nil {
		return nil, err
	}
	if node.Deps == nil {
		node.Deps = map[string]any{}
	}
	parent.Deps[name] = node
	return &node, nil
}

func internListItem(parent *PluginNode, name string, items []any, index int) (*PluginNode, error) {
	item := items[index]
	if _, ok := item.(string); ok {
		return nil, fmt.Errorf("ref")
	}
	if p, ok := item.(*PluginNode); ok {
		if p.Deps == nil {
			p.Deps = map[string]any{}
		}
		return p, nil
	}
	node, err := decodeDepNode(item)
	if err != nil {
		return nil, err
	}
	if node.Deps == nil {
		node.Deps = map[string]any{}
	}
	items[index] = node
	parent.Deps[name] = items
	return &node, nil
}

func isDepList(raw any) bool {
	switch raw.(type) {
	case []any, []PluginNode:
		return true
	default:
		return false
	}
}

func depResolved(parent *PluginNode, ext string, index int, raw any, fullPath string) (resolved, error) {
	out := resolved{Parent: parent, Ext: ext, Index: index}
	if id, ok := raw.(string); ok {
		out.RefID = id
		return out, nil
	}
	node, err := decodeDepNode(raw)
	if err != nil {
		return resolved{}, fmt.Errorf("unknown path %q", fullPath)
	}
	out.Node = nodePtr(node)
	return out, nil
}

func parseDepsWalk(s string) ([]pathSeg, error) {
	var segs []pathSeg
	rest := s
	for rest != "" {
		if !strings.HasPrefix(rest, ".deps.") {
			return nil, fmt.Errorf("invalid path")
		}
		rest = strings.TrimPrefix(rest, ".deps.")
		name, index, leftover, err := parseNameIndex(rest)
		if err != nil {
			return nil, err
		}
		segs = append(segs, pathSeg{name: name, index: index})
		rest = leftover
	}
	if len(segs) == 0 {
		return nil, fmt.Errorf("invalid path")
	}
	return segs, nil
}

func parseNameIndex(s string) (name string, index int, leftover string, err error) {
	index = -1
	i := 0
	for i < len(s) && s[i] != '.' && s[i] != '[' {
		i++
	}
	if i == 0 {
		return "", -1, "", fmt.Errorf("empty name")
	}
	name = s[:i]
	rest := s[i:]
	if strings.HasPrefix(rest, "[") {
		end := strings.IndexByte(rest, ']')
		if end < 0 {
			return "", -1, "", fmt.Errorf("bad index")
		}
		n, convErr := strconv.Atoi(rest[1:end])
		if convErr != nil || n < 0 {
			return "", -1, "", fmt.Errorf("bad index")
		}
		return name, n, rest[end+1:], nil
	}
	return name, -1, rest, nil
}

func nodePtr(n PluginNode) *PluginNode {
	return &n
}
