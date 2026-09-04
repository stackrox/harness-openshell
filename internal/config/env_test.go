package config

import (
	"strings"
	"testing"
)

func TestExpandSetVar(t *testing.T) {
	// ${VAR} set → value substituted
	getenv := func(name string) string {
		if name == "TEST_VAR" {
			return "test_value"
		}
		return ""
	}

	result, err := expandStrict("prefix${TEST_VAR}suffix", getenv)
	if err != nil {
		t.Fatalf("Expand failed: %v", err)
	}
	if result != "prefixtest_valuesuffix" {
		t.Errorf("got %q, want %q", result, "prefixtest_valuesuffix")
	}
}

func TestExpandUnsetVar(t *testing.T) {
	// ${VAR} unset → error naming the var
	getenv := func(name string) string {
		return ""
	}

	_, err := expandStrict("${UNSET_VAR}", getenv)
	if err == nil {
		t.Fatal("expected error for unset variable")
	}
	if !strings.Contains(err.Error(), "UNSET_VAR") {
		t.Errorf("error should mention UNSET_VAR, got: %v", err)
	}
}

func TestExpandExplicitEmpty(t *testing.T) {
	// Literal empty string (no variable reference) → ok
	result, err := expandStrict("", func(name string) string {
		return ""
	})
	if err != nil {
		t.Fatalf("Expand empty string failed: %v", err)
	}
	if result != "" {
		t.Errorf("got %q, want %q", result, "")
	}
}

func TestExpandDoubleDollar(t *testing.T) {
	// Literal $$ → left as $$
	getenv := func(name string) string {
		return ""
	}

	result, err := expandStrict("test$$var", getenv)
	if err != nil {
		t.Fatalf("Expand failed: %v", err)
	}
	if result != "test$$var" {
		t.Errorf("got %q, want %q", result, "test$$var")
	}
}

func TestExpandBareDoller(t *testing.T) {
	// Bare $ not followed by { → left unchanged
	getenv := func(name string) string {
		return ""
	}

	result, err := expandStrict("price$10", getenv)
	if err != nil {
		t.Fatalf("Expand failed: %v", err)
	}
	if result != "price$10" {
		t.Errorf("got %q, want %q", result, "price$10")
	}
}

func TestExpandMultipleMissing(t *testing.T) {
	// Multiple missing vars → one aggregated error naming all
	getenv := func(name string) string {
		return ""
	}

	_, err := expandStrict("${VAR1} and ${VAR2} and ${VAR3}", getenv)
	if err == nil {
		t.Fatal("expected error for multiple unset variables")
	}
	errMsg := err.Error()
	if !strings.Contains(errMsg, "VAR1") || !strings.Contains(errMsg, "VAR2") || !strings.Contains(errMsg, "VAR3") {
		t.Errorf("error should mention all vars, got: %v", err)
	}
}

func TestResolveEmptyString(t *testing.T) {
	// Harness with empty field → stays empty
	h := &Harness{
		APIVersion: "harness.openshell.dev/v1alpha1",
		Kind:       "Harness",
		Metadata:   Metadata{Name: "test"},
		Spec: Spec{
			Target: Target{
				Gateway:   "",
				Workspace: "",
			},
		},
	}

	resolved, err := Resolve(h, func(name string) string {
		return ""
	})
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	if resolved.Spec.Target.Gateway != "" || resolved.Spec.Target.Workspace != "" {
		t.Errorf("empty fields should stay empty")
	}
}

func TestResolveInvalidTimeout(t *testing.T) {
	h := &Harness{
		APIVersion: "harness.openshell.dev/v1alpha1",
		Kind:       "Harness",
		Metadata:   Metadata{Name: "test"},
		Spec:       Spec{Inference: Inference{Timeout: "60"}}, // bare integer, no unit
	}

	if _, err := Resolve(h, func(string) string { return "" }); err == nil {
		t.Fatal("expected Resolve to reject a unitless inference timeout")
	}
}

func TestResolveValidTimeout(t *testing.T) {
	h := &Harness{
		APIVersion: "harness.openshell.dev/v1alpha1",
		Kind:       "Harness",
		Metadata:   Metadata{Name: "test"},
		// Provider+model are required whenever the inference block is configured;
		// this test only exercises timeout expansion, so supply them as fixtures.
		Spec: Spec{Inference: Inference{Provider: "gcp", Model: "claude-opus-4-8", Timeout: "${INF_TIMEOUT}"}},
	}

	resolved, err := Resolve(h, func(name string) string {
		if name == "INF_TIMEOUT" {
			return "90s"
		}
		return ""
	})
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	secs, err := resolved.Spec.Inference.TimeoutSecs()
	if err != nil || secs != 90 {
		t.Errorf("resolved+parsed timeout = %d (err %v), want 90", secs, err)
	}
}

func TestResolve_RejectsBadManagement(t *testing.T) {
	h := &Harness{
		APIVersion: "harness.openshell.dev/v1alpha1",
		Kind:       "Harness",
		Metadata:   Metadata{Name: "test"},
		Spec: Spec{Providers: []Provider{
			{Name: "gh", Type: "github", Management: "bogus"},
		}},
	}

	_, err := Resolve(h, func(string) string { return "" })
	if err == nil {
		t.Fatal("expected Resolve to reject an invalid management value")
	}
	if !strings.Contains(err.Error(), "management") || !strings.Contains(err.Error(), "bogus") {
		t.Errorf("error should name the field and bad value: %v", err)
	}
}

func TestResolve_DefaultsEmptyManagementToReferenced(t *testing.T) {
	h := &Harness{
		APIVersion: "harness.openshell.dev/v1alpha1",
		Kind:       "Harness",
		Metadata:   Metadata{Name: "test"},
		Spec: Spec{Providers: []Provider{
			{Name: "gh", Type: "github"}, // no management
		}},
	}

	resolved, err := Resolve(h, func(string) string { return "" })
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if got := resolved.Spec.Providers[0].Management; got != "referenced" {
		t.Errorf("empty management should default to referenced, got %q", got)
	}
}

func TestResolve_RejectsDestinationTraversal(t *testing.T) {
	h := &Harness{
		APIVersion: "harness.openshell.dev/v1alpha1",
		Kind:       "Harness",
		Metadata:   Metadata{Name: "test"},
		Spec: Spec{
			Source:   Source{Repo: "https://example.com/x.git", Destination: "../escape"},
			Payloads: []Payload{{Content: "x", Destination: "/sandbox/../../etc/passwd"}},
		},
	}
	_, err := Resolve(h, func(string) string { return "" })
	if err == nil {
		t.Fatal("expected Resolve to reject destinations containing \"..\"")
	}
	if !strings.Contains(err.Error(), "spec.source.destination") || !strings.Contains(err.Error(), "spec.payloads[0].destination") {
		t.Errorf("error should name both offending fields: %v", err)
	}
}

func TestResolve_AllowsAbsoluteDestination(t *testing.T) {
	// Sandbox destinations are conventionally absolute; only ".." is rejected.
	h := &Harness{
		APIVersion: "harness.openshell.dev/v1alpha1",
		Kind:       "Harness",
		Metadata:   Metadata{Name: "test"},
		Spec: Spec{
			Source:   Source{Repo: "https://example.com/x.git", Destination: "/sandbox/src"},
			Payloads: []Payload{{Content: "x", Destination: "/sandbox/review.md"}},
		},
	}
	if _, err := Resolve(h, func(string) string { return "" }); err != nil {
		t.Errorf("Resolve rejected valid absolute destinations: %v", err)
	}
}

func TestResolve_RejectsDuplicateProviderNames(t *testing.T) {
	h := &Harness{
		APIVersion: "harness.openshell.dev/v1alpha1",
		Kind:       "Harness",
		Metadata:   Metadata{Name: "test"},
		Spec: Spec{Providers: []Provider{
			{Name: "github", Management: "referenced"},
			{Name: "github", Management: "referenced"},
		}},
	}

	_, err := Resolve(h, func(string) string { return "" })
	if err == nil || !strings.Contains(err.Error(), "duplicate provider") {
		t.Fatalf("error = %v, want duplicate provider", err)
	}
}

func TestResolve_RejectsMalformedRoute(t *testing.T) {
	h := &Harness{
		APIVersion: "harness.openshell.dev/v1alpha1",
		Kind:       "Harness",
		Metadata:   Metadata{Name: "test"},
		// Provider+model supplied so only the route-format error can fire.
		Spec: Spec{Inference: Inference{Provider: "gcp", Model: "claude-opus-4-8", Route: "bad route"}},
	}

	_, err := Resolve(h, func(string) string { return "" })
	if err == nil {
		t.Fatal("expected Resolve to reject a malformed route name")
	}
	if !strings.Contains(err.Error(), "route") {
		t.Errorf("error should name the route field: %v", err)
	}
}

func TestResolve_AcceptsDottedRoute(t *testing.T) {
	h := &Harness{
		APIVersion: "harness.openshell.dev/v1alpha1",
		Kind:       "Harness",
		Metadata:   Metadata{Name: "test"},
		Spec:       Spec{Inference: Inference{Provider: "gcp", Model: "claude-opus-4-8", Route: "inference.local"}},
	}

	if _, err := Resolve(h, func(string) string { return "" }); err != nil {
		t.Fatalf("Resolve rejected a valid dotted route: %v", err)
	}
}

func TestResolveVerifyRoundTrips(t *testing.T) {
	// verify:false must survive YAML parse + Resolve as an explicit false, not
	// collapse to the nil→true default, and must not alias the input pointer.
	src := `apiVersion: harness.openshell.dev/v1alpha1
kind: Harness
metadata:
  name: test
spec:
  inference:
    provider: gcp
    model: claude-opus-4-8
    verify: false
`
	h, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	resolved, err := Resolve(h, func(string) string { return "" })
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if resolved.Spec.Inference.VerifyEnabled() {
		t.Error("verify:false should resolve to VerifyEnabled()==false")
	}
	if resolved.Spec.Inference.Verify == h.Spec.Inference.Verify {
		t.Error("resolved Verify aliases the input's *bool pointer")
	}
}

func TestResolveNonSecretField(t *testing.T) {
	// Build Harness with ${SECRET_ISH} in non-secret field
	h := &Harness{
		APIVersion: "harness.openshell.dev/v1alpha1",
		Kind:       "Harness",
		Metadata:   Metadata{Name: "test"},
		Spec: Spec{
			Target: Target{
				Gateway: "${GATEWAY_VAR}",
			},
		},
	}

	// getenv returns real-looking secret value
	secretValue := "secret-gateway-token-12345"
	getenv := func(name string) string {
		if name == "GATEWAY_VAR" {
			return secretValue
		}
		return ""
	}

	resolved, err := Resolve(h, getenv)
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	// Assert resolved struct DOES contain the substituted value
	if resolved.Spec.Target.Gateway != secretValue {
		t.Errorf("non-secret field should be resolved: got %q, want %q", resolved.Spec.Target.Gateway, secretValue)
	}

	// Original should be unchanged
	if h.Spec.Target.Gateway != "${GATEWAY_VAR}" {
		t.Errorf("original should be unchanged, got %q", h.Spec.Target.Gateway)
	}
}

func TestResolveProviderConfig(t *testing.T) {
	// Test resolving provider config map
	h := &Harness{
		APIVersion: "harness.openshell.dev/v1alpha1",
		Kind:       "Harness",
		Metadata:   Metadata{Name: "test"},
		Spec: Spec{
			Providers: []Provider{
				{
					Name:       "vertex",
					Type:       "vertex",
					Management: "managed",
					Config: map[string]string{
						"PROJECT_ID": "${VERTEX_PROJECT_ID}",
						"LOCATION":   "us-central1",
					},
				},
			},
		},
	}

	getenv := func(name string) string {
		if name == "VERTEX_PROJECT_ID" {
			return "my-gcp-project"
		}
		return ""
	}

	resolved, err := Resolve(h, getenv)
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	config := resolved.Spec.Providers[0].Config
	if config["PROJECT_ID"] != "my-gcp-project" {
		t.Errorf("PROJECT_ID should be resolved: got %q, want %q", config["PROJECT_ID"], "my-gcp-project")
	}
	if config["LOCATION"] != "us-central1" {
		t.Errorf("LOCATION should stay as-is: got %q, want %q", config["LOCATION"], "us-central1")
	}
}

func TestResolveMultipleMissingVars(t *testing.T) {
	// Test that Resolve aggregates all missing vars into one error
	h := &Harness{
		APIVersion: "harness.openshell.dev/v1alpha1",
		Kind:       "Harness",
		Metadata:   Metadata{Name: "test"},
		Spec: Spec{
			Target: Target{
				Gateway:   "${MISSING_GATEWAY}",
				Workspace: "${MISSING_WORKSPACE}",
			},
		},
	}

	_, err := Resolve(h, func(name string) string {
		return ""
	})
	if err == nil {
		t.Fatal("expected error for missing variables")
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "MISSING_GATEWAY") {
		t.Errorf("error should mention MISSING_GATEWAY, got: %v", err)
	}
	if !strings.Contains(errMsg, "MISSING_WORKSPACE") {
		t.Errorf("error should mention MISSING_WORKSPACE, got: %v", err)
	}
}

func TestResolveSandboxEnv(t *testing.T) {
	// Test resolving sandbox env map
	h := &Harness{
		APIVersion: "harness.openshell.dev/v1alpha1",
		Kind:       "Harness",
		Metadata:   Metadata{Name: "test"},
		Spec: Spec{
			Sandbox: Sandbox{
				Env: map[string]string{
					"API_KEY": "${SANDBOX_API_KEY}",
					"DEBUG":   "true",
				},
			},
		},
	}

	getenv := func(name string) string {
		if name == "SANDBOX_API_KEY" {
			return "key-12345"
		}
		return ""
	}

	resolved, err := Resolve(h, getenv)
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	env := resolved.Spec.Sandbox.Env
	if env["API_KEY"] != "key-12345" {
		t.Errorf("API_KEY should be resolved: got %q, want %q", env["API_KEY"], "key-12345")
	}
	if env["DEBUG"] != "true" {
		t.Errorf("DEBUG should stay as-is: got %q, want %q", env["DEBUG"], "true")
	}
}

func TestResolveSandboxPolicyFile(t *testing.T) {
	h := &Harness{
		APIVersion: "harness.openshell.dev/v1alpha1",
		Kind:       "Harness",
		Metadata:   Metadata{Name: "test"},
		Spec: Spec{
			Sandbox: Sandbox{
				Policy: &PolicyRef{File: "${POLICY_DIR}/fact.yaml"},
			},
		},
	}

	getenv := func(name string) string {
		if name == "POLICY_DIR" {
			return "/etc/policies"
		}
		return ""
	}

	resolved, err := Resolve(h, getenv)
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if got := resolved.Spec.Sandbox.Policy.File; got != "/etc/policies/fact.yaml" {
		t.Errorf("policy.file should be expanded: got %q", got)
	}
	// The input's PolicyRef must not be mutated or aliased.
	if h.Spec.Sandbox.Policy.File != "${POLICY_DIR}/fact.yaml" {
		t.Errorf("input policy.file mutated: got %q", h.Spec.Sandbox.Policy.File)
	}
	if resolved.Spec.Sandbox.Policy == h.Spec.Sandbox.Policy {
		t.Error("resolved policy aliases the input PolicyRef pointer")
	}
}

func TestResolveSourceFields(t *testing.T) {
	// Test resolving source fields
	h := &Harness{
		APIVersion: "harness.openshell.dev/v1alpha1",
		Kind:       "Harness",
		Metadata:   Metadata{Name: "test"},
		Spec: Spec{
			Source: Source{
				Repo:        "${GIT_REPO}",
				Ref:         "main",
				Destination: "/workspace",
			},
		},
	}

	getenv := func(name string) string {
		if name == "GIT_REPO" {
			return "https://github.com/example/repo.git"
		}
		return ""
	}

	resolved, err := Resolve(h, getenv)
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	if resolved.Spec.Source.Repo != "https://github.com/example/repo.git" {
		t.Errorf("Repo should be resolved, got %q", resolved.Spec.Source.Repo)
	}
}

func TestExpandNoVarReference(t *testing.T) {
	// Test string with no ${...} references
	result, err := expandStrict("just a plain string", func(name string) string {
		return ""
	})
	if err != nil {
		t.Fatalf("Expand failed: %v", err)
	}
	if result != "just a plain string" {
		t.Errorf("got %q, want %q", result, "just a plain string")
	}
}

func TestExpandMissingCloseBrace(t *testing.T) {
	// Test ${VAR without closing brace → treat $ as literal
	result, err := expandStrict("test ${VAR with no close", func(name string) string {
		return ""
	})
	if err != nil {
		t.Fatalf("Expand failed: %v", err)
	}
	if result != "test ${VAR with no close" {
		t.Errorf("got %q, want %q", result, "test ${VAR with no close")
	}
}

func TestResolvePayloads(t *testing.T) {
	// Test resolving payload fields
	h := &Harness{
		APIVersion: "harness.openshell.dev/v1alpha1",
		Kind:       "Harness",
		Metadata:   Metadata{Name: "test"},
		Spec: Spec{
			Payloads: []Payload{
				{
					Source:      "${LOCAL_PATH}",
					Destination: "/sandbox/dest",
				},
				{
					Content:     "# Config\nDEBUG=${DEBUG_MODE}",
					Destination: "/sandbox/config.txt",
				},
			},
		},
	}

	getenv := func(name string) string {
		switch name {
		case "LOCAL_PATH":
			return "./scripts"
		case "DEBUG_MODE":
			return "true"
		default:
			return ""
		}
	}

	resolved, err := Resolve(h, getenv)
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	if resolved.Spec.Payloads[0].Source != "./scripts" {
		t.Errorf("Payload[0].Source should be resolved, got %q", resolved.Spec.Payloads[0].Source)
	}
	if resolved.Spec.Payloads[1].Content != "# Config\nDEBUG=true" {
		t.Errorf("Payload[1].Content should be resolved, got %q", resolved.Spec.Payloads[1].Content)
	}
}

func TestResolveInferenceFields(t *testing.T) {
	// Test resolving inference fields
	h := &Harness{
		APIVersion: "harness.openshell.dev/v1alpha1",
		Kind:       "Harness",
		Metadata:   Metadata{Name: "test"},
		Spec: Spec{
			Inference: Inference{
				Route:    "${INFERENCE_ROUTE}",
				Provider: "vertex",
				Model:    "${MODEL_NAME}",
				Timeout:  "30s",
			},
		},
	}

	getenv := func(name string) string {
		switch name {
		case "INFERENCE_ROUTE":
			return "prediction"
		case "MODEL_NAME":
			return "gemini-pro"
		default:
			return ""
		}
	}

	resolved, err := Resolve(h, getenv)
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	if resolved.Spec.Inference.Route != "prediction" {
		t.Errorf("Route should be resolved, got %q", resolved.Spec.Inference.Route)
	}
	if resolved.Spec.Inference.Model != "gemini-pro" {
		t.Errorf("Model should be resolved, got %q", resolved.Spec.Inference.Model)
	}
}

func TestResolveAgentFields(t *testing.T) {
	// Test resolving agent fields
	h := &Harness{
		APIVersion: "harness.openshell.dev/v1alpha1",
		Kind:       "Harness",
		Metadata:   Metadata{Name: "test"},
		Spec: Spec{
			Agent: Agent{
				Type: "${AGENT_TYPE}",
				Args: []string{"--flag1", "${ARG_VALUE}"},
			},
		},
	}

	getenv := func(name string) string {
		switch name {
		case "AGENT_TYPE":
			return "claude"
		case "ARG_VALUE":
			return "resolved-arg"
		default:
			return ""
		}
	}

	resolved, err := Resolve(h, getenv)
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	if resolved.Spec.Agent.Type != "claude" {
		t.Errorf("Agent.Type should be resolved, got %q", resolved.Spec.Agent.Type)
	}
	if resolved.Spec.Agent.Args[1] != "resolved-arg" {
		t.Errorf("Agent.Args[1] should be resolved, got %q", resolved.Spec.Agent.Args[1])
	}
}

func TestExpandMultipleInSameString(t *testing.T) {
	// Test multiple ${VAR} in the same string
	result, err := expandStrict("${VAR1}-${VAR2}-${VAR3}", func(name string) string {
		switch name {
		case "VAR1":
			return "a"
		case "VAR2":
			return "b"
		case "VAR3":
			return "c"
		default:
			return ""
		}
	})
	if err != nil {
		t.Fatalf("Expand failed: %v", err)
	}
	if result != "a-b-c" {
		t.Errorf("got %q, want %q", result, "a-b-c")
	}
}

func TestResolveDoesNotMutateInput(t *testing.T) {
	// Verify that Resolve returns a new copy and doesn't mutate input
	original := &Harness{
		APIVersion: "harness.openshell.dev/v1alpha1",
		Kind:       "Harness",
		Metadata:   Metadata{Name: "test"},
		Spec: Spec{
			Target: Target{
				Gateway: "${GATEWAY_VAR}",
			},
		},
	}

	originalGateway := original.Spec.Target.Gateway

	getenv := func(name string) string {
		if name == "GATEWAY_VAR" {
			return "resolved-gateway"
		}
		return ""
	}

	resolved, err := Resolve(original, getenv)
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	// Original should still have the variable reference
	if original.Spec.Target.Gateway != originalGateway {
		t.Errorf("original was mutated! got %q, want %q", original.Spec.Target.Gateway, originalGateway)
	}

	// Resolved should have the substituted value
	if resolved.Spec.Target.Gateway != "resolved-gateway" {
		t.Errorf("resolved should have substituted value, got %q", resolved.Spec.Target.Gateway)
	}
}

func TestResolveRegistrationOIDC(t *testing.T) {
	// Test resolving OIDC fields in registration
	h := &Harness{
		APIVersion: "harness.openshell.dev/v1alpha1",
		Kind:       "Harness",
		Metadata:   Metadata{Name: "test"},
		Spec: Spec{
			Target: Target{
				Registration: &Registration{
					Endpoint: "${OIDC_ENDPOINT}",
					OIDC: &OIDC{
						Issuer:   "${OIDC_ISSUER}",
						ClientID: "${OIDC_CLIENT_ID}",
						Audience: "my-app",
					},
				},
			},
		},
	}

	getenv := func(name string) string {
		switch name {
		case "OIDC_ENDPOINT":
			return "https://oidc.example.com"
		case "OIDC_ISSUER":
			return "https://issuer.example.com"
		case "OIDC_CLIENT_ID":
			return "client-id-12345"
		default:
			return ""
		}
	}

	resolved, err := Resolve(h, getenv)
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	if resolved.Spec.Target.Registration.Endpoint != "https://oidc.example.com" {
		t.Errorf("Endpoint should be resolved")
	}
	if resolved.Spec.Target.Registration.OIDC.Issuer != "https://issuer.example.com" {
		t.Errorf("OIDC.Issuer should be resolved")
	}
	if resolved.Spec.Target.Registration.OIDC.ClientID != "client-id-12345" {
		t.Errorf("OIDC.ClientID should be resolved")
	}
}

func TestResolveRegistrationRequiresCompleteDirectOIDC(t *testing.T) {
	h := &Harness{
		APIVersion: "harness.openshell.dev/v1alpha1",
		Kind:       "Harness",
		Metadata:   Metadata{Name: "test"},
		Spec: Spec{Target: Target{Registration: &Registration{
			OIDC: &OIDC{},
		}}},
	}

	_, err := Resolve(h, func(string) string { return "" })
	if err == nil {
		t.Fatal("expected incomplete direct OIDC configuration to fail")
	}
	for _, field := range []string{"endpoint", "issuer", "clientId", "audience"} {
		if !strings.Contains(err.Error(), field) {
			t.Errorf("error %q does not mention %s", err, field)
		}
	}
}
