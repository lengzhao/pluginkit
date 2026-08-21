package main

import (
	"fmt"

	"github.com/lengzhao/pluginkit"
)

type LLM interface {
	Model() string
}

type Tool interface {
	Name() string
}

type Hook interface {
	Name() string
	Write(string)
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

type Agent interface {
	LLM() LLM
	Tools() []Tool
}

type agentDeps struct {
	LLM   LLM    `json:"llm"`
	Tools []Tool `json:"tools"`
	Hook  Hook   `json:"hook,omitempty"`
}

type agent struct {
	llm   LLM
	tools []Tool
	hook  Hook
}

func (a *agent) LLM() LLM { return a.llm }

func (a *agent) Tools() []Tool { return a.tools }

type openaiCfg struct {
	Model string `json:"model"`
}

type openai struct{ model string }

func (o *openai) Model() string { return o.model }

func newOpenAI(cfg openaiCfg) (LLM, error) {
	return &openai{model: cfg.Model}, nil
}

type readFileCfg struct {
	Root string `json:"root"`
}

type readFile struct{ root string }

func (r *readFile) Name() string { return "read-file" }

func newReadFile(cfg readFileCfg) (Tool, error) {
	return &readFile{root: cfg.Root}, nil
}

type shell struct{}

func (s *shell) Name() string { return "shell" }

func newShell() (Tool, error) { return &shell{}, nil }

type logHookCfg struct {
	Prefix string `json:"prefix"`
}

type logHook struct {
	prefix string
}

func (h *logHook) Name() string {
	if h.prefix == "" {
		return "log-hook"
	}
	return h.prefix
}

func (h *logHook) Write(msg string) {
	fmt.Println(h.prefix, msg)
}

func newLogHook(cfg logHookCfg) (Hook, error) {
	return &logHook{prefix: cfg.Prefix}, nil
}

func newAgent(_ struct{}, deps agentDeps) (Agent, error) {
	return &agent{llm: deps.LLM, tools: deps.Tools, hook: deps.Hook}, nil
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

func newStore() (Store, error) { return &sqliteStore{}, nil }

type httpStep struct{}

func (h *httpStep) Run() string { return "fetch" }

func newHTTP() (Step, error) { return &httpStep{}, nil }

type saveDeps struct {
	Store Store `json:"store"`
}

type saveStep struct {
	store Store
}

func (s *saveStep) Run() string { return "save" }

func newSave(_ struct{}, deps saveDeps) (Step, error) {
	return &saveStep{store: deps.Store}, nil
}

func newWorkflow(_ struct{}, deps workflowDeps) (Workflow, error) {
	return &sequentialWorkflow{steps: deps.Steps}, nil
}

func registerDemoPlugins() {
	pluginkit.Register("agent", newAgent)
	pluginkit.Register("openai", newOpenAI)
	pluginkit.Register("read-file", newReadFile)
	pluginkit.Register("shell", newShell)
	pluginkit.Register("log-hook", newLogHook)
	pluginkit.Register("sqlite-store", newStore)
	pluginkit.Register("http-step", newHTTP)
	pluginkit.Register("save-step", newSave)
	pluginkit.Register("sequential-workflow", newWorkflow)
}
