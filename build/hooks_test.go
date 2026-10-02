package build

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/lengzhao/pluginkit"
	"github.com/lengzhao/pluginkit/config"
)

type hookCfg struct {
	Timeout int    `json:"timeout"`
	Mode    string `json:"mode"`
}

func (c *hookCfg) SetDefaults() {
	if c.Timeout == 0 {
		c.Timeout = 30
	}
}

func (c *hookCfg) Validate() error {
	switch c.Mode {
	case "fast", "safe":
		return nil
	default:
		return fmt.Errorf("mode must be fast or safe, got %q", c.Mode)
	}
}

type hookPlugin struct {
	timeout int
	mode    string
}

func (p *hookPlugin) Name() string { return "hook" }

func TestConfigHooksValueConfig(t *testing.T) {
	pluginkit.Register("hook-value", func(cfg hookCfg) (*hookPlugin, error) {
		return &hookPlugin{timeout: cfg.Timeout, mode: cfg.Mode}, nil
	})

	graph := map[string]any{
		"tool": config.PluginUse{
			Use:    "hook-value",
			Config: map[string]any{"mode": "fast"},
		},
	}
	inst, _, err := Build[*hookPlugin](context.Background(), graph, "tool")
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}
	if inst.timeout != 30 {
		t.Fatalf("expected default timeout 30, got %d", inst.timeout)
	}
	if inst.mode != "fast" {
		t.Fatalf("expected mode fast, got %q", inst.mode)
	}
}

func TestConfigHooksPointerConfig(t *testing.T) {
	pluginkit.Register("hook-ptr", func(cfg *hookCfg) (*hookPlugin, error) {
		return &hookPlugin{timeout: cfg.Timeout, mode: cfg.Mode}, nil
	})

	graph := map[string]any{
		"tool": config.PluginUse{
			Use:    "hook-ptr",
			Config: map[string]any{"mode": "safe", "timeout": 10},
		},
	}
	inst, _, err := Build[*hookPlugin](context.Background(), graph, "tool")
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}
	if inst.timeout != 10 {
		t.Fatalf("expected explicit timeout 10, got %d", inst.timeout)
	}
}

func TestConfigHooksValidateError(t *testing.T) {
	pluginkit.Register("hook-bad", func(cfg hookCfg) (*hookPlugin, error) {
		return &hookPlugin{timeout: cfg.Timeout, mode: cfg.Mode}, nil
	})

	graph := map[string]any{
		"tool": config.PluginUse{
			Use:    "hook-bad",
			Config: map[string]any{"mode": "yolo"},
		},
	}
	_, _, err := Build[*hookPlugin](context.Background(), graph, "tool")
	if err == nil {
		t.Fatal("expected validate error")
	}
	var aerr *Error
	if !errors.As(err, &aerr) {
		t.Fatalf("expected *Error, got %T", err)
	}
	if aerr.Stage != StageValidate {
		t.Fatalf("expected stage %q, got %q", StageValidate, aerr.Stage)
	}
	if aerr.ID != "tool" || aerr.Use != "hook-bad" {
		t.Fatalf("unexpected error identity: %+v", aerr)
	}
	if !strings.Contains(err.Error(), "mode must be fast or safe") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

type hookParentDeps struct {
	Tools []Tool `json:"tools"`
}

type hookParent struct {
	tools []Tool
}

func (p *hookParent) Run() string { return "parent" }

func TestValidatePlanAggregatesConfigErrors(t *testing.T) {
	pluginkit.Register("hook-plan", func(cfg hookCfg) (*hookPlugin, error) {
		return &hookPlugin{timeout: cfg.Timeout, mode: cfg.Mode}, nil
	})
	pluginkit.Register("hook-parent", func(_ struct{}, deps hookParentDeps) (*hookParent, error) {
		return &hookParent{tools: deps.Tools}, nil
	})

	graph := map[string]any{
		"root": config.PluginUse{
			Use: "hook-parent",
			Deps: map[string]any{
				"tools": []any{
					map[string]any{"use": "hook-plan", "config": map[string]any{"mode": "x"}},
					map[string]any{"use": "hook-plan", "config": map[string]any{"mode": "y"}},
				},
			},
		},
	}
	err := ValidatePlan(graph, "root", reflect.TypeOf((*Pipeline)(nil)).Elem())
	if err == nil {
		t.Fatal("expected aggregated validate errors")
	}
	var aerr *Error
	if !errors.As(err, &aerr) || aerr.Stage != StageValidate {
		t.Fatalf("expected validate stage error, got %v", err)
	}
	// 两个子实例的错误应聚合在一次返回里。
	if got := strings.Count(err.Error(), string(StageValidate)); got != 2 {
		t.Fatalf("expected 2 aggregated validate errors, got %d: %v", got, err)
	}

	// 合法配置通过校验，且 SetDefaults 不报错。
	ok := map[string]any{
		"root": config.PluginUse{
			Use: "hook-parent",
			Deps: map[string]any{
				"tools": []any{
					map[string]any{"use": "hook-plan", "config": map[string]any{"mode": "fast"}},
				},
			},
		},
	}
	if err := ValidatePlan(ok, "root", reflect.TypeOf((*Pipeline)(nil)).Elem()); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}
