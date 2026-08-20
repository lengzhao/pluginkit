package build

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/lengzhao/pluginkit"
	"github.com/lengzhao/pluginkit/config"
)

type LLM interface {
	Model() string
}

type Tool interface {
	Name() string
}

type Store interface {
	Get(key string) string
}

type Step interface {
	Run() string
}

type Pipeline interface {
	Run() string
}

type openaiCfg struct {
	Model string `json:"model"`
}

type openai struct {
	model string
}

func (o *openai) Model() string { return o.model }

type readFileCfg struct {
	Root string `json:"root"`
}

type readFile struct {
	root string
}

func (r *readFile) Name() string { return "read-file" }

type shell struct{}

func (s *shell) Name() string { return "shell" }

type sqliteStore struct{}

func (s *sqliteStore) Get(key string) string { return key }

type saveDeps struct {
	Store Store `json:"store"`
}

type saveStep struct {
	store Store
}

func (s *saveStep) Run() string { return "save:" + s.store.Get("x") }

type httpStep struct{}

func (h *httpStep) Run() string { return "http" }

type pipelineDeps struct {
	Steps []Step `json:"steps"`
}

type pipelineImpl struct {
	steps []Step
}

func (p *pipelineImpl) Run() string {
	out := ""
	for i, step := range p.steps {
		if i > 0 {
			out += ","
		}
		out += step.Run()
	}
	return out
}

type agentPlugins struct {
	LLM   LLM    `json:"llm"`
	Tools []Tool `json:"tools"`
}

type workflowPlugins struct {
	Store Store  `json:"store"`
	Steps []Step `json:"steps"`
}

func kind(t *testing.T, name string) string {
	t.Helper()
	return t.Name() + "/" + name
}

func TestBuildAgent(t *testing.T) {
	openaiKind := kind(t, "openai")
	readKind := kind(t, "read-file")
	shellKind := kind(t, "shell")
	pluginkit.Register(openaiKind, func(cfg openaiCfg) (*openai, error) {
		return &openai{model: cfg.Model}, nil
	})
	pluginkit.Register(readKind, func(cfg readFileCfg) (*readFile, error) {
		return &readFile{root: cfg.Root}, nil
	})
	pluginkit.Register(shellKind, func() (*shell, error) {
		return &shell{}, nil
	})

	var plugins agentPlugins
	result, err := BuildInto(context.Background(), map[string]any{
		"llm": config.PluginUse{Use: openaiKind, Config: map[string]any{"model": "gpt-5.5"}},
		"tools": []config.PluginUse{
			{ID: "read-file", Use: readKind, Config: map[string]any{"root": "."}},
			{ID: "shell", Use: shellKind},
		},
	}, &plugins)
	if err != nil {
		t.Fatal(err)
	}
	if plugins.LLM.Model() != "gpt-5.5" {
		t.Fatalf("llm=%s", plugins.LLM.Model())
	}
	if len(plugins.Tools) != 2 || plugins.Tools[0].Name() != "read-file" || plugins.Tools[1].Name() != "shell" {
		t.Fatalf("tools=%v", plugins.Tools)
	}
	got, ok := GetByID[*openai](result, "llm")
	if !ok || got.Model() != "gpt-5.5" {
		t.Fatalf("GetByID llm: ok=%v v=%v", ok, got)
	}
}

func TestBuildWorkflowDeps(t *testing.T) {
	storeKind := kind(t, "sqlite-store")
	httpKind := kind(t, "http-step")
	saveKind := kind(t, "save-step")
	pluginkit.Register(storeKind, func() (*sqliteStore, error) { return &sqliteStore{}, nil })
	pluginkit.Register(httpKind, func() (*httpStep, error) { return &httpStep{}, nil })
	pluginkit.Register(saveKind, func(_ struct{}, deps saveDeps) (*saveStep, error) {
		return &saveStep{store: deps.Store}, nil
	})

	var plugins workflowPlugins
	_, err := BuildInto(context.Background(), map[string]any{
		"store": config.PluginUse{ID: "store", Use: storeKind},
		"steps": []config.PluginUse{
			{ID: "fetch", Use: httpKind},
			{ID: "save", Use: saveKind, Deps: map[string]any{"store": "store"}},
		},
	}, &plugins)
	if err != nil {
		t.Fatal(err)
	}
	if plugins.Store == nil || len(plugins.Steps) != 2 || plugins.Steps[1].Run() != "save:x" {
		t.Fatalf("plugins=%+v", plugins)
	}
}

func TestBuildRootConstructsReachableGraph(t *testing.T) {
	storeKind := kind(t, "sqlite-store")
	httpKind := kind(t, "http-step")
	saveKind := kind(t, "save-step")
	workflowKind := kind(t, "workflow")
	pluginkit.Register(storeKind, func() (*sqliteStore, error) { return &sqliteStore{}, nil })
	pluginkit.Register(httpKind, func() (*httpStep, error) { return &httpStep{}, nil })
	pluginkit.Register(saveKind, func(_ struct{}, deps saveDeps) (*saveStep, error) {
		return &saveStep{store: deps.Store}, nil
	})
	pluginkit.Register(workflowKind, func(_ struct{}, deps pipelineDeps) (*pipelineImpl, error) {
		return &pipelineImpl{steps: deps.Steps}, nil
	})

	workflow, result, err := Build[Pipeline](context.Background(), map[string]any{
		"workflow": config.PluginUse{Use: workflowKind, Deps: map[string]any{"steps": []string{"fetch", "save"}}},
		"fetch":    config.PluginUse{Use: httpKind},
		"save":     config.PluginUse{Use: saveKind, Deps: map[string]any{"store": "store"}},
		"store":    config.PluginUse{Use: storeKind},
	}, "workflow")
	if err != nil {
		t.Fatal(err)
	}
	if workflow.Run() != "http,save:x" {
		t.Fatalf("workflow=%s", workflow.Run())
	}
	if len(result.Instances) != 4 {
		t.Fatalf("instances=%d want 4", len(result.Instances))
	}
}

func TestBuildRootSupportsInlineDeps(t *testing.T) {
	storeKind := kind(t, "sqlite-store")
	httpKind := kind(t, "http-step")
	saveKind := kind(t, "save-step")
	workflowKind := kind(t, "workflow")
	pluginkit.Register(storeKind, func() (*sqliteStore, error) { return &sqliteStore{}, nil })
	pluginkit.Register(httpKind, func() (*httpStep, error) { return &httpStep{}, nil })
	pluginkit.Register(saveKind, func(_ struct{}, deps saveDeps) (*saveStep, error) {
		return &saveStep{store: deps.Store}, nil
	})
	pluginkit.Register(workflowKind, func(_ struct{}, deps pipelineDeps) (*pipelineImpl, error) {
		return &pipelineImpl{steps: deps.Steps}, nil
	})

	workflow, result, err := Build[Pipeline](context.Background(), map[string]any{
		"workflow": config.PluginUse{Use: workflowKind, Deps: map[string]any{
			"steps": []any{
				"fetch",
				config.PluginUse{Use: saveKind, Deps: map[string]any{
					"store": config.PluginUse{Use: storeKind},
				}},
			},
		}},
		"fetch": config.PluginUse{Use: httpKind},
	}, "workflow")
	if err != nil {
		t.Fatal(err)
	}
	if workflow.Run() != "http,save:x" {
		t.Fatalf("workflow=%s", workflow.Run())
	}
	gotIDs := make(map[string]bool)
	for _, inst := range result.Instances {
		gotIDs[inst.ID] = true
	}
	for _, id := range []string{"fetch", "workflow.steps[1]", "workflow.steps[1].store", "workflow"} {
		if !gotIDs[id] {
			t.Fatalf("missing instance id %q in %#v", id, result.Instances)
		}
	}
}

func TestBuildRootRejectsDependencyTypeMismatch(t *testing.T) {
	storeKind := kind(t, "sqlite-store")
	httpKind := kind(t, "http-step")
	workflowKind := kind(t, "workflow")
	pluginkit.Register(storeKind, func() (*sqliteStore, error) { return &sqliteStore{}, nil })
	pluginkit.Register(httpKind, func() (*httpStep, error) { return &httpStep{}, nil })
	pluginkit.Register(workflowKind, func(_ struct{}, deps pipelineDeps) (*pipelineImpl, error) {
		return &pipelineImpl{steps: deps.Steps}, nil
	})

	_, _, err := Build[Pipeline](context.Background(), map[string]any{
		"workflow": config.PluginUse{Use: workflowKind, Deps: map[string]any{"steps": []string{"fetch", "store"}}},
		"fetch":    config.PluginUse{Use: httpKind},
		"store":    config.PluginUse{Use: storeKind},
	}, "workflow")
	if err == nil {
		t.Fatal("expected error")
	}
	var ae *Error
	if !errors.As(err, &ae) {
		t.Fatalf("got %T %v", err, err)
	}
	if ae.Stage != StageDeps || ae.ID != "workflow" {
		t.Fatalf("unexpected error: %+v", ae)
	}
}

func TestBuild_JSONConfig(t *testing.T) {
	k := kind(t, "openai")
	pluginkit.Register(k, func(cfg openaiCfg) (*openai, error) {
		return &openai{model: cfg.Model}, nil
	})
	var raw map[string]any
	if err := json.Unmarshal([]byte(`{"llm":{"use":"`+k+`","config":{"model":"m"}}}`), &raw); err != nil {
		t.Fatal(err)
	}
	var plugins struct {
		LLM LLM `json:"llm"`
	}
	if _, err := BuildInto(context.Background(), raw, &plugins); err != nil {
		t.Fatal(err)
	}
	if plugins.LLM.Model() != "m" {
		t.Fatal(plugins.LLM.Model())
	}
}

func TestBuild_ErrorCases(t *testing.T) {
	okKind := kind(t, "ok")
	badKind := kind(t, "bad")
	aKind := kind(t, "a")
	bKind := kind(t, "b")
	failKind := kind(t, "fail")
	pluginkit.Register(okKind, func() (*openai, error) { return &openai{model: "x"}, nil })
	pluginkit.Register(badKind, func() (*shell, error) { return &shell{}, nil })
	pluginkit.Register(aKind, func(_ struct{}, deps struct {
		Other LLM `json:"other"`
	}) (*openai, error) {
		return &openai{}, nil
	})
	pluginkit.Register(bKind, func(_ struct{}, deps struct {
		Other LLM `json:"other"`
	}) (*openai, error) {
		return &openai{}, nil
	})
	pluginkit.Register(failKind, func() (*openai, error) { return nil, errors.New("boom") })
	cfgKind := kind(t, "cfg")
	pluginkit.Register(cfgKind, func(openaiCfg) (*openai, error) { return &openai{}, nil })

	type llmOnly struct {
		LLM LLM `json:"llm"`
	}

	cases := []struct {
		name   string
		cfg    map[string]any
		target any
		stage  Stage
	}{
		{
			name:   "unknown field",
			cfg:    map[string]any{"foo": config.PluginUse{Use: okKind}},
			target: &llmOnly{},
			stage:  StageResolve,
		},
		{
			name:   "shape mismatch",
			cfg:    map[string]any{"llm": []config.PluginUse{{Use: okKind}}},
			target: &llmOnly{},
			stage:  StageResolve,
		},
		{
			name:   "unknown plugin",
			cfg:    map[string]any{"llm": config.PluginUse{Use: "no-such"}},
			target: &llmOnly{},
			stage:  StageResolve,
		},
		{
			name:   "unknown config field",
			cfg:    map[string]any{"llm": config.PluginUse{Use: cfgKind, Config: map[string]any{"nope": 1}}},
			target: &llmOnly{},
			stage:  StageDecode,
		},
		{
			name:   "missing dep",
			cfg:    map[string]any{"llm": config.PluginUse{Use: aKind, Deps: map[string]any{"other": "missing"}}},
			target: &llmOnly{},
			stage:  StageDeps,
		},
		{
			name: "cycle",
			cfg: map[string]any{
				"llm":   config.PluginUse{ID: "a", Use: aKind, Deps: map[string]any{"other": "b"}},
				"extra": config.PluginUse{ID: "b", Use: bKind, Deps: map[string]any{"other": "a"}},
			},
			target: &struct {
				LLM   LLM `json:"llm"`
				Extra LLM `json:"extra"`
			}{},
			stage: StageDeps,
		},
		{
			name:   "typecheck",
			cfg:    map[string]any{"llm": config.PluginUse{Use: badKind}},
			target: &llmOnly{},
			stage:  StageTypeCheck,
		},
		{
			name:   "construct",
			cfg:    map[string]any{"llm": config.PluginUse{Use: failKind}},
			target: &llmOnly{},
			stage:  StageConstruct,
		},
		{
			name:   "not pointer",
			cfg:    map[string]any{},
			target: llmOnly{},
			stage:  StageResolve,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := BuildInto(context.Background(), tc.cfg, tc.target)
			if err == nil {
				t.Fatal("expected error")
			}
			var ae *Error
			if !errors.As(err, &ae) {
				t.Fatalf("got %T %v", err, err)
			}
			if ae.Stage != tc.stage {
				t.Fatalf("stage=%s want %s err=%v", ae.Stage, tc.stage, err)
			}
		})
	}
}

func TestBuild_OptionalDepAndSlice(t *testing.T) {
	optKind := kind(t, "opt")
	pluginkit.Register(optKind, func(_ struct{}, deps struct {
		Logger LLM `json:"logger,omitempty"`
	}) (*openai, error) {
		if deps.Logger != nil {
			return &openai{model: deps.Logger.Model()}, nil
		}
		return &openai{model: "none"}, nil
	})
	var plugins struct {
		LLM   LLM    `json:"llm"`
		Tools []Tool `json:"tools"`
	}
	_, err := BuildInto(context.Background(), map[string]any{
		"llm": config.PluginUse{Use: optKind},
	}, &plugins)
	if err != nil {
		t.Fatal(err)
	}
	if plugins.LLM.Model() != "none" || plugins.Tools == nil {
		t.Fatalf("%+v", plugins)
	}
}

func TestBuild_TypeMismatchDoesNotCallConstructor(t *testing.T) {
	badKind := kind(t, "bad-static")
	called := false
	pluginkit.Register(badKind, func() (*shell, error) {
		called = true
		return &shell{}, nil
	})

	var plugins struct {
		LLM LLM `json:"llm"`
	}
	_, err := BuildInto(context.Background(), map[string]any{
		"llm": config.PluginUse{Use: badKind},
	}, &plugins)
	if err == nil {
		t.Fatal("expected error")
	}
	if called {
		t.Fatal("constructor should not be called when registered return type cannot satisfy target field")
	}
	var ae *Error
	if !errors.As(err, &ae) {
		t.Fatalf("got %T %v", err, err)
	}
	if ae.Stage != StageTypeCheck {
		t.Fatalf("stage=%s want %s err=%v", ae.Stage, StageTypeCheck, err)
	}
}

func TestCompilePlanDoesNotCallConstructor(t *testing.T) {
	k := kind(t, "compile-only")
	called := false
	pluginkit.Register(k, func() (*openai, error) {
		called = true
		return &openai{model: "x"}, nil
	})

	var plugins struct {
		LLM LLM `json:"llm"`
	}
	_, fields, err := inspectTarget(&plugins)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := config.Parse(map[string]any{
		"llm": config.PluginUse{Use: k},
	})
	if err != nil {
		t.Fatal(err)
	}

	p, err := compilePlan(parsed, fields)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.steps) != 1 {
		t.Fatalf("steps=%d want 1", len(p.steps))
	}
	if called {
		t.Fatal("compilePlan should not call constructors")
	}
}

func TestCompilePlanRejectsDuplicateIDs(t *testing.T) {
	k := kind(t, "duplicate-id")
	pluginkit.Register(k, func() (*openai, error) {
		return &openai{model: "x"}, nil
	})

	var plugins struct {
		Primary LLM `json:"primary"`
		Backup  LLM `json:"backup"`
	}
	_, fields, err := inspectTarget(&plugins)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := config.Parse(map[string]any{
		"primary": config.PluginUse{ID: "same", Use: k},
		"backup":  config.PluginUse{ID: "same", Use: k},
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = compilePlan(parsed, fields)
	if err == nil {
		t.Fatal("expected error")
	}
	var ae *Error
	if !errors.As(err, &ae) {
		t.Fatalf("got %T %v", err, err)
	}
	if ae.Stage != StageResolve || ae.ID != "same" {
		t.Fatalf("unexpected error: %+v", ae)
	}
}

func TestExecutePlanPreservesTargetSliceOrder(t *testing.T) {
	storeKind := kind(t, "store")
	firstKind := kind(t, "first")
	secondKind := kind(t, "second")
	pluginkit.Register(storeKind, func() (*sqliteStore, error) { return &sqliteStore{}, nil })
	pluginkit.Register(firstKind, func(_ struct{}, deps saveDeps) (*saveStep, error) {
		return &saveStep{store: deps.Store}, nil
	})
	pluginkit.Register(secondKind, func() (*httpStep, error) { return &httpStep{}, nil })

	var plugins workflowPlugins
	tv, fields, err := inspectTarget(&plugins)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := config.Parse(map[string]any{
		"store": config.PluginUse{ID: "store", Use: storeKind},
		"steps": []config.PluginUse{
			{ID: "first", Use: firstKind, Deps: map[string]any{"store": "store"}},
			{ID: "second", Use: secondKind},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	p, err := compilePlan(parsed, fields)
	if err != nil {
		t.Fatal(err)
	}
	exec, err := executePlan(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	if err := assignTarget(tv, p, exec.instances); err != nil {
		t.Fatal(err)
	}
	if len(plugins.Steps) != 2 || plugins.Steps[0].Run() != "save:x" || plugins.Steps[1].Run() != "http" {
		t.Fatalf("steps order changed: %#v", plugins.Steps)
	}
}

func TestBuild_DepMismatchDoesNotCallDependentConstructor(t *testing.T) {
	storeKind := kind(t, "store")
	saveKind := kind(t, "save")
	calledSave := false
	pluginkit.Register(storeKind, func() (*shell, error) { return &shell{}, nil })
	pluginkit.Register(saveKind, func(_ struct{}, deps saveDeps) (*saveStep, error) {
		calledSave = true
		return &saveStep{store: deps.Store}, nil
	})

	var plugins struct {
		Store Tool `json:"store"`
		Step  Step `json:"step"`
	}
	_, err := BuildInto(context.Background(), map[string]any{
		"store": config.PluginUse{ID: "store", Use: storeKind},
		"step":  config.PluginUse{ID: "save", Use: saveKind, Deps: map[string]any{"store": "store"}},
	}, &plugins)
	if err == nil {
		t.Fatal("expected error")
	}
	if calledSave {
		t.Fatal("dependent constructor should not be called when dependency does not satisfy Deps field")
	}
	var ae *Error
	if !errors.As(err, &ae) {
		t.Fatalf("got %T %v", err, err)
	}
	if ae.Stage != StageDeps {
		t.Fatalf("stage=%s want %s err=%v", ae.Stage, StageDeps, err)
	}
}

func TestRequireByID_ValidatesBusinessReferences(t *testing.T) {
	storeKind := kind(t, "store")
	httpKind := kind(t, "http")
	saveKind := kind(t, "save")
	pluginkit.Register(storeKind, func() (*sqliteStore, error) { return &sqliteStore{}, nil })
	pluginkit.Register(httpKind, func() (*httpStep, error) { return &httpStep{}, nil })
	pluginkit.Register(saveKind, func(_ struct{}, deps saveDeps) (*saveStep, error) {
		return &saveStep{store: deps.Store}, nil
	})

	var plugins workflowPlugins
	result, err := BuildInto(context.Background(), map[string]any{
		"store": config.PluginUse{ID: "store", Use: storeKind},
		"steps": []config.PluginUse{
			{ID: "fetch", Use: httpKind},
			{ID: "save", Use: saveKind, Deps: map[string]any{"store": "store"}},
		},
	}, &plugins)
	if err != nil {
		t.Fatal(err)
	}
	_, err = RequireByID[Step](result, "workflow.steps[2]", "store")
	if err == nil {
		t.Fatal("expected error")
	}
	var ae *Error
	if !errors.As(err, &ae) {
		t.Fatalf("got %T %v", err, err)
	}
	if ae.Stage != StageTypeCheck || ae.Field != "workflow.steps[2]" || ae.ID != "store" {
		t.Fatalf("unexpected error: %+v", ae)
	}
}

func TestErrorFormat(t *testing.T) {
	t.Parallel()
	err := assembleErr("llm", "openai", "llm", StageTypeCheck, errors.New("not an LLM"))
	var ae *Error
	if !errors.As(err, &ae) {
		t.Fatal("errors.As")
	}
	s := ae.Error()
	for _, want := range []string{"typecheck", "field=\"llm\"", "use=\"openai\"", "id=\"llm\""} {
		if !strings.Contains(s, want) {
			t.Fatalf("Error()=%q missing %q", s, want)
		}
	}
}
