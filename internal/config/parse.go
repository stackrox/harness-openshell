package config

import (
	"bytes"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

const apiVersionV1alpha1 = "harness.openshell.dev/v1alpha1"

// Parse decodes a v1alpha1 Harness document from raw YAML bytes.
//
// It validates:
//   - apiVersion must equal "harness.openshell.dev/v1alpha1"; a missing or wrong
//     apiVersion is rejected with the supported version in the error
//   - unknown fields within a v1alpha1 document are errors (this rejects
//     spec.context, the dead terminology whose replacement is spec.target)
//   - kind must equal "Harness"
//   - metadata.name must be non-empty
func Parse(data []byte) (*Harness, error) {
	// Detect apiVersion with a lenient pass first so an unversioned document gets
	// the version error before strict unknown-field validation.
	var probe struct {
		APIVersion string `yaml:"apiVersion"`
	}
	if err := yaml.Unmarshal(data, &probe); err != nil {
		return nil, fmt.Errorf("parsing YAML: %w", err)
	}
	if probe.APIVersion != apiVersionV1alpha1 {
		return nil, fmt.Errorf("unsupported or missing apiVersion %q; expected %s", probe.APIVersion, apiVersionV1alpha1)
	}

	// Strict decode: unknown fields within a v1alpha1 document are errors.
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	var h Harness
	if err := dec.Decode(&h); err != nil {
		return nil, fmt.Errorf("parsing YAML: %w", err)
	}

	if h.Kind != "Harness" {
		return nil, fmt.Errorf("invalid kind %q; expected Harness", h.Kind)
	}
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
