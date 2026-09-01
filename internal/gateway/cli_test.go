package gateway

import (
	"errors"
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
		Providers: []string{"github", "google-vertex-ai"},
		TTY:       true,
		Keep:      false,
		Uploads: []Upload{{Src: "/tmp/openshell", Dst: "/sandbox/.config"}},
		Command:   []string{"bash", "-c", "exec claude"},
	})
	data, _ := os.ReadFile(argsFile)
	args := strings.TrimSpace(string(data))

	for _, want := range []string{
		"--name my-agent",
		"--tty",
		"--from quay.io/test:latest",
		"--provider github",
		"--provider google-vertex-ai",
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
		{"openshell 0.0.59", "0.0.59"},
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
	err := gw.CheckMinVersion("0.0.59")
	if err == nil {
		t.Fatal("expected error for version below minimum")
	}
	// A definitively-old CLI must report ErrVersionBelowMinimum so callers can
	// treat it as a hard failure rather than a warning.
	if !errors.Is(err, ErrVersionBelowMinimum) {
		t.Errorf("error = %v, want wrapping ErrVersionBelowMinimum", err)
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
	err := gw.CheckMinVersion("0.0.59")
	if err == nil {
		t.Fatal("expected error when CLI not found")
	}
	// An unreadable version is NOT a below-minimum failure: callers should warn
	// and proceed, not hard-fail, so it must not wrap ErrVersionBelowMinimum.
	if errors.Is(err, ErrVersionBelowMinimum) {
		t.Errorf("error = %v, should not wrap ErrVersionBelowMinimum", err)
	}
}

func TestProviderCreate_FromExisting(t *testing.T) {
	dir := t.TempDir()
	argsFile := filepath.Join(dir, "args")
	bin := writeStub(t, `#!/bin/bash
printf '%s\n' "$*" > `+argsFile+`
`)
	gw := New(bin)
	gw.ProviderCreate("github", "github", ProviderCreateOpts{
		FromExisting: true,
	})
	data, _ := os.ReadFile(argsFile)
	args := strings.TrimSpace(string(data))
	if !strings.Contains(args, "--from-existing") {
		t.Errorf("missing --from-existing in: %s", args)
	}
	if strings.Contains(args, "--from-gcloud-adc") {
		t.Errorf("should not have --from-gcloud-adc: %s", args)
	}
}

// TestProviderCreate_Args covers the bridge's credential+config passthrough
// (the shape gws uses: a placeholder credential and config, no --from-* flag).
// The gcloud-ADC create is no longer a bridge concern — it is SDK-native
// (sdkclient.CreateVertexProviderFromADC), so no --from-gcloud-adc flag exists.
func TestProviderCreate_Args(t *testing.T) {
	dir := t.TempDir()
	argsFile := filepath.Join(dir, "args")
	bin := writeStub(t, `#!/bin/bash
printf '%s\n' "$*" > `+argsFile+`
`)
	gw := New(bin)
	gw.ProviderCreate("google-workspace", "google-workspace", ProviderCreateOpts{
		Credentials: []string{"TOKEN=abc"},
		Configs:     []string{"PROJECT=my-proj", "REGION=us-east5"},
	})
	data, _ := os.ReadFile(argsFile)
	args := strings.TrimSpace(string(data))
	for _, want := range []string{
		"--name google-workspace",
		"--type google-workspace",
		"--credential TOKEN=abc",
		"--config PROJECT=my-proj",
		"--config REGION=us-east5",
	} {
		if !strings.Contains(args, want) {
			t.Errorf("missing %q in: %s", want, args)
		}
	}
	if strings.Contains(args, "--from-gcloud-adc") {
		t.Errorf("bridge must not emit --from-gcloud-adc (ADC is SDK-native): %s", args)
	}
}

// Test sandboxCreateArgs with all new fields set to verify pinned argv order.
func TestSandboxCreateArgs_AllNewFieldsSet(t *testing.T) {
	opts := SandboxCreateOpts{
		Name:            "my-sandbox",
		Gateway:         "remote-gw",
		Workspace:       "dev",
		Policy:          "/tmp/policy.yaml",
		From:            "quay.io/test:latest",
		Providers:       []string{"github", "google-vertex-ai"},
		NoAutoProviders: true,
		TTY:             true,
		Env: map[string]string{
			"KEY_B": "value_b",
			"KEY_A": "value_a",
		},
		Uploads: []Upload{
			{Src: "/src", Dst: "/dst"},
		},
		Keep: false,
		Labels: map[string]string{
			"label_z": "z_val",
			"label_a": "a_val",
		},
		Command: []string{"bash", "-c", "echo test"},
	}

	args := sandboxCreateArgs(opts)

	// Expected order per spec:
	// sandbox create --name <n>
	//   [--gateway <g>]
	//   [--workspace <w>]
	//   [--policy <p>]
	//   [--from <img>]
	//   [--provider <p>]...
	//   [--no-auto-providers]
	//   (--tty | --no-tty)
	//   [--env k=v]...    (sorted)
	//   [--upload src:dst]... --no-git-ignore
	//   [--no-keep]
	//   [--label k=v]...   (sorted)
	//   [-- <cmd>...]

	expectedOrder := []string{
		"sandbox", "create",
		"--name", "my-sandbox",
		"--gateway", "remote-gw",
		"--workspace", "dev",
		"--policy", "/tmp/policy.yaml",
		"--from", "quay.io/test:latest",
		"--provider", "github",
		"--provider", "google-vertex-ai",
		"--no-auto-providers",
		"--tty",
		"--env", "KEY_A=value_a",
		"--env", "KEY_B=value_b",
		"--upload", "/src:/dst",
		"--no-git-ignore",
		"--no-keep",
		"--label", "label_a=a_val",
		"--label", "label_z=z_val",
		"--",
		"bash", "-c", "echo test",
	}

	if len(args) != len(expectedOrder) {
		t.Fatalf("got %d args, want %d. got: %v", len(args), len(expectedOrder), args)
	}

	for i, want := range expectedOrder {
		if args[i] != want {
			t.Errorf("arg[%d]: got %q, want %q", i, args[i], want)
			t.Logf("Full args: %v", args)
		}
	}
}

// Test sandboxCreateArgs with all new fields zero-valued to ensure
// byte-identical output to existing callers.
func TestSandboxCreateArgs_AllNewFieldsZero(t *testing.T) {
	opts := SandboxCreateOpts{
		Name:      "test",
		From:      "quay.io/test:v1",
		Providers: []string{"provider1", "provider2"},
		TTY:       false,
		Keep:      true,
		Uploads: []Upload{
			{Src: "/src", Dst: "/dst"},
		},
		Env: map[string]string{
			"KEY_Z": "val_z",
			"KEY_A": "val_a",
		},
		Command: []string{"cmd"},
		// All new fields are zero-valued
		Policy:          "",
		Gateway:         "",
		Workspace:       "",
		Labels:          nil,
		NoAutoProviders: false,
	}

	args := sandboxCreateArgs(opts)

	// Should match the old behavior exactly (order per existing code)
	expectedOrder := []string{
		"sandbox", "create",
		"--name", "test",
		"--from", "quay.io/test:v1",
		"--provider", "provider1",
		"--provider", "provider2",
		"--no-tty",
		"--env", "KEY_A=val_a",
		"--env", "KEY_Z=val_z",
		"--upload", "/src:/dst",
		"--no-git-ignore",
		// No --no-keep (Keep=true)
		// No --label (Labels=nil)
		// No --policy (Policy="")
		// No --gateway (Gateway="")
		// No --workspace (Workspace="")
		// No --no-auto-providers (NoAutoProviders=false)
		"--",
		"cmd",
	}

	if len(args) != len(expectedOrder) {
		t.Fatalf("got %d args, want %d. got: %v", len(args), len(expectedOrder), args)
	}

	for i, want := range expectedOrder {
		if args[i] != want {
			t.Errorf("arg[%d]: got %q, want %q", i, args[i], want)
			t.Logf("Full args: %v", args)
		}
	}
}

// Test that providers maintain declared order (not sorted).
func TestSandboxCreateArgs_ProvidersPreserveDeclaredOrder(t *testing.T) {
	opts := SandboxCreateOpts{
		Name:      "test",
		Providers: []string{"z-provider", "a-provider", "m-provider"},
		TTY:       false,
		Keep:      true,
	}

	args := sandboxCreateArgs(opts)

	// Find the provider flags and verify they appear in declared order
	providerArgs := []string{}
	for i, arg := range args {
		if arg == "--provider" && i+1 < len(args) {
			providerArgs = append(providerArgs, args[i+1])
		}
	}

	expectedProviders := []string{"z-provider", "a-provider", "m-provider"}
	if len(providerArgs) != len(expectedProviders) {
		t.Fatalf("got %d providers, want %d: %v", len(providerArgs), len(expectedProviders), providerArgs)
	}

	for i, want := range expectedProviders {
		if providerArgs[i] != want {
			t.Errorf("provider[%d]: got %q, want %q (providers must be in declared order, not sorted)", i, providerArgs[i], want)
		}
	}
}

// Test that labels are sorted by key.
func TestSandboxCreateArgs_LabelsSorted(t *testing.T) {
	opts := SandboxCreateOpts{
		Name: "test",
		TTY:  false,
		Keep: true,
		Labels: map[string]string{
			"zebra": "z_val",
			"apple": "a_val",
			"mango": "m_val",
		},
	}

	args := sandboxCreateArgs(opts)

	// Find the label flags and verify they appear in sorted key order
	labelArgs := []string{}
	for i, arg := range args {
		if arg == "--label" && i+1 < len(args) {
			labelArgs = append(labelArgs, args[i+1])
		}
	}

	expectedLabels := []string{
		"apple=a_val",
		"mango=m_val",
		"zebra=z_val",
	}

	if len(labelArgs) != len(expectedLabels) {
		t.Fatalf("got %d labels, want %d: %v", len(labelArgs), len(expectedLabels), labelArgs)
	}

	for i, want := range expectedLabels {
		if labelArgs[i] != want {
			t.Errorf("label[%d]: got %q, want %q (labels must be sorted by key)", i, labelArgs[i], want)
		}
	}
}

// Test that env and label values do not leak from each other.
// Labels should only come from opts.Labels, never from opts.Env.
func TestSandboxCreateArgs_NoSecretLeakFromEnvToLabels(t *testing.T) {
	opts := SandboxCreateOpts{
		Name: "test",
		TTY:  false,
		Keep: true,
		Env: map[string]string{
			"SECRET_KEY":            "super-secret-123",
			"ANTHROPIC_API_KEY":     "sk-ant-abc123",
			"GITHUB_TOKEN":          "ghp_secret",
		},
		Labels: map[string]string{
			"config-hash": "abc123",
			"run-id":      "run-789",
		},
	}

	args := sandboxCreateArgs(opts)

	// Collect all label values
	labelValues := []string{}
	for i, arg := range args {
		if arg == "--label" && i+1 < len(args) {
			parts := strings.Split(args[i+1], "=")
			if len(parts) == 2 {
				labelValues = append(labelValues, parts[1])
			}
		}
	}

	// Verify no env secret values appear in labels
	for _, labelVal := range labelValues {
		if labelVal == "super-secret-123" || labelVal == "sk-ant-abc123" || labelVal == "ghp_secret" {
			t.Errorf("env secret value leaked into labels: %q", labelVal)
		}
	}

	// Verify expected label values are present
	if len(labelValues) != 2 {
		t.Fatalf("got %d label values, want 2: %v", len(labelValues), labelValues)
	}
}

// Test each new field individually when set (others zero).
func TestSandboxCreateArgs_PolicyField(t *testing.T) {
	opts := SandboxCreateOpts{
		Name:   "test",
		Policy: "/etc/custom-policy.yaml",
		TTY:    false,
		Keep:   true,
	}
	args := sandboxCreateArgs(opts)
	found := false
	for i, arg := range args {
		if arg == "--policy" && i+1 < len(args) && args[i+1] == "/etc/custom-policy.yaml" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("--policy flag not found or has wrong value in: %v", args)
	}
}

func TestSandboxCreateArgs_GatewayField(t *testing.T) {
	opts := SandboxCreateOpts{
		Name:    "test",
		Gateway: "remote-gateway",
		TTY:     false,
		Keep:    true,
	}
	args := sandboxCreateArgs(opts)
	found := false
	for i, arg := range args {
		if arg == "--gateway" && i+1 < len(args) && args[i+1] == "remote-gateway" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("--gateway flag not found or has wrong value in: %v", args)
	}
}

func TestSandboxCreateArgs_WorkspaceField(t *testing.T) {
	opts := SandboxCreateOpts{
		Name:      "test",
		Workspace: "development",
		TTY:       false,
		Keep:      true,
	}
	args := sandboxCreateArgs(opts)
	found := false
	for i, arg := range args {
		if arg == "--workspace" && i+1 < len(args) && args[i+1] == "development" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("--workspace flag not found or has wrong value in: %v", args)
	}
}

func TestSandboxCreateArgs_NoAutoProvidersField(t *testing.T) {
	opts := SandboxCreateOpts{
		Name:            "test",
		NoAutoProviders: true,
		TTY:             false,
		Keep:            true,
	}
	args := sandboxCreateArgs(opts)
	found := false
	for _, arg := range args {
		if arg == "--no-auto-providers" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("--no-auto-providers flag not found in: %v", args)
	}
}

func TestSandboxCreateArgs_NoAutoProvidersFieldNotEmitted(t *testing.T) {
	opts := SandboxCreateOpts{
		Name:            "test",
		NoAutoProviders: false,
		TTY:             false,
		Keep:            true,
	}
	args := sandboxCreateArgs(opts)
	for _, arg := range args {
		if arg == "--no-auto-providers" {
			t.Errorf("--no-auto-providers should not be present when false, but found in: %v", args)
		}
	}
}
