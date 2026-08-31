// Package config defines the canonical harness.openshell.dev/v1alpha1 configuration schema.
//
// This package is SDK-free and cobra-free, defining only the desired-resource model
// and strict parsing. Secret values are never materialized — SecretRef carries only
// the source (e.g. "gcloud-adc").
package config

import (
	"fmt"
	"strings"
	"time"
)

// Harness is the root v1alpha1 configuration document.
type Harness struct {
	APIVersion string   `yaml:"apiVersion"` // must equal "harness.openshell.dev/v1alpha1"
	Kind       string   `yaml:"kind"`       // must equal "Harness"
	Metadata   Metadata `yaml:"metadata"`
	Spec       Spec     `yaml:"spec"`
}

// Metadata holds document identity.
type Metadata struct {
	Name string `yaml:"name"` // required
}

// Spec is the desired state.
type Spec struct {
	Target    Target     `yaml:"target"`
	Providers []Provider `yaml:"providers,omitempty"` // desired RESOURCES
	Inference Inference  `yaml:"inference,omitempty"`
	Sandbox   Sandbox    `yaml:"sandbox,omitempty"`
	Agent     Agent      `yaml:"agent,omitempty"`
	Source    Source     `yaml:"source,omitempty"`
	Payloads  []Payload  `yaml:"payloads,omitempty"`
}

// Target specifies the openshell gateway and workspace.
type Target struct {
	Gateway      string        `yaml:"gateway,omitempty"`   // logical name; CLI registration name when Registration is omitted
	Workspace    string        `yaml:"workspace,omitempty"` // "" → default (owned by sdkclient)
	Registration *Registration `yaml:"registration,omitempty"`
}

// Registration describes a direct, in-memory gateway connection. Despite the
// v1alpha1 field name, apply does not persist a CLI gateway registration.
type Registration struct {
	Endpoint      string `yaml:"endpoint,omitempty"`
	AutoProviders bool   `yaml:"autoProviders,omitempty"`
	OIDC          *OIDC  `yaml:"oidc,omitempty"`
}

// OIDC holds OIDC issuer and client configuration.
// The client secret is NOT stored here — sdkclient reads it exclusively from
// OPENSHELL_OIDC_CLIENT_SECRET.
type OIDC struct {
	Issuer   string `yaml:"issuer,omitempty"`
	ClientID string `yaml:"clientId,omitempty"`
	Audience string `yaml:"audience,omitempty"`
}

// SecretRef refers to a secret source without materializing the value.
// Valid sources include "gcloud-adc" and "environment:VAR_NAME".
type SecretRef struct {
	Source string `yaml:"source"` // e.g. "gcloud-adc", "environment:OPENSHELL_OIDC_CLIENT_SECRET"
}

// Describe returns a human-safe description of where the secret comes from,
// never the value. For example, "gcloud ADC" for "gcloud-adc",
// or "environment OPENSHELL_OIDC_CLIENT_SECRET" for "environment:OPENSHELL_OIDC_CLIENT_SECRET".
func (s SecretRef) Describe() string {
	switch {
	case s.Source == "gcloud-adc":
		return "gcloud ADC"
	case strings.HasPrefix(s.Source, "environment:"):
		return "environment " + strings.TrimPrefix(s.Source, "environment:")
	default:
		return s.Source
	}
}

// Provider represents a desired provider resource.
type Provider struct {
	Name       string `yaml:"name"`
	Type       string `yaml:"type,omitempty"`
	Management string `yaml:"management"` // "managed" or "referenced"; empty → referenced
	// Adopt authorizes reconcile to take over an existing provider that does not
	// carry this harness's owner label. Without it, a matching-but-unowned
	// provider is reported adoption-required and never overwritten. It is the
	// operator's explicit opt-in to manage a pre-existing provider.
	Adopt       bool              `yaml:"adopt,omitempty"`
	Credentials *SecretRef        `yaml:"credentials,omitempty"`
	Config      map[string]string `yaml:"config,omitempty"`
}

// Inference specifies the LLM inference route configuration.
type Inference struct {
	Route    string `yaml:"route,omitempty"`
	Provider string `yaml:"provider,omitempty"`
	Model    string `yaml:"model,omitempty"`
	Timeout  string `yaml:"timeout,omitempty"`
	// Verify controls the gateway's synchronous endpoint validation on write.
	// It is a *bool so "unset" is distinct from "false": unset (nil) means
	// verify (the safe default), so only an explicit `verify: false` skips it.
	// Verify is not part of the reconcile diff, so changing only this field does
	// not by itself trigger a re-write; it takes effect on the next write caused
	// by a provider/model/timeout change.
	Verify *bool `yaml:"verify,omitempty"`
}

// VerifyEnabled reports whether endpoint verification should run on write.
// Unset (nil) defaults to true; this is the single owner of the nil→verify rule.
func (inf Inference) VerifyEnabled() bool {
	return inf.Verify == nil || *inf.Verify
}

// TimeoutSecs parses Timeout (a Go duration string like "60s" or "2m") into
// whole seconds. Empty → 0, meaning "let the gateway apply its default". A bare
// number without a unit (e.g. "60") is an error — the unit is required so the
// meaning is unambiguous. This is the single owner of the Timeout → seconds
// conversion; the plan diff and the reconcile write both call it. Validated at
// Resolve time, so by plan/reconcile time it cannot fail.
func (inf Inference) TimeoutSecs() (uint64, error) {
	if inf.Timeout == "" {
		return 0, nil
	}
	d, err := time.ParseDuration(inf.Timeout)
	if err != nil {
		return 0, fmt.Errorf("invalid timeout %q: want a duration string like \"60s\" or \"2m\"", inf.Timeout)
	}
	if d < 0 {
		return 0, fmt.Errorf("invalid timeout %q: must not be negative", inf.Timeout)
	}
	return uint64(d.Round(time.Second) / time.Second), nil
}

// Sandbox describes the execution sandbox for this run.
type Sandbox struct {
	Image     string            `yaml:"image,omitempty"`
	Providers []string          `yaml:"providers,omitempty"` // run CAPABILITIES (distinct from Spec.Providers)
	Policy    *PolicyRef        `yaml:"policy,omitempty"`
	Env       map[string]string `yaml:"env,omitempty"`
	Keep      bool              `yaml:"keep,omitempty"`
	TTY       bool              `yaml:"tty,omitempty"`
}

// PolicyRef refers to a policy file.
type PolicyRef struct {
	File string `yaml:"file,omitempty"`
}

// Agent specifies the agent to use in the sandbox.
type Agent struct {
	Type  string   `yaml:"type,omitempty"`
	Model string   `yaml:"model,omitempty"`
	Args  []string `yaml:"args,omitempty"`
}

// Source specifies the source repository to clone.
type Source struct {
	Repo        string `yaml:"repo,omitempty"`
	Ref         string `yaml:"ref,omitempty"`
	Destination string `yaml:"destination,omitempty"`
	Submodules  string `yaml:"submodules,omitempty"`
}

// Payload represents a file or content to be placed in the sandbox.
// This replaces the legacy sandbox_path/local_path fields.
type Payload struct {
	Source      string `yaml:"source,omitempty"`  // local path (was local_path)
	Content     string `yaml:"content,omitempty"` // inline content
	Destination string `yaml:"destination"`       // target path in sandbox (was sandbox_path)
}
