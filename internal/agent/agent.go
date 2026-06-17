package agent

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type ProviderRef struct {
	Profile string            `yaml:"profile"`
	Env     map[string]string `yaml:"env,omitempty"`
}

type PayloadEntry struct {
	SandboxPath string `yaml:"sandbox_path"`
	LocalPath   string `yaml:"local_path,omitempty"`
	Content     string `yaml:"content,omitempty"`
}

type AgentConfig struct {
	Name       string            `yaml:"name"`
	Gateway    string            `yaml:"gateway,omitempty"`
	Providers  []ProviderRef     `yaml:"providers"`
	Env        map[string]string `yaml:"env,omitempty"`
	Task       string            `yaml:"task,omitempty"`
	Entrypoint string            `yaml:"entrypoint,omitempty"`
	TTY        *bool             `yaml:"tty,omitempty"`
	Policy     string            `yaml:"policy,omitempty"`
	Image      string            `yaml:"image,omitempty"`
	Include    []string          `yaml:"include,omitempty"`
	Payloads   []PayloadEntry    `yaml:"payloads,omitempty"`
}

func (c *AgentConfig) NoTTY() bool {
	if c.TTY == nil {
		return false
	}
	return !*c.TTY
}

func (c *AgentConfig) EffectiveEntrypoint() string {
	if c.Entrypoint == "" {
		return "claude"
	}
	return c.Entrypoint
}

func (c *AgentConfig) ProviderNames() []string {
	names := make([]string, len(c.Providers))
	for i, p := range c.Providers {
		names[i] = p.Profile
	}
	return names
}

func ParseFile(path string) (*AgentConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading agent config: %w", err)
	}
	return Parse(data)
}

func Parse(data []byte) (*AgentConfig, error) {
	var cfg AgentConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing agent config: %w", err)
	}
	if cfg.Name == "" {
		return nil, fmt.Errorf("agent config: name is required")
	}
	for i, p := range cfg.Providers {
		if p.Profile == "" {
			return nil, fmt.Errorf("agent config: providers[%d].profile is required", i)
		}
	}
	return &cfg, nil
}

// Harness holds all documents parsed from a multi-document YAML file.
// A single-document agent YAML (no kind field) produces a Harness with
// just the Agent field populated.
type Harness struct {
	Agent     *AgentConfig
	Gateways  map[string][]byte // name -> raw gateway YAML
	Providers map[string][]byte // name -> raw provider profile YAML
	Payloads  []PayloadEntry    // files to upload to sandbox
	Policy    []byte            // raw policy YAML
}

// kindHeader peeks at the kind and name fields of a YAML document.
type kindHeader struct {
	Kind string `yaml:"kind"`
	Name string `yaml:"name"`
}

// payloadDoc holds a kind: payload document.
type payloadDoc struct {
	Kind        string `yaml:"kind"`
	SandboxPath string `yaml:"sandbox_path"`
	LocalPath   string `yaml:"local_path,omitempty"`
	Content     string `yaml:"content,omitempty"`
}

func ParseHarnessFile(path string) (*Harness, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading harness file: %w", err)
	}
	return ParseHarness(data)
}

func ParseHarness(data []byte) (*Harness, error) {
	h := &Harness{
		Gateways:  make(map[string][]byte),
		Providers: make(map[string][]byte),
	}

	dec := yaml.NewDecoder(bytes.NewReader(data))
	docIndex := 0
	for {
		var node yaml.Node
		err := dec.Decode(&node)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("parsing document %d: %w", docIndex, err)
		}

		raw, err := yaml.Marshal(&node)
		if err != nil {
			return nil, fmt.Errorf("re-marshaling document %d: %w", docIndex, err)
		}

		var header kindHeader
		if err := yaml.Unmarshal(raw, &header); err != nil {
			return nil, fmt.Errorf("reading kind from document %d: %w", docIndex, err)
		}

		switch header.Kind {
		case "", "agent":
			if h.Agent != nil {
				return nil, fmt.Errorf("multiple agent documents found")
			}
			cfg, err := Parse(raw)
			if err != nil {
				return nil, err
			}
			h.Agent = cfg

		case "provider":
			if header.Name == "" {
				return nil, fmt.Errorf("document %d: kind: provider requires a name field", docIndex)
			}
			h.Providers[header.Name] = raw

		case "gateway":
			if header.Name == "" {
				return nil, fmt.Errorf("document %d: kind: gateway requires a name field", docIndex)
			}
			h.Gateways[header.Name] = raw

		case "payload", "config":
			var doc payloadDoc
			if err := yaml.Unmarshal(raw, &doc); err != nil {
				return nil, fmt.Errorf("document %d: parsing payload: %w", docIndex, err)
			}
			if doc.SandboxPath == "" {
				return nil, fmt.Errorf("document %d: kind: payload requires a sandbox_path field", docIndex)
			}
			if doc.Content == "" && doc.LocalPath == "" {
				return nil, fmt.Errorf("document %d: kind: payload requires content or local_path field", docIndex)
			}
			if doc.Content != "" && doc.LocalPath != "" {
				return nil, fmt.Errorf("document %d: kind: payload cannot have both content and local_path", docIndex)
			}
			h.Payloads = append(h.Payloads, PayloadEntry{
				SandboxPath: doc.SandboxPath,
				Content:     doc.Content,
				LocalPath:   doc.LocalPath,
			})

		case "policy":
			if h.Policy != nil {
				return nil, fmt.Errorf("multiple policy documents found")
			}
			h.Policy = raw

		default:
			return nil, fmt.Errorf("document %d: unknown kind %q", docIndex, header.Kind)
		}
		docIndex++
	}

	if h.Agent == nil {
		return nil, fmt.Errorf("no agent document found")
	}
	// Merge agent-level payloads into harness payloads
	h.Payloads = append(h.Payloads, h.Agent.Payloads...)
	return h, nil
}

// RenderHarness writes a complete multi-document YAML from a Harness.
// builtinProviders are labeled with a comment; custom providers are included as-is.
func RenderHarness(h *Harness, builtinProviders map[string][]byte) ([]byte, error) {
	var buf bytes.Buffer

	agentData, err := yaml.Marshal(h.Agent)
	if err != nil {
		return nil, fmt.Errorf("marshaling agent: %w", err)
	}
	buf.WriteString("---\nkind: agent\n")
	buf.Write(agentData)

	for name, data := range h.Gateways {
		buf.WriteString("---\nkind: gateway\nname: " + name + "\n")
		buf.Write(data)
	}

	// Built-in providers (from OpenShell profiles)
	for name, data := range builtinProviders {
		if _, custom := h.Providers[name]; custom {
			continue // custom override takes precedence
		}
		buf.WriteString("---\n# built-in (from OpenShell provider profiles)\nkind: provider\nname: " + name + "\n")
		buf.Write(data)
	}

	// Custom providers (from harness config)
	for name, data := range h.Providers {
		buf.WriteString("---\n# custom\nkind: provider\nname: " + name + "\n")
		buf.Write(data)
	}

	for _, p := range h.Payloads {
		buf.WriteString("---\nkind: payload\nsandbox_path: " + p.SandboxPath + "\n")
		if p.LocalPath != "" {
			buf.WriteString("local_path: " + p.LocalPath + "\n")
		} else if p.Content != "" {
			buf.WriteString("content: |\n")
			for _, line := range strings.Split(p.Content, "\n") {
				buf.WriteString("  " + line + "\n")
			}
		}
	}

	if h.Policy != nil {
		buf.WriteString("---\nkind: policy\n")
		buf.Write(h.Policy)
	}

	return buf.Bytes(), nil
}

func expandEnvVar(key, value string) string {
	if value == "" {
		return os.Getenv(key)
	}
	return os.ExpandEnv(value)
}

func (c *AgentConfig) BuildEnvMap() map[string]string {
	env := make(map[string]string)
	for k, v := range c.Env {
		if val := expandEnvVar(k, v); val != "" {
			env[k] = val
		}
	}
	for _, p := range c.Providers {
		for k, v := range p.Env {
			if val := expandEnvVar(k, v); val != "" {
				env[k] = val
			}
		}
	}
	return env
}

func (c *AgentConfig) BuildRunSh() string {
	var b strings.Builder
	b.WriteString("#!/usr/bin/env bash\nset -euo pipefail\n\n")
	b.WriteString("PAYLOAD_DIR=\"$(cd \"$(dirname \"$0\")\" && pwd)\"\n\n")
	b.WriteString("# Prepend payload bin to PATH\n")
	b.WriteString("export PATH=\"$PAYLOAD_DIR/bin:$PATH\"\n\n")
	b.WriteString("# Validate entrypoint\n")
	entrypoint := c.EffectiveEntrypoint()
	epBin := strings.Fields(entrypoint)[0]
	fmt.Fprintf(&b, "if ! command -v %q >/dev/null 2>&1; then\n", epBin)
	fmt.Fprintf(&b, "  echo \"ERROR: entrypoint %q not found in PATH\" >&2\n", epBin)
	b.WriteString("  exit 1\n")
	b.WriteString("fi\n\n")
	b.WriteString("# Execute entrypoint\n")
	if c.Task != "" {
		b.WriteString("TASK=\"$(cat \"$PAYLOAD_DIR/task.md\")\"\n")
		if c.NoTTY() {
			// Headless: use --print (claude) or run (opencode) for stdout output
			switch epBin {
			case "opencode":
				fmt.Fprintf(&b, "exec %s run \"$TASK\"\n", entrypoint)
			default:
				fmt.Fprintf(&b, "exec %s --print \"$TASK\"\n", entrypoint)
			}
		} else {
			fmt.Fprintf(&b, "exec %s -p \"$TASK\"\n", entrypoint)
		}
	} else {
		fmt.Fprintf(&b, "exec %s\n", entrypoint)
	}
	return b.String()
}

func RenderPayload(cfg *AgentConfig, baseDir, destDir string) error {
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("creating payload dir: %w", err)
	}
	binDir := filepath.Join(destDir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return fmt.Errorf("creating bin dir: %w", err)
	}

	runSh := cfg.BuildRunSh()
	if err := os.WriteFile(filepath.Join(destDir, "run.sh"), []byte(runSh), 0o755); err != nil {
		return fmt.Errorf("writing run.sh: %w", err)
	}

	if cfg.Task != "" {
		taskSrc := cfg.Task
		if !filepath.IsAbs(taskSrc) {
			taskSrc = filepath.Join(baseDir, taskSrc)
		}
		data, err := os.ReadFile(taskSrc)
		if err != nil {
			return fmt.Errorf("reading task file %s: %w", cfg.Task, err)
		}
		expanded := os.ExpandEnv(string(data))
		if err := os.WriteFile(filepath.Join(destDir, "task.md"), []byte(expanded), 0o644); err != nil {
			return fmt.Errorf("writing task.md: %w", err)
		}
	}

	for _, inc := range cfg.Include {
		// Absolute includes are allowed as-is (the user authors the config);
		// relative includes must stay within the base directory.
		src := inc
		if !filepath.IsAbs(src) {
			src = filepath.Join(baseDir, src)
			rel, err := filepath.Rel(filepath.Clean(baseDir), filepath.Clean(src))
			if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				return fmt.Errorf("include path %q escapes base directory", inc)
			}
		}
		clean := filepath.Clean(src)
		data, err := os.ReadFile(clean)
		if err != nil {
			return fmt.Errorf("reading include %s: %w", inc, err)
		}
		dst := filepath.Join(destDir, filepath.Base(inc))
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			return fmt.Errorf("writing include %s: %w", filepath.Base(inc), err)
		}
	}

	return nil
}

// ResolvePayloads resolves payload entries into source/destination pairs for upload.
// Content payloads are written to temp files. File payloads are resolved relative to baseDir.
// ResolvedUpload is a source/destination pair for sandbox file upload.
type ResolvedUpload struct {
	Src string
	Dst string
}

func ResolvePayloads(payloads []PayloadEntry, baseDir, tmpDir string) ([]ResolvedUpload, error) {
	var uploads []ResolvedUpload
	for _, p := range payloads {
		if !strings.HasPrefix(p.SandboxPath, "/sandbox/") && !strings.HasPrefix(p.SandboxPath, "/etc/openshell/") {
			return nil, fmt.Errorf("payload sandbox_path %q must start with /sandbox/ or /etc/openshell/", p.SandboxPath)
		}
		var src string
		if p.Content != "" {
			f, err := os.CreateTemp(tmpDir, "payload-*")
			if err != nil {
				return nil, fmt.Errorf("creating temp file for payload %s: %w", p.SandboxPath, err)
			}
			if _, err := f.WriteString(p.Content); err != nil {
				f.Close()
				return nil, fmt.Errorf("writing payload %s: %w", p.SandboxPath, err)
			}
			f.Close()
			src = f.Name()
		} else if p.LocalPath != "" {
			resolved := p.LocalPath
			if !filepath.IsAbs(resolved) {
				resolved = filepath.Join(baseDir, resolved)
			}
			clean := filepath.Clean(resolved)
			rel, err := filepath.Rel(filepath.Clean(baseDir), clean)
			if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				return nil, fmt.Errorf("payload local_path %q escapes base directory", p.LocalPath)
			}
			if _, err := os.Stat(clean); err != nil {
				return nil, fmt.Errorf("payload local_path %s: %w", p.LocalPath, err)
			}
			src = clean
		}
		uploads = append(uploads, ResolvedUpload{Src: src, Dst: p.SandboxPath})
	}
	return uploads, nil
}
