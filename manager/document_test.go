package manager

import (
	"strings"
	"testing"
)

func TestDocumentInlineRoundTrip(t *testing.T) {
	doc := Document{
		RootID: "agent",
		Plugin: PluginNode{
			Use: "agent",
			Deps: map[string]any{
				"llm": PluginNode{
					Use:    "openai",
					Config: map[string]any{"model": "gpt-5.5"},
				},
				"tools": []any{
					PluginNode{Use: "read-file", Config: map[string]any{"root": "."}},
					PluginNode{Use: "shell"},
				},
			},
		},
	}

	yamlBytes, err := doc.ToYAML()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(yamlBytes), "openai-1") {
		t.Fatalf("inline export must not use reference ids: %s", yamlBytes)
	}

	loaded, err := FromYAML(yamlBytes)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.RootID != "agent" || loaded.Plugin.Use != "agent" {
		t.Fatalf("loaded=%+v", loaded)
	}
}

func TestFromYAMLRejectsReference(t *testing.T) {
	raw := []byte(`agent:
  use: agent
  deps:
    llm: openai
openai:
  use: openai
`)
	_, err := FromYAML(raw)
	if err == nil {
		t.Fatal("expected reference yaml to fail")
	}
}
