package cmd

import (
	"sort"
	"strings"

	"github.com/stackrox/harness-openshell/internal/config"
	"github.com/stackrox/harness-openshell/internal/plan"
)

// redactedPlan returns a display-only copy of p. Planning and reconciliation
// still use the resolved values; only values that came from interpolation are
// replaced before a plan is rendered as table, JSON, or YAML.
func redactedPlan(p *plan.Plan, resolved, input *config.Harness) *plan.Plan {
	if p == nil || resolved == nil || input == nil {
		return p
	}
	values := interpolatedValues(resolved, input)
	if len(values) == 0 {
		return p
	}

	out := *p
	out.Target.Gateway = redactPlanString(out.Target.Gateway, values)
	out.Target.Workspace = redactPlanString(out.Target.Workspace, values)
	out.Groups = make([]plan.Group, len(p.Groups))
	for i, group := range p.Groups {
		out.Groups[i] = group
		out.Groups[i].Resources = make([]plan.Resource, len(group.Resources))
		for j, resource := range group.Resources {
			resource.Name = redactPlanString(resource.Name, values)
			resource.Detail = redactPlanString(resource.Detail, values)
			out.Groups[i].Resources[j] = resource
		}
	}
	return &out
}

// interpolatedValues collects resolved values whose source contained a ${VAR}
// reference. The plan has already collapsed several fields into descriptions,
// so replacing matching substrings at the final display boundary is safer and
// smaller than maintaining a second field-to-resource mapping.
func interpolatedValues(resolved, input *config.Harness) []string {
	var values []string
	add := func(value, raw string) {
		if strings.Contains(raw, "$") && value != "" {
			values = append(values, value)
		}
	}

	add(resolved.Name, input.Name)
	add(resolved.Spec.Target.Gateway, input.Spec.Target.Gateway)
	add(resolved.Spec.Target.Workspace, input.Spec.Target.Workspace)
	add(resolved.Spec.Inference.Route, input.Spec.Inference.Route)
	add(resolved.Spec.Inference.Provider, input.Spec.Inference.Provider)
	add(resolved.Spec.Inference.Model, input.Spec.Inference.Model)
	add(resolved.Spec.Inference.Timeout, input.Spec.Inference.Timeout)
	add(resolved.Spec.Sandbox.Image, input.Spec.Sandbox.Image)
	if resolved.Spec.Sandbox.Policy != nil {
		rawPolicy := ""
		if input.Spec.Sandbox.Policy != nil {
			rawPolicy = input.Spec.Sandbox.Policy.File
		}
		add(resolved.Spec.Sandbox.Policy.File, rawPolicy)
	}
	for i, value := range resolved.Spec.Sandbox.Providers {
		if i < len(input.Spec.Sandbox.Providers) {
			add(value, input.Spec.Sandbox.Providers[i])
		}
	}
	for key, value := range resolved.Spec.Sandbox.Env {
		add(value, input.Spec.Sandbox.Env[key])
	}
	add(resolved.Spec.Agent.Type, input.Spec.Agent.Type)
	for i, value := range resolved.Spec.Agent.Args {
		if i < len(input.Spec.Agent.Args) {
			add(value, input.Spec.Agent.Args[i])
		}
	}
	add(resolved.Spec.Source.Repo, input.Spec.Source.Repo)
	add(resolved.Spec.Source.Ref, input.Spec.Source.Ref)
	add(resolved.Spec.Source.Destination, input.Spec.Source.Destination)
	add(resolved.Spec.Source.Submodules, input.Spec.Source.Submodules)
	for i, payload := range resolved.Spec.Payloads {
		if i >= len(input.Spec.Payloads) {
			continue
		}
		raw := input.Spec.Payloads[i]
		add(payload.Source, raw.Source)
		add(payload.Content, raw.Content)
		add(payload.Destination, raw.Destination)
	}

	// Longest first prevents a shorter interpolated value from partially
	// consuming a longer one in a compound detail string.
	sort.Slice(values, func(i, j int) bool {
		if len(values[i]) != len(values[j]) {
			return len(values[i]) > len(values[j])
		}
		return values[i] < values[j]
	})
	return values
}

func redactPlanString(value string, sensitive []string) string {
	for _, secret := range sensitive {
		if secret != "" && strings.Contains(value, secret) {
			value = strings.ReplaceAll(value, secret, "<redacted>")
		}
	}
	return value
}
