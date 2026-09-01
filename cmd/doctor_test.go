package cmd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	fake "github.com/NVIDIA/OpenShell/sdk/go/openshell/v1/fake"
	"github.com/NVIDIA/OpenShell/sdk/go/openshell/v1/types"
	"github.com/stackrox/harness-openshell/internal/config"
	"github.com/stackrox/harness-openshell/internal/openshell"
	"github.com/stackrox/harness-openshell/internal/testutil"
)

func TestConfiguredProvidersIncludesSandboxOnlyWithoutDuplicates(t *testing.T) {
	cfg := &config.Harness{Spec: config.Spec{
		Providers: []config.Provider{{Name: "declared"}},
		Sandbox:   config.Sandbox{Providers: []string{"declared", "platform-owned", "platform-owned"}},
	}}
	got := configuredProviders(cfg)
	want := []configuredProvider{{Name: "declared"}, {Name: "platform-owned"}}
	if len(got) != len(want) {
		t.Fatalf("configured providers = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("configured provider %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestDoctorCmdTargetResolutionAndActiveDefault(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		env         string
		wantGateway string
	}{
		{name: "active default"},
		{name: "flag", args: []string{"--gateway", "from-flag"}, wantGateway: "from-flag"},
		{name: "env fallback", env: "from-env", wantGateway: "from-env"},
		{name: "flag wins", args: []string{"--gateway", "from-flag"}, env: "from-env", wantGateway: "from-flag"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(openshell.EnvGateway, tt.env)
			client, raw := testutil.NewFakeClient("default", fake.WithHealthResult(&types.HealthResult{Healthy: true}))
			raw.AddProvider("default", &types.Provider{Name: "google-vertex-ai"})
			var got openshell.Target
			factory := func(_ context.Context, target openshell.Target) (openshell.Client, error) {
				got = target
				return client, nil
			}
			command := NewDoctorCmd(testDefaultConfig, factory)
			command.SetArgs(append(tt.args, "--output", "json"))
			if err := command.Execute(); err != nil {
				t.Fatalf("doctor: %v", err)
			}
			if got.Gateway != tt.wantGateway {
				t.Errorf("factory gateway = %q, want %q", got.Gateway, tt.wantGateway)
			}
		})
	}
}

func TestDoctorCmdUsesCanonicalConfigTargetAndRegisteredProviders(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "harness.yaml")
	if err := os.WriteFile(path, []byte(`apiVersion: harness.openshell.dev/v1alpha1
kind: Harness
metadata:
  name: doctor-test
spec:
  target:
    gateway: from-config
    workspace: team
  providers:
    - name: github-team
      management: referenced
`), 0o600); err != nil {
		t.Fatal(err)
	}
	client, raw := testutil.NewFakeClient("team", fake.WithHealthResult(&types.HealthResult{Healthy: true}))
	raw.AddProvider("team", &types.Provider{Name: "github-team", Type: "github"})
	var got openshell.Target
	factory := func(_ context.Context, target openshell.Target) (openshell.Client, error) {
		got = target
		return client, nil
	}
	command := NewDoctorCmd(testDefaultConfig, factory)
	command.SetArgs([]string{"--file", path})
	if _, err := captureStdout(t, command.Execute); err != nil {
		t.Fatalf("doctor: %v", err)
	}
	if got.Gateway != "from-config" || got.Workspace != "team" {
		t.Errorf("factory target = %+v", got)
	}
}

func TestDoctorDirectTargetDoesNotNeedCLIOrLocalProviderCredentials(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "harness.yaml")
	if err := os.WriteFile(path, []byte(`apiVersion: harness.openshell.dev/v1alpha1
kind: Harness
metadata:
  name: direct
spec:
  target:
    registration:
      endpoint: https://gateway.example.com
      oidc:
        issuer: https://issuer.example.com
        clientId: user
        audience: gateway
  providers:
    - name: github
      management: referenced
`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", t.TempDir())
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv(openshell.EnvGateway, "")
	client, raw := testutil.NewFakeClient("default", fake.WithHealthResult(&types.HealthResult{Healthy: true}))
	raw.AddProvider("default", &types.Provider{Name: "github"})
	factory := func(_ context.Context, target openshell.Target) (openshell.Client, error) {
		if target.Direct == nil {
			t.Fatal("direct target not propagated")
		}
		return client, nil
	}
	command := NewDoctorCmd(testDefaultConfig, factory)
	command.SetArgs([]string{"--file", path})
	if _, err := captureStdout(t, command.Execute); err != nil {
		t.Fatalf("doctor: %v", err)
	}
}

func TestCheckOnlineSDKProviderRegistration(t *testing.T) {
	client, raw := testutil.NewFakeClient("default", fake.WithHealthResult(&types.HealthResult{Healthy: true}))
	raw.AddProvider("default", &types.Provider{Name: "present"})
	results := checkOnlineSDK(context.Background(), client, []string{"present", "missing"})
	if len(results) != 3 || results[0].Status != "pass" || results[1].Status != "pass" || results[2].Status != "fail" {
		t.Fatalf("results = %+v", results)
	}
	if !strings.Contains(results[2].Message, "platform bootstrap") {
		t.Errorf("missing provider message = %q", results[2].Message)
	}
}

func TestRunOnlineChecksAlwaysUsesFactory(t *testing.T) {
	client := testutil.NewFake("default", fake.WithHealthResult(&types.HealthResult{Healthy: true}))
	called := false
	factory := func(_ context.Context, target openshell.Target) (openshell.Client, error) {
		called = true
		if target != (openshell.Target{}) {
			t.Fatalf("target = %+v, want active default", target)
		}
		return client, nil
	}
	results := runOnlineChecks(context.Background(), factory, openshell.Target{}, nil)
	if !called || len(results) != 1 || results[0].Status != "pass" {
		t.Fatalf("called=%v results=%+v", called, results)
	}
}

func TestRunOnlineChecksFailsAuthenticationAndConfigurationErrors(t *testing.T) {
	for _, sentinel := range []error{openshell.ErrConfig, openshell.ErrUnauthenticated} {
		results := runOnlineChecks(context.Background(), func(context.Context, openshell.Target) (openshell.Client, error) {
			return nil, sentinel
		}, openshell.Target{}, nil)
		if len(results) != 1 || results[0].Status != "fail" {
			t.Errorf("error %v: results = %+v, want fail", sentinel, results)
		}
	}
}

func TestRunOnlineChecksGatewayIsolation(t *testing.T) {
	client := testutil.NewFake("default", fake.WithHealthResult(&types.HealthResult{Healthy: true}))
	var constructed []string
	factory := func(_ context.Context, target openshell.Target) (openshell.Client, error) {
		constructed = append(constructed, target.Gateway)
		return client, nil
	}
	runOnlineChecks(context.Background(), factory, openshell.Target{Gateway: "A"}, nil)
	if len(constructed) != 1 || constructed[0] != "A" {
		t.Fatalf("constructed = %v", constructed)
	}
}
