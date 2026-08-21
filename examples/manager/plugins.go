package main

import (
	"github.com/lengzhao/pluginkit"
)

type LLM interface {
	Model() string
}

type Tool interface {
	Name() string
}

type Store interface {
	Name() string
}

type Step interface {
	Run() string
}

type Workflow interface {
	Run()
}

type agentDeps struct {
	LLM   LLM    `json:"llm"`
	Tools []Tool `json:"tools"`
}

type agent struct {
	llm   LLM
	tools []Tool
}

type openaiCfg struct {
	Model string `json:"model"`
}

type openai struct{ model string }

func (o *openai) Model() string { return o.model }

func newOpenAI(cfg openaiCfg) (*openai, error) {
	return &openai{model: cfg.Model}, nil
}

type readFileCfg struct {
	Root string `json:"root"`
}

type readFile struct{ root string }

func (r *readFile) Name() string { return "read-file" }

func newReadFile(cfg readFileCfg) (*readFile, error) {
	return &readFile{root: cfg.Root}, nil
}

type shell struct{}

func (s *shell) Name() string { return "shell" }

func newShell() (*shell, error) { return &shell{}, nil }

func newAgent(_ struct{}, deps agentDeps) (*agent, error) {
	return &agent{llm: deps.LLM, tools: deps.Tools}, nil
}

type workflowDeps struct {
	Steps []Step `json:"steps"`
}

type sequentialWorkflow struct {
	steps []Step
}

func (w *sequentialWorkflow) Run() {}

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

func (s *saveStep) Run() string { return "save" }

func newSave(_ struct{}, deps saveDeps) (*saveStep, error) {
	return &saveStep{store: deps.Store}, nil
}

func newWorkflow(_ struct{}, deps workflowDeps) (*sequentialWorkflow, error) {
	return &sequentialWorkflow{steps: deps.Steps}, nil
}

func registerDemoPlugins() {
	pluginkit.Register("agent", newAgent)
	pluginkit.Register("openai", newOpenAI)
	pluginkit.Register("read-file", newReadFile)
	pluginkit.Register("shell", newShell)
	pluginkit.Register("sqlite-store", newStore)
	pluginkit.Register("http-step", newHTTP)
	pluginkit.Register("save-step", newSave)
	pluginkit.Register("sequential-workflow", newWorkflow)
}
