package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/robbycochran/harness-openshell/internal/agent"
)

func setupProvidersTest(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "profiles", "providers"), 0o755)
	return dir
}

func TestRegisterProviders_GitHubWhenTokenSet(t *testing.T) {
	dir := setupProvidersTest(t)
	t.Setenv("GITHUB_TOKEN", "ghp_test123")

	gw := &mockGW{providers: map[string]bool{}}

	err := registerProviders(dir, gw, false, []agent.ProviderRef{
		{Profile: "github"},
	})
	if err != nil {
		t.Fatalf("registerProviders: %v", err)
	}
}

func TestRegisterProviders_SkipsWhenTokenMissing(t *testing.T) {
	dir := setupProvidersTest(t)
	t.Setenv("GITHUB_TOKEN", "")

	gw := &mockGW{providers: map[string]bool{}}

	err := registerProviders(dir, gw, false, []agent.ProviderRef{
		{Profile: "github"},
	})
	if err != nil {
		t.Fatalf("registerProviders: %v", err)
	}
}

func TestRegisterProviders_SkipsExistingProvider(t *testing.T) {
	dir := setupProvidersTest(t)
	t.Setenv("GITHUB_TOKEN", "ghp_test123")

	gw := &mockGW{providers: map[string]bool{"github": true}}

	err := registerProviders(dir, gw, false, []agent.ProviderRef{
		{Profile: "github"},
	})
	if err != nil {
		t.Fatalf("registerProviders: %v", err)
	}
}

func TestRegisterProviders_ForceWithRunningSandboxes(t *testing.T) {
	dir := setupProvidersTest(t)

	gw := &mockGWWithSandboxes{
		mockGW:    &mockGW{providers: map[string]bool{"github": true}},
		sandboxes: []string{"test-sandbox"},
	}

	err := registerProviders(dir, gw, true, []agent.ProviderRef{
		{Profile: "github"},
	})
	if err == nil {
		t.Fatal("expected error with --force and running sandboxes")
	}
	if !strings.Contains(err.Error(), "cannot --provider-refresh") {
		t.Errorf("error = %q, want 'cannot --provider-refresh'", err)
	}
}

func TestRegisterProviders_ForceDeletesAndRecreates(t *testing.T) {
	dir := setupProvidersTest(t)
	t.Setenv("GITHUB_TOKEN", "ghp_test123")

	gw := &mockGW{providers: map[string]bool{}}

	err := registerProviders(dir, gw, true, []agent.ProviderRef{
		{Profile: "github"},
	})
	if err != nil {
		t.Fatalf("registerProviders: %v", err)
	}
}

func TestRegisterProviders_OnlyRegistersRequestedProviders(t *testing.T) {
	dir := setupProvidersTest(t)
	t.Setenv("GITHUB_TOKEN", "ghp_test123")
	t.Setenv("JIRA_API_TOKEN", "jira_test")

	gw := &mockGW{providers: map[string]bool{}}

	err := registerProviders(dir, gw, false, []agent.ProviderRef{
		{Profile: "github"},
	})
	if err != nil {
		t.Fatalf("registerProviders: %v", err)
	}
}

func TestRegisterProviders_PassesConfigToProvider(t *testing.T) {
	dir := setupProvidersTest(t)
	t.Setenv("JIRA_API_TOKEN", "jira_test")
	t.Setenv("JIRA_URL", "https://test.atlassian.net")
	t.Setenv("JIRA_USERNAME", "test@example.com")

	gw := &mockGW{providers: map[string]bool{}}

	err := registerProviders(dir, gw, false, []agent.ProviderRef{
		{Profile: "atlassian", Env: map[string]string{
			"JIRA_URL":      "${JIRA_URL}",
			"JIRA_USERNAME": "${JIRA_USERNAME}",
		}},
	})
	if err != nil {
		t.Fatalf("registerProviders: %v", err)
	}
}

func TestRegisterProviders_EmptyList(t *testing.T) {
	dir := setupProvidersTest(t)
	gw := &mockGW{providers: map[string]bool{}}

	err := registerProviders(dir, gw, false, nil)
	if err != nil {
		t.Fatalf("registerProviders: %v", err)
	}
}

// mockGWWithSandboxes wraps mockGW to return a non-empty sandbox list.
type mockGWWithSandboxes struct {
	*mockGW
	sandboxes []string
}

func (m *mockGWWithSandboxes) SandboxList() ([]string, error) {
	return m.sandboxes, nil
}
