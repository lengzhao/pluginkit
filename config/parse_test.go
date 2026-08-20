package config

import (
	"encoding/json"
	"testing"
)

func TestParse_ObjectAndList(t *testing.T) {
	t.Parallel()

	fields, err := Parse(map[string]any{
		"llm": PluginUse{Use: "openai"},
		"tools": []PluginUse{
			{ID: "read-file", Use: "read-file"},
			{Use: "shell"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(fields) != 2 {
		t.Fatalf("len=%d", len(fields))
	}
	if fields[0].Name != "llm" || fields[0].List || fields[0].Uses[0].ID != "llm" {
		t.Fatalf("llm field: %+v", fields[0])
	}
	if !fields[1].List || fields[1].Uses[1].ID != "tools[1]" {
		t.Fatalf("tools field: %+v", fields[1])
	}
}

func TestParse_FromJSONMap(t *testing.T) {
	t.Parallel()

	var raw map[string]any
	if err := json.Unmarshal([]byte(`{
		"store": {"id":"store","use":"sqlite-store"},
		"steps": [{"use":"http-step"},{"id":"save","use":"save-step","deps":{"store":"store"}}]
	}`), &raw); err != nil {
		t.Fatal(err)
	}
	fields, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if fields[0].Name != "steps" || fields[0].Uses[0].ID != "steps[0]" {
		t.Fatalf("steps: %+v", fields[0])
	}
	if fields[1].Name != "store" || fields[1].Uses[0].ID != "store" {
		t.Fatalf("store: %+v", fields[1])
	}
}

func TestParse_Errors(t *testing.T) {
	t.Parallel()

	if _, err := Parse(map[string]any{"llm": "openai"}); err == nil {
		t.Fatal("expected error for string value")
	}
	if _, err := Parse(map[string]any{"llm": map[string]any{}}); err == nil {
		t.Fatal("expected error for missing use")
	}
}
