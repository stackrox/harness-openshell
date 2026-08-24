package legacy

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/stackrox/harness-openshell/internal/agent"
	"github.com/stackrox/harness-openshell/internal/config"
	"gopkg.in/yaml.v3"
)

var update = flag.Bool("update", false, "regenerate golden files")

// TestMigrateGolden migrates every legacy fixture and compares the marshaled
// v1alpha1 output against its golden file. Run with -update to regenerate.
func TestMigrateGolden(t *testing.T) {
	fixtures, err := filepath.Glob(filepath.Join("testdata", "legacy", "*.yaml"))
	if err != nil {
		t.Fatalf("globbing fixtures: %v", err)
	}
	if len(fixtures) == 0 {
		t.Fatal("no legacy fixtures found")
	}

	for _, fixture := range fixtures {
		name := filepath.Base(fixture)
		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile(fixture)
			if err != nil {
				t.Fatalf("reading fixture: %v", err)
			}
			out, _, err := MigrateBytes(data)
			if err != nil {
				t.Fatalf("MigrateBytes: %v", err)
			}
			// Migrated output must re-parse as valid v1alpha1.
			if _, err := config.Parse(out); err != nil {
				t.Fatalf("migrated output does not re-parse: %v", err)
			}

			golden := filepath.Join("testdata", "golden", name[:len(name)-len(".yaml")]+".v1alpha1.yaml")
			if *update {
				if err := os.WriteFile(golden, out, 0o644); err != nil {
					t.Fatalf("writing golden: %v", err)
				}
				return
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("reading golden (run -update to create): %v", err)
			}
			if string(out) != string(want) {
				t.Errorf("migrated output differs from %s:\n got:\n%s\nwant:\n%s", golden, out, want)
			}
		})
	}
}

// TestMigrateBasic validates basic gateway, repo, and entrypoint mapping.
func TestMigrateBasic(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "legacy", "basic.yaml"))
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}

	legacy, err := agent.ParseHarness(data)
	if err != nil {
		t.Fatalf("parsing legacy: %v", err)
	}

	migrated, warnings, err := Migrate(legacy)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}

	if migrated.APIVersion != "harness.openshell.dev/v1alpha1" {
		t.Errorf("apiVersion: got %q, want harness.openshell.dev/v1alpha1", migrated.APIVersion)
	}
	if migrated.Kind != "Harness" {
		t.Errorf("kind: got %q, want Harness", migrated.Kind)
	}
	if migrated.Metadata.Name != "basic-test" {
		t.Errorf("name: got %q, want basic-test", migrated.Metadata.Name)
	}
	if migrated.Spec.Target.Gateway != "rc-dev" {
		t.Errorf("target.gateway: got %q, want rc-dev", migrated.Spec.Target.Gateway)
	}
	if migrated.Spec.Source.Repo != "https://github.com/example/repo" {
		t.Errorf("source.repo: got %q, want https://github.com/example/repo", migrated.Spec.Source.Repo)
	}
	if migrated.Spec.Source.Ref != "main" {
		t.Errorf("source.ref: got %q, want main", migrated.Spec.Source.Ref)
	}
	if migrated.Spec.Agent.Type != "claude" {
		t.Errorf("agent.type: got %q, want claude", migrated.Spec.Agent.Type)
	}
	if len(warnings) != 0 {
		t.Errorf("warnings: got %d, want 0", len(warnings))
	}

	// Round-trip: parse the migrated YAML
	out, err := yaml.Marshal(migrated)
	if err != nil {
		t.Fatalf("marshaling migrated: %v", err)
	}
	reparsed, err := config.Parse(out)
	if err != nil {
		t.Fatalf("reparsing migrated: %v", err)
	}
	if reparsed.Metadata.Name != migrated.Metadata.Name {
		t.Errorf("round-trip: name changed")
	}
}

// TestMigrateProviders validates provider conversion to referenced providers.
func TestMigrateProviders(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "legacy", "with-providers.yaml"))
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}

	legacy, err := agent.ParseHarness(data)
	if err != nil {
		t.Fatalf("parsing legacy: %v", err)
	}

	migrated, warnings, err := Migrate(legacy)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}

	if len(migrated.Spec.Providers) != 2 {
		t.Errorf("providers count: got %d, want 2", len(migrated.Spec.Providers))
	}

	if migrated.Spec.Providers[0].Name != "github-fact" {
		t.Errorf("providers[0].name: got %q, want github-fact", migrated.Spec.Providers[0].Name)
	}
	if migrated.Spec.Providers[0].Management != "referenced" {
		t.Errorf("providers[0].management: got %q, want referenced", migrated.Spec.Providers[0].Management)
	}

	if migrated.Spec.Providers[1].Name != "gcp-vertex" {
		t.Errorf("providers[1].name: got %q, want gcp-vertex", migrated.Spec.Providers[1].Name)
	}
	if migrated.Spec.Providers[1].Management != "referenced" {
		t.Errorf("providers[1].management: got %q, want referenced", migrated.Spec.Providers[1].Management)
	}

	// Check warnings
	if len(warnings) != 2 {
		t.Errorf("warnings count: got %d, want 2 (one per provider)", len(warnings))
	}
	for i, w := range warnings {
		if w.Field != "providers[].profile" {
			t.Errorf("warning[%d].field: got %q, want providers[].profile", i, w.Field)
		}
	}
}

// TestMigratePayloads validates payload field renaming: sandbox_path → destination, local_path → source.
func TestMigratePayloads(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "legacy", "with-payloads.yaml"))
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}

	legacy, err := agent.ParseHarness(data)
	if err != nil {
		t.Fatalf("parsing legacy: %v", err)
	}

	migrated, warnings, err := Migrate(legacy)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}

	if len(migrated.Spec.Payloads) != 2 {
		t.Errorf("payloads count: got %d, want 2", len(migrated.Spec.Payloads))
	}

	// First payload: content-based
	if migrated.Spec.Payloads[0].Destination != "/sandbox/config" {
		t.Errorf("payloads[0].destination: got %q, want /sandbox/config", migrated.Spec.Payloads[0].Destination)
	}
	if migrated.Spec.Payloads[0].Content == "" {
		t.Errorf("payloads[0].content: should be non-empty")
	}
	if migrated.Spec.Payloads[0].Source != "" {
		t.Errorf("payloads[0].source: got %q, want empty", migrated.Spec.Payloads[0].Source)
	}

	// Second payload: source-based
	if migrated.Spec.Payloads[1].Destination != "/sandbox/script.sh" {
		t.Errorf("payloads[1].destination: got %q, want /sandbox/script.sh", migrated.Spec.Payloads[1].Destination)
	}
	if migrated.Spec.Payloads[1].Source != "scripts/setup.sh" {
		t.Errorf("payloads[1].source: got %q, want scripts/setup.sh", migrated.Spec.Payloads[1].Source)
	}
	if migrated.Spec.Payloads[1].Content != "" {
		t.Errorf("payloads[1].content: got %q, want empty", migrated.Spec.Payloads[1].Content)
	}

	if len(warnings) != 0 {
		t.Errorf("warnings: got %d, want 0", len(warnings))
	}
}

// TestMigratePayloadRename explicitly pins sandbox_path → destination, local_path → source.
func TestMigratePayloadRename(t *testing.T) {
	legacy := &agent.Harness{
		Agent: &agent.AgentConfig{
			Name:       "rename-test",
			Gateway:    "rc-dev",
			Entrypoint: "claude",
		},
		Payloads: []agent.PayloadEntry{
			{
				SandboxPath: "/sandbox/dest",
				LocalPath:   "/local/src",
			},
			{
				SandboxPath: "/sandbox/config",
				Content:     "key: value",
			},
		},
	}

	migrated, _, err := Migrate(legacy)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}

	if len(migrated.Spec.Payloads) != 2 {
		t.Errorf("payloads count: got %d, want 2", len(migrated.Spec.Payloads))
	}

	// Check that SandboxPath → Destination
	if migrated.Spec.Payloads[0].Destination != "/sandbox/dest" {
		t.Errorf("payload[0].destination (was sandbox_path): got %q, want /sandbox/dest", migrated.Spec.Payloads[0].Destination)
	}

	// Check that LocalPath → Source
	if migrated.Spec.Payloads[0].Source != "/local/src" {
		t.Errorf("payload[0].source (was local_path): got %q, want /local/src", migrated.Spec.Payloads[0].Source)
	}

	// Check that Content is preserved
	if migrated.Spec.Payloads[1].Content != "key: value" {
		t.Errorf("payload[1].content: got %q, want key: value", migrated.Spec.Payloads[1].Content)
	}
}

// TestMigrateDeprecatedGateway checks that deprecated gateway profiles emit warnings.
func TestMigrateDeprecatedGateway(t *testing.T) {
	tests := []string{"helm", "openshift", "local-container"}
	for _, profile := range tests {
		legacy := &agent.Harness{
			Agent: &agent.AgentConfig{
				Name:       "deprecated-test",
				Gateway:    profile,
				Entrypoint: "claude",
			},
		}

		_, warnings, err := Migrate(legacy)
		if err != nil {
			t.Fatalf("migrate %q: %v", profile, err)
		}

		if len(warnings) == 0 {
			t.Errorf("gateway %q: expected deprecation warning, got none", profile)
		}

		found := false
		for _, w := range warnings {
			if w.Field == "gateway" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("gateway %q: no warning with field=gateway", profile)
		}
	}
}

// TestMigrateInlineDocsWarn checks that inline kind:gateway and kind:provider
// documents are flagged rather than silently dropped.
func TestMigrateInlineDocsWarn(t *testing.T) {
	legacy := &agent.Harness{
		Agent:     &agent.AgentConfig{Name: "inline-test", Entrypoint: "claude"},
		Gateways:  map[string][]byte{"gw-a": []byte("kind: gateway\n")},
		Providers: map[string][]byte{"prov-b": []byte("kind: provider\n")},
	}

	_, warnings, err := Migrate(legacy)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}

	fields := make(map[string]int)
	for _, w := range warnings {
		fields[w.Field]++
	}
	if fields["kind:gateway"] != 1 {
		t.Errorf("kind:gateway warnings: got %d, want 1", fields["kind:gateway"])
	}
	if fields["kind:provider"] != 1 {
		t.Errorf("kind:provider warnings: got %d, want 1", fields["kind:provider"])
	}
}

// TestMigrateBytes validates the full pipeline: parse → migrate → marshal.
func TestMigrateBytes(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "legacy", "basic.yaml"))
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}

	out, warnings, err := MigrateBytes(data)
	if err != nil {
		t.Fatalf("MigrateBytes: %v", err)
	}

	if len(out) == 0 {
		t.Errorf("output is empty")
	}

	// Validate that output can be parsed as v1alpha1
	migrated, err := config.Parse(out)
	if err != nil {
		t.Fatalf("reparsing output: %v", err)
	}

	if migrated.APIVersion != "harness.openshell.dev/v1alpha1" {
		t.Errorf("apiVersion: got %q", migrated.APIVersion)
	}
	if migrated.Kind != "Harness" {
		t.Errorf("kind: got %q", migrated.Kind)
	}
	if migrated.Metadata.Name != "basic-test" {
		t.Errorf("name: got %q", migrated.Metadata.Name)
	}

	if len(warnings) != 0 {
		t.Errorf("warnings: got %d, want 0", len(warnings))
	}
}

// TestMigrateBytesWithWarnings validates warning collection.
func TestMigrateBytesWithWarnings(t *testing.T) {
	legacyYAML := `
name: warn-test
gateway: helm
repo: https://github.com/example/repo
entrypoint: claude
providers:
  - profile: github
`

	out, warnings, err := MigrateBytes([]byte(legacyYAML))
	if err != nil {
		t.Fatalf("MigrateBytes: %v", err)
	}

	// Should have 2 warnings: 1 for deprecated gateway, 1 for provider profile
	if len(warnings) < 2 {
		t.Errorf("warnings count: got %d, want at least 2", len(warnings))
	}

	// Validate output still parses
	migrated, err := config.Parse(out)
	if err != nil {
		t.Fatalf("reparsing output with warnings: %v", err)
	}
	if migrated.Metadata.Name != "warn-test" {
		t.Errorf("name preserved: got %q", migrated.Metadata.Name)
	}
}

// TestMigrateNoAgent validates error on missing agent document.
func TestMigrateNoAgent(t *testing.T) {
	legacy := &agent.Harness{
		Agent: nil,
	}

	_, _, err := Migrate(legacy)
	if err == nil {
		t.Errorf("Migrate with nil Agent: expected error, got none")
	}
}

// TestMigrateDefaultEntrypoint validates that entrypoint defaults to "claude".
func TestMigrateDefaultEntrypoint(t *testing.T) {
	legacy := &agent.Harness{
		Agent: &agent.AgentConfig{
			Name:    "default-entrypoint-test",
			Gateway: "rc-dev",
			// Entrypoint is empty
		},
	}

	migrated, _, err := Migrate(legacy)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}

	if migrated.Spec.Agent.Type != "claude" {
		t.Errorf("agent.type: got %q, want claude (default)", migrated.Spec.Agent.Type)
	}
}
