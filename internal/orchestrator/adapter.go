package orchestrator

import (
	"fmt"
	"os/exec"
)

type HarnessAdapter interface {
	Name() string
	BuildCommand(task string, headless bool) *exec.Cmd
}

func NewAdapter(name string) (HarnessAdapter, error) {
	switch name {
	case "claude":
		return ClaudeAdapter{}, nil
	case "codex":
		return CodexAdapter{}, nil
	case "opencode":
		return OpenCodeAdapter{}, nil
	default:
		return nil, fmt.Errorf("unknown adapter %q", name)
	}
}

type ClaudeAdapter struct{}

func (ClaudeAdapter) Name() string { return "claude" }

func (ClaudeAdapter) BuildCommand(task string, headless bool) *exec.Cmd {
	if task == "" {
		return exec.Command("claude")
	}
	if headless {
		return exec.Command("claude", "--print", task)
	}
	return exec.Command("claude", "-p", task)
}

type CodexAdapter struct{}

func (CodexAdapter) Name() string { return "codex" }

func (CodexAdapter) BuildCommand(task string, headless bool) *exec.Cmd {
	if task == "" {
		return exec.Command("codex")
	}
	if headless {
		return exec.Command("codex", "--print", task)
	}
	return exec.Command("codex", "-p", task)
}

type OpenCodeAdapter struct{}

func (OpenCodeAdapter) Name() string { return "opencode" }

func (OpenCodeAdapter) BuildCommand(task string, headless bool) *exec.Cmd {
	if task == "" {
		return exec.Command("opencode")
	}
	return exec.Command("opencode", "run", task)
}
