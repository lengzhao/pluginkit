package build

import (
	"context"
	"testing"

	"github.com/lengzhao/pluginkit"
	"github.com/lengzhao/pluginkit/config"
)

type commandDef struct {
	Name string
}

type commandProvider interface {
	Commands() []commandDef
}

type compactionService struct{}

func (s *compactionService) Commands() []commandDef {
	return []commandDef{{Name: "compact"}}
}

type sessionService struct{}

func (s *sessionService) Commands() []commandDef {
	return []commandDef{{Name: "session"}}
}

type appDeps struct {
	Services []any `json:"services"`
}

type appImpl struct {
	services []any
}

func TestCollectNilResult(t *testing.T) {
	if got := Collect[Tool](nil); got != nil {
		t.Fatalf("Collect(nil)=%#v want nil", got)
	}
	if got := CollectInstances[Tool](nil); got != nil {
		t.Fatalf("CollectInstances(nil)=%#v want nil", got)
	}
}

func TestCollectFiltersByType(t *testing.T) {
	result := &Result{
		Instances: []Instance{
			{ID: "llm", Use: "openai", Value: &openai{model: "m"}},
			{ID: "tool", Use: "shell", Value: &shell{}},
			{ID: "store", Use: "sqlite", Value: &sqliteStore{}},
		},
	}

	tools := Collect[Tool](result)
	if len(tools) != 1 || tools[0].Name() != "shell" {
		t.Fatalf("tools=%#v", tools)
	}

	llms := Collect[LLM](result)
	if len(llms) != 1 || llms[0].Model() != "m" {
		t.Fatalf("llms=%#v", llms)
	}

	stores := Collect[Store](result)
	if len(stores) != 1 || stores[0].Get("x") != "x" {
		t.Fatalf("stores=%#v", stores)
	}
}

func TestCollectPreservesInstanceOrder(t *testing.T) {
	result := &Result{
		Instances: []Instance{
			{ID: "a", Use: "shell", Value: &shell{}},
			{ID: "b", Use: "read-file", Value: &readFile{root: "."}},
			{ID: "c", Use: "shell", Value: &shell{}},
		},
	}

	items := CollectInstances[Tool](result)
	if len(items) != 3 {
		t.Fatalf("items=%#v", items)
	}
	if items[0].ID != "a" || items[1].ID != "b" || items[2].ID != "c" {
		t.Fatalf("order=%v %v %v", items[0].ID, items[1].ID, items[2].ID)
	}
}

func TestCollectInstancesIncludesMetadata(t *testing.T) {
	result := &Result{
		Instances: []Instance{
			{ID: "tool", Use: "shell", Value: &shell{}},
			{ID: "llm", Use: "openai", Value: &openai{model: "m"}},
		},
	}

	items := CollectInstances[Tool](result)
	if len(items) != 1 {
		t.Fatalf("items=%#v", items)
	}
	if items[0].ID != "tool" || items[0].Use != "shell" || items[0].Value.Name() != "shell" {
		t.Fatalf("item=%#v", items[0])
	}
}

func TestCollectFromBuildResult(t *testing.T) {
	compactionKind := kind(t, "compaction")
	sessionKind := kind(t, "session")
	appKind := kind(t, "app")
	pluginkit.Register(compactionKind, func() (*compactionService, error) {
		return &compactionService{}, nil
	})
	pluginkit.Register(sessionKind, func() (*sessionService, error) {
		return &sessionService{}, nil
	})
	pluginkit.Register(appKind, func(_ struct{}, deps appDeps) (*appImpl, error) {
		return &appImpl{services: deps.Services}, nil
	})

	_, result, err := Build[*appImpl](context.Background(), map[string]any{
		"app": config.PluginUse{
			Use: appKind,
			Deps: map[string]any{
				"services": []string{"compaction", "session"},
			},
		},
		"compaction": config.PluginUse{Use: compactionKind},
		"session":    config.PluginUse{Use: sessionKind},
	}, "app")
	if err != nil {
		t.Fatal(err)
	}

	providers := Collect[commandProvider](result)
	if len(providers) != 2 {
		t.Fatalf("providers=%d want 2", len(providers))
	}

	items := CollectInstances[commandProvider](result)
	if len(items) != 2 {
		t.Fatalf("items=%d want 2", len(items))
	}
	gotIDs := map[string]bool{items[0].ID: true, items[1].ID: true}
	for _, id := range []string{"compaction", "session"} {
		if !gotIDs[id] {
			t.Fatalf("missing provider id %q in %#v", id, items)
		}
	}
}

func TestCollectSkipsNonMatchingInstances(t *testing.T) {
	result := &Result{
		Instances: []Instance{
			{ID: "only", Use: "shell", Value: &shell{}},
		},
	}
	if got := Collect[LLM](result); len(got) != 0 {
		t.Fatalf("Collect[LLM]=%#v want empty", got)
	}
	if got := CollectInstances[LLM](result); len(got) != 0 {
		t.Fatalf("CollectInstances[LLM]=%#v want empty", got)
	}
}
