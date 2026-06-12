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
	ValuesPath   string         `yaml:"-"`
	ValuesInline map[string]any `yaml:"-"`
}

func (h *HelmSection) UnmarshalYAML(value *yaml.Node) error {
	var raw struct {
		Values yaml.Node `yaml:"values"`
	}
	if err := value.Decode(&raw); err != nil {
		return err
	}
	if raw.Values.Kind == 0 {
		return nil
	}
	switch raw.Values.Kind {
	case yaml.ScalarNode:
		h.ValuesPath = raw.Values.Value
	case yaml.MappingNode:
		var m map[string]any
		if err := raw.Values.Decode(&m); err != nil {
			return fmt.Errorf("decoding inline helm values: %w", err)
		}
		h.ValuesInline = m
	}
	return nil
}

type ManifestRef struct {
	Path   string         `yaml:"-"`
	Inline map[string]any `yaml:"-"`
}

type AddonsSection struct {
	Manifests []ManifestRef
}

func (a *AddonsSection) UnmarshalYAML(value *yaml.Node) error {
	var raw struct {
		Manifests []yaml.Node `yaml:"manifests"`
	}
	if err := value.Decode(&raw); err != nil {
		return err
	}
	for _, node := range raw.Manifests {
		var ref ManifestRef
		switch node.Kind {
		case yaml.ScalarNode:
			ref.Path = node.Value
		case yaml.MappingNode:
			var m map[string]any
			if err := node.Decode(&m); err != nil {
				return fmt.Errorf("decoding inline manifest: %w", err)
			}
			ref.Inline = m
		}
		a.Manifests = append(a.Manifests, ref)
	}
	return nil
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
	data, err := os.ReadFile(filepath.Join(dir, "gateway.yaml"))
	if err != nil {
		return nil, fmt.Errorf("reading gateway config: %w", err)
	}
	cfg, err := LoadConfigFromBytes(data)
	if err != nil {
		return nil, err
	}
	cfg.Dir = dir
	return cfg, nil
}

func LoadConfigFromBytes(data []byte) (*GatewayConfig, error) {
	var cfg GatewayConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing gateway config: %w", err)
	}
	cfg.applyDefaults()
	cfg.applyEnvOverrides()
	return &cfg, nil
}

func LoadProfile(harnessDir, name string) (*GatewayConfig, error) {
	path := filepath.Join(harnessDir, "profiles", "gateways", name+".yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return LoadConfigFromBytes(data)
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

func (c *GatewayConfig) HelmValuesFile(tmpDir string) (string, error) {
	if c.Helm.ValuesInline != nil {
		data, err := yaml.Marshal(c.Helm.ValuesInline)
		if err != nil {
			return "", fmt.Errorf("marshaling inline helm values: %w", err)
		}
		path := filepath.Join(tmpDir, "values.yaml")
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return "", err
		}
		return path, nil
	}
	if c.Helm.ValuesPath == "" {
		return "", nil
	}
	return filepath.Join(c.Dir, "helm", c.Helm.ValuesPath), nil
}

func (c *GatewayConfig) ManifestFilePaths() []string {
	var paths []string
	for _, m := range c.Addons.Manifests {
		if m.Path != "" {
			paths = append(paths, filepath.Join(c.Dir, m.Path))
		}
	}
	return paths
}

func (c *GatewayConfig) ManifestInline() []map[string]any {
	var manifests []map[string]any
	for _, m := range c.Addons.Manifests {
		if m.Inline != nil {
			manifests = append(manifests, m.Inline)
		}
	}
	return manifests
}
