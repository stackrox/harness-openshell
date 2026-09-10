package config

import (
	"bytes"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

const formatVersion = 1

// Parse decodes a version 1 workflow document from raw YAML bytes.
//
// It validates:
//   - version must equal 1
//   - unknown fields are errors
//   - name must be non-empty
func Parse(data []byte) (*Harness, error) {
	// Check the version before strict decoding so missing, non-numeric, or
	// unsupported versions produce a format error instead of an unrelated
	// unknown-field error.
	var probe struct {
		Version *int `yaml:"version"`
	}
	if err := yaml.Unmarshal(data, &probe); err != nil {
		return nil, fmt.Errorf("invalid version: %w", err)
	}
	if probe.Version == nil || *probe.Version != formatVersion {
		got := 0
		if probe.Version != nil {
			got = *probe.Version
		}
		return nil, fmt.Errorf("unsupported or missing version %d; expected %d", got, formatVersion)
	}

	// Strict decode: unknown fields are errors so the document remains an
	// executable format contract rather than silently accepting typos.
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	var h Harness
	if err := dec.Decode(&h); err != nil {
		return nil, fmt.Errorf("parsing YAML: %w", err)
	}

	if h.Name == "" {
		return nil, fmt.Errorf("name is required")
	}

	return &h, nil
}

// Load reads and parses a version 1 workflow document from a file path.
func Load(path string) (*Harness, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	return Parse(data)
}
