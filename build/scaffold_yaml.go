package build

import (
	"bytes"
	"fmt"
	"sort"

	"gopkg.in/yaml.v3"
)

// ScaffoldYAML 根据 target struct 生成带注释的 YAML 配置。
// 单值字段存在多个兼容插件时，会在 use 行附加 default/alternatives 注释。
func ScaffoldYAML(target any, opts ScaffoldOptions) ([]byte, error) {
	cfg, comments, err := scaffoldConfig(target, opts)
	if err != nil {
		return nil, err
	}
	return encodeYAMLWithComments(cfg, comments)
}

func encodeYAMLWithComments(cfg map[string]any, comments map[string]string) ([]byte, error) {
	doc := &yaml.Node{Kind: yaml.DocumentNode}
	doc.Content = []*yaml.Node{valueToYAMLNode(cfg, comments, "")}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func valueToYAMLNode(v any, comments map[string]string, path string) *yaml.Node {
	switch val := v.(type) {
	case map[string]any:
		return mapToYAMLNode(val, comments, path)
	case []any:
		return sliceToYAMLNode(val, comments, path)
	case string:
		node := &yaml.Node{Kind: yaml.ScalarNode, Value: val}
		if c := comments[path]; c != "" {
			node.LineComment = c
		}
		return node
	default:
		node := &yaml.Node{Kind: yaml.ScalarNode}
		_ = node.Encode(val)
		if c := comments[path]; c != "" {
			node.LineComment = c
		}
		return node
	}
}

func mapToYAMLNode(m map[string]any, comments map[string]string, path string) *yaml.Node {
	node := &yaml.Node{Kind: yaml.MappingNode}
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		keyNode := &yaml.Node{Kind: yaml.ScalarNode, Value: key}
		childPath := joinYAMLPath(path, key)
		valNode := valueToYAMLNode(m[key], comments, childPath)
		if key == "use" {
			if c := comments[childPath]; c != "" {
				valNode.LineComment = c
			}
		}
		node.Content = append(node.Content, keyNode, valNode)
	}
	return node
}

func sliceToYAMLNode(items []any, comments map[string]string, path string) *yaml.Node {
	node := &yaml.Node{Kind: yaml.SequenceNode}
	for i, item := range items {
		itemPath := fmt.Sprintf("%s[%d]", path, i)
		node.Content = append(node.Content, valueToYAMLNode(item, comments, itemPath))
	}
	return node
}

func joinYAMLPath(prefix, key string) string {
	if prefix == "" {
		return key
	}
	return prefix + "." + key
}
