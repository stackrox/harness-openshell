package cmd

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/robbycochran/harness-openshell/internal/agent"
	"github.com/robbycochran/harness-openshell/internal/gateway"
)

func resolveSandboxName(gw gateway.Gateway, args []string) (string, error) {
	if len(args) > 0 {
		return args[0], nil
	}
	names, err := gw.SandboxList()
	if err != nil {
		return "", fmt.Errorf("listing sandboxes: %w", err)
	}
	if len(names) == 0 {
		return "", fmt.Errorf("no sandboxes running")
	}
	if len(names) > 1 {
		return "", fmt.Errorf("multiple sandboxes running, specify one: %v", names)
	}
	return names[0], nil
}

func resolveAgentPath(harnessDir, agentName, agentFile string) string {
	if agentFile != "" {
		return agentFile
	}
	return filepath.Join(harnessDir, "agents", agentName+".yaml")
}

func resolveAgentConfig(harnessDir, agentName, agentFile string) (*agent.AgentConfig, error) {
	path := resolveAgentPath(harnessDir, agentName, agentFile)
	cfg, err := agent.ParseFile(path)
	if err == nil {
		return cfg, nil
	}
	if agentFile != "" || agentName != "default" || len(DefaultAgentConfig) == 0 {
		return nil, err
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		return nil, err
	}
	return agent.Parse(DefaultAgentConfig)
}

func versionedImage(name string) string {
	base := "ghcr.io/robbycochran/harness-openshell"
	if Version == "" || Version == "dev" {
		return base + ":" + name
	}
	return base + ":" + name + "-" + Version
}

// EmbeddedGatewayProfiles holds embedded gateway profile YAML, set from main.go.
var EmbeddedGatewayProfiles map[string][]byte

func resolveGatewayConfig(harnessDir, name string) (*gateway.GatewayConfig, error) {
	cfg, err := gateway.LoadProfile(harnessDir, name)
	if err == nil {
		return cfg, nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	gwDir := filepath.Join(harnessDir, "gateways", name)
	cfg, err = gateway.LoadConfig(gwDir)
	if err == nil {
		return cfg, nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	if data, ok := EmbeddedGatewayProfiles[name]; ok {
		return gateway.LoadConfigFromBytes(data)
	}
	return nil, fmt.Errorf("gateway profile %q not found", name)
}

func resolveGatewayConfigFromFile(path string) (*gateway.GatewayConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading gateway profile %s: %w", path, err)
	}
	cfg, err := gateway.LoadConfigFromBytes(data)
	if err != nil {
		return nil, err
	}
	cfg.Dir = filepath.Dir(path)
	return cfg, nil
}
