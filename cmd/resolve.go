package cmd

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/stackrox/harness-openshell/internal/agent"
)


func resolveAgentPath(harnessDir, agentName, agentFile string) string {
	if agentFile != "" {
		return agentFile
	}
	filename := "agent-" + agentName + ".yaml"
	match, _ := findFile(harnessDir, filename)
	if match != "" {
		return match
	}
	return filepath.Join(harnessDir, filename)
}

func findFile(root, name string) (string, error) {
	var match string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.Name() == ".git" || d.Name() == "node_modules" {
			return filepath.SkipDir
		}
		if d.Name() == name {
			match = path
			return filepath.SkipAll
		}
		return nil
	})
	return match, err
}

func resolveHarness(harnessDir, agentName, agentFile string) (*agent.Harness, error) {
	path := resolveAgentPath(harnessDir, agentName, agentFile)
	h, err := agent.ParseHarnessFile(path)
	if err == nil {
		if h.Agent.BaseAgent != "" {
			h, err = resolveBaseAgent(harnessDir, h)
			if err != nil {
				return nil, err
			}
		}
		return h, nil
	}
	if agentFile != "" || agentName != "default" || len(DefaultAgentConfig) == 0 {
		return nil, err
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		return nil, err
	}
	return agent.ParseHarness(DefaultAgentConfig)
}

func resolveBaseAgent(harnessDir string, overlay *agent.Harness) (*agent.Harness, error) {
	baseName := overlay.Agent.BaseAgent
	basePath := resolveAgentPath(harnessDir, baseName, "")
	baseH, err := agent.ParseHarnessFile(basePath)
	if err != nil {
		if len(DefaultAgentConfig) > 0 && baseName == "default" {
			baseH, err = agent.ParseHarness(DefaultAgentConfig)
		}
		if err != nil {
			return nil, fmt.Errorf("resolving base_agent %q: %w", baseName, err)
		}
	}
	overlay.Agent = baseH.Agent.MergeOver(overlay.Agent)
	// Merge base harness-level payloads, providers, gateways, policy
	for name, data := range baseH.Providers {
		if _, exists := overlay.Providers[name]; !exists {
			overlay.Providers[name] = data
		}
	}
	for name, data := range baseH.Gateways {
		if _, exists := overlay.Gateways[name]; !exists {
			overlay.Gateways[name] = data
		}
	}
	if overlay.Policy == nil {
		overlay.Policy = baseH.Policy
	}
	return overlay, nil
}

func versionedImage(name string) string {
	base := "quay.io/rcochran/openshell"
	if Version == "" || Version == "dev" {
		return base + ":" + name
	}
	return base + ":" + name + "-" + Version
}

func loadProviderProfiles(harnessDir string) map[string][]byte {
	profiles := make(map[string][]byte)
	dir := filepath.Join(harnessDir, "profiles", "providers")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return profiles
	}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".yaml" {
			continue
		}
		name := e.Name()[:len(e.Name())-5]
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err == nil {
			profiles[name] = data
		}
	}
	return profiles
}

