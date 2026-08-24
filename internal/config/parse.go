package config

import (
	"bytes"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Parse decodes a v1alpha1 Harness document from raw YAML bytes.
//
// It uses strict YAML decoding (unknown fields are errors) and validates:
//   - apiVersion must equal "harness.openshell.dev/v1alpha1" (or omitted with migration hint)
//   - kind must equal "Harness"
//   - metadata.name must be non-empty
//
// The parser rejects spec.context as dead terminology (the desired key is spec.target).
func Parse(data []byte) (*Harness, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)

	var h Harness
	if err := dec.Decode(&h); err != nil {
		return nil, fmt.Errorf("parsing YAML: %w", err)
	}

	// Validate apiVersion
	if h.APIVersion == "" {
		return nil, fmt.Errorf("unsupported or missing apiVersion %q; expected harness.openshell.dev/v1alpha1 (run 'harness migrate' to convert a legacy config)", h.APIVersion)
	}
	if h.APIVersion != "harness.openshell.dev/v1alpha1" {
		return nil, fmt.Errorf("unsupported apiVersion %q; expected harness.openshell.dev/v1alpha1 (run 'harness migrate' to convert a legacy config)", h.APIVersion)
	}

	// Validate kind
	if h.Kind != "Harness" {
		return nil, fmt.Errorf("invalid kind %q; expected Harness", h.Kind)
	}

	// Validate metadata.name
	if h.Metadata.Name == "" {
		return nil, fmt.Errorf("metadata.name is required")
	}

	return &h, nil
}

// Load reads and parses a v1alpha1 Harness document from a file path.
func Load(path string) (*Harness, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	return Parse(data)
}
