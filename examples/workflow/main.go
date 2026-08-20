package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/lengzhao/pluginkit"
	"github.com/lengzhao/pluginkit/build"
	"gopkg.in/yaml.v3"
)

type Store interface {
	Name() string
}

type Step interface {
	Run() string
}

type Workflow interface {
	Run()
}

type workflowDeps struct {
	Steps []Step `json:"steps"`
}

type sequentialWorkflow struct {
	steps []Step
}

func (w *sequentialWorkflow) Run() {
	for _, step := range w.steps {
		fmt.Println("run:", step.Run())
	}
}

type sqliteStore struct{}

func (s *sqliteStore) Name() string { return "sqlite" }

func newStore() (*sqliteStore, error) { return &sqliteStore{}, nil }

type httpStep struct{}

func (h *httpStep) Run() string { return "fetch" }

func newHTTP() (*httpStep, error) { return &httpStep{}, nil }

type saveDeps struct {
	Store Store `json:"store"`
}

type saveStep struct {
	store Store
}

func (s *saveStep) Run() string { return "save->" + s.store.Name() }

func newSave(_ struct{}, deps saveDeps) (*saveStep, error) {
	return &saveStep{store: deps.Store}, nil
}

func newWorkflow(_ struct{}, deps workflowDeps) (*sequentialWorkflow, error) {
	return &sequentialWorkflow{steps: deps.Steps}, nil
}

func init() {
	pluginkit.Register("sqlite-store", newStore)
	pluginkit.Register("http-step", newHTTP)
	pluginkit.Register("save-step", newSave)
	pluginkit.Register("sequential-workflow", newWorkflow)
}

func main() {
	raw, err := os.ReadFile(configPath())
	if err != nil {
		fatal(err)
	}
	var graph map[string]any
	if err := yaml.Unmarshal(raw, &graph); err != nil {
		fatal(err)
	}

	workflow, _, err := build.Build[Workflow](context.Background(), graph, "workflow")
	if err != nil {
		fatal(err)
	}
	workflow.Run()
}

func fatal(err error) {
	slog.Error("example failed", "err", err)
	os.Exit(1)
}

func configPath() string {
	if len(os.Args) > 1 {
		return os.Args[1]
	}
	return "config.yaml"
}
