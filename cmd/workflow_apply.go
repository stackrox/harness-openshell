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
	"github.com/stackrox/harness-openshell/internal/run"
	"github.com/stackrox/harness-openshell/internal/source"
	"github.com/stackrox/harness-openshell/internal/status"
	"gopkg.in/yaml.v3"
)

type applyOptions struct {
	SetupOnly bool
	DryRun    bool
	Output    string
	OutputDir string
	Result    *applyResult
}

type preparedRun struct {
	run.SandboxRunRequest
	SourceCommit string
}

// applyWorkflow is a compatibility wrapper used by existing tests.
func applyWorkflow(ctx context.Context, workflow *resolvedWorkflow, p *plan.Plan, current plan.CurrentState, client openshell.Client, opts applyOptions) error {
	return executeResolvedWorkflow(ctx, workflow, p, current, client, opts)
}

// targetDescription names a target for error messages: direct registrations
// carry no CLI gateway name, so a quoted empty gateway would read as nonsense.
func targetDescription(target openshell.Target) string {
	if target.Direct != nil {
		return "direct target"
	}
	if target.Gateway == "" {
		return "active gateway"
	}
	return fmt.Sprintf("gateway %q", target.Gateway)
}

// verifyProviderReferences checks providers used by inference or the sandbox.
func verifyProviderReferences(ctx context.Context, client openshell.Client, desired *config.Harness) error {
	for _, name := range desired.Spec.ProviderReferences() {
		if _, err := client.GetProvider(ctx, name); err != nil {
			return fmt.Errorf("verifying referenced provider %q: %w", name, err)
		}
	}
	return nil
}

// preflightPlan rejects missing references before any inference write occurs.
func preflightPlan(p *plan.Plan) error {
	for _, group := range p.Groups {
		for _, resource := range group.Resources {
			switch {
			case group.Section == plan.SectionTarget && resource.Action == plan.ActionLoginRequired:
				return fmt.Errorf("gateway %q is not reachable or authenticated", p.Target.Gateway)
			case group.Section == plan.SectionProviders && resource.Action == plan.ActionMissing:
				return fmt.Errorf("referenced provider %q does not exist; create it through platform bootstrap before apply", resource.Name)
			case group.Section == plan.SectionInference && resource.Action == plan.ActionValidate:
				return fmt.Errorf("gateway does not support inference route reconciliation")
			}
		}
	}
	return nil
}

func inferenceConfigured(inf config.Inference) bool {
	return inf.Route != "" || inf.Provider != "" || inf.Model != "" || inf.Timeout != ""
}

func buildRunRequest(workflow *resolvedWorkflow, outputDir string) (preparedRun, func(), error) {
	desired := workflow.Desired
	cleanups := []func(){}
	cleanup := func() {
		for i := len(cleanups) - 1; i >= 0; i-- {
			cleanups[i]()
		}
	}
	fail := func(err error) (preparedRun, func(), error) {
		cleanup()
		return preparedRun{}, func() {}, err
	}
	image := resolveSandboxImagePath(desired.Spec.Sandbox.Image, workflow.BaseDir)
	if filepath.IsAbs(image) {
		return fail(fmt.Errorf("local sandbox images are unsupported; use a registry image reference"))
	}

	var uploads []run.Upload
	var sourceCommit string
	if desired.Spec.Source.Repo != "" {
		if mode := desired.Spec.Source.Submodules; mode != "" && mode != "shallow" {
			return fail(fmt.Errorf("source.submodules %q is not supported; use shallow or omit it", mode))
		}
		runID, err := source.NewRunID()
		if err != nil {
			return fail(fmt.Errorf("generating run ID: %w", err))
		}
		upload, sourceCleanup, commit, err := cloneRepo(desired.Spec.Source.Repo, desired.Spec.Source.Ref, runID)
		if err != nil {
			return fail(fmt.Errorf("cloning source: %w", err))
		}
		cleanups = append(cleanups, sourceCleanup)
		sourceCommit = commit
		if desired.Spec.Source.Destination != "" {
			upload.Dst = desired.Spec.Source.Destination
		}
		uploads = append(uploads, upload)
	}

	contentDir := ""
	for i, payload := range desired.Spec.Payloads {
		if payload.Destination == "" {
			return fail(fmt.Errorf("payloads[%d].destination is required", i))
		}
		switch {
		case payload.Source != "" && payload.Content != "":
			return fail(fmt.Errorf("payloads[%d] cannot set both source and content", i))
		case payload.Source != "":
			source := payload.Source
			if !filepath.IsAbs(source) {
				source = filepath.Join(workflow.BaseDir, source)
			}
			if _, err := os.Stat(source); err != nil {
				return fail(fmt.Errorf("reading payloads[%d].source: %w", i, err))
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
				return fail(fmt.Errorf("staging payloads[%d].content: %w", i, err))
			}
			uploads = append(uploads, run.Upload{Src: source, Dst: payload.Destination})
		default:
			return fail(fmt.Errorf("payloads[%d] requires source or content", i))
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
			return fail(fmt.Errorf("reading sandbox.policy.file: %w", err))
		}
	}

	var command []string
	if desired.Spec.Agent.Type != "" {
		command = append([]string{desired.Spec.Agent.Type}, desired.Spec.Agent.Args...)
	}

	var downloads []run.Download
	if len(desired.Spec.Outputs) > 0 {
		if outputDir == "" {
			return fail(fmt.Errorf("outputs are declared; --output-dir is required"))
		}
		outputRoot, err := filepath.Abs(outputDir)
		if err != nil {
			return fail(fmt.Errorf("resolving --output-dir: %w", err))
		}
		if err := os.MkdirAll(outputRoot, 0o750); err != nil {
			return fail(fmt.Errorf("creating --output-dir: %w", err))
		}
		for i, output := range desired.Spec.Outputs {
			destination := filepath.Join(outputRoot, filepath.FromSlash(output.Destination))
			if !pathWithin(outputRoot, destination) {
				return fail(fmt.Errorf("outputs[%d].destination escapes --output-dir", i))
			}
			downloads = append(downloads, run.Download{
				Src:      output.Source,
				Dst:      destination,
				Required: output.RequiredEnabled(),
			})
		}
	}

	return preparedRun{SourceCommit: sourceCommit, SandboxRunRequest: run.SandboxRunRequest{
		Name:      desired.Name,
		Image:     image,
		Providers: append([]string(nil), desired.Spec.Sandbox.Providers...),
		Env:       desired.Spec.Sandbox.Env,
		Command:   command,
		Uploads:   uploads,
		Downloads: downloads,
		TTY:       desired.Spec.Sandbox.TTY,
		Keep:      desired.Spec.Sandbox.Keep,
		Policy:    policyBytes,
	}}, cleanup, nil
}

func pathWithin(root, candidate string) bool {
	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(candidate))
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

// cloneRepo prepares an isolated checkout on the host so repository credentials
// never enter the sandbox.
func cloneRepo(repo, ref, runID string) (run.Upload, func(), string, error) {
	if ref != "" {
		status.Infof("Repo:  %s (ref: %s)", repo, ref)
	} else {
		status.Infof("Repo:  %s", repo)
	}
	cache, err := source.DefaultCache()
	if err != nil {
		return run.Upload{}, nil, "", err
	}
	prepared, err := cache.Prepare(repo, ref, runID)
	if err != nil {
		return run.Upload{}, nil, "", fmt.Errorf("preparing repo %s: %w", repo, err)
	}
	status.OKf("Prepared %s (commit: %s)", source.RepoName(repo), prepared.Commit)
	cleanup := func() {
		if err := prepared.Cleanup(); err != nil {
			status.Warnf("cleaning up repo checkout: %v", err)
		}
	}
	return run.Upload{Src: prepared.Dir, Dst: "/sandbox"}, cleanup, prepared.Commit, nil
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
	desired := redactedWorkflow(workflow.Desired, workflow.Input)
	switch output {
	case "yaml":
		data, err := yaml.Marshal(desired)
		if err != nil {
			return fmt.Errorf("marshaling resolved config: %w", err)
		}
		_, err = os.Stdout.Write(data)
		return err
	case "json":
		data, err := yaml.Marshal(desired)
		if err != nil {
			return fmt.Errorf("marshaling resolved config: %w", err)
		}
		var document any
		if err := yaml.Unmarshal(data, &document); err != nil {
			return fmt.Errorf("converting resolved config to JSON: %w", err)
		}
		return printStructured(formatJSON, document)
	default:
		return errors.New("version 1 apply output must be json or yaml")
	}
}

// redactedWorkflow keeps the resolved document shape while exposing only keys
// for maps whose values cross a credential boundary. Sandbox environment
// values may originate in the host environment and must
// never be serialized by -o yaml/json.
func redactedWorkflow(resolved, input *config.Harness) *config.Harness {
	out := &config.Harness{
		Version: resolved.Version,
		Name:    redactInterpolated(resolved.Name, input.Name),
		Spec: config.Spec{
			Target: redactedTarget(resolved.Spec.Target, input.Spec.Target),
			Inference: config.Inference{
				Route:    redactInterpolated(resolved.Spec.Inference.Route, input.Spec.Inference.Route),
				Provider: redactInterpolated(resolved.Spec.Inference.Provider, input.Spec.Inference.Provider),
				Model:    redactInterpolated(resolved.Spec.Inference.Model, input.Spec.Inference.Model),
				Timeout:  redactInterpolated(resolved.Spec.Inference.Timeout, input.Spec.Inference.Timeout),
				Verify:   copyBool(resolved.Spec.Inference.Verify),
			},
			Sandbox: redactedSandbox(resolved.Spec.Sandbox, input.Spec.Sandbox),
			Agent: config.Agent{
				Type: redactInterpolated(resolved.Spec.Agent.Type, input.Spec.Agent.Type),
				Args: redactStrings(resolved.Spec.Agent.Args, input.Spec.Agent.Args),
			},
			Source: config.Source{
				Repo:        redactInterpolated(resolved.Spec.Source.Repo, input.Spec.Source.Repo),
				Ref:         redactInterpolated(resolved.Spec.Source.Ref, input.Spec.Source.Ref),
				Destination: redactInterpolated(resolved.Spec.Source.Destination, input.Spec.Source.Destination),
				Submodules:  redactInterpolated(resolved.Spec.Source.Submodules, input.Spec.Source.Submodules),
			},
		},
	}
	out.Spec.Payloads = redactedPayloads(resolved.Spec.Payloads, input.Spec.Payloads)
	out.Spec.Outputs = redactedOutputs(resolved.Spec.Outputs, input.Spec.Outputs)
	return out
}

func redactedTarget(resolved, input config.Target) config.Target {
	out := config.Target{
		Gateway:   redactInterpolated(resolved.Gateway, input.Gateway),
		Workspace: redactInterpolated(resolved.Workspace, input.Workspace),
	}
	if resolved.Registration != nil {
		var raw config.Registration
		if input.Registration != nil {
			raw = *input.Registration
		}
		out.Registration = &config.Registration{
			Endpoint: redactInterpolated(resolved.Registration.Endpoint, raw.Endpoint),
		}
		if resolved.Registration.OIDC != nil {
			var rawOIDC config.OIDC
			if raw.OIDC != nil {
				rawOIDC = *raw.OIDC
			}
			out.Registration.OIDC = &config.OIDC{
				Issuer:   redactInterpolated(resolved.Registration.OIDC.Issuer, rawOIDC.Issuer),
				ClientID: redactInterpolated(resolved.Registration.OIDC.ClientID, rawOIDC.ClientID),
				Audience: redactInterpolated(resolved.Registration.OIDC.Audience, rawOIDC.Audience),
			}
		}
	}
	return out
}

func redactedSandbox(resolved, input config.Sandbox) config.Sandbox {
	out := config.Sandbox{
		Image:     redactInterpolated(resolved.Image, input.Image),
		Providers: redactStrings(resolved.Providers, input.Providers),
		Env:       redactedStringMap(resolved.Env),
		Keep:      resolved.Keep,
		TTY:       resolved.TTY,
	}
	if resolved.Policy != nil {
		var raw config.PolicyRef
		if input.Policy != nil {
			raw = *input.Policy
		}
		out.Policy = &config.PolicyRef{File: redactInterpolated(resolved.Policy.File, raw.File)}
	}
	return out
}

func redactedPayloads(resolved, input []config.Payload) []config.Payload {
	out := make([]config.Payload, len(resolved))
	for i, payload := range resolved {
		var raw config.Payload
		if i < len(input) {
			raw = input[i]
		}
		out[i] = config.Payload{
			Source:      redactInterpolated(payload.Source, raw.Source),
			Content:     redactInterpolated(payload.Content, raw.Content),
			Destination: redactInterpolated(payload.Destination, raw.Destination),
		}
	}
	return out
}

func redactedOutputs(resolved, input []config.Output) []config.Output {
	out := make([]config.Output, len(resolved))
	for i, output := range resolved {
		var raw config.Output
		if i < len(input) {
			raw = input[i]
		}
		out[i] = config.Output{
			Source:      redactInterpolated(output.Source, raw.Source),
			Destination: redactInterpolated(output.Destination, raw.Destination),
			Required:    copyBool(output.Required),
		}
	}
	return out
}

func redactedStringMap(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for key := range in {
		out[key] = "<redacted>"
	}
	return out
}

func redactInterpolated(resolved, input string) string {
	if strings.Contains(input, "$") {
		return "<redacted>"
	}
	return resolved
}

func redactStrings(resolved, input []string) []string {
	out := make([]string, len(resolved))
	for i, value := range resolved {
		var raw string
		if i < len(input) {
			raw = input[i]
		}
		out[i] = redactInterpolated(value, raw)
	}
	return out
}

func copyBool(value *bool) *bool {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
