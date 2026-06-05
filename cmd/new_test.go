package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/robbycochran/harness-openshell/internal/gateway"
)

type mockGW struct {
	inferenceErr  error
	providers     map[string]bool
	providerList  []string
	providerErr   error
	createErr     error
	createCalls   int
	createOpts    []gateway.SandboxCreateOpts
	deletedNames  []string
}

func (m *mockGW) InferenceGet() error { return m.inferenceErr }
func (m *mockGW) ProviderGet(name string) error {
	if m.providers[name] {
		return nil
	}
	return fmt.Errorf("not found")
}
func (m *mockGW) ProviderList() ([]string, error) { return m.providerList, m.providerErr }
func (m *mockGW) SandboxCreate(opts gateway.SandboxCreateOpts) error {
	m.createCalls++
	m.createOpts = append(m.createOpts, opts)
	if m.createErr != nil && m.createCalls == 1 {
		return m.createErr
	}
	return nil
}
func (m *mockGW) SandboxDelete(name string) error {
	m.deletedNames = append(m.deletedNames, name)
	return nil
}
func (m *mockGW) CLIVersion() string                                            { return "openshell v0.0.55" }
func (m *mockGW) CLIPath() string                                               { return "/usr/bin/openshell" }
func (m *mockGW) InferenceModel() string                                        { return "" }
func (m *mockGW) InferenceSet(string, string) error                             { return nil }
func (m *mockGW) InferenceRemove() error                                        { return nil }
func (m *mockGW) ActiveGateway() string                                         { return "" }
func (m *mockGW) ProviderCreate(string, string, gateway.ProviderCreateOpts) error { return nil }
func (m *mockGW) ProviderDelete(string) error                                   { return nil }
func (m *mockGW) ProviderProfileImport(string) error                            { return nil }
func (m *mockGW) ProviderProfileDelete(string) error                            { return nil }
func (m *mockGW) SettingsSet(string, string) error                              { return nil }
func (m *mockGW) SandboxList() ([]string, error)                                { return nil, nil }
func (m *mockGW) SandboxConnect(string) error                                   { return nil }
func (m *mockGW) SandboxUpload(string, string, string) error                    { return nil }
func (m *mockGW) SandboxExec(string, ...string) error                           { return nil }
func (m *mockGW) GatewayAdd(string, string, bool) error                         { return nil }
func (m *mockGW) GatewayRemove(string) error                                    { return nil }
func (m *mockGW) GatewayList() ([]gateway.GatewayInfo, error)                   { return nil, nil }
func (m *mockGW) GatewaySelect(string) error                                    { return nil }

func setupTestProfile(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "profiles"), 0o755)
	os.WriteFile(filepath.Join(dir, "profiles", "default.toml"), []byte(`
name = "test-agent"
image = "quay.io/test:latest"
command = "claude --bare"
providers = ["github", "vertex-local", "atlassian"]

[env]
FOO = "bar"
`), 0o644)
	return dir
}

func noopScript(name string, args ...string) error { return nil }

func TestNewLocal_NoGateway(t *testing.T) {
	dir := setupTestProfile(t)
	gw := &mockGW{inferenceErr: fmt.Errorf("connection refused")}

	err := newLocal(newLocalOpts{
		harnessDir:  dir,
		gw:          gw,
		profileName: "default",
		noTTY:       true,
		runScript:   noopScript,
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "no active gateway") {
		t.Errorf("error = %q, want 'no active gateway'", err)
	}
}

func TestNewLocal_NoProviders_CallsScript(t *testing.T) {
	dir := setupTestProfile(t)
	scriptCalled := false
	gw := &mockGW{
		providerList: nil,
		providers:    map[string]bool{},
	}

	newLocal(newLocalOpts{
		harnessDir:  dir,
		gw:          gw,
		profileName: "default",
		noTTY:       true,
		runScript: func(name string, args ...string) error {
			if name == "providers.sh" {
				scriptCalled = true
			}
			return nil
		},
	})
	if !scriptCalled {
		t.Error("expected providers.sh to be called when no providers registered")
	}
}

func TestNewLocal_MissingProviders(t *testing.T) {
	dir := setupTestProfile(t)
	gw := &mockGW{
		providerList: []string{"github"},
		providers:    map[string]bool{"github": true},
	}

	err := newLocal(newLocalOpts{
		harnessDir:  dir,
		gw:          gw,
		profileName: "default",
		noTTY:       true,
		runScript:   noopScript,
	})
	if err != nil {
		t.Fatalf("newLocal: %v", err)
	}
	if gw.createCalls != 1 {
		t.Fatalf("createCalls = %d, want 1", gw.createCalls)
	}
	opts := gw.createOpts[0]
	if len(opts.Providers) != 1 || opts.Providers[0] != "github" {
		t.Errorf("Providers = %v, want [github] only", opts.Providers)
	}
}

func TestNewLocal_AllProvidersMissing(t *testing.T) {
	dir := setupTestProfile(t)
	gw := &mockGW{
		providerList: []string{"github"},
		providers:    map[string]bool{},
	}

	err := newLocal(newLocalOpts{
		harnessDir:  dir,
		gw:          gw,
		profileName: "default",
		noTTY:       true,
		runScript:   noopScript,
	})
	if err != nil {
		t.Fatalf("newLocal: %v", err)
	}
	opts := gw.createOpts[0]
	if len(opts.Providers) != 0 {
		t.Errorf("Providers = %v, want empty", opts.Providers)
	}
}

func TestNewLocal_ProfileNotFound(t *testing.T) {
	dir := setupTestProfile(t)
	gw := &mockGW{providerList: []string{"github"}}

	err := newLocal(newLocalOpts{
		harnessDir:  dir,
		gw:          gw,
		profileName: "nonexistent",
		noTTY:       true,
		runScript:   noopScript,
	})
	if err == nil {
		t.Fatal("expected error for missing profile")
	}
}

func TestNewLocal_SandboxCreateRetry(t *testing.T) {
	dir := setupTestProfile(t)
	gw := &mockGW{
		providerList: []string{"github"},
		providers:    map[string]bool{"github": true},
		createErr:    fmt.Errorf("supervisor race"),
	}

	err := newLocal(newLocalOpts{
		harnessDir:  dir,
		gw:          gw,
		profileName: "default",
		noTTY:       true,
		runScript:   noopScript,
		retrySleep:  0,
	})
	if err != nil {
		t.Fatalf("newLocal: %v", err)
	}
	if gw.createCalls != 2 {
		t.Errorf("createCalls = %d, want 2 (first fails, second succeeds)", gw.createCalls)
	}
	if len(gw.deletedNames) != 1 {
		t.Errorf("deletedNames = %v, want 1 cleanup delete", gw.deletedNames)
	}
}

func TestNewLocal_SandboxCreateOpts(t *testing.T) {
	dir := setupTestProfile(t)
	gw := &mockGW{
		providerList: []string{"github", "vertex-local"},
		providers:    map[string]bool{"github": true, "vertex-local": true},
	}

	err := newLocal(newLocalOpts{
		harnessDir:  dir,
		gw:          gw,
		profileName: "default",
		sandboxName: "custom-name",
		noTTY:       true,
		runScript:   noopScript,
	})
	if err != nil {
		t.Fatalf("newLocal: %v", err)
	}
	opts := gw.createOpts[0]
	if opts.Name != "custom-name" {
		t.Errorf("Name = %q, want custom-name", opts.Name)
	}
	if opts.Image != "quay.io/test:latest" {
		t.Errorf("Image = %q", opts.Image)
	}
	if opts.TTY {
		t.Error("TTY = true, want false (noTTY)")
	}
	if !opts.Keep {
		t.Error("Keep = false, want true (default)")
	}
}
