package gateway

import (
	"os"
	"path/filepath"
	"testing"
)

func writeGatewayTOML(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "gateway.toml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadConfig_FullOCP(t *testing.T) {
	dir := t.TempDir()
	writeGatewayTOML(t, dir, `
[gateway]
type = "remote"
platform = "ocp"
service = "route"
name = "my-ocp"
mode = "launcher"

[providers]
enabled = ["github", "vertex-local"]
custom = ["gws"]

[chart]
oci = "oci://example.com/chart"
version = "1.2.3"
[chart.crd]
url = "https://example.com/crd.yaml"

[helm]
values = "values.yaml"

[addons]
manifests = ["addons/rbac.yaml", "addons/route.yaml"]

[images]
runner = "example.com/runner:v1"

[ocp]
scc-privileged = ["sa1", "sa2"]
scc-anyuid = ["sa1"]

[secrets]
names = ["secret-a", "secret-b"]
mtls = "my-mtls-secret"

[launcher]
service-account = "my-launcher-sa"
gateway-endpoint = "https://gw.cluster.local:9090"
`)

	cfg, err := LoadConfig(dir)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Gateway.Type != "remote" {
		t.Errorf("type = %q, want remote", cfg.Gateway.Type)
	}
	if cfg.Gateway.Platform != "ocp" {
		t.Errorf("platform = %q, want ocp", cfg.Gateway.Platform)
	}
	if cfg.Gateway.Service != "route" {
		t.Errorf("service = %q, want route", cfg.Gateway.Service)
	}
	if cfg.Gateway.Name != "my-ocp" {
		t.Errorf("name = %q, want my-ocp", cfg.Gateway.Name)
	}
	if cfg.Gateway.Mode != "launcher" {
		t.Errorf("mode = %q, want launcher", cfg.Gateway.Mode)
	}
	if len(cfg.Providers.Enabled) != 2 {
		t.Errorf("providers.enabled = %v, want 2 entries", cfg.Providers.Enabled)
	}
	if len(cfg.Providers.Custom) != 1 || cfg.Providers.Custom[0] != "gws" {
		t.Errorf("providers.custom = %v, want [gws]", cfg.Providers.Custom)
	}
	if cfg.Chart.OCI != "oci://example.com/chart" {
		t.Errorf("chart.oci = %q, want oci://example.com/chart", cfg.Chart.OCI)
	}
	if cfg.Chart.Version != "1.2.3" {
		t.Errorf("chart.version = %q, want 1.2.3", cfg.Chart.Version)
	}
	if cfg.Chart.CRD.URL != "https://example.com/crd.yaml" {
		t.Errorf("chart.crd.url = %q", cfg.Chart.CRD.URL)
	}
	if cfg.Images.Runner != "example.com/runner:v1" {
		t.Errorf("images.runner = %q", cfg.Images.Runner)
	}
	if len(cfg.OCP.SCCPrivileged) != 2 {
		t.Errorf("ocp.scc-privileged = %v, want 2 entries", cfg.OCP.SCCPrivileged)
	}
	if len(cfg.OCP.SCCAnyuid) != 1 {
		t.Errorf("ocp.scc-anyuid = %v, want 1 entry", cfg.OCP.SCCAnyuid)
	}
	if cfg.Secrets.MTLS != "my-mtls-secret" {
		t.Errorf("secrets.mtls = %q", cfg.Secrets.MTLS)
	}
	if cfg.Launcher.ServiceAccount != "my-launcher-sa" {
		t.Errorf("launcher.service-account = %q", cfg.Launcher.ServiceAccount)
	}
	if cfg.Launcher.GatewayEndpoint != "https://gw.cluster.local:9090" {
		t.Errorf("launcher.gateway-endpoint = %q", cfg.Launcher.GatewayEndpoint)
	}
	if cfg.Dir != dir {
		t.Errorf("Dir = %q, want %q", cfg.Dir, dir)
	}
}

func TestLoadConfig_MinimalLocal(t *testing.T) {
	dir := t.TempDir()
	writeGatewayTOML(t, dir, `
[gateway]
type = "local"
`)

	cfg, err := LoadConfig(dir)
	if err != nil {
		t.Fatal(err)
	}

	if !cfg.IsLocal() {
		t.Error("IsLocal() = false, want true")
	}
	if cfg.Gateway.Mode != "direct" {
		t.Errorf("default mode for local = %q, want direct", cfg.Gateway.Mode)
	}

	// Defaults applied
	if cfg.Chart.OCI != "oci://ghcr.io/nvidia/openshell/helm-chart" {
		t.Errorf("default chart.oci = %q", cfg.Chart.OCI)
	}
	if cfg.Secrets.MTLS != "openshell-client-tls" {
		t.Errorf("default secrets.mtls = %q", cfg.Secrets.MTLS)
	}
	if cfg.Launcher.ServiceAccount != "openshell-launcher" {
		t.Errorf("default launcher.service-account = %q", cfg.Launcher.ServiceAccount)
	}
}

func TestLoadConfig_MinimalRemote(t *testing.T) {
	dir := t.TempDir()
	writeGatewayTOML(t, dir, `
[gateway]
type = "remote"
platform = "k8s"
`)

	cfg, err := LoadConfig(dir)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Gateway.Mode != "launcher" {
		t.Errorf("default mode for remote = %q, want launcher", cfg.Gateway.Mode)
	}
	if cfg.IsLocal() {
		t.Error("IsLocal() = true for remote")
	}
	if cfg.IsOCP() {
		t.Error("IsOCP() = true for k8s")
	}
}

func TestLoadConfig_Missing(t *testing.T) {
	_, err := LoadConfig(t.TempDir())
	if err == nil {
		t.Error("expected error for missing gateway.toml")
	}
}

func TestLoadConfig_InvalidTOML(t *testing.T) {
	dir := t.TempDir()
	writeGatewayTOML(t, dir, `[gateway
broken toml`)

	_, err := LoadConfig(dir)
	if err == nil {
		t.Error("expected error for invalid TOML")
	}
}

func TestEnvOverrides(t *testing.T) {
	dir := t.TempDir()
	writeGatewayTOML(t, dir, `
[gateway]
type = "remote"
name = "original-name"

[images]
runner = "original-runner"
`)

	t.Setenv("RUNNER_IMAGE", "env-runner:v2")
	t.Setenv("GATEWAY_NAME", "env-gw-name")

	cfg, err := LoadConfig(dir)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Images.Runner != "env-runner:v2" {
		t.Errorf("RUNNER_IMAGE override: got %q", cfg.Images.Runner)
	}
	if cfg.Gateway.Name != "env-gw-name" {
		t.Errorf("GATEWAY_NAME override: got %q", cfg.Gateway.Name)
	}
}

func TestEnvOverrides_NotSet(t *testing.T) {
	dir := t.TempDir()
	writeGatewayTOML(t, dir, `
[gateway]
type = "remote"
name = "original-name"

`)

	// Ensure env vars are not set
	t.Setenv("RUNNER_IMAGE", "")
	t.Setenv("GATEWAY_NAME", "")

	cfg, err := LoadConfig(dir)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Gateway.Name != "original-name" {
		t.Errorf("expected original value, got %q", cfg.Gateway.Name)
	}
}

func TestHelmValuesPath(t *testing.T) {
	dir := t.TempDir()
	writeGatewayTOML(t, dir, `
[gateway]
type = "remote"

[helm]
values = "values.yaml"
`)

	cfg, err := LoadConfig(dir)
	if err != nil {
		t.Fatal(err)
	}

	want := filepath.Join(dir, "helm", "values.yaml")
	if got := cfg.HelmValuesPath(); got != want {
		t.Errorf("HelmValuesPath() = %q, want %q", got, want)
	}
}

func TestHelmValuesPath_Empty(t *testing.T) {
	dir := t.TempDir()
	writeGatewayTOML(t, dir, `
[gateway]
type = "remote"
`)

	cfg, err := LoadConfig(dir)
	if err != nil {
		t.Fatal(err)
	}

	if got := cfg.HelmValuesPath(); got != "" {
		t.Errorf("HelmValuesPath() = %q, want empty", got)
	}
}

func TestManifestPaths(t *testing.T) {
	dir := t.TempDir()
	writeGatewayTOML(t, dir, `
[gateway]
type = "remote"

[addons]
manifests = ["addons/rbac.yaml", "addons/route.yaml"]
`)

	cfg, err := LoadConfig(dir)
	if err != nil {
		t.Fatal(err)
	}

	paths := cfg.ManifestPaths()
	if len(paths) != 2 {
		t.Fatalf("ManifestPaths() returned %d paths, want 2", len(paths))
	}
	if paths[0] != filepath.Join(dir, "addons", "rbac.yaml") {
		t.Errorf("paths[0] = %q", paths[0])
	}
	if paths[1] != filepath.Join(dir, "addons", "route.yaml") {
		t.Errorf("paths[1] = %q", paths[1])
	}
}

func TestManifestPaths_Empty(t *testing.T) {
	dir := t.TempDir()
	writeGatewayTOML(t, dir, `
[gateway]
type = "remote"
`)

	cfg, err := LoadConfig(dir)
	if err != nil {
		t.Fatal(err)
	}

	if len(cfg.ManifestPaths()) != 0 {
		t.Errorf("ManifestPaths() should be empty, got %v", cfg.ManifestPaths())
	}
}

func TestPredicates(t *testing.T) {
	tests := []struct {
		name         string
		toml         string
		isLocal      bool
		isOCP        bool
		usesLauncher bool
	}{
		{
			name: "local",
			toml: "[gateway]\ntype = \"local\"",

			isLocal:      true,
			isOCP:        false,
			usesLauncher: false,
		},
		{
			name: "remote ocp launcher",
			toml: "[gateway]\ntype = \"remote\"\nplatform = \"ocp\"\nmode = \"launcher\"",

			isLocal:      false,
			isOCP:        true,
			usesLauncher: true,
		},
		{
			name: "remote k8s direct",
			toml: "[gateway]\ntype = \"remote\"\nplatform = \"k8s\"\nmode = \"direct\"",

			isLocal:      false,
			isOCP:        false,
			usesLauncher: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			writeGatewayTOML(t, dir, tt.toml)

			cfg, err := LoadConfig(dir)
			if err != nil {
				t.Fatal(err)
			}

			if cfg.IsLocal() != tt.isLocal {
				t.Errorf("IsLocal() = %v, want %v", cfg.IsLocal(), tt.isLocal)
			}
			if cfg.IsOCP() != tt.isOCP {
				t.Errorf("IsOCP() = %v, want %v", cfg.IsOCP(), tt.isOCP)
			}
			if cfg.UsesLauncher() != tt.usesLauncher {
				t.Errorf("UsesLauncher() = %v, want %v", cfg.UsesLauncher(), tt.usesLauncher)
			}
		})
	}
}

func TestHasProviders(t *testing.T) {
	dir := t.TempDir()
	writeGatewayTOML(t, dir, `
[gateway]
type = "local"

[providers]
enabled = ["github"]
custom = ["gws"]
`)

	cfg, err := LoadConfig(dir)
	if err != nil {
		t.Fatal(err)
	}

	if !cfg.HasProviders() {
		t.Error("HasProviders() = false, want true")
	}

	all := cfg.AllProviders()
	if len(all) != 2 {
		t.Errorf("AllProviders() = %v, want [github gws]", all)
	}
}

func TestHasProviders_Empty(t *testing.T) {
	dir := t.TempDir()
	writeGatewayTOML(t, dir, `
[gateway]
type = "local"
`)

	cfg, err := LoadConfig(dir)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.HasProviders() {
		t.Error("HasProviders() = true with no providers section")
	}
}
