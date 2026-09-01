package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stackrox/harness-openshell/internal/gateway"
)

type mockGW struct {
	providers       map[string]bool
	createErr       error
	createCalls     int
	createOpts      []gateway.SandboxCreateOpts
	deletedNames    []string
	onSandboxCreate func(gateway.SandboxCreateOpts) error
	providerCreates []providerCreateCall
}

// providerCreateCall records one ProviderCreate for assertions on which
// bootstrap strategy fired.
type providerCreateCall struct {
	name        string
	profileType string
	opts        gateway.ProviderCreateOpts
}

func (m *mockGW) ProviderGet(name string) error {
	if m.providers[name] {
		return nil
	}
	return fmt.Errorf("not found")
}
func (m *mockGW) SandboxCreate(opts gateway.SandboxCreateOpts) error {
	m.createCalls++
	m.createOpts = append(m.createOpts, opts)
	if m.onSandboxCreate != nil {
		if err := m.onSandboxCreate(opts); err != nil {
			return err
		}
	}
	if m.createErr != nil && m.createCalls == 1 {
		return m.createErr
	}
	return nil
}
func (m *mockGW) SandboxDelete(name string) error {
	m.deletedNames = append(m.deletedNames, name)
	return nil
}
func (m *mockGW) ProviderCreate(name, profileType string, opts gateway.ProviderCreateOpts) error {
	m.providerCreates = append(m.providerCreates, providerCreateCall{name, profileType, opts})
	return nil
}
func (m *mockGW) ProviderProfileImport(string) error                                 { return nil }
func (m *mockGW) ProviderRefreshConfigure(string, gateway.ProviderRefreshOpts) error { return nil }
func (m *mockGW) ProviderRefreshRotate(string, string) error                         { return nil }

func setupTestAgent(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "agents"), 0o755)
	os.WriteFile(filepath.Join(dir, "agents", "default.yaml"), []byte(`name: test-agent
image: quay.io/test:latest
entrypoint: claude
providers:
  - profile: github
  - profile: google-vertex-ai
  - profile: atlassian
env:
  FOO: bar
`), 0o644)
	return dir
}
