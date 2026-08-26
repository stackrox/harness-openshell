package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/stackrox/harness-openshell/internal/agent"
	"github.com/stackrox/harness-openshell/internal/config"
	"github.com/stackrox/harness-openshell/internal/gateway"
	"github.com/stackrox/harness-openshell/internal/k8s"
	"github.com/stackrox/harness-openshell/internal/openshell"
	"github.com/stackrox/harness-openshell/internal/plan"
	"github.com/stackrox/harness-openshell/internal/reconcile"
	"github.com/stackrox/harness-openshell/internal/status"
)

var Version = "dev"

var DefaultAgentConfig []byte

type upLocalOpts struct {
	harnessDir      string
	gw              gateway.Gateway
	gwCfg           *gateway.GatewayConfig
	ensureLocal     bool
	agentCfg        *agent.AgentConfig
	agentPath       string
	sandboxName     string
	noTTY           bool
	providerRefresh bool
	setupOnly       bool
	harness         *agent.Harness
	newClient       openshell.Factory
	retrySleep      time.Duration
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

	if opts.ensureLocal {
		if err := deployLocal(gw); err != nil {
			return fmt.Errorf("deploy failed: %w", err)
		}
	} else if gw.InferenceGet() != nil {
		if opts.gwCfg == nil {
			return fmt.Errorf("no active gateway -- use --gateway local or: harness deploy ocp")
		}
		kc := k8s.New("", k8s.DefaultNamespace())
		clusterRunner := k8s.New("", "")
		if err := deployFromConfig(opts.harnessDir, opts.gwCfg, gw, kc, clusterRunner); err != nil {
			return fmt.Errorf("deploy failed: %w", err)
		}
	}

	registered := ensureProviders(opts.harnessDir, gw, agentCfg, opts.providerRefresh, opts.harness)

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
		upload, cleanup, err := cloneRepo(agentCfg.Repo, agentCfg.RepoRef)
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
	// They MUST NOT live inside payloadDir: createSandbox renames payloadDir
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
	var sandboxCmd []string
	if noTTY && agentCfg.Task == "" {
		sandboxCmd = []string{"true"}
	} else {
		sandboxCmd = []string{"bash", "/sandbox/.config/openshell/run.sh"}
	}

	err = createSandbox(sandboxOpts{
		harnessDir: opts.harnessDir,
		gw:         gw,
		name:       sandboxName,
		image:      sandboxImage,
		providers:  registered,
		noTTY:      noTTY,
		retrySleep: opts.retrySleep,
		sandboxCmd: sandboxCmd,
		payloadDir: payloadDir,
		uploads:    extraUploads,
		env:        agentCfg.BuildEnvMap(),
	})
	if err != nil {
		return err
	}

	// Apply custom policy after sandbox creation (kind: policy in harness YAML).
	// /etc/openshell/policy.yaml is read-only in the image, so policy changes
	// must go through the openshell CLI which hot-reloads the policy.
	if opts.harness != nil && opts.harness.Policy != nil {
		policyFile, writeErr := os.CreateTemp("", "harness-policy-*.yaml")
		if writeErr != nil {
			return fmt.Errorf("creating policy temp file: %w", writeErr)
		}
		defer os.Remove(policyFile.Name())
		if _, writeErr := policyFile.Write(opts.harness.Policy); writeErr != nil {
			policyFile.Close()
			return fmt.Errorf("writing policy: %w", writeErr)
		}
		policyFile.Close()

		status.Info("Applying custom policy...")
		if err := gw.PolicySet(sandboxName, policyFile.Name()); err != nil {
			return fmt.Errorf("applying policy: %w", err)
		}
		status.OK("Policy applied")
	}

	return nil
}

// cloneRepo clones or updates a cached git repository and returns an Upload
// that places it at /sandbox/<repo-name>. Repos are cached in
// ~/.cache/harness-openshell/repos/<repo-name>/ so subsequent runs only fetch
// deltas. The clone happens outside the sandbox so git credentials never enter
// it. Returns a cleanup function (no-op since the cache is persistent).
func cloneRepo(repo, ref string) (gateway.Upload, func(), error) {
	repoName := strings.TrimSuffix(path.Base(repo), ".git")

	if ref != "" {
		status.Infof("Repo:  %s (ref: %s)", repo, ref)
	} else {
		status.Infof("Repo:  %s", repo)
	}

	cacheDir, err := repoCacheDir(repoName)
	if err != nil {
		return gateway.Upload{}, nil, err
	}

	if isGitRepo(cacheDir) {
		if err := fetchRepo(cacheDir, ref); err != nil {
			return gateway.Upload{}, nil, err
		}
		status.OKf("Updated %s (cached)", repoName)
	} else {
		if err := freshClone(repo, ref, cacheDir); err != nil {
			return gateway.Upload{}, nil, fmt.Errorf("git clone %s: %w", repo, err)
		}
		status.OKf("Cloned %s", repoName)
	}

	return gateway.Upload{Src: cacheDir, Dst: "/sandbox"}, func() {}, nil
}

func repoCacheDir(repoName string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("determining home dir: %w", err)
	}
	dir := filepath.Join(home, ".cache", "harness-openshell", "repos", repoName)
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return "", fmt.Errorf("creating cache dir: %w", err)
	}
	return dir, nil
}

func isGitRepo(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil
}

func freshClone(repo, ref, dest string) error {
	args := []string{"clone", "--depth", "1"}
	if ref != "" {
		args = append(args, "--branch", ref)
	}
	args = append(args, repo, dest)
	cmd := exec.Command("git", args...)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}
	return initSubmodules(dest)
}

func fetchRepo(dir, ref string) error {
	fetchArgs := []string{"-C", dir, "fetch", "--depth", "1", "origin"}
	if ref != "" {
		fetchArgs = append(fetchArgs, ref)
	}
	cmd := exec.Command("git", fetchArgs...)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git fetch: %w", err)
	}

	target := "FETCH_HEAD"
	if ref == "" {
		target = "origin/HEAD"
	}
	cmd = exec.Command("git", "-C", dir, "checkout", target, "--force")
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git checkout %s: %w", target, err)
	}

	if err := initSubmodules(dir); err != nil {
		return err
	}

	// Clean untracked files from previous runs
	cmd = exec.Command("git", "-C", dir, "clean", "-fdx")
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	cmd.Run()

	return nil
}

func initSubmodules(dir string) error {
	cmd := exec.Command("git", "-C", dir, "submodule", "update", "--init", "--depth", "1")
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git submodule update: %w", err)
	}
	return nil
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
	target, err := resolveApplyTarget(opts.gw)
	if err != nil {
		status.Warnf("gateway reconcile skipped: %v", err)
		return
	}
	ctx := context.Background()
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
		switch r.Action {
		case plan.ActionAdoptionRequired:
			status.Warnf("provider %s: %s (set adopt: true or re-create managed)", r.Name, r.Action)
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
