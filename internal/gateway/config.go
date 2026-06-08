package gateway

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

type GatewayConfig struct {
	Gateway   GatewaySection   `toml:"gateway"`
	Providers ProvidersSection `toml:"providers"`
	Chart     ChartSection     `toml:"chart"`
	Helm      HelmSection      `toml:"helm"`
	Addons    AddonsSection    `toml:"addons"`
	Images    ImagesSection    `toml:"images"`
	OCP       OCPSection       `toml:"ocp"`
	Secrets   SecretsSection   `toml:"secrets"`
	Launcher  LauncherSection  `toml:"launcher"`

	// Dir is the directory containing the gateway.toml (set after parsing).
	// Used to resolve relative paths (helm values, addon manifests).
	Dir string `toml:"-"`
}

type GatewaySection struct {
	Type     string `toml:"type"`     // "local" or "remote"
	Platform string `toml:"platform"` // "ocp" or "k8s"
	Service  string `toml:"service"`  // "route", "nodeport", "loadbalancer"
	Name     string `toml:"name"`     // CLI gateway registration name
	Mode     string `toml:"mode"`     // "launcher" or "direct"
}

type ProvidersSection struct {
	Enabled []string `toml:"enabled"`
	Custom  []string `toml:"custom"`
}

type ChartSection struct {
	OCI     string    `toml:"oci"`
	Version string    `toml:"version"`
	CRD     CRDConfig `toml:"crd"`
}

type CRDConfig struct {
	URL string `toml:"url"`
}

type HelmSection struct {
	Values string `toml:"values"` // relative to <dir>/helm/
}

type AddonsSection struct {
	Manifests []string `toml:"manifests"` // relative to <dir>/
}

type ImagesSection struct {
	Runner string `toml:"runner"`
}

type OCPSection struct {
	SCCPrivileged []string `toml:"scc-privileged"`
	SCCAnyuid     []string `toml:"scc-anyuid"`
}

type SecretsSection struct {
	Names []string `toml:"names"`
	MTLS  string   `toml:"mtls"`
}

type LauncherSection struct {
	ServiceAccount  string `toml:"service-account"`
	GatewayEndpoint string `toml:"gateway-endpoint"`
}

func LoadConfig(dir string) (*GatewayConfig, error) {
	path := filepath.Join(dir, "gateway.toml")
	var cfg GatewayConfig
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	cfg.Dir = dir
	cfg.applyDefaults()
	cfg.applyEnvOverrides()
	return &cfg, nil
}

func (c *GatewayConfig) applyDefaults() {
	if c.Chart.OCI == "" {
		c.Chart.OCI = "oci://ghcr.io/nvidia/openshell/helm-chart"
	}
	if c.Chart.CRD.URL == "" {
		c.Chart.CRD.URL = "https://github.com/kubernetes-sigs/agent-sandbox/releases/latest/download/manifest.yaml"
	}
	if c.Images.Runner == "" {
		c.Images.Runner = "ghcr.io/robbycochran/harness-openshell:runner"
	}
	if c.Secrets.MTLS == "" {
		c.Secrets.MTLS = "openshell-client-tls"
	}
	if c.Launcher.ServiceAccount == "" {
		c.Launcher.ServiceAccount = "openshell-launcher"
	}
	if c.Launcher.GatewayEndpoint == "" {
		c.Launcher.GatewayEndpoint = "https://openshell.openshell.svc.cluster.local:8080"
	}
	if c.Gateway.Mode == "" {
		if c.Gateway.Type == "local" {
			c.Gateway.Mode = "direct"
		} else {
			c.Gateway.Mode = "launcher"
		}
	}
}

func (c *GatewayConfig) applyEnvOverrides() {
	if v := os.Getenv("RUNNER_IMAGE"); v != "" {
		c.Images.Runner = v
	}
	if v := os.Getenv("GATEWAY_NAME"); v != "" {
		c.Gateway.Name = v
	}
}

func (c *GatewayConfig) IsLocal() bool {
	return c.Gateway.Type == "local"
}

func (c *GatewayConfig) IsOCP() bool {
	return c.Gateway.Platform == "ocp"
}

func (c *GatewayConfig) UsesLauncher() bool {
	return c.Gateway.Mode == "launcher"
}

// HasProviders returns true if the gateway config specifies its own provider lists,
// overriding the global openshell.toml.
func (c *GatewayConfig) HasProviders() bool {
	return len(c.Providers.Enabled) > 0 || len(c.Providers.Custom) > 0
}

// AllProviders returns the combined enabled + custom provider names.
func (c *GatewayConfig) AllProviders() []string {
	all := make([]string, 0, len(c.Providers.Enabled)+len(c.Providers.Custom))
	all = append(all, c.Providers.Enabled...)
	all = append(all, c.Providers.Custom...)
	return all
}

func (c *GatewayConfig) HelmValuesPath() string {
	if c.Helm.Values == "" {
		return ""
	}
	return filepath.Join(c.Dir, "helm", c.Helm.Values)
}

func (c *GatewayConfig) ManifestPaths() []string {
	paths := make([]string, len(c.Addons.Manifests))
	for i, m := range c.Addons.Manifests {
		paths[i] = filepath.Join(c.Dir, m)
	}
	return paths
}
