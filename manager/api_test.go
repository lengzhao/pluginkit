package manager

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEditAPIAttachReturnsViewAndDiagnostics(t *testing.T) {
	ensureTestPlugins(t)
	handler, err := New(Options{})
	if err != nil {
		t.Fatal(err)
	}
	body := `{"document":{"rootId":"","plugin":{"use":""}},"op":{"type":"newRoot","kind":"agent"}}`
	req := httptest.NewRequest(http.MethodPost, "/api/edit", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.Bytes())
	}
	var resp editResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Document.Plugin.Use != "agent" || resp.View.Root == nil {
		t.Fatalf("resp=%#v", resp)
	}
	if !hasDiag(resp.Diagnostics, "root.deps.llm", "missing_dep") {
		t.Fatalf("diags=%#v", resp.Diagnostics)
	}
	if resp.YAML == "" {
		t.Fatal("expected yaml")
	}
}

func TestBuildAPIDoesNotMutate(t *testing.T) {
	ensureTestPlugins(t)
	called := false
	handler, err := New(Options{
		ValidateBuild: func(ctx context.Context, doc Document) error {
			called = true
			return fmt.Errorf("boom")
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	doc := Document{
		RootID: "agent",
		Plugin: PluginNode{
			Use: "agent",
			Deps: map[string]any{
				"llm":   PluginNode{Use: "openai", Config: map[string]any{"model": "m"}},
				"tools": []any{PluginNode{Use: "shell"}},
			},
		},
	}
	raw, _ := json.Marshal(map[string]any{"document": doc})
	req := httptest.NewRequest(http.MethodPost, "/api/build", bytes.NewReader(raw))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.Bytes())
	}
	if !called {
		t.Fatal("expected ValidateBuild")
	}
	var resp editResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !hasBuildDiag(resp.Diagnostics) {
		t.Fatalf("diags=%#v", resp.Diagnostics)
	}
}

func TestRemovedRoutesGone(t *testing.T) {
	handler, err := New(Options{})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/validate", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != 404 {
		t.Fatalf("status=%d", rec.Code)
	}
}

func hasBuildDiag(diags []Diagnostic) bool {
	for _, d := range diags {
		if d.Stage == "build" || d.Code == "build" {
			return true
		}
	}
	return false
}
