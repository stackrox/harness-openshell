package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/stackrox/harness-openshell/internal/config"
	"github.com/stackrox/harness-openshell/internal/openshell"
	"github.com/stackrox/harness-openshell/internal/plan"
	"github.com/stackrox/harness-openshell/internal/reconcile"
	"github.com/stackrox/harness-openshell/internal/run"
	"github.com/stackrox/harness-openshell/internal/source"
	"github.com/stackrox/harness-openshell/internal/status"
	"gopkg.in/yaml.v3"
)

type applyOptions struct {
	SetupOnly bool
	DryRun    bool
	Output    string
}

// applyWorkflow executes a resolved v1alpha1 workflow. The caller supplies the
// same plan snapshot that it may render for --dry-run; writes are delegated to
// reconcilers that share plan's provider and inference action functions.
func applyWorkflow(ctx context.Context, workflow *resolvedWorkflow, p *plan.Plan, current plan.CurrentState, client openshell.Client, opts applyOptions) error {
	if opts.DryRun {
		return renderPlan(p, opts.Output)
	}
	if workflow.Target.Gateway == "" && workflow.Target.Direct == nil {
		return fmt.Errorf("spec.target.gateway or spec.target.registration is required for apply (or set --gateway or %s)", openshell.EnvGateway)
	}
	if client == nil || !current.Reachable {
		return fmt.Errorf("%s is not reachable or authenticated", targetDescription(workflow.Target))
	}
	if err := preflightPlan(workflow.Desired, p); err != nil {
		return err
	}
	if err := verifySandboxProviders(ctx, client, workflow.Desired); err != nil {
		return err
	}

	var req run.SandboxRunRequest
	if !opts.SetupOnly && runConfigured(workflow.Desired) {
		var (
			cleanup func()
			err     error
		)
		req, cleanup, err = buildRunRequest(workflow)
		if err != nil {
			return err
		}
		defer cleanup()
	}

	if err := reconcileProviders(ctx, client, workflow.Desired.Spec.Providers); err != nil {
		return err
	}
	if inferenceConfigured(workflow.Desired.Spec.Inference) {
		result, err := reconcile.ReconcileInference(ctx, client, workflow.Desired.Spec.Inference)
		if err != nil {
			return fmt.Errorf("reconciling inference: %w", err)
		}
		status.OKf("inference: %s (model %s)", result.Action, workflow.Desired.Spec.Inference.Model)
	}
	if opts.SetupOnly {
		status.OK("Setup complete (--setup-only): skipping sandbox creation")
		return nil
	}
	if !runConfigured(workflow.Desired) {
		status.OK("Reconciliation complete: workflow declares no sandbox run")
		return nil
	}
	executor, ok := client.(openshell.SandboxExecutionClient)
	if !ok {
		return fmt.Errorf("configured OpenShell client does not support SDK sandbox execution")
	}
	return run.Run(ctx, executor, req, os.Stdin, os.Stdout, os.Stderr)
}

// targetDescription names a target for error messages: direct registrations
// carry no CLI gateway name, so a quoted empty gateway would read as nonsense.
func targetDescription(target openshell.Target) string {
	if target.Direct != nil {
		return "direct target"
	}
	return fmt.Sprintf("gateway %q", target.Gateway)
}

// verifySandboxProviders checks capabilities attached directly to the sandbox.
// They are intentionally distinct from spec.providers (desired resources), so a
// workflow may consume a platform-owned provider without declaring ownership or
// configuration for it.
func verifySandboxProviders(ctx context.Context, client openshell.Client, desired *config.Harness) error {
	declared := make(map[string]struct{}, len(desired.Spec.Providers))
	for _, provider := range desired.Spec.Providers {
		declared[provider.Name] = struct{}{}
	}
	for _, name := range desired.Spec.Sandbox.Providers {
		if _, alreadyChecked := declared[name]; alreadyChecked {
			continue
		}
		if _, err := client.GetProvider(ctx, name); err != nil {
			return fmt.Errorf("verifying sandbox provider %q: %w", name, err)
		}
	}
	return nil
}

// preflightPlan rejects actions this execution path cannot safely
// realize before any provider or inference write occurs. Reconcile repeats its
// reads to remain race-safe, but it uses the same action functions.
func preflightPlan(desired *config.Harness, p *plan.Plan) error {
	management := make(map[string]string, len(desired.Spec.Providers))
	for _, provider := range desired.Spec.Providers {
		management[provider.Name] = provider.Management
	}
	for _, group := range p.Groups {
		for _, resource := range group.Resources {
			switch {
			case group.Section == plan.SectionTarget && resource.Action == plan.ActionLoginRequired:
				return fmt.Errorf("gateway %q is not reachable or authenticated", p.Target.Gateway)
			case group.Section == plan.SectionProviders && resource.Action == plan.ActionCreate:
				return fmt.Errorf("managed provider %q does not exist; create it through the platform bootstrap path before apply", resource.Name)
			case group.Section == plan.SectionProviders && resource.Action == plan.ActionAdoptionRequired:
				if management[resource.Name] == "referenced" {
					return fmt.Errorf("referenced provider %q does not exist", resource.Name)
				}
				return fmt.Errorf("provider %q requires explicit adoption before apply", resource.Name)
			case group.Section == plan.SectionInference && resource.Action == plan.ActionValidate:
				return fmt.Errorf("gateway does not support inference route reconciliation")
			}
		}
	}
	return nil
}

func reconcileProviders(ctx context.Context, client openshell.Client, desired []config.Provider) error {
	results, err := reconcile.ReconcileProviders(ctx, client, desired)
	if err != nil {
		return fmt.Errorf("reconciling providers: %w", err)
	}
	for _, result := range results {
		switch result.Action {
		case plan.ActionCreate:
			return fmt.Errorf("managed provider %q does not exist; create it through the platform bootstrap path before apply", result.Name)
		case plan.ActionAdoptionRequired:
			return fmt.Errorf("provider %q requires explicit adoption before apply", result.Name)
		default:
			status.OKf("provider %s: %s", result.Name, result.Action)
		}
	}
	return nil
}

func inferenceConfigured(inf config.Inference) bool {
	return inf.Route != "" || inf.Provider != "" || inf.Model != "" || inf.Timeout != ""
}

func buildRunRequest(workflow *resolvedWorkflow) (run.SandboxRunRequest, func(), error) {
	desired := workflow.Desired
	cleanups := []func(){}
	cleanup := func() {
		for i := len(cleanups) - 1; i >= 0; i-- {
			cleanups[i]()
		}
	}
	fail := func(err error) (run.SandboxRunRequest, func(), error) {
		cleanup()
		return run.SandboxRunRequest{}, func() {}, err
	}
	image := resolveSandboxImagePath(desired.Spec.Sandbox.Image, workflow.BaseDir)
	if filepath.IsAbs(image) {
		return fail(fmt.Errorf("local sandbox images are unsupported; use a registry image reference"))
	}

	var uploads []run.Upload
	if desired.Spec.Source.Repo != "" {
		if mode := desired.Spec.Source.Submodules; mode != "" && mode != "shallow" {
			return fail(fmt.Errorf("spec.source.submodules %q is not supported; use shallow or omit it", mode))
		}
		runID, err := source.NewRunID()
		if err != nil {
			return fail(fmt.Errorf("generating run ID: %w", err))
		}
		upload, sourceCleanup, err := cloneRepo(desired.Spec.Source.Repo, desired.Spec.Source.Ref, runID)
		if err != nil {
			return fail(fmt.Errorf("cloning source: %w", err))
		}
		cleanups = append(cleanups, sourceCleanup)
		if desired.Spec.Source.Destination != "" {
			upload.Dst = desired.Spec.Source.Destination
		}
		uploads = append(uploads, upload)
	}

	contentDir := ""
	for i, payload := range desired.Spec.Payloads {
		if payload.Destination == "" {
			return fail(fmt.Errorf("spec.payloads[%d].destination is required", i))
		}
		switch {
		case payload.Source != "" && payload.Content != "":
			return fail(fmt.Errorf("spec.payloads[%d] cannot set both source and content", i))
		case payload.Source != "":
			source := payload.Source
			if !filepath.IsAbs(source) {
				source = filepath.Join(workflow.BaseDir, source)
			}
			if _, err := os.Stat(source); err != nil {
				return fail(fmt.Errorf("reading spec.payloads[%d].source: %w", i, err))
			}
			uploads = append(uploads, run.Upload{Src: source, Dst: payload.Destination})
		case payload.Content != "":
			if contentDir == "" {
				stagedDir, err := os.MkdirTemp("", "harness-v1alpha-payload-")
				if err != nil {
					return fail(fmt.Errorf("creating payload directory: %w", err))
				}
				contentDir = stagedDir
				cleanups = append(cleanups, func() { _ = os.RemoveAll(contentDir) })
			}
			source := filepath.Join(contentDir, fmt.Sprintf("payload-%d", i))
			if err := os.WriteFile(source, []byte(payload.Content), 0o600); err != nil {
				return fail(fmt.Errorf("staging spec.payloads[%d].content: %w", i, err))
			}
			uploads = append(uploads, run.Upload{Src: source, Dst: payload.Destination})
		default:
			return fail(fmt.Errorf("spec.payloads[%d] requires source or content", i))
		}
	}

	var policyBytes []byte
	if policy := desired.Spec.Sandbox.Policy; policy != nil && policy.File != "" {
		policyPath := policy.File
		if !filepath.IsAbs(policyPath) {
			policyPath = filepath.Join(workflow.BaseDir, policyPath)
		}
		var err error
		policyBytes, err = os.ReadFile(policyPath)
		if err != nil {
			return fail(fmt.Errorf("reading spec.sandbox.policy.file: %w", err))
		}
	}

	var command []string
	if desired.Spec.Agent.Type != "" {
		command = append([]string{desired.Spec.Agent.Type}, desired.Spec.Agent.Args...)
	}

	return run.SandboxRunRequest{
		Name:      desired.Metadata.Name,
		Image:     image,
		Providers: append([]string(nil), desired.Spec.Sandbox.Providers...),
		Env:       desired.Spec.Sandbox.Env,
		Command:   command,
		Uploads:   uploads,
		TTY:       desired.Spec.Sandbox.TTY,
		Keep:      desired.Spec.Sandbox.Keep,
		Policy:    policyBytes,
	}, cleanup, nil
}

// cloneRepo prepares an isolated checkout on the host so repository credentials
// never enter the sandbox.
func cloneRepo(repo, ref, runID string) (run.Upload, func(), error) {
	if ref != "" {
		status.Infof("Repo:  %s (ref: %s)", repo, ref)
	} else {
		status.Infof("Repo:  %s", repo)
	}
	cache, err := source.DefaultCache()
	if err != nil {
		return run.Upload{}, nil, err
	}
	prepared, err := cache.Prepare(repo, ref, runID)
	if err != nil {
		return run.Upload{}, nil, fmt.Errorf("preparing repo %s: %w", repo, err)
	}
	status.OKf("Prepared %s", source.RepoName(repo))
	cleanup := func() {
		if err := prepared.Cleanup(); err != nil {
			status.Warnf("cleaning up repo checkout: %v", err)
		}
	}
	return run.Upload{Src: prepared.Dir, Dst: "/sandbox"}, cleanup, nil
}

func renderPlan(p *plan.Plan, output string) error {
	format := formatTable
	if output != "" {
		var err error
		format, err = parseOutputFormat(output)
		if err != nil {
			return err
		}
	}
	if format != formatTable {
		return printStructured(format, p)
	}
	for _, section := range p.TableSections() {
		fmt.Fprintln(os.Stdout, strings.ToUpper(section.Title))
		printTable(section.Headers, section.Rows)
	}
	return nil
}

func renderWorkflow(workflow *resolvedWorkflow, output string) error {
	switch output {
	case "yaml":
		data, err := yaml.Marshal(workflow.Desired)
		if err != nil {
			return fmt.Errorf("marshaling resolved config: %w", err)
		}
		_, err = os.Stdout.Write(data)
		return err
	case "json":
		data, err := yaml.Marshal(workflow.Desired)
		if err != nil {
			return fmt.Errorf("marshaling resolved config: %w", err)
		}
		var document any
		if err := yaml.Unmarshal(data, &document); err != nil {
			return fmt.Errorf("converting resolved config to JSON: %w", err)
		}
		return printStructured(formatJSON, document)
	default:
		return errors.New("v1alpha1 apply output must be json or yaml")
	}
}
