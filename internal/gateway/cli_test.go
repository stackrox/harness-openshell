package gateway

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeStub(t *testing.T, script string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "stub")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestProviderList_ParsesTable(t *testing.T) {
	bin := writeStub(t, `#!/bin/bash
printf "NAME\tTYPE\tSTATUS\n"
printf "github\tgithub\tactive\n"
printf "vertex-local\tgoogle-vertex-ai\tactive\n"
printf "atlassian\tatlassian\tactive\n"
`)
	gw := NewCLI(bin)
	names, err := gw.ProviderList()
	if err != nil {
		t.Fatalf("ProviderList: %v", err)
	}
	if len(names) != 3 {
		t.Fatalf("got %d providers, want 3: %v", len(names), names)
	}
	if names[0] != "github" || names[1] != "vertex-local" || names[2] != "atlassian" {
		t.Errorf("names = %v", names)
	}
}

func TestProviderList_Empty(t *testing.T) {
	bin := writeStub(t, `#!/bin/bash
printf "NAME\tTYPE\tSTATUS\n"
`)
	gw := NewCLI(bin)
	names, err := gw.ProviderList()
	if err != nil {
		t.Fatalf("ProviderList: %v", err)
	}
	if len(names) != 0 {
		t.Errorf("got %d providers, want 0: %v", len(names), names)
	}
}

func TestProviderList_CLINotFound(t *testing.T) {
	gw := NewCLI("/nonexistent/openshell")
	_, err := gw.ProviderList()
	if err == nil {
		t.Error("expected error for missing CLI")
	}
}

func TestProviderGet_Exists(t *testing.T) {
	bin := writeStub(t, `#!/bin/bash
[[ "$3" == "github" ]] && exit 0
exit 1
`)
	gw := NewCLI(bin)
	if err := gw.ProviderGet("github"); err != nil {
		t.Errorf("ProviderGet(github) = %v, want nil", err)
	}
}

func TestProviderGet_Missing(t *testing.T) {
	bin := writeStub(t, `#!/bin/bash
exit 1
`)
	gw := NewCLI(bin)
	if err := gw.ProviderGet("nonexistent"); err == nil {
		t.Error("ProviderGet(nonexistent) = nil, want error")
	}
}

func TestInferenceGet_Active(t *testing.T) {
	bin := writeStub(t, `#!/bin/bash
exit 0
`)
	gw := NewCLI(bin)
	if err := gw.InferenceGet(); err != nil {
		t.Errorf("InferenceGet = %v, want nil", err)
	}
}

func TestInferenceGet_NoGateway(t *testing.T) {
	bin := writeStub(t, `#!/bin/bash
exit 1
`)
	gw := NewCLI(bin)
	if err := gw.InferenceGet(); err == nil {
		t.Error("InferenceGet = nil, want error")
	}
}

func TestSandboxCreate_ArgsMinimal(t *testing.T) {
	bin := writeStub(t, `#!/bin/bash
echo "$@" > /tmp/test-create-args
`)
	gw := NewCLI(bin)
	gw.SandboxCreate(SandboxCreateOpts{
		Name: "test",
		TTY:  false,
		Keep: true,
	})
	data, _ := os.ReadFile("/tmp/test-create-args")
	args := strings.TrimSpace(string(data))
	if !strings.Contains(args, "--name test") {
		t.Errorf("missing --name: %s", args)
	}
	if !strings.Contains(args, "--no-tty") {
		t.Errorf("missing --no-tty: %s", args)
	}
	if strings.Contains(args, "--no-keep") {
		t.Errorf("should not have --no-keep: %s", args)
	}
	if strings.Contains(args, "--from") {
		t.Errorf("should not have --from: %s", args)
	}
	os.Remove("/tmp/test-create-args")
}

func TestSandboxCreate_ArgsFull(t *testing.T) {
	dir := t.TempDir()
	argsFile := filepath.Join(dir, "args")
	bin := writeStub(t, `#!/bin/bash
echo "$@" > `+argsFile+`
`)
	gw := NewCLI(bin)
	gw.SandboxCreate(SandboxCreateOpts{
		Name:      "my-agent",
		Image:     "quay.io/test:latest",
		Providers: []string{"github", "vertex-local"},
		TTY:       true,
		Keep:      false,
		UploadSrc: "/tmp/openshell",
		UploadDst: "/sandbox/.config",
		Command:   []string{"bash", "-c", "exec claude"},
	})
	data, _ := os.ReadFile(argsFile)
	args := strings.TrimSpace(string(data))

	for _, want := range []string{
		"--name my-agent",
		"--tty",
		"--from quay.io/test:latest",
		"--provider github",
		"--provider vertex-local",
		"--no-keep",
		"--upload /tmp/openshell:/sandbox/.config",
		"--no-git-ignore",
		"-- bash -c exec claude",
	} {
		if !strings.Contains(args, want) {
			t.Errorf("missing %q in: %s", want, args)
		}
	}
}

func TestSandboxDelete_Silent(t *testing.T) {
	bin := writeStub(t, `#!/bin/bash
exit 0
`)
	gw := NewCLI(bin)
	if err := gw.SandboxDelete("test"); err != nil {
		t.Errorf("SandboxDelete = %v", err)
	}
}

func TestSandboxDelete_NotFound(t *testing.T) {
	bin := writeStub(t, `#!/bin/bash
exit 1
`)
	gw := NewCLI(bin)
	if err := gw.SandboxDelete("missing"); err == nil {
		t.Error("SandboxDelete = nil, want error")
	}
}
