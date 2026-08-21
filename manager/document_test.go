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

	loaded, err := FromYAML(yamlBytes)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.RootID != "agent" || loaded.Plugin.Use != "agent" {
		t.Fatalf("loaded=%+v", loaded)
	}
	if len(loaded.Shared) != 0 {
		t.Fatalf("expected no shared instances, got %#v", loaded.Shared)
	}
}

func TestDocumentSharedRoundTrip(t *testing.T) {
	doc := Document{
		RootID: "workflow",
		Plugin: PluginNode{
			Use: "workflow",
			Deps: map[string]any{
				"steps": []any{
					"fetch",
					PluginNode{
						Use: "save-step",
						Deps: map[string]any{
							"store": "store",
						},
					},
				},
			},
		},
		Shared: map[string]PluginNode{
			"fetch": {Use: "http-step"},
			"store": {Use: "sqlite-store", Config: map[string]any{"path": "/tmp/db"}},
		},
	}

	yamlBytes, err := doc.ToYAML()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(yamlBytes), "store: store") {
		t.Fatalf("expected reference in yaml: %s", yamlBytes)
	}
	if !strings.Contains(string(yamlBytes), "fetch:") {
		t.Fatalf("expected shared fetch in yaml: %s", yamlBytes)
	}

	loaded, err := FromYAML(yamlBytes)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.RootID != "workflow" {
		t.Fatalf("root=%q", loaded.RootID)
	}
	if len(loaded.Shared) != 2 {
		t.Fatalf("shared=%#v", loaded.Shared)
	}
	steps, ok := loaded.Plugin.Deps["steps"].([]any)
	if !ok || len(steps) != 2 {
		t.Fatalf("steps=%#v", loaded.Plugin.Deps["steps"])
	}
	if steps[0] != "fetch" {
		t.Fatalf("steps[0]=%#v want fetch ref", steps[0])
	}
}

func TestFromYAMLRejectsAmbiguousRoot(t *testing.T) {
	raw := []byte(`workflow:
  use: workflow
  deps:
    cache: cache
cache:
  use: redis-cache
agent:
  use: agent
`)
	_, err := FromYAML(raw)
	if err == nil {
		t.Fatal("expected ambiguous root error")
	}
	if !strings.Contains(err.Error(), "multiple unreferenced") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFromYAMLImportSharedOnlyReferences(t *testing.T) {
	raw := []byte(`workflow:
  use: workflow
  deps:
    steps:
      - fetch
      - save
fetch:
  use: http-step
save:
  use: save-step
  deps:
    store: store
store:
  use: sqlite-store
`)
	doc, err := FromYAML(raw)
	if err != nil {
		t.Fatal(err)
	}
	if doc.RootID != "workflow" {
		t.Fatalf("root=%q", doc.RootID)
	}
	if len(doc.Shared) != 3 {
		t.Fatalf("shared=%#v", doc.Shared)
	}
}
