package gateway

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"

	"github.com/stackrox/harness-openshell/internal/status"
)

// ErrVersionBelowMinimum is returned (wrapped) by CheckMinVersion when the
// installed openshell CLI is definitively older than the required minimum.
// Callers can distinguish this from a version we simply couldn't read
// (empty/unparseable output) via errors.Is and treat it as a hard failure.
var ErrVersionBelowMinimum = errors.New("openshell version below minimum")

// MinOpenShellVersion is the lowest openshell CLI/gateway version the harness
// supports. It is kept in lockstep with the repo-root .openshell-version pin
// (the single source of truth that CI and `make openshell` also read);
// TestMinOpenShellVersionMatchesPin fails if the two drift. Re-baseline both
// together.
const MinOpenShellVersion = "0.0.110"

// CLI implements Gateway by shelling out to the openshell binary.
type CLI struct {
	bin string // path or name of the openshell binary
}

func New(bin string) *CLI {
	return &CLI{bin: bin}
}

func (c *CLI) CLIVersion() string {
	out, err := c.output("--version")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func ParseCLIVersion(raw string) string {
	raw = strings.TrimSpace(raw)
	if i := strings.LastIndex(raw, "v"); i >= 0 {
		return raw[i+1:]
	}
	if i := strings.LastIndex(raw, " "); i >= 0 {
		return raw[i+1:]
	}
	return raw
}

func parseVersionParts(v string) (major, minor, patch int, ok bool) {
	parts := strings.SplitN(v, ".", 3)
	if len(parts) != 3 {
		return 0, 0, 0, false
	}
	var err error
	major, err = strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, 0, false
	}
	minor, err = strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, 0, false
	}
	patch, err = strconv.Atoi(parts[2])
	if err != nil {
		return 0, 0, 0, false
	}
	return major, minor, patch, true
}

func (c *CLI) CheckMinVersion(minVersion string) error {
	raw := c.CLIVersion()
	if raw == "" {
		return fmt.Errorf("could not determine openshell version")
	}
	installed := ParseCLIVersion(raw)
	iMaj, iMin, iPatch, ok := parseVersionParts(installed)
	if !ok {
		return fmt.Errorf("could not parse openshell version %q", installed)
	}
	mMaj, mMin, mPatch, ok := parseVersionParts(minVersion)
	if !ok {
		return fmt.Errorf("invalid minimum version %q", minVersion)
	}
	if iMaj < mMaj || (iMaj == mMaj && iMin < mMin) || (iMaj == mMaj && iMin == mMin && iPatch < mPatch) {
		return fmt.Errorf("openshell %s is below minimum %s (upgrade: openshell update): %w", installed, minVersion, ErrVersionBelowMinimum)
	}
	return nil
}

func (c *CLI) ProviderGet(name string) error {
	return c.silent("provider", "get", name)
}

func (c *CLI) ProviderCreate(name, providerType string, opts ProviderCreateOpts) error {
	args := []string{"provider", "create", "--name", name, "--type", providerType}
	if opts.FromExisting {
		args = append(args, "--from-existing")
	} else if opts.FromADC {
		args = append(args, "--from-gcloud-adc")
	}
	for _, cred := range opts.Credentials {
		args = append(args, "--credential", cred)
	}
	for _, cfg := range opts.Configs {
		args = append(args, "--config", cfg)
	}
	return c.passthrough(args...)
}

func (c *CLI) ProviderProfileImport(dir string) error {
	return c.silent("provider", "profile", "import", "--from", dir)
}

func (c *CLI) ProviderRefreshConfigure(name string, opts ProviderRefreshOpts) error {
	args := []string{"provider", "refresh", "configure", name,
		"--credential-key", opts.CredentialKey,
		"--strategy", opts.Strategy,
	}
	for _, m := range opts.Material {
		args = append(args, "--material", m)
	}
	for _, k := range opts.SecretMaterialKeys {
		args = append(args, "--secret-material-key", k)
	}
	return c.passthrough(args...)
}

func (c *CLI) ProviderRefreshRotate(name, credentialKey string) error {
	return c.silent("provider", "refresh", "rotate", name, "--credential-key", credentialKey)
}

func sandboxCreateArgs(opts SandboxCreateOpts) []string {
	args := []string{"sandbox", "create", "--name", opts.Name}
	if opts.Gateway != "" {
		args = append(args, "--gateway", opts.Gateway)
	}
	if opts.Workspace != "" {
		args = append(args, "--workspace", opts.Workspace)
	}
	if opts.Policy != "" {
		args = append(args, "--policy", opts.Policy)
	}
	if opts.From != "" {
		args = append(args, "--from", opts.From)
	}
	for _, p := range opts.Providers {
		args = append(args, "--provider", p)
	}
	if opts.NoAutoProviders {
		args = append(args, "--no-auto-providers")
	}
	if opts.TTY {
		args = append(args, "--tty")
	} else {
		args = append(args, "--no-tty")
	}
	if len(opts.Env) > 0 {
		keys := make([]string, 0, len(opts.Env))
		for k := range opts.Env {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			args = append(args, "--env", k+"="+opts.Env[k])
		}
	}
	if len(opts.Uploads) > 0 {
		for _, u := range opts.Uploads {
			args = append(args, "--upload", u.Src+":"+u.Dst)
		}
		args = append(args, "--no-git-ignore")
	}
	if !opts.Keep {
		args = append(args, "--no-keep")
	}
	if len(opts.Labels) > 0 {
		keys := make([]string, 0, len(opts.Labels))
		for k := range opts.Labels {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			args = append(args, "--label", k+"="+opts.Labels[k])
		}
	}
	if len(opts.Command) > 0 {
		args = append(args, "--")
		args = append(args, opts.Command...)
	}
	return args
}

func (c *CLI) SandboxCreate(opts SandboxCreateOpts) error {
	args := sandboxCreateArgs(opts)
	return c.passthrough(args...)
}

func (c *CLI) SandboxDelete(name string) error {
	return c.silent("sandbox", "delete", name)
}

// passthrough runs the CLI with stdin/stdout/stderr connected.
func (c *CLI) passthrough(args ...string) error {
	status.Cmd(c.bin, args...)
	cmd := exec.Command(c.bin, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// silent runs the CLI with all output discarded.
func (c *CLI) silent(args ...string) error {
	status.Cmd(c.bin, args...)
	cmd := exec.Command(c.bin, args...)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return cmd.Run()
}

// output runs the CLI and returns stdout.
func (c *CLI) output(args ...string) ([]byte, error) {
	status.Cmd(c.bin, args...)
	cmd := exec.Command(c.bin, args...)
	cmd.Stderr = io.Discard
	return cmd.Output()
}
