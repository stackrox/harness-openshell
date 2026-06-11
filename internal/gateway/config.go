package gateway

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type GatewayConfig struct {
	Gateway   GatewaySection   `yaml:"gateway"`
	Providers ProvidersSection `yaml:"providers"`
	Chart     ChartSection     `yaml:"chart"`
	Helm      HelmSection      `yaml:"helm"`
	Addons    AddonsSection    `yaml:"addons"`
	OCP       OCPSection       `yaml:"ocp"`
	Secrets   SecretsSection   `yaml:"secrets"`

	Dir string `yaml:"-"`
}

type GatewaySection struct {
	Type     string `yaml:"type"`
	Platform string `yaml:"platform"`
	Service  string `yaml:"service"`
	Name     string `yaml:"name"`
	Mode     string `yaml:"mode"`
}

type ProvidersSection struct {
	Enabled []string `yaml:"enabled"`
	Custom  []string `yaml:"custom"`
}

type ChartSection struct {
	OCI     string    `yaml:"oci"`
	Version string    `yaml:"version"`
	CRD     CRDConfig `yaml:"crd"`
}

type CRDConfig struct {
	URL string `yaml:"url"`
}

type HelmSection struct {
	Values string `yaml:"values"`
}

type AddonsSection struct {
	Manifests []string `yaml:"manifests"`
}

type OCPSection struct {
	SCCPrivileged []string `yaml:"scc-privileged"`
	SCCAnyuid     []string `yaml:"scc-anyuid"`
}

type SecretsSection struct {
	Names []string `yaml:"names"`
	MTLS  string   `yaml:"mtls"`
}

func LoadConfig(dir string) (*GatewayConfig, error) {
	path := filepath.Join(dir, "gateway.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	var cfg GatewayConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
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
}

func (c *GatewayConfig) applyEnvOverrides() {
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

func (c *GatewayConfig) HasProviders() bool {
	return len(c.Providers.Enabled) > 0 || len(c.Providers.Custom) > 0
}

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
