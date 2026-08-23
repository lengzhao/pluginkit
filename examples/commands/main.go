package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/lengzhao/pluginkit"
	"github.com/lengzhao/pluginkit/build"
	"gopkg.in/yaml.v3"
)

type Command struct {
	Name        string
	Description string
	Run         func() error
}

type Provider interface {
	Commands() []Command
}

type Service interface {
	Name() string
}

type appDeps struct {
	Services []Service `json:"services"`
}

type app struct {
	services []Service
}

type compactionService struct{}

func (s *compactionService) Name() string { return "compaction" }

func (s *compactionService) Commands() []Command {
	return []Command{{
		Name:        "compact",
		Description: "compact older conversation history",
		Run: func() error {
			fmt.Println("compacted conversation history")
			return nil
		},
	}}
}

type sessionService struct{}

func (s *sessionService) Name() string { return "session" }

func (s *sessionService) Commands() []Command {
	return []Command{{
		Name:        "session",
		Description: "show current session info",
		Run: func() error {
			fmt.Println("session: demo-session")
			return nil
		},
	}}
}

type Registry struct {
	byName map[string]Command
}

func NewRegistry(providers []Provider) (*Registry, error) {
	r := &Registry{byName: make(map[string]Command)}
	for _, provider := range providers {
		for _, cmd := range provider.Commands() {
			if cmd.Name == "" {
				return nil, fmt.Errorf("empty command name")
			}
			if _, exists := r.byName[cmd.Name]; exists {
				return nil, fmt.Errorf("duplicate command %q", cmd.Name)
			}
			r.byName[cmd.Name] = cmd
		}
	}
	return r, nil
}

func (r *Registry) List() []Command {
	out := make([]Command, 0, len(r.byName))
	for _, cmd := range r.byName {
		out = append(out, cmd)
	}
	return out
}

func (r *Registry) Run(name string) error {
	cmd, ok := r.byName[name]
	if !ok {
		return fmt.Errorf("unknown command %q", name)
	}
	return cmd.Run()
}

func newApp(_ struct{}, deps appDeps) (*app, error) {
	return &app{services: deps.Services}, nil
}

func newCompaction() (*compactionService, error) { return &compactionService{}, nil }

func newSession() (*sessionService, error) { return &sessionService{}, nil }

func init() {
	pluginkit.Register("app", newApp)
	pluginkit.Register("compaction", newCompaction)
	pluginkit.Register("session", newSession)
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

	app, result, err := build.Build[*app](context.Background(), graph, "app")
	if err != nil {
		fatal(err)
	}

	providers := build.Collect[Provider](result)
	registry, err := NewRegistry(providers)
	if err != nil {
		fatal(err)
	}

	fmt.Printf("app services: %d\n", len(app.services))
	for _, inst := range build.CollectInstances[Provider](result) {
		fmt.Printf("provider %s (%s)\n", inst.ID, inst.Use)
	}

	if command := commandName(); command != "" {
		if err := registry.Run(command); err != nil {
			fatal(err)
		}
		return
	}

	fmt.Println("commands:")
	for _, cmd := range registry.List() {
		fmt.Printf("  /%s - %s\n", cmd.Name, cmd.Description)
	}
	fmt.Println()
	fmt.Println("run one command:")
	fmt.Printf("  go run . %s\n", exampleCommandName(registry))
}

func exampleCommandName(registry *Registry) string {
	for _, cmd := range registry.List() {
		return cmd.Name
	}
	return "compact"
}

func configPath() string {
	if len(os.Args) > 1 && isYAML(os.Args[1]) {
		return os.Args[1]
	}
	return "config.yaml"
}

func commandName() string {
	if len(os.Args) <= 1 {
		return ""
	}
	if isYAML(os.Args[1]) {
		if len(os.Args) > 2 {
			return os.Args[2]
		}
		return ""
	}
	return os.Args[1]
}

func isYAML(path string) bool {
	return strings.HasSuffix(path, ".yaml") || strings.HasSuffix(path, ".yml")
}

func fatal(err error) {
	slog.Error("example failed", "err", err)
	os.Exit(1)
}
