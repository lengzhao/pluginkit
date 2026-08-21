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
	flag.Parse()

	registerDemoPlugins()

	if err := manager.Run(manager.Options{
		Addr:          *addr,
		ValidateBuild: demoValidateBuild,
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
