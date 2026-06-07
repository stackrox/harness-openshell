package cmd

import (
	"fmt"
	"testing"

	"github.com/robbycochran/harness-openshell/internal/k8s"
)

func TestEnsureCreds_NamespaceNotFound(t *testing.T) {
	nsRunner := k8s.NewMockRunner()
	nsRunner.Errors["namespace-exists"] = fmt.Errorf("not found")

	err := ensureCreds(nsRunner, "openshell", false)
	if err == nil {
		t.Fatal("expected error for missing namespace")
	}
}

func TestEnsureCreds_SecretsExist(t *testing.T) {
	nsRunner := k8s.NewMockRunner()
	// namespace exists (no error), secrets exist (no error)

	err := ensureCreds(nsRunner, "openshell", false)
	if err != nil {
		t.Fatalf("ensureCreds: %v", err)
	}
	// Should NOT attempt to create secrets
	if nsRunner.HasCall("create secret") {
		t.Error("should not create secrets when they already exist")
	}
}

func TestEnsureCreds_ForceDeletesExisting(t *testing.T) {
	nsRunner := k8s.NewMockRunner()

	err := ensureCreds(nsRunner, "openshell", true)
	if err != nil {
		t.Fatalf("ensureCreds: %v", err)
	}
	// Should attempt to delete atlassian secret
	if nsRunner.CallCount("delete secret") != 1 {
		t.Errorf("expected 1 secret delete, got %d: %v",
			nsRunner.CallCount("delete secret"), nsRunner.Calls)
	}
}

func TestEnsureCreds_AtlassianCreated(t *testing.T) {
	nsRunner := k8s.NewMockRunner()
	nsRunner.Errors["secret-exists openshell-atlassian"] = fmt.Errorf("not found")

	t.Setenv("JIRA_URL", "https://example.atlassian.net")
	t.Setenv("JIRA_USERNAME", "user@example.com")

	err := ensureCreds(nsRunner, "openshell", false)
	if err != nil {
		t.Fatalf("ensureCreds: %v", err)
	}
	if !nsRunner.HasCall("create secret generic openshell-atlassian") {
		t.Errorf("expected atlassian secret creation, calls: %v", nsRunner.Calls)
	}
}

func TestEnsureCreds_AtlassianSkipped(t *testing.T) {
	nsRunner := k8s.NewMockRunner()
	nsRunner.Errors["secret-exists openshell-atlassian"] = fmt.Errorf("not found")
	t.Setenv("JIRA_URL", "")
	t.Setenv("JIRA_USERNAME", "")

	err := ensureCreds(nsRunner, "openshell", false)
	if err != nil {
		t.Fatalf("ensureCreds: %v", err)
	}
	if nsRunner.HasCall("create secret generic openshell-atlassian") {
		t.Error("should not create atlassian secret without JIRA_URL")
	}
}
