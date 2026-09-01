package plan

import (
	"encoding/json"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/stackrox/harness-openshell/internal/config"
	"github.com/stackrox/harness-openshell/internal/openshell"
)

func TestTableSections_RepresentativePlan(t *testing.T) {
	desired := &config.Harness{
		Spec: config.Spec{
			Target: config.Target{
				Gateway:   "rc-dev",
				Workspace: "default",
			},
			Providers: []config.Provider{
				{
					Name:       "github",
					Type:       "github",
					Management: "managed",
				},
				{
					Name:       "gcp",
					Type:       "google-vertex-ai",
					Management: "managed",
				},
			},
			Inference: config.Inference{
				Provider: "gcp",
				Model:    "claude-haiku-4-5",
			},
			Sandbox: config.Sandbox{
				Image:     "quay.io/test:latest",
				Providers: []string{"github", "gcp"},
				Keep:      false,
			},
			Agent: config.Agent{
				Type: "claude",
				Args: []string{"--bare"},
			},
		},
	}

	current := CurrentState{
		Reachable: true,
		Health: openshell.Health{
			Healthy: true,
			Version: "0.0.110",
		},
		Providers: []openshell.Provider{
			{Name: "github", Type: "github"},
		},
		Inference: InferenceState{Capable: false},
	}

	plan := Build(desired, current)
	sections := plan.TableSections()

	// Should have: TARGET, PROVIDERS, INFERENCE, RUN
	if len(sections) != 4 {
		t.Errorf("expected 4 sections, got %d", len(sections))
	}

	expectedTitles := []string{"target", "providers", "inference", "run"}
	for i, title := range expectedTitles {
		if i >= len(sections) {
			t.Errorf("missing section %s", title)
			continue
		}
		if sections[i].Title != title {
			t.Errorf("section %d: expected %q, got %q", i, title, sections[i].Title)
		}
	}

	// Verify all sections have the right header
	for i, section := range sections {
		if len(section.Headers) != 3 {
			t.Errorf("section %d (%s): expected 3 headers, got %d", i, section.Title, len(section.Headers))
		}
		if section.Headers[0] != "ACTION" || section.Headers[1] != "NAME" || section.Headers[2] != "DETAIL" {
			t.Errorf("section %d (%s): unexpected headers %v", i, section.Title, section.Headers)
		}
	}

	// TARGET: 1 resource
	if len(sections[0].Rows) != 1 {
		t.Errorf("TARGET: expected 1 row, got %d", len(sections[0].Rows))
	}

	// PROVIDERS: 2 resources
	if len(sections[1].Rows) != 2 {
		t.Errorf("PROVIDERS: expected 2 rows, got %d", len(sections[1].Rows))
	}

	// INFERENCE: 1 resource
	if len(sections[2].Rows) != 1 {
		t.Errorf("INFERENCE: expected 1 row, got %d", len(sections[2].Rows))
	}

	// RUN: multiple resources (create-sandbox, uploads, execute, delete-sandbox)
	if len(sections[3].Rows) < 3 {
		t.Errorf("RUN: expected at least 3 rows, got %d", len(sections[3].Rows))
	}
}

func TestPlan_JSONMarshal(t *testing.T) {
	desired := &config.Harness{
		Spec: config.Spec{
			Target: config.Target{Gateway: "test-gateway"},
		},
	}
	current := CurrentState{
		Reachable: true,
		Health:    openshell.Health{Healthy: true, Version: "0.0.110"},
	}

	plan := Build(desired, current)

	// Marshal to JSON
	data, err := json.Marshal(plan)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	// Unmarshal back
	var recovered Plan
	err = json.Unmarshal(data, &recovered)
	if err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	// Verify round-trip
	if recovered.Target.Gateway != plan.Target.Gateway {
		t.Errorf("round-trip failed for target.gateway")
	}
	if len(recovered.Groups) != len(plan.Groups) {
		t.Errorf("round-trip failed: expected %d groups, got %d", len(plan.Groups), len(recovered.Groups))
	}
}

func TestPlan_YAMLMarshal(t *testing.T) {
	desired := &config.Harness{
		Spec: config.Spec{
			Target: config.Target{Gateway: "test-gateway"},
		},
	}
	current := CurrentState{
		Reachable: true,
		Health:    openshell.Health{Healthy: true, Version: "0.0.110"},
	}

	plan := Build(desired, current)

	// Marshal to YAML
	data, err := yaml.Marshal(plan)
	if err != nil {
		t.Fatalf("yaml.Marshal: %v", err)
	}

	// Unmarshal back
	var recovered Plan
	err = yaml.Unmarshal(data, &recovered)
	if err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}

	// Verify round-trip
	if recovered.Target.Gateway != plan.Target.Gateway {
		t.Errorf("round-trip failed for target.gateway")
	}
	if len(recovered.Groups) != len(plan.Groups) {
		t.Errorf("round-trip failed: expected %d groups, got %d", len(plan.Groups), len(recovered.Groups))
	}
}
