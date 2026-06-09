package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/robbycochran/harness-openshell/internal/gateway"
)

type mockGW struct {
	inferenceErr      error
	providers         map[string]bool
	providerList      []string
	providerErr       error
	createErr         error
	createCalls       int
	createOpts        []gateway.SandboxCreateOpts
	deletedNames      []string
	gatewayListResult []gateway.GatewayInfo
	onGatewayRemove   func(string)
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
func (m *mockGW) CLIVersion() string                                              { return "openshell v0.0.58" }
func (m *mockGW) CLIPath() string                                                 { return "/usr/bin/openshell" }
func (m *mockGW) InferenceModel() string                                          { return "" }
func (m *mockGW) InferenceSet(string, string) error                               { return nil }
func (m *mockGW) InferenceRemove() error                                          { return nil }
func (m *mockGW) ActiveGateway() string                                           { return "" }
func (m *mockGW) ProviderCreate(string, string, gateway.ProviderCreateOpts) error { return nil }
func (m *mockGW) ProviderDelete(string) error                                     { return nil }
func (m *mockGW) ProviderProfileImport(string) error                              { return nil }
func (m *mockGW) ProviderProfileDelete(string) error                              { return nil }
func (m *mockGW) SettingsSet(string, string) error                                { return nil }
func (m *mockGW) SandboxList() ([]string, error)                                  { return nil, nil }
func (m *mockGW) SandboxStatus() ([]gateway.SandboxInfo, error)                   { return nil, nil }
func (m *mockGW) SandboxConnect(string) error                                     { return nil }
func (m *mockGW) SandboxLogs(string, bool) error                                 { return nil }
func (m *mockGW) SandboxStop(string) error                                        { return nil }
func (m *mockGW) SandboxStart(string) error                                       { return nil }
func (m *mockGW) GatewayAdd(string, string, bool, bool) error                    { return nil }
func (m *mockGW) GatewayRemove(name string) error {
	if m.onGatewayRemove != nil {
		m.onGatewayRemove(name)
	}
	return nil
}
func (m *mockGW) GatewayList() ([]gateway.GatewayInfo, error) {
	return m.gatewayListResult, nil
}
func (m *mockGW) GatewaySelect(string) error                                             { return nil }
func (m *mockGW) ProviderRefreshConfigure(string, gateway.ProviderRefreshOpts) error     { return nil }
func (m *mockGW) ProviderRefreshRotate(string, string) error                             { return nil }

func setupTestAgent(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "agents"), 0o755)
	os.WriteFile(filepath.Join(dir, "agents", "default.yaml"), []byte(`name: test-agent
image: quay.io/test:latest
entrypoint: claude --bare
providers:
  - profile: github
  - profile: vertex-local
  - profile: atlassian
env:
  FOO: bar
`), 0o644)
	return dir
}
