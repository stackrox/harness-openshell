package config

import (
	"bytes"
	"os"
	"reflect"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestParseValidFixture(t *testing.T) {
	testCases := []struct {
		name            string
		fixture         string
		expectedName    string
		expectedGW      string
		expectedWS      string
		expectedNumRefs int
		expectedNumPay  int
	}{
		{
			name:            "fact-dev full config",
			fixture:         "testdata/fact-dev.yaml",
			expectedName:    "fact-dev",
			expectedGW:      "rc-dev",
			expectedWS:      "default",
			expectedNumRefs: 2,
			expectedNumPay:  2,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			data, err := os.ReadFile(tc.fixture)
			if err != nil {
				t.Fatalf("failed to read fixture: %v", err)
			}

			h, err := Parse(data)
			if err != nil {
				t.Fatalf("Parse failed: %v", err)
			}

			if h.Version != formatVersion {
				t.Errorf("version: got %d, want %d", h.Version, formatVersion)
			}
			if h.Name != tc.expectedName {
				t.Errorf("name: got %q, want %q", h.Name, tc.expectedName)
			}
			if h.Spec.Target.Gateway != tc.expectedGW {
				t.Errorf("target.gateway: got %q, want %q", h.Spec.Target.Gateway, tc.expectedGW)
			}
			if h.Spec.Target.Workspace != tc.expectedWS {
				t.Errorf("target.workspace: got %q, want %q", h.Spec.Target.Workspace, tc.expectedWS)
			}
			if len(h.Spec.ProviderReferences()) != tc.expectedNumRefs {
				t.Errorf("len(provider references): got %d, want %d", len(h.Spec.ProviderReferences()), tc.expectedNumRefs)
			}
			if len(h.Spec.Payloads) != tc.expectedNumPay {
				t.Errorf("len(payloads): got %d, want %d", len(h.Spec.Payloads), tc.expectedNumPay)
			}
		})
	}
}

func TestRoundTrip(t *testing.T) {
	fixture := "testdata/fact-dev.yaml"
	data1, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatalf("failed to read fixture: %v", err)
	}

	h1, err := Parse(data1)
	if err != nil {
		t.Fatalf("first Parse failed: %v", err)
	}

	// Marshal back to YAML
	data2, err := yaml.Marshal(h1)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	// Parse the marshaled version
	h2, err := Parse(data2)
	if err != nil {
		t.Fatalf("second Parse failed: %v", err)
	}

	if !reflect.DeepEqual(h1, h2) {
		t.Errorf("round-trip mismatch:\n first: %+v\nsecond: %+v", h1, h2)
	}
}

func TestMissingVersionError(t *testing.T) {
	doc := `
name: test
target:
  gateway: rc-dev
`
	_, err := Parse([]byte(doc))
	if err == nil {
		t.Fatal("expected error for missing version")
	}
	if !bytes.Contains([]byte(err.Error()), []byte("version")) {
		t.Errorf("error should name the supported version, got: %v", err)
	}
}

func TestUnknownTopLevelContext(t *testing.T) {
	// Config with context (dead terminology)
	doc := `
version: 1
name: test
context:
  gateway: x
`
	_, err := Parse([]byte(doc))
	if err == nil {
		t.Fatal("expected error for context")
	}
	if !bytes.Contains([]byte(err.Error()), []byte("context")) {
		t.Errorf("error should mention 'context', got: %v", err)
	}
}

func TestUnknownTopLevelKey(t *testing.T) {
	doc := `
version: 1
name: test
target:
  gateway: x
unknown_key: value
`
	_, err := Parse([]byte(doc))
	if err == nil {
		t.Fatal("expected error for unknown top-level key")
	}
	if !bytes.Contains([]byte(err.Error()), []byte("unknown")) {
		t.Errorf("error should mention 'unknown', got: %v", err)
	}
}

func TestLegacyEnvelopeRejected(t *testing.T) {
	doc := `
apiVersion: harness.openshell.dev/v1alpha1
kind: OpenShellWorkflow
metadata:
  name: legacy
spec:
  target: {}
`
	_, err := Parse([]byte(doc))
	if err == nil {
		t.Fatal("expected legacy envelope to be rejected")
	}
	if !bytes.Contains([]byte(err.Error()), []byte("version")) {
		t.Errorf("error should identify the required version field: %v", err)
	}
}

func TestRemovedCredentialAndAutoProviderFieldsAreRejected(t *testing.T) {
	for name, field := range map[string]string{
		"top-level providers":        "providers:\n  - name: github\n",
		"registration autoProviders": "target:\n  registration:\n    autoProviders: true\n",
		"agent model":                "agent:\n  type: claude\n  model: claude-haiku\n",
	} {
		t.Run(name, func(t *testing.T) {
			data := "version: 1\nname: test\n" + field
			if _, err := Parse([]byte(data)); err == nil {
				t.Fatal("removed field was accepted")
			}
		})
	}
}

func TestProviderReferences(t *testing.T) {
	data, err := os.ReadFile("testdata/fact-dev.yaml")
	if err != nil {
		t.Fatalf("failed to read fixture: %v", err)
	}

	h, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	refs := h.Spec.ProviderReferences()
	if len(refs) != 2 || refs[0] != "my-gcp" || refs[1] != "github-fact" {
		t.Fatalf("provider references = %v, want [my-gcp github-fact]", refs)
	}
}

func TestUnsupportedVersion(t *testing.T) {
	doc := `
version: 2
name: test
target:
  gateway: x
`
	_, err := Parse([]byte(doc))
	if err == nil {
		t.Fatal("expected error for unsupported version")
	}
	if !bytes.Contains([]byte(err.Error()), []byte("version")) {
		t.Errorf("error should name the supported version, got: %v", err)
	}
}

func TestVersionMustBeNumeric(t *testing.T) {
	doc := `
version: nope
name: test
target:
  gateway: x
`
	_, err := Parse([]byte(doc))
	if err == nil {
		t.Fatal("expected error for non-numeric version")
	}
	if !bytes.Contains([]byte(err.Error()), []byte("version")) {
		t.Errorf("error should name the version, got: %v", err)
	}
}

func TestMissingName(t *testing.T) {
	doc := `
version: 1
target:
  gateway: x
`
	_, err := Parse([]byte(doc))
	if err == nil {
		t.Fatal("expected error for missing name")
	}
	if !bytes.Contains([]byte(err.Error()), []byte("name")) {
		t.Errorf("error should mention 'name', got: %v", err)
	}
}

func TestTrailingYAMLDocumentRejected(t *testing.T) {
	doc := `version: 1
name: first
---
version: 1
name: second
`
	if _, err := Parse([]byte(doc)); err == nil {
		t.Fatal("expected trailing YAML document to be rejected")
	}
}

func TestLoad(t *testing.T) {
	// Test Load function using the fixture file
	h, err := Load("testdata/fact-dev.yaml")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if h.Name != "fact-dev" {
		t.Errorf("name: got %q, want %q", h.Name, "fact-dev")
	}
}

func TestLoadNonexistent(t *testing.T) {
	_, err := Load("testdata/nonexistent.yaml")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestPayloadSourceAndDestination(t *testing.T) {
	fixture := "testdata/fact-dev.yaml"
	data, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatalf("failed to read fixture: %v", err)
	}

	h, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(h.Spec.Payloads) < 1 {
		t.Fatal("expected at least one payload")
	}

	p0 := h.Spec.Payloads[0]
	if p0.Source != ".agents/skills/fact" {
		t.Errorf("payload[0].source: got %q, want %q", p0.Source, ".agents/skills/fact")
	}
	if p0.Destination != "/sandbox/.agents/skills/fact" {
		t.Errorf("payload[0].destination: got %q, want %q", p0.Destination, "/sandbox/.agents/skills/fact")
	}

	// Second payload should have content and destination
	p1 := h.Spec.Payloads[1]
	if p1.Content == "" {
		t.Error("payload[1].content should not be empty")
	}
	if p1.Destination != "/sandbox/CLAUDE.md" {
		t.Errorf("payload[1].destination: got %q, want %q", p1.Destination, "/sandbox/CLAUDE.md")
	}
}
