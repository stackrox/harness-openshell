package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/stackrox/harness-openshell/internal/orchestrator"
)

var version = "dev"

func main() {
	configPath := flag.String("config", "/sandbox/.config/openshell/orchestrator.yaml", "path to orchestrator config")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println("harness-orchestrator", version)
		os.Exit(0)
	}

	cfg, err := orchestrator.LoadConfig(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "harness-orchestrator: %v\n", err)
		os.Exit(1)
	}

	configDir := filepath.Dir(*configPath)
	if cfg.Task != "" && !filepath.IsAbs(cfg.Task) {
		cfg.Task = filepath.Join(configDir, cfg.Task)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	orch, err := orchestrator.New(cfg, configDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "harness-orchestrator: %v\n", err)
		os.Exit(1)
	}

	if err := orch.Run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "harness-orchestrator: %v\n", err)
		os.Exit(1)
	}
}
