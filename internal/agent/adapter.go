package agent

import (
	"strings"
)

const (
	// SandboxTaskPath is the in-sandbox path to task.md written by the payload uploader.
	SandboxTaskPath = "/sandbox/.config/openshell/task.md"
	// SandboxPayloadBinDir is the in-sandbox path to the payload bin directory.
	SandboxPayloadBinDir = "/sandbox/.config/openshell/bin"
)

// AgentAdapter owns per-agent-type command construction for sandbox execution.
// Given an agent config and the in-sandbox task path, it returns the argv to
// exec inside the sandbox, replacing the generated run.sh shell script.
type AgentAdapter interface {
	// Environment returns agent-type-specific env not already supplied by config.
	// Returns empty map for all adapters today (env is config-owned via BuildEnvMap).
	Environment(cfg *AgentConfig) map[string]string

	// Command returns the argv to exec inside the sandbox. taskPath is the
	// in-sandbox path to task.md ("" when no task). Reproduces BuildRunSh behavior:
	//   - Prepends PATH with payload bin directory
	//   - For tasks with headless mode: uses --print (claude/codex) or run (opencode)
	//   - For tasks with interactive mode: uses -p
	//   - For no task: just the entrypoint
	Command(cfg *AgentConfig, taskPath string) []string
}

// AdapterFor returns the appropriate AgentAdapter for the given entrypoint.
// Dispatch: "claude" or "" -> claude adapter, "codex" -> codex adapter,
// "opencode" -> opencode adapter, else -> custom adapter.
func AdapterFor(entrypoint string) AgentAdapter {
	switch entrypoint {
	case "claude", "":
		return &claudeAdapter{}
	case "codex":
		return &codexAdapter{}
	case "opencode":
		return &opencodeAdapter{}
	default:
		return &customAdapter{}
	}
}

// claudeAdapter implements AgentAdapter for the claude agent.
type claudeAdapter struct{}

func (a *claudeAdapter) Environment(cfg *AgentConfig) map[string]string {
	return make(map[string]string)
}

func (a *claudeAdapter) Command(cfg *AgentConfig, taskPath string) []string {
	return buildCommand("claude", cfg, taskPath)
}

// codexAdapter implements AgentAdapter for the codex agent.
type codexAdapter struct{}

func (a *codexAdapter) Environment(cfg *AgentConfig) map[string]string {
	return make(map[string]string)
}

func (a *codexAdapter) Command(cfg *AgentConfig, taskPath string) []string {
	return buildCommand("codex", cfg, taskPath)
}

// opencodeAdapter implements AgentAdapter for the opencode agent.
type opencodeAdapter struct{}

func (a *opencodeAdapter) Environment(cfg *AgentConfig) map[string]string {
	return make(map[string]string)
}

func (a *opencodeAdapter) Command(cfg *AgentConfig, taskPath string) []string {
	return buildCommand("opencode", cfg, taskPath)
}

// customAdapter implements AgentAdapter for custom entrypoints.
type customAdapter struct{}

func (a *customAdapter) Environment(cfg *AgentConfig) map[string]string {
	return make(map[string]string)
}

func (a *customAdapter) Command(cfg *AgentConfig, taskPath string) []string {
	entrypoint := cfg.EffectiveEntrypoint()
	// For custom entrypoints, treat them as the base agent type but use their
	// custom entrypoint instead of a predefined one.
	return buildCommand(entrypoint, cfg, taskPath)
}

// buildCommand constructs the argv for the given base entrypoint (could be
// "claude", "codex", "opencode", or a custom entrypoint). It handles:
//   - PATH prepending to /sandbox/.config/openshell/bin
//   - Task dispatch (--print for headless, -p for interactive, none for no task)
//   - Entrypoint validation via command -v check
//   - Wrapping in bash -lc for shell setup
func buildCommand(baseEntrypoint string, cfg *AgentConfig, taskPath string) []string {
	epBin := strings.Fields(baseEntrypoint)[0]

	var cmdBuilder strings.Builder

	// Prepend PATH and validate entrypoint
	cmdBuilder.WriteString("export PATH=\"")
	cmdBuilder.WriteString(SandboxPayloadBinDir)
	cmdBuilder.WriteString(":$PATH\"; ")

	cmdBuilder.WriteString("if ! command -v ")
	cmdBuilder.WriteString(epBin)
	cmdBuilder.WriteString(" >/dev/null 2>&1; then echo \"ERROR: entrypoint ")
	cmdBuilder.WriteString(epBin)
	cmdBuilder.WriteString(" not found in PATH\" >&2; exit 1; fi; ")

	cmdBuilder.WriteString("exec ")
	cmdBuilder.WriteString(baseEntrypoint)

	// Handle task dispatch
	if taskPath != "" {
		if cfg.NoTTY() {
			// Headless mode
			switch epBin {
			case "opencode":
				cmdBuilder.WriteString(" run \"$(cat ")
				cmdBuilder.WriteString(taskPath)
				cmdBuilder.WriteString(")\"")
			default:
				// claude, codex, and custom use --print
				cmdBuilder.WriteString(" --print \"$(cat ")
				cmdBuilder.WriteString(taskPath)
				cmdBuilder.WriteString(")\"")
			}
		} else {
			// Interactive mode
			cmdBuilder.WriteString(" -p \"$(cat ")
			cmdBuilder.WriteString(taskPath)
			cmdBuilder.WriteString(")\"")
		}
	}

	return []string{"bash", "-lc", cmdBuilder.String()}
}
