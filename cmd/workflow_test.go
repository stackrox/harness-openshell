package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadWorkflowBuildsDirectTargetAndDefaultsWorkspace(t *testing.T) {
	t.Setenv("OPENSHELL_GATEWAY", "")
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

	workflow, err := loadWorkflow(path, "", "", applyOverrides{})
	if err != nil {
		t.Fatalf("loadWorkflow: %v", err)
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

func TestLoadWorkflowExternalGatewayOverridesDirectRegistration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "workflow.yaml")
	data := []byte(`apiVersion: harness.openshell.dev/v1alpha1
kind: Harness
metadata:
  name: direct
spec:
  target:
    registration:
      endpoint: https://gateway.example.com
      oidc:
        issuer: https://issuer.example.com
        clientId: ci-user
        audience: openshell-gateway
`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write workflow: %v", err)
	}

	for _, tc := range []struct {
		name, flag, environment, want string
	}{
		{name: "flag", flag: "flag-gateway", want: "flag-gateway"},
		{name: "environment", environment: "environment-gateway", want: "environment-gateway"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("OPENSHELL_GATEWAY", tc.environment)
			workflow, err := loadWorkflow(path, tc.flag, "", applyOverrides{})
			if err != nil {
				t.Fatalf("loadWorkflow: %v", err)
			}
			if workflow.Target.Gateway != tc.want || workflow.Target.Direct != nil {
				t.Fatalf("target = %+v, want named gateway %q without direct registration", workflow.Target, tc.want)
			}
		})
	}
}
