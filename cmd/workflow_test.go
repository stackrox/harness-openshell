package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadCanonicalWorkflowBuildsDirectTargetAndDefaultsWorkspace(t *testing.T) {
	t.Setenv("DIRECT_ENDPOINT", "https://gateway.example.com")
	t.Setenv("DIRECT_ISSUER", "https://issuer.example.com")
	t.Setenv("DIRECT_CLIENT_ID", "ci-user")
	t.Setenv("DIRECT_AUDIENCE", "openshell-gateway")

	path := filepath.Join(t.TempDir(), "workflow.yaml")
	data := []byte(`apiVersion: harness.openshell.dev/v1alpha1
kind: Harness
metadata:
  name: direct
spec:
  target:
    gateway: hypershell
    registration:
      endpoint: ${DIRECT_ENDPOINT}
      oidc:
        issuer: ${DIRECT_ISSUER}
        clientId: ${DIRECT_CLIENT_ID}
        audience: ${DIRECT_AUDIENCE}
`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write workflow: %v", err)
	}

	workflow, err := loadCanonicalWorkflow(path, "", "", canonicalOverrides{})
	if err != nil {
		t.Fatalf("loadCanonicalWorkflow: %v", err)
	}
	if workflow.Target.Workspace != "" {
		t.Fatalf("workspace=%q, want implicit default", workflow.Target.Workspace)
	}
	if workflow.Target.Direct == nil {
		t.Fatal("direct connection was not constructed")
	}
	if got := workflow.Target.Direct.Endpoint; got != "https://gateway.example.com" {
		t.Fatalf("endpoint=%q", got)
	}
	if got := workflow.Target.Direct.OIDC.Audience; got != "openshell-gateway" {
		t.Fatalf("audience=%q", got)
	}
}
