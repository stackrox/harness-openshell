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
	gw := New(bin)
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
	gw := New(bin)
	names, err := gw.ProviderList()
	if err != nil {
		t.Fatalf("ProviderList: %v", err)
	}
	if len(names) != 0 {
		t.Errorf("got %d providers, want 0: %v", len(names), names)
	}
}

func TestProviderList_CLINotFound(t *testing.T) {
	gw := New("/nonexistent/openshell")
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
	gw := New(bin)
	if err := gw.ProviderGet("github"); err != nil {
		t.Errorf("ProviderGet(github) = %v, want nil", err)
	}
}

func TestProviderGet_Missing(t *testing.T) {
	bin := writeStub(t, `#!/bin/bash
exit 1
`)
	gw := New(bin)
	if err := gw.ProviderGet("nonexistent"); err == nil {
		t.Error("ProviderGet(nonexistent) = nil, want error")
	}
}

func TestInferenceGet_Active(t *testing.T) {
	bin := writeStub(t, `#!/bin/bash
exit 0
`)
	gw := New(bin)
	if err := gw.InferenceGet(); err != nil {
		t.Errorf("InferenceGet = %v, want nil", err)
	}
}

func TestInferenceGet_NoGateway(t *testing.T) {
	bin := writeStub(t, `#!/bin/bash
exit 1
`)
	gw := New(bin)
	if err := gw.InferenceGet(); err == nil {
		t.Error("InferenceGet = nil, want error")
	}
}

func TestSandboxCreate_ArgsMinimal(t *testing.T) {
	argsFile := filepath.Join(t.TempDir(), "args")
	bin := writeStub(t, `#!/bin/bash
echo "$@" > `+argsFile+`
`)
	gw := New(bin)
	gw.SandboxCreate(SandboxCreateOpts{
		Name: "test",
		TTY:  false,
		Keep: true,
	})
	data, _ := os.ReadFile(argsFile)
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
}

func TestSandboxCreate_ArgsFull(t *testing.T) {
	dir := t.TempDir()
	argsFile := filepath.Join(dir, "args")
	bin := writeStub(t, `#!/bin/bash
echo "$@" > `+argsFile+`
`)
	gw := New(bin)
	gw.SandboxCreate(SandboxCreateOpts{
		Name:      "my-agent",
		From:      "quay.io/test:latest",
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
	gw := New(bin)
	if err := gw.SandboxDelete("test"); err != nil {
		t.Errorf("SandboxDelete = %v", err)
	}
}

func TestSandboxDelete_NotFound(t *testing.T) {
	bin := writeStub(t, `#!/bin/bash
exit 1
`)
	gw := New(bin)
	if err := gw.SandboxDelete("missing"); err == nil {
		t.Error("SandboxDelete = nil, want error")
	}
}

func TestGatewayList_ParsesActiveAndInactive(t *testing.T) {
	bin := writeStub(t, `#!/bin/bash
printf "NAME\tENDPOINT\tTYPE\tAUTH\n"
printf "  openshell\thttps://127.0.0.1:17670\tlocal\tmtls\n"
printf "* openshell-remote-ocp\thttps://gw.example.com:443\tlocal\tmtls\n"
`)
	gw := New(bin)
	gateways, err := gw.GatewayList()
	if err != nil {
		t.Fatalf("GatewayList: %v", err)
	}
	if len(gateways) != 2 {
		t.Fatalf("got %d gateways, want 2", len(gateways))
	}
	if gateways[0].Active {
		t.Error("first gateway should not be active")
	}
	if !gateways[1].Active {
		t.Error("second gateway should be active")
	}
	if !strings.Contains(gateways[0].Endpoint, "127.0.0.1") {
		t.Errorf("first endpoint = %q, want 127.0.0.1", gateways[0].Endpoint)
	}
}

func TestSandboxList_ParsesWithANSI(t *testing.T) {
	bin := writeStub(t, `#!/bin/bash
printf "NAME\tPHASE\n"
printf "\033[32magent\033[0m\tReady\n"
printf "\033[32mtest-agent\033[0m\tReady\n"
`)
	gw := New(bin)
	names, err := gw.SandboxList()
	if err != nil {
		t.Fatalf("SandboxList: %v", err)
	}
	if len(names) != 2 {
		t.Fatalf("got %d sandboxes, want 2: %v", len(names), names)
	}
	if names[0] != "agent" || names[1] != "test-agent" {
		t.Errorf("names = %v", names)
	}
}

func TestSandboxStatus_ParsesTable(t *testing.T) {
	bin := writeStub(t, `#!/bin/bash
printf "NAME\tPHASE\n"
printf "agent\tReady\n"
printf "test-agent\tStopped\n"
`)
	gw := New(bin)
	infos, err := gw.SandboxStatus()
	if err != nil {
		t.Fatalf("SandboxStatus: %v", err)
	}
	if len(infos) != 2 {
		t.Fatalf("got %d sandboxes, want 2: %v", len(infos), infos)
	}
	if infos[0].Name != "agent" || infos[0].Phase != "Ready" {
		t.Errorf("infos[0] = %+v", infos[0])
	}
	if infos[1].Name != "test-agent" || infos[1].Phase != "Stopped" {
		t.Errorf("infos[1] = %+v", infos[1])
	}
}

func TestSandboxStatus_Empty(t *testing.T) {
	bin := writeStub(t, `#!/bin/bash
printf "NAME\tPHASE\n"
`)
	gw := New(bin)
	infos, err := gw.SandboxStatus()
	if err != nil {
		t.Fatalf("SandboxStatus: %v", err)
	}
	if len(infos) != 0 {
		t.Errorf("got %d, want 0", len(infos))
	}
}

func TestSandboxStop_Silent(t *testing.T) {
	bin := writeStub(t, `#!/bin/bash
exit 0
`)
	gw := New(bin)
	if err := gw.SandboxStop("test"); err != nil {
		t.Errorf("SandboxStop = %v", err)
	}
}

func TestSandboxStart_Silent(t *testing.T) {
	bin := writeStub(t, `#!/bin/bash
exit 0
`)
	gw := New(bin)
	if err := gw.SandboxStart("test"); err != nil {
		t.Errorf("SandboxStart = %v", err)
	}
}

func TestSandboxLogs_PassesFollow(t *testing.T) {
	argsFile := filepath.Join(t.TempDir(), "args")
	bin := writeStub(t, `#!/bin/bash
echo "$@" > `+argsFile+`
`)
	gw := New(bin)
	gw.SandboxLogs("my-agent", true)
	data, _ := os.ReadFile(argsFile)
	args := strings.TrimSpace(string(data))
	if !strings.Contains(args, "sandbox logs my-agent --follow") {
		t.Errorf("expected sandbox logs with --follow, got: %s", args)
	}
}

func TestSandboxLogs_NoFollow(t *testing.T) {
	argsFile := filepath.Join(t.TempDir(), "args")
	bin := writeStub(t, `#!/bin/bash
echo "$@" > `+argsFile+`
`)
	gw := New(bin)
	gw.SandboxLogs("my-agent", false)
	data, _ := os.ReadFile(argsFile)
	args := strings.TrimSpace(string(data))
	if strings.Contains(args, "--follow") {
		t.Errorf("should not have --follow: %s", args)
	}
	if !strings.Contains(args, "sandbox logs my-agent") {
		t.Errorf("missing sandbox logs: %s", args)
	}
}

func TestSandboxList_Empty(t *testing.T) {
	bin := writeStub(t, `#!/bin/bash
printf "NAME\tPHASE\n"
`)
	gw := New(bin)
	names, err := gw.SandboxList()
	if err != nil {
		t.Fatalf("SandboxList: %v", err)
	}
	if len(names) != 0 {
		t.Errorf("got %d, want 0", len(names))
	}
}

func TestActiveGateway_WithStar(t *testing.T) {
	bin := writeStub(t, `#!/bin/bash
printf "NAME\tENDPOINT\n"
printf "  local\thttps://127.0.0.1:17670\n"
printf "* remote\thttps://gw.example.com\n"
`)
	gw := New(bin)
	active := gw.ActiveGateway()
	if active != "remote" {
		t.Errorf("ActiveGateway = %q, want remote", active)
	}
}

func TestActiveGateway_None(t *testing.T) {
	bin := writeStub(t, `#!/bin/bash
printf "NAME\tENDPOINT\n"
printf "  local\thttps://127.0.0.1:17670\n"
`)
	gw := New(bin)
	active := gw.ActiveGateway()
	if active != "" {
		t.Errorf("ActiveGateway = %q, want empty", active)
	}
}

func TestInferenceModel_ParsesModel(t *testing.T) {
	bin := writeStub(t, `#!/bin/bash
echo "Provider: vertex-local"
echo "Model: claude-sonnet-4-6"
`)
	gw := New(bin)
	model := gw.InferenceModel()
	if model != "claude-sonnet-4-6" {
		t.Errorf("InferenceModel = %q, want claude-sonnet-4-6", model)
	}
}

func TestCLIVersion(t *testing.T) {
	bin := writeStub(t, `#!/bin/bash
echo "openshell v0.0.58"
`)
	gw := New(bin)
	ver := gw.CLIVersion()
	if ver != "openshell v0.0.58" {
		t.Errorf("CLIVersion = %q", ver)
	}
}

func TestCLIPath(t *testing.T) {
	bin := writeStub(t, `#!/bin/bash
exit 0
`)
	gw := New(bin)
	path := gw.CLIPath()
	if path == "" {
		t.Error("CLIPath = empty, want non-empty")
	}
}

func TestCLIPath_NotFound(t *testing.T) {
	gw := New("/nonexistent/openshell")
	path := gw.CLIPath()
	if path != "" {
		t.Errorf("CLIPath = %q, want empty", path)
	}
}

func TestSandboxCreate_WithEnv(t *testing.T) {
	argsFile := filepath.Join(t.TempDir(), "args")
	bin := writeStub(t, `#!/bin/bash
echo "$@" > `+argsFile+`
`)
	gw := New(bin)
	gw.SandboxCreate(SandboxCreateOpts{
		Name: "env-test",
		TTY:  false,
		Keep: true,
		Env: map[string]string{
			"FOO":               "bar",
			"ANTHROPIC_API_KEY": "sk-proxy",
		},
		Command: []string{"true"},
	})
	data, _ := os.ReadFile(argsFile)
	args := strings.TrimSpace(string(data))
	for _, want := range []string{
		"--env ANTHROPIC_API_KEY=sk-proxy",
		"--env FOO=bar",
	} {
		if !strings.Contains(args, want) {
			t.Errorf("missing %q in: %s", want, args)
		}
	}
	envIdx := strings.Index(args, "--env")
	cmdIdx := strings.Index(args, "-- true")
	if envIdx > cmdIdx {
		t.Errorf("--env should appear before -- command separator: %s", args)
	}
}

func TestSandboxCreate_NoEnv(t *testing.T) {
	argsFile := filepath.Join(t.TempDir(), "args")
	bin := writeStub(t, `#!/bin/bash
echo "$@" > `+argsFile+`
`)
	gw := New(bin)
	gw.SandboxCreate(SandboxCreateOpts{
		Name: "no-env",
		TTY:  false,
		Keep: true,
	})
	data, _ := os.ReadFile(argsFile)
	args := strings.TrimSpace(string(data))
	if strings.Contains(args, "--env") {
		t.Errorf("should not have --env when env map is nil: %s", args)
	}
}

func TestParseCLIVersion(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"openshell v0.0.59", "0.0.59"},
		{"openshell v0.0.58", "0.0.58"},
		{"v1.2.3", "1.2.3"},
		{"0.0.59", "0.0.59"},
	}
	for _, tt := range tests {
		got := ParseCLIVersion(tt.input)
		if got != tt.want {
			t.Errorf("ParseCLIVersion(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestCheckMinVersion_Below(t *testing.T) {
	bin := writeStub(t, `#!/bin/bash
echo "openshell v0.0.57"
`)
	gw := New(bin)
	if err := gw.CheckMinVersion("0.0.59"); err == nil {
		t.Error("expected error for version below minimum")
	}
}

func TestCheckMinVersion_Equal(t *testing.T) {
	bin := writeStub(t, `#!/bin/bash
echo "openshell v0.0.59"
`)
	gw := New(bin)
	if err := gw.CheckMinVersion("0.0.59"); err != nil {
		t.Errorf("CheckMinVersion: %v", err)
	}
}

func TestCheckMinVersion_Above(t *testing.T) {
	bin := writeStub(t, `#!/bin/bash
echo "openshell v0.0.60"
`)
	gw := New(bin)
	if err := gw.CheckMinVersion("0.0.59"); err != nil {
		t.Errorf("CheckMinVersion: %v", err)
	}
}

func TestCheckMinVersion_NoCLI(t *testing.T) {
	gw := New("/nonexistent/openshell")
	if err := gw.CheckMinVersion("0.0.59"); err == nil {
		t.Error("expected error when CLI not found")
	}
}

func TestProviderCreate_Args(t *testing.T) {
	dir := t.TempDir()
	argsFile := filepath.Join(dir, "args")
	bin := writeStub(t, `#!/bin/bash
printf '%s\n' "$*" > `+argsFile+`
`)
	gw := New(bin)
	gw.ProviderCreate("vertex-local", "google-vertex-ai", ProviderCreateOpts{
		FromADC:     true,
		Credentials: []string{"TOKEN=abc"},
		Configs:     []string{"PROJECT=my-proj", "REGION=us-east5"},
	})
	data, _ := os.ReadFile(argsFile)
	args := strings.TrimSpace(string(data))
	for _, want := range []string{
		"--name vertex-local",
		"--type google-vertex-ai",
		"--from-gcloud-adc",
		"--credential TOKEN=abc",
		"--config PROJECT=my-proj",
		"--config REGION=us-east5",
	} {
		if !strings.Contains(args, want) {
			t.Errorf("missing %q in: %s", want, args)
		}
	}
}

func TestInferenceSet_Args(t *testing.T) {
	dir := t.TempDir()
	argsFile := filepath.Join(dir, "args")
	bin := writeStub(t, `#!/bin/bash
printf '%s\n' "$*" > `+argsFile+`
`)
	gw := New(bin)
	gw.InferenceSet("vertex-local", "claude-sonnet-4-6")
	data, _ := os.ReadFile(argsFile)
	args := strings.TrimSpace(string(data))
	for _, want := range []string{
		"inference set",
		"--provider vertex-local",
		"--model claude-sonnet-4-6",
		"--no-verify",
	} {
		if !strings.Contains(args, want) {
			t.Errorf("missing %q in: %s", want, args)
		}
	}
}

func TestGatewayAdd_Args(t *testing.T) {
	dir := t.TempDir()
	argsFile := filepath.Join(dir, "args")
	bin := writeStub(t, `#!/bin/bash
printf '%s\n' "$*" > `+argsFile+`
`)
	gw := New(bin)
	gw.GatewayAdd("https://gw.example.com:443", "my-ocp", true, false)
	data, _ := os.ReadFile(argsFile)
	args := strings.TrimSpace(string(data))
	for _, want := range []string{
		"gateway add",
		"https://gw.example.com:443",
		"--name my-ocp",
		"--local",
	} {
		if !strings.Contains(args, want) {
			t.Errorf("missing %q in: %s", want, args)
		}
	}
}

func TestGatewayRemove(t *testing.T) {
	bin := writeStub(t, `#!/bin/bash
exit 0
`)
	gw := New(bin)
	if err := gw.GatewayRemove("old-gw"); err != nil {
		t.Errorf("GatewayRemove: %v", err)
	}
}

func TestProviderProfileDelete(t *testing.T) {
	bin := writeStub(t, `#!/bin/bash
exit 0
`)
	gw := New(bin)
	if err := gw.ProviderProfileDelete("profile-123"); err != nil {
		t.Errorf("ProviderProfileDelete: %v", err)
	}
}
