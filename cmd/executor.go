package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/robbycochran/harness-openshell/internal/agent"
	"github.com/robbycochran/harness-openshell/internal/gateway"
	"github.com/robbycochran/harness-openshell/internal/k8s"
	"github.com/robbycochran/harness-openshell/internal/status"
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
	harness         *agent.Harness
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

	// Clone repo outside the sandbox so git credentials never enter it.
	var repoUpload *gateway.Upload
	if agentCfg.Repo != "" {
		upload, cleanup, err := cloneRepo(agentCfg.Repo)
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

	// Resolve payload entries into upload pairs
	var extraUploads []gateway.Upload
	if opts.harness != nil && len(opts.harness.Payloads) > 0 {
		resolved, err := agent.ResolvePayloads(opts.harness.Payloads, opts.harnessDir, payloadDir)
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

// cloneRepo clones a git repository to a temp directory and returns an Upload
// that places it at /sandbox/<repo-name>. The clone happens outside the sandbox
// so git credentials never enter it. Returns a cleanup function that removes
// the temp directory.
func cloneRepo(repo string) (gateway.Upload, func(), error) {
	repoName := strings.TrimSuffix(path.Base(repo), ".git")
	tmpDir, err := os.MkdirTemp("", "harness-repo-")
	if err != nil {
		return gateway.Upload{}, nil, fmt.Errorf("creating temp dir: %w", err)
	}
	cleanup := func() { os.RemoveAll(tmpDir) }

	cloneDir := filepath.Join(tmpDir, repoName)
	status.Infof("Repo:  %s", repo)

	cmd := exec.Command("git", "clone", "--depth", "1", repo, cloneDir)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		cleanup()
		return gateway.Upload{}, nil, fmt.Errorf("git clone %s: %w", repo, err)
	}

	status.OKf("Cloned %s", repoName)
	return gateway.Upload{Src: cloneDir, Dst: "/sandbox"}, cleanup, nil
}
