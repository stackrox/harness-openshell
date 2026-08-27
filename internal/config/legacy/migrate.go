// Package legacy converts v1 (legacy) harness configs into the canonical
// harness.openshell.dev/v1alpha1 schema. It is the sole old→new bridge: it reads
// the legacy format via internal/agent and never the reverse, so the legacy
// parser can later be retired without untangling an import cycle.
package legacy

import (
	"fmt"
	"maps"
	"slices"

	"github.com/stackrox/harness-openshell/internal/agent"
	"github.com/stackrox/harness-openshell/internal/config"
	"gopkg.in/yaml.v3"
)

// sortedKeys returns the map keys in deterministic order so migration warnings
// are stable across runs.
func sortedKeys[V any](m map[string]V) []string {
	return slices.Sorted(maps.Keys(m))
}

// Warning is a non-fatal deprecation or conversion notice surfaced during migration.
type Warning struct {
	Field   string
	Message string
}

// Migrate converts an already-parsed legacy Harness into a v1alpha1 config.Harness,
// returning any deprecation/conversion warnings. It never mutates the input.
func Migrate(legacy *agent.Harness) (*config.Harness, []Warning, error) {
	if legacy.Agent == nil {
		return nil, nil, fmt.Errorf("legacy harness has no agent document")
	}
	a := legacy.Agent
	var warnings []Warning
	warn := func(field, msg string) { warnings = append(warnings, Warning{Field: field, Message: msg}) }

	// base_agent references an external base config that ParseHarness does not
	// load, so byte-level migration cannot resolve the inheritance here.
	if a.BaseAgent != "" {
		warn("base_agent", fmt.Sprintf("base_agent %q is not resolved during migration; flatten the base config before migrating", a.BaseAgent))
	}

	h := &config.Harness{
		APIVersion: "harness.openshell.dev/v1alpha1",
		Kind:       "Harness",
		Metadata:   config.Metadata{Name: a.Name},
		Spec: config.Spec{
			// spec.target.gateway names a registered OpenShell gateway and is
			// left for the user to set: the legacy gateway: field named a deploy
			// profile (local-container/helm/openshift), a concept the harness no
			// longer owns (provisioning is OpenShell's job), so it has no
			// v1alpha1 home and is not carried over.
			Source: config.Source{Repo: a.Repo, Ref: a.RepoRef},
			Agent:  config.Agent{Type: a.EffectiveEntrypoint()},
		},
	}

	for _, p := range a.Providers {
		warn("providers[].profile", fmt.Sprintf("provider profile %q converted to a referenced provider", p.Profile))
		h.Spec.Providers = append(h.Spec.Providers, config.Provider{Name: p.Profile, Management: "referenced"})
		if len(p.Env) > 0 {
			warn("providers[].env", fmt.Sprintf("provider %q env is not migrated; a referenced provider carries no env", p.Profile))
		}
	}

	// Sandbox fields carried losslessly from the legacy agent. Env is copied so
	// the result never aliases the parsed legacy struct's map.
	h.Spec.Sandbox.Image = a.Image
	if len(a.Env) > 0 {
		h.Spec.Sandbox.Env = make(map[string]string, len(a.Env))
		maps.Copy(h.Spec.Sandbox.Env, a.Env)
	}
	if a.TTY != nil {
		h.Spec.Sandbox.TTY = *a.TTY
	}
	if a.Policy != "" {
		h.Spec.Sandbox.Policy = &config.PolicyRef{File: a.Policy}
	}
	// An inline `kind: policy` document has no file path; v1alpha1 references
	// policies by file, so it cannot be carried automatically.
	if legacy.Policy != nil {
		warn("kind:policy", "inline policy document not migrated; save it to a file and set spec.sandbox.policy.file")
	}

	if a.Task != "" {
		warn("task", "task is not migrated; drive the agent via spec.agent.args or a payload")
	}
	for _, inc := range a.Include {
		warn("include", fmt.Sprintf("include %q is not migrated; express it as a spec.payloads entry", inc))
	}

	for _, p := range legacy.Payloads {
		h.Spec.Payloads = append(h.Spec.Payloads, config.Payload{
			Source:      p.LocalPath,
			Content:     p.Content,
			Destination: p.SandboxPath,
		})
	}

	// Inline kind:gateway / kind:provider documents have no v1alpha1 home: a
	// gateway is registered out-of-band via the openshell CLI, and a provider
	// profile is a separate artifact. Warn rather than drop them silently.
	for _, name := range sortedKeys(legacy.Gateways) {
		warn("kind:gateway", fmt.Sprintf("inline gateway document %q not migrated; register the gateway with the openshell CLI", name))
	}
	for _, name := range sortedKeys(legacy.Providers) {
		warn("kind:provider", fmt.Sprintf("inline provider profile %q not migrated; keep it as a separate provider profile", name))
	}

	return h, warnings, nil
}

// MigrateBytes parses legacy YAML, migrates it, and marshals the result to
// v1alpha1 YAML.
func MigrateBytes(data []byte) ([]byte, []Warning, error) {
	legacy, err := agent.ParseHarness(data)
	if err != nil {
		return nil, nil, fmt.Errorf("parsing legacy harness: %w", err)
	}
	migrated, warnings, err := Migrate(legacy)
	if err != nil {
		return nil, warnings, err
	}
	out, err := yaml.Marshal(migrated)
	if err != nil {
		return nil, warnings, fmt.Errorf("marshaling v1alpha1: %w", err)
	}
	return out, warnings, nil
}
