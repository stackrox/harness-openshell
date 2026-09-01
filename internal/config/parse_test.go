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
		expectedNumProv int
		expectedNumPay  int
	}{
		{
			name:            "fact-dev full config",
			fixture:         "testdata/fact-dev.v1alpha1.yaml",
			expectedName:    "fact-dev",
			expectedGW:      "rc-dev",
			expectedWS:      "default",
			expectedNumProv: 2,
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

			if h.Metadata.Name != tc.expectedName {
				t.Errorf("metadata.name: got %q, want %q", h.Metadata.Name, tc.expectedName)
			}
			if h.Spec.Target.Gateway != tc.expectedGW {
				t.Errorf("target.gateway: got %q, want %q", h.Spec.Target.Gateway, tc.expectedGW)
			}
			if h.Spec.Target.Workspace != tc.expectedWS {
				t.Errorf("target.workspace: got %q, want %q", h.Spec.Target.Workspace, tc.expectedWS)
			}
			if len(h.Spec.Providers) != tc.expectedNumProv {
				t.Errorf("len(providers): got %d, want %d", len(h.Spec.Providers), tc.expectedNumProv)
			}
			if len(h.Spec.Payloads) != tc.expectedNumPay {
				t.Errorf("len(payloads): got %d, want %d", len(h.Spec.Payloads), tc.expectedNumPay)
			}
		})
	}
}

func TestRoundTrip(t *testing.T) {
	fixture := "testdata/fact-dev.v1alpha1.yaml"
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

func TestUnversionedConfigError(t *testing.T) {
	unversioned := `
name: test-agent
gateway: rc-dev
entrypoint: claude
repo: https://github.com/example/repo
providers:
  - profile: github
`
	_, err := Parse([]byte(unversioned))
	if err == nil {
		t.Fatal("expected error for unversioned config")
	}
	if !bytes.Contains([]byte(err.Error()), []byte("harness.openshell.dev/v1alpha1")) {
		t.Errorf("error should name the supported apiVersion, got: %v", err)
	}
}

func TestSpecContextRejected(t *testing.T) {
	// Config with spec.context (dead terminology)
	doc := `
apiVersion: harness.openshell.dev/v1alpha1
kind: Harness
metadata:
  name: test
spec:
  context:
    gateway: x
`
	_, err := Parse([]byte(doc))
	if err == nil {
		t.Fatal("expected error for spec.context")
	}
	if !bytes.Contains([]byte(err.Error()), []byte("context")) {
		t.Errorf("error should mention 'context', got: %v", err)
	}
}

func TestUnknownTopLevelKey(t *testing.T) {
	doc := `
apiVersion: harness.openshell.dev/v1alpha1
kind: Harness
metadata:
  name: test
spec:
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

func TestProvidersAndSandboxProviders(t *testing.T) {
	// Verify that spec.providers[] is []Provider and spec.sandbox.providers[] is []string
	data, err := os.ReadFile("testdata/fact-dev.v1alpha1.yaml")
	if err != nil {
		t.Fatalf("failed to read fixture: %v", err)
	}

	h, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	// Check spec.providers is typed as []Provider with Management field
	if len(h.Spec.Providers) < 1 {
		t.Fatal("expected at least one provider")
	}
	if h.Spec.Providers[0].Name == "" {
		t.Error("provider name should not be empty")
	}
	if h.Spec.Providers[0].Management == "" {
		t.Error("provider management field should not be empty")
	}

	// Check spec.sandbox.providers is typed as []string
	if len(h.Spec.Sandbox.Providers) < 1 {
		t.Fatal("expected at least one sandbox provider")
	}
	// Sandbox providers are just strings, not structs
	if h.Spec.Sandbox.Providers[0] == "" {
		t.Error("sandbox provider string should not be empty")
	}
}

func TestMissingAPIVersion(t *testing.T) {
	doc := `
kind: Harness
metadata:
  name: test
spec:
  target:
    gateway: x
`
	_, err := Parse([]byte(doc))
	if err == nil {
		t.Fatal("expected error for missing apiVersion")
	}
	if !bytes.Contains([]byte(err.Error()), []byte("harness.openshell.dev/v1alpha1")) {
		t.Errorf("error should name the supported apiVersion, got: %v", err)
	}
}

func TestWrongAPIVersion(t *testing.T) {
	doc := `
apiVersion: some-other/v1
kind: Harness
metadata:
  name: test
spec:
  target:
    gateway: x
`
	_, err := Parse([]byte(doc))
	if err == nil {
		t.Fatal("expected error for wrong apiVersion")
	}
	if !bytes.Contains([]byte(err.Error()), []byte("harness.openshell.dev/v1alpha1")) {
		t.Errorf("error should name the supported apiVersion, got: %v", err)
	}
}

func TestMissingMetadataName(t *testing.T) {
	doc := `
apiVersion: harness.openshell.dev/v1alpha1
kind: Harness
metadata: {}
spec:
  target:
    gateway: x
`
	_, err := Parse([]byte(doc))
	if err == nil {
		t.Fatal("expected error for missing metadata.name")
	}
	if !bytes.Contains([]byte(err.Error()), []byte("metadata.name")) {
		t.Errorf("error should mention 'metadata.name', got: %v", err)
	}
}

func TestWrongKind(t *testing.T) {
	doc := `
apiVersion: harness.openshell.dev/v1alpha1
kind: WrongKind
metadata:
  name: test
spec:
  target:
    gateway: x
`
	_, err := Parse([]byte(doc))
	if err == nil {
		t.Fatal("expected error for wrong kind")
	}
	if !bytes.Contains([]byte(err.Error()), []byte("kind")) {
		t.Errorf("error should mention 'kind', got: %v", err)
	}
}

func TestLoad(t *testing.T) {
	// Test Load function using the fixture file
	h, err := Load("testdata/fact-dev.v1alpha1.yaml")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if h.Metadata.Name != "fact-dev" {
		t.Errorf("metadata.name: got %q, want %q", h.Metadata.Name, "fact-dev")
	}
}

func TestLoadNonexistent(t *testing.T) {
	_, err := Load("testdata/nonexistent.yaml")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestSecretRefDescribe(t *testing.T) {
	testCases := []struct {
		source   string
		expected string
	}{
		{"gcloud-adc", "gcloud ADC"},
		{"environment:OPENSHELL_OIDC_CLIENT_SECRET", "environment OPENSHELL_OIDC_CLIENT_SECRET"},
		{"environment:MY_VAR", "environment MY_VAR"},
		{"unknown-source", "unknown-source"},
	}

	for _, tc := range testCases {
		t.Run(tc.source, func(t *testing.T) {
			ref := SecretRef{Source: tc.source}
			got := ref.Describe()
			if got != tc.expected {
				t.Errorf("Describe: got %q, want %q", got, tc.expected)
			}
		})
	}
}

func TestPayloadSourceAndDestination(t *testing.T) {
	fixture := "testdata/fact-dev.v1alpha1.yaml"
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
