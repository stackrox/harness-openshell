package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/stackrox/harness-openshell/internal/config"
	"github.com/stackrox/harness-openshell/internal/openshell"
	"github.com/stackrox/harness-openshell/internal/plan"
	"gopkg.in/yaml.v3"
)

// canonicalWorkflow is the single resolved input shared by plan and v1alpha1
// apply. Target precedence and execution defaults are applied before either
// command reads gateway state, so both commands build their action graph from
// the same desired object.
type canonicalWorkflow struct {
	Desired *config.Harness
	Target  openshell.Target
	BaseDir string
}

// canonicalOverrides are apply-only overrides. They are applied to the desired
// object before planning, so apply never executes a value absent from its plan.
type canonicalOverrides struct {
	Name      string
	AgentType string
	ForceTTY  bool
}

func isCanonicalWorkflow(path string) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return false, fmt.Errorf("reading %s: %w", path, err)
	}
	var header struct {
		APIVersion string `yaml:"apiVersion"`
		Kind       string `yaml:"kind"`
	}
	if err := yaml.Unmarshal(data, &header); err != nil {
		return false, fmt.Errorf("parsing YAML: %w", err)
	}
	return header.APIVersion == "harness.openshell.dev/v1alpha1" || header.Kind == "Harness", nil
}

func loadCanonicalWorkflow(path, flagGateway, flagWorkspace string, overrides canonicalOverrides) (*canonicalWorkflow, error) {
	h, err := config.Load(path)
	if err != nil {
		return nil, fmt.Errorf("loading config: %w", err)
	}
	resolved, err := config.Resolve(h, os.Getenv)
	if err != nil {
		return nil, fmt.Errorf("resolving config: %w", err)
	}

	target := openshell.ResolveTarget(flagGateway, flagWorkspace, resolved.Spec.Target.Gateway, resolved.Spec.Target.Workspace, os.Getenv)
	// Store the effective target in the resolved desired object. plan.Build reads
	// this object, while apply passes the same target to its SDK and CLI paths.
	resolved.Spec.Target.Gateway = target.Gateway
	resolved.Spec.Target.Workspace = target.Workspace

	if overrides.Name != "" {
		resolved.Metadata.Name = overrides.Name
	}
	if overrides.AgentType != "" {
		resolved.Spec.Agent.Type = overrides.AgentType
	}
	if overrides.ForceTTY {
		resolved.Spec.Sandbox.TTY = true
	}
	// Preserve the legacy image default for workflows that actually declare a
	// run, while leaving provider/inference-only workflows setup-only. The
	// resolved image is visible in both the plan and apply action graph.
	if canonicalRunConfigured(resolved) {
		resolved.Spec.Sandbox.Image = resolveSandboxImage(resolved.Spec.Sandbox.Image)
	}

	return &canonicalWorkflow{
		Desired: resolved,
		Target:  target,
		BaseDir: filepath.Dir(path),
	}, nil
}

func canonicalRunConfigured(desired *config.Harness) bool {
	sandbox := desired.Spec.Sandbox
	return sandbox.Image != "" || len(sandbox.Providers) > 0 || sandbox.Policy != nil || len(sandbox.Env) > 0 || sandbox.TTY || sandbox.Keep ||
		desired.Spec.Agent.Type != "" || desired.Spec.Source.Repo != "" || len(desired.Spec.Payloads) > 0
}

func (w *canonicalWorkflow) buildPlan(ctx context.Context, client openshell.Client) (*plan.Plan, plan.CurrentState, error) {
	var current plan.CurrentState
	if client != nil {
		var err error
		current, err = plan.ReadCurrentState(ctx, client, w.Desired)
		if err != nil {
			return nil, current, err
		}
	}
	return plan.Build(w.Desired, current), current, nil
}
