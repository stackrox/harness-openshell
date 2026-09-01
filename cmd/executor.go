package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/stackrox/harness-openshell/internal/agent"
	"github.com/stackrox/harness-openshell/internal/config"
	"github.com/stackrox/harness-openshell/internal/gateway"
	"github.com/stackrox/harness-openshell/internal/openshell"
	"github.com/stackrox/harness-openshell/internal/payload"
	"github.com/stackrox/harness-openshell/internal/plan"
	"github.com/stackrox/harness-openshell/internal/reconcile"
	"github.com/stackrox/harness-openshell/internal/run"
	"github.com/stackrox/harness-openshell/internal/source"
	"github.com/stackrox/harness-openshell/internal/status"
)

var Version = "dev"

// reconcileTimeout bounds the SDK reconcile (provider Get/Update + inference
// verify) so a stalled gateway or provider endpoint degrades to a warning
// instead of hanging apply indefinitely.
const reconcileTimeout = 60 * time.Second

var DefaultAgentConfig []byte

type upLocalOpts struct {
	harnessDir  string
	gw          gateway.Gateway
	target      openshell.Target
	agentCfg    *agent.AgentConfig
	agentPath   string
	sandboxName string
	noTTY       bool
	setupOnly   bool
	harness     *agent.Harness
	newClient   openshell.Factory
	retrySleep  time.Duration
}

func upLocal(opts upLocalOpts) error {
	gw := opts.gw

	agentCfg := opts.agentCfg
	if agentCfg == nil {
		var err error
		agentCfg, err = agent.ParseFile(opts.agentPath)
		if err != nil {
			return err
		}
	}
	sandboxName := agentCfg.Name
	if opts.sandboxName != "" {
		sandboxName = opts.sandboxName
	}
	noTTY := opts.noTTY || agentCfg.NoTTY()

	sandboxImage := resolveSandboxImage(agentCfg.Image)

	status.Infof("Agent: %s (%s)", sandboxName, filepath.Base(opts.agentPath))
	status.Infof("Image: %s", sandboxImage)
	if agentCfg.Task != "" {
		status.Infof("Task:  %s", agentCfg.Task)
	}

	// The harness no longer provisions gateways: apply runs against a gateway
	// OpenShell already stood up and the user selected. Fail up front — before
	// touching providers or creating a sandbox — if none is reachable. The SDK
	// health check doubles as the active-gateway resolution probe: an unset
	// target surfaces ErrNoActiveGateway here.
	if err := checkGatewayReachable(context.Background(), opts.newClient, opts.target); err != nil {
		return fmt.Errorf("no active gateway is reachable — provision one with the OpenShell installer or 'helm install openshell', then select it with 'openshell gateway select <name>': %w", err)
	}

	registered := ensureProviders(opts.harnessDir, gw, opts.target, agentCfg, opts.harness)

	if needsInference(agentCfg.EffectiveEntrypoint()) && !hasInferenceProvider(agentCfg.Providers) {
		status.Warn("No inference provider configured — the agent will not be able to authenticate. Add google-vertex-ai to providers.")
	}

	reconcileGateway(opts, agentCfg)

	// --setup-only stops here: the gateway is deployed and providers/inference
	// are reconciled, but no sandbox is created and no agent is run.
	if opts.setupOnly {
		status.OK("Setup complete (--setup-only): skipping sandbox creation")
		return nil
	}

	// Clone repo outside the sandbox so git credentials never enter it.
	var repoUpload *gateway.Upload
	if agentCfg.Repo != "" {
		runID, err := source.NewRunID()
		if err != nil {
			return err
		}
		upload, cleanup, err := cloneRepo(agentCfg.Repo, agentCfg.RepoRef, runID)
		if err != nil {
			return fmt.Errorf("cloning repo: %w", err)
		}
		defer cleanup()
		repoUpload = &upload
	}

	payloadDir, err := os.MkdirTemp("", "harness-payload-")
	if err != nil {
		return fmt.Errorf("creating payload dir: %w", err)
	}
	defer os.RemoveAll(payloadDir)

	if err := agent.RenderPayload(agentCfg, opts.harnessDir, payloadDir); err != nil {
		return fmt.Errorf("rendering payload: %w", err)
	}

	// Resolve payload entries into upload pairs. Inline content payloads are
	// written to temp files that are uploaded individually by their own
	// sandbox_path, so their source paths must survive until SandboxCreate.
	// They MUST NOT live inside payloadDir: stagePayloadUpload renames payloadDir
	// into a staging directory, which would invalidate any path pointing inside
	// it and fail every upload with "local path does not exist" (issue #84).
	var extraUploads []gateway.Upload
	if opts.harness != nil && len(opts.harness.Payloads) > 0 {
		contentDir, err := os.MkdirTemp("", "harness-payload-content-")
		if err != nil {
			return fmt.Errorf("creating payload content dir: %w", err)
		}
		defer os.RemoveAll(contentDir)

		resolved, err := agent.ResolvePayloads(opts.harness.Payloads, opts.harnessDir, contentDir)
		if err != nil {
			return fmt.Errorf("resolving payloads: %w", err)
		}
		for _, u := range resolved {
			extraUploads = append(extraUploads, gateway.Upload{Src: u.Src, Dst: u.Dst})
		}
	}

	if repoUpload != nil {
		extraUploads = append(extraUploads, *repoUpload)
	}

	status.Header("Sandbox")

	// Command: the agent adapter owns entrypoint + task dispatch (replaces the
	// generated run.sh). Headless with no task starts the sandbox without an agent.
	taskPath := ""
	if agentCfg.Task != "" {
		taskPath = agent.SandboxTaskPath
	}
	var sandboxCmd []string
	if noTTY && agentCfg.Task == "" {
		sandboxCmd = []string{"true"}
	} else {
		cmd, cmdErr := agent.AdapterFor(agentCfg.EffectiveEntrypoint()).Command(agentCfg, taskPath)
		if cmdErr != nil {
			return fmt.Errorf("building sandbox command: %w", cmdErr)
		}
		sandboxCmd = cmd
	}

	// Stage the rendered payload for upload (--upload lands it at
	// /sandbox/.config/openshell/*).
	uploadDir, cleanupUpload, err := stagePayloadUpload(payloadDir)
	if err != nil {
		return err
	}
	defer cleanupUpload()

	uploads := []gateway.Upload{{Src: uploadDir, Dst: "/sandbox/.config"}}
	uploads = append(uploads, extraUploads...)

	// Stage the effective policy and apply it AT CREATE via --policy.
	// /etc/openshell/policy.yaml is read-only in the image; policy-at-create is
	// authoritative (the old post-create hot-reload could be silently dropped).
	var policyPath string
	if opts.harness != nil && opts.harness.Policy != nil {
		policyDir, mkErr := os.MkdirTemp("", "harness-policy-")
		if mkErr != nil {
			return fmt.Errorf("creating policy dir: %w", mkErr)
		}
		defer os.RemoveAll(policyDir)
		p, wErr := payload.WriteEffectivePolicy(policyDir, opts.harness.Policy)
		if wErr != nil {
			return fmt.Errorf("writing policy: %w", wErr)
		}
		policyPath = p
	}

	return run.RunSandbox(context.Background(), gw, run.SandboxRunRequest{
		Name:       sandboxName,
		Gateway:    opts.target.Gateway,
		Image:      resolveSandboxImagePath(sandboxImage, opts.harnessDir),
		Providers:  registered,
		Env:        agentCfg.BuildEnvMap(),
		Command:    sandboxCmd,
		Uploads:    uploads,
		TTY:        !noTTY,
		Keep:       true,
		PolicyPath: policyPath,
		RetrySleep: opts.retrySleep,
	})
}

// cloneRepo prepares an isolated per-run checkout of repo at ref and returns an
// Upload that places it at /sandbox/<repo-name>, plus a cleanup that removes the
// checkout. The mirror + checkout are built on the host so git credentials never
// enter the sandbox; see internal/source for the URL-hashed mirror + per-run
// checkout layout that keeps distinct same-basename repos and concurrent runs
// from colliding.
func cloneRepo(repo, ref, runID string) (gateway.Upload, func(), error) {
	if ref != "" {
		status.Infof("Repo:  %s (ref: %s)", repo, ref)
	} else {
		status.Infof("Repo:  %s", repo)
	}

	cache, err := source.DefaultCache()
	if err != nil {
		return gateway.Upload{}, nil, err
	}
	prepared, err := cache.Prepare(repo, ref, runID)
	if err != nil {
		return gateway.Upload{}, nil, fmt.Errorf("preparing repo %s: %w", repo, err)
	}
	status.OKf("Prepared %s", source.RepoName(repo))

	cleanup := func() {
		if cerr := prepared.Cleanup(); cerr != nil {
			status.Warnf("cleaning up repo checkout: %v", cerr)
		}
	}
	return gateway.Upload{Src: prepared.Dir, Dst: "/sandbox"}, cleanup, nil
}

// reconcileGateway drives the gateway's providers and inference route to match
// the agent config through the SDK reconcile path. It is the single apply-spine
// owner of SDK contact: it constructs one client for the resolved target and runs
// the provider reconcile (credential-preserving update / owner adoption for the
// providers registerProviders bootstrapped) followed by the inference reconcile.
//
// Behavior change (PR4a S5): the legacy inference write always passed --no-verify;
// the reconcile path verifies by default (see config.Inference.VerifyEnabled), so
// a route write is validated against the provider endpoint. The apply path has no
// opt-out field yet — the escape hatch (inference.verify: false) lives in the
// config.Harness path consumed by `harness plan`/reconcile.
//
// It is non-fatal by construction: if nothing is configured it is a no-op, and any
// client-construction or reconcile failure degrades to a warning rather than
// aborting apply — provider registration (the CLI-bridge create) has already
// happened and the sandbox can still be created. The engines themselves never
// degrade; the best-effort posture is the caller's.
func reconcileGateway(opts upLocalOpts, agentCfg *agent.AgentConfig) {
	providers, inference := desiredFromAgent(agentCfg, os.Getenv)
	if len(providers) == 0 && inference.Provider == "" {
		return // nothing to reconcile
	}

	if opts.newClient == nil {
		status.Warn("gateway reconcile skipped: no SDK client factory")
		return
	}
	// Reconcile acts on the same target apply resolved once and threaded in —
	// not a re-derivation — so providers/inference and the sandbox always land on
	// the same gateway.
	target := opts.target
	// Bound the whole reconcile: verify-by-default makes the inference write
	// contact the provider endpoint synchronously, so a stalled gateway or
	// endpoint would otherwise hang apply with no deadline. Every other failure
	// here degrades to a warning; a timeout must too.
	ctx, cancel := context.WithTimeout(context.Background(), reconcileTimeout)
	defer cancel()
	client, err := opts.newClient(ctx, target)
	if err != nil {
		status.Warnf("gateway reconcile skipped: %v", err)
		return
	}
	defer client.Close()

	reconcileProvidersStep(ctx, client, providers)
	reconcileInferenceStep(ctx, client, inference)
}

// reconcileProvidersStep verifies/updates/adopts the desired providers against the
// gateway. Each result is reported; a reconcile error degrades to a warning.
func reconcileProvidersStep(ctx context.Context, client openshell.Client, providers []config.Provider) {
	if len(providers) == 0 {
		return
	}
	results, err := reconcile.ReconcileProviders(ctx, client, providers)
	if err != nil {
		status.Warnf("provider reconcile: %v", err)
		return
	}
	for _, r := range results {
		switch {
		case r.Action == plan.ActionAdoptionRequired:
			status.Warnf("provider %s: %s (set adopt: true or re-create managed)", r.Name, r.Action)
		case r.Action == plan.ActionCreate:
			// Reconcile never SDK-creates: an ActionCreate means the managed
			// provider is absent and the CLI-bridge bootstrap did not create it
			// (e.g. gws missing, or an ADC create that ensureProviders degraded
			// to a warning). Surface it, never report the missing provider as OK.
			status.Warnf("provider %s: not present on the gateway (bootstrap did not create it)", r.Name)
		case r.Adopted:
			status.Warnf("provider %s: adopted (was unowned — harness now owns it)", r.Name)
		default:
			status.OKf("provider %s: %s", r.Name, r.Action)
		}
	}
}

// reconcileInferenceStep drives the inference route to match desired. A no-provider
// desired is a no-op; a reconcile error degrades to a warning.
func reconcileInferenceStep(ctx context.Context, client openshell.Client, desired config.Inference) {
	if desired.Provider == "" {
		return // no inference provider in this agent — nothing to reconcile
	}
	result, err := reconcile.ReconcileInference(ctx, client, desired)
	if err != nil {
		status.Warnf("inference reconcile: %v", err)
		return
	}
	status.OKf("inference: %s (model %s)", result.Action, desired.Model)
}

var inferenceProviders = map[string]bool{
	"google-vertex-ai": true,
}

func needsInference(entrypoint string) bool {
	switch entrypoint {
	case "claude", "opencode":
		return true
	}
	return false
}

func hasInferenceProvider(providers []agent.ProviderRef) bool {
	for _, p := range providers {
		if inferenceProviders[p.Profile] {
			return true
		}
	}
	return false
}
