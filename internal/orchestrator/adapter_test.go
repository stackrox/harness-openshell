package orchestrator

import (
	"strings"
	"testing"
)

func TestNewAdapterValid(t *testing.T) {
	for _, name := range []string{"claude", "codex", "opencode"} {
		a, err := NewAdapter(name)
		if err != nil {
			t.Errorf("NewAdapter(%q): %v", name, err)
		}
		if a.Name() != name {
			t.Errorf("Name() = %q, want %q", a.Name(), name)
		}
	}
}

func TestNewAdapterInvalid(t *testing.T) {
	_, err := NewAdapter("unknown")
	if err == nil {
		t.Error("expected error for unknown adapter")
	}
}

func TestClaudeAdapterCommands(t *testing.T) {
	a := ClaudeAdapter{}

	cmd := a.BuildCommand("do stuff", true)
	args := strings.Join(cmd.Args, " ")
	if args != "claude --print do stuff" {
		t.Errorf("headless+task = %q", args)
	}

	cmd = a.BuildCommand("do stuff", false)
	args = strings.Join(cmd.Args, " ")
	if args != "claude -p do stuff" {
		t.Errorf("tty+task = %q", args)
	}

	cmd = a.BuildCommand("", false)
	args = strings.Join(cmd.Args, " ")
	if args != "claude" {
		t.Errorf("no task = %q", args)
	}
}

func TestCodexAdapterCommands(t *testing.T) {
	a := CodexAdapter{}

	cmd := a.BuildCommand("review code", true)
	args := strings.Join(cmd.Args, " ")
	if args != "codex --print review code" {
		t.Errorf("headless+task = %q", args)
	}

	cmd = a.BuildCommand("review code", false)
	args = strings.Join(cmd.Args, " ")
	if args != "codex -p review code" {
		t.Errorf("tty+task = %q", args)
	}
}

func TestOpenCodeAdapterCommands(t *testing.T) {
	a := OpenCodeAdapter{}

	cmd := a.BuildCommand("fix bugs", true)
	args := strings.Join(cmd.Args, " ")
	if args != "opencode run fix bugs" {
		t.Errorf("headless+task = %q", args)
	}

	cmd = a.BuildCommand("fix bugs", false)
	args = strings.Join(cmd.Args, " ")
	if args != "opencode run fix bugs" {
		t.Errorf("tty+task (opencode always uses run) = %q", args)
	}

	cmd = a.BuildCommand("", false)
	args = strings.Join(cmd.Args, " ")
	if args != "opencode" {
		t.Errorf("no task = %q", args)
	}
}
