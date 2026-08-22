package main

import (
	"context"
	"flag"
	"log/slog"
	"os"

	"github.com/lengzhao/pluginkit/build"
	"github.com/lengzhao/pluginkit/manager"
)

func main() {
	addr := flag.String("addr", ":8080", "HTTP listen address")
	config := flag.String("config", "", "initial YAML config file to load")
	flag.Parse()

	registerDemoPlugins()

	var initialYAML string
	if *config != "" {
		raw, err := os.ReadFile(*config)
		if err != nil {
			slog.Error("read config failed", "path", *config, "err", err)
			os.Exit(1)
		}
		initialYAML = string(raw)
	}

	if err := manager.Run(manager.Options{
		Addr:          *addr,
		InitialYAML:   initialYAML,
		ValidateBuild: demoValidateBuild,
		OnChange: func(ctx context.Context, evt manager.DocumentEvent) error {
			slog.Info("document changed",
				"reason", evt.Reason,
				"operation", evt.Operation,
				"rootId", evt.Document.RootID,
				"yamlBytes", len(evt.YAML),
			)
			return nil
		},
		OnBuild: func(ctx context.Context, evt manager.DocumentEvent) error {
			errors := 0
			for _, d := range evt.Diagnostics {
				if d.Severity == "error" {
					errors++
				}
			}
			slog.Info("build finished", "errors", errors)
			return nil
		},
	}); err != nil {
		slog.Error("manager failed", "err", err)
		os.Exit(1)
	}
}

func demoValidateBuild(ctx context.Context, doc manager.Document) error {
	graph := doc.ToGraph()
	switch doc.Plugin.Use {
	case "agent":
		_, _, err := build.Build[Agent](ctx, graph, doc.RootID)
		return err
	case "sequential-workflow":
		_, _, err := build.Build[Workflow](ctx, graph, doc.RootID)
		return err
	default:
		return nil
	}
}
