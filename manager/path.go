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

func resolvePath(doc Document, path string) (resolved, error) {
	unknown := func() (resolved, error) {
		return resolved{}, fmt.Errorf("unknown path %q", path)
	}

	switch {
	case path == "root":
		node := doc.Plugin
		return resolved{Node: &node, Index: -1}, nil
	case strings.HasPrefix(path, "root."):
		segs, err := parseDepsWalk(strings.TrimPrefix(path, "root"))
		if err != nil {
			return unknown()
		}
		plugin := doc.Plugin
		return resolveFrom(&plugin, segs, path)
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
		node, ok := doc.Shared[id]
		if !ok {
			return unknown()
		}
		if suffix == "" {
			return resolved{Node: &node, Index: -1}, nil
		}
		segs, err := parseDepsWalk(suffix)
		if err != nil {
			return unknown()
		}
		return resolveFrom(&node, segs, path)
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
		raw, ok := current.Deps[seg.name]
		if !ok {
			return unknown()
		}
		last := i == len(segs)-1
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
			node, err := decodeDepNode(item)
			if err != nil {
				return unknown()
			}
			current = nodePtr(node)
			continue
		}
		if last {
			return depResolved(current, seg.name, -1, raw, fullPath)
		}
		node, err := decodeDepNode(raw)
		if err != nil {
			return unknown()
		}
		current = nodePtr(node)
	}
	return unknown()
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
