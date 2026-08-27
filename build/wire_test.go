package build

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/lengzhao/pluginkit"
	"github.com/lengzhao/pluginkit/config"
)

type wireCommand struct {
	Name string
}

type wireProvider interface {
	Commands() []wireCommand
}

type wireCompaction struct{}

func (s *wireCompaction) Commands() []wireCommand {
	return []wireCommand{{Name: "compact"}}
}

type wireSession struct{}

func (s *wireSession) Commands() []wireCommand {
	return []wireCommand{{Name: "session"}}
}

type wireRegistry struct {
	names []string
}

func (r *wireRegistry) SetContributions(providers []wireProvider) error {
	r.names = nil
	for _, provider := range providers {
		for _, cmd := range provider.Commands() {
			r.names = append(r.names, cmd.Name)
		}
	}
	return nil
}

type wireNamedCollector struct {
	id    string
	names []string
}

func (c *wireNamedCollector) SetContributions(providers []wireProvider) error {
	c.names = nil
	for _, provider := range providers {
		for _, cmd := range provider.Commands() {
			c.names = append(c.names, fmt.Sprintf("%s:%s", c.id, cmd.Name))
		}
	}
	return nil
}

type wireAppDeps struct {
	Services []any         `json:"services"`
	Registry *wireRegistry `json:"registry"`
}

type wireApp struct {
	services []any
}

func TestWireContributionsNilResult(t *testing.T) {
	if err := WireContributions[wireProvider, *wireRegistry](nil, func(*wireRegistry, []wireProvider) error {
		t.Fatal("attach should not run")
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := WireSetter[wireProvider](nil); err != nil {
		t.Fatal(err)
	}
}

func TestWireContributionsNoContributors(t *testing.T) {
	result := &Result{
		Instances: []Instance{
			{ID: "registry", Use: "registry", Value: &wireRegistry{}},
		},
	}
	called := false
	err := WireContributions[wireProvider, *wireRegistry](result, func(*wireRegistry, []wireProvider) error {
		called = true
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("attach should not run without contributors")
	}
	if err := WireSetter[wireProvider](result); err != nil {
		t.Fatal(err)
	}
}

func TestWireContributionsSkipsNilCollector(t *testing.T) {
	result := &Result{
		Instances: []Instance{
			{ID: "compact", Use: "compaction", Value: &wireCompaction{}},
			{ID: "registry", Use: "registry", Value: (*wireRegistry)(nil)},
		},
	}
	err := WireContributions[wireProvider, *wireRegistry](result, func(*wireRegistry, []wireProvider) error {
		t.Fatal("attach should not run")
		return nil
	})
	if !errors.Is(err, ErrNoContributionsCollector) {
		t.Fatalf("err=%v want %v", err, ErrNoContributionsCollector)
	}
}

func TestWireContributionsNoCollector(t *testing.T) {
	result := &Result{
		Instances: []Instance{
			{ID: "compact", Use: "compaction", Value: &wireCompaction{}},
		},
	}
	err := WireContributions[wireProvider, *wireRegistry](result, func(*wireRegistry, []wireProvider) error {
		t.Fatal("attach should not run")
		return nil
	})
	if !errors.Is(err, ErrNoContributionsCollector) {
		t.Fatalf("err=%v want %v", err, ErrNoContributionsCollector)
	}
}

func TestWireSetterSkipsNilCollector(t *testing.T) {
	result := &Result{
		Instances: []Instance{
			{ID: "compact", Use: "compaction", Value: &wireCompaction{}},
			{ID: "registry", Use: "registry", Value: (*wireRegistry)(nil)},
		},
	}
	err := WireSetter[wireProvider](result)
	if !errors.Is(err, ErrNoContributionsCollector) {
		t.Fatalf("err=%v want %v", err, ErrNoContributionsCollector)
	}
}

func TestWireSetterNoCollector(t *testing.T) {
	result := &Result{
		Instances: []Instance{
			{ID: "compact", Use: "compaction", Value: &wireCompaction{}},
		},
	}
	err := WireSetter[wireProvider](result)
	if !errors.Is(err, ErrNoContributionsCollector) {
		t.Fatalf("err=%v want %v", err, ErrNoContributionsCollector)
	}
}

func TestWireSetterMultipleCollectors(t *testing.T) {
	left := &wireNamedCollector{id: "left"}
	right := &wireNamedCollector{id: "right"}
	result := &Result{
		Instances: []Instance{
			{ID: "compact", Use: "compaction", Value: &wireCompaction{}},
			{ID: "left", Use: "collector", Value: left},
			{ID: "right", Use: "collector", Value: right},
		},
	}

	if err := WireSetter[wireProvider](result); err != nil {
		t.Fatal(err)
	}
	if len(left.names) != 1 || left.names[0] != "left:compact" {
		t.Fatalf("left=%v", left.names)
	}
	if len(right.names) != 1 || right.names[0] != "right:compact" {
		t.Fatalf("right=%v", right.names)
	}
}

func TestWireContributionsAttach(t *testing.T) {
	result := &Result{
		Instances: []Instance{
			{ID: "compact", Use: "compaction", Value: &wireCompaction{}},
			{ID: "session", Use: "session", Value: &wireSession{}},
			{ID: "registry", Use: "registry", Value: &wireRegistry{}},
		},
	}

	registry := result.Instances[2].Value.(*wireRegistry)
	err := WireContributions[wireProvider, *wireRegistry](result, func(r *wireRegistry, providers []wireProvider) error {
		return r.SetContributions(providers)
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(registry.names) != 2 {
		t.Fatalf("names=%v want 2 entries", registry.names)
	}
}

func TestWireContributionsMultipleCollectors(t *testing.T) {
	left := &wireNamedCollector{id: "left"}
	right := &wireNamedCollector{id: "right"}
	result := &Result{
		Instances: []Instance{
			{ID: "compact", Use: "compaction", Value: &wireCompaction{}},
			{ID: "left", Use: "collector", Value: left},
			{ID: "right", Use: "collector", Value: right},
		},
	}

	err := WireContributions[wireProvider, *wireNamedCollector](result, func(c *wireNamedCollector, providers []wireProvider) error {
		return c.SetContributions(providers)
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(left.names) != 1 || left.names[0] != "left:compact" {
		t.Fatalf("left=%v", left.names)
	}
	if len(right.names) != 1 || right.names[0] != "right:compact" {
		t.Fatalf("right=%v", right.names)
	}
}

func TestWireContributionsPropagatesError(t *testing.T) {
	result := &Result{
		Instances: []Instance{
			{ID: "compact", Use: "compaction", Value: &wireCompaction{}},
			{ID: "registry", Use: "registry", Value: &wireRegistry{}},
		},
	}
	want := errors.New("boom")
	err := WireContributions[wireProvider, *wireRegistry](result, func(*wireRegistry, []wireProvider) error {
		return want
	})
	if !errors.Is(err, want) {
		t.Fatalf("err=%v want %v", err, want)
	}
}

func TestWireSetterFromBuildResult(t *testing.T) {
	compactionKind := kind(t, "wire-compaction")
	sessionKind := kind(t, "wire-session")
	registryKind := kind(t, "wire-registry")
	appKind := kind(t, "wire-app")

	pluginkit.Register(compactionKind, func() (*wireCompaction, error) {
		return &wireCompaction{}, nil
	})
	pluginkit.Register(sessionKind, func() (*wireSession, error) {
		return &wireSession{}, nil
	})
	pluginkit.Register(registryKind, func() (*wireRegistry, error) {
		return &wireRegistry{}, nil
	})
	pluginkit.Register(appKind, func(_ struct{}, deps wireAppDeps) (*wireApp, error) {
		return &wireApp{services: deps.Services}, nil
	})

	_, result, err := Build[*wireApp](context.Background(), map[string]any{
		"app": config.PluginUse{
			Use: appKind,
			Deps: map[string]any{
				"services": []string{"compaction", "session"},
				"registry": "registry",
			},
		},
		"compaction": config.PluginUse{Use: compactionKind},
		"session":    config.PluginUse{Use: sessionKind},
		"registry":   config.PluginUse{Use: registryKind},
	}, "app")
	if err != nil {
		t.Fatal(err)
	}

	if err := WireSetter[wireProvider](result); err != nil {
		t.Fatal(err)
	}

	registry, ok := GetByID[*wireRegistry](result, "registry")
	if !ok {
		t.Fatal("registry not found")
	}
	if len(registry.names) != 2 {
		t.Fatalf("names=%v want compact and session", registry.names)
	}
}
