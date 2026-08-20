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

type LLM interface {
	Model() string
}

type Tool interface {
	Name() string
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

func init() {
	pluginkit.Register("agent", newAgent)
	pluginkit.Register("openai", newOpenAI)
	pluginkit.Register("read-file", newReadFile)
	pluginkit.Register("shell", newShell)
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

	agent, _, err := build.Build[*agent](context.Background(), graph, "agent")
	if err != nil {
		fatal(err)
	}

	fmt.Println("llm:", agent.llm.Model())
	for _, tool := range agent.tools {
		fmt.Println("tool:", tool.Name())
	}
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
