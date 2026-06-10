package gateway

import (
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"syscall"

	"github.com/robbycochran/harness-openshell/internal/status"
)

var ansiRE = regexp.MustCompile(`\x1b\[[0-9;]*m`)

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

func (c *CLI) CLIPath() string {
	path, err := exec.LookPath(c.bin)
	if err != nil {
		return ""
	}
	return path
}

func (c *CLI) InferenceGet() error {
	return c.silent("inference", "get")
}

func (c *CLI) InferenceModel() string {
	out, err := c.output("inference", "get")
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		cleaned := ansiRE.ReplaceAllString(line, "")
		if strings.Contains(cleaned, "Model:") {
			return strings.TrimSpace(strings.SplitN(cleaned, "Model:", 2)[1])
		}
	}
	return ""
}

func (c *CLI) ActiveGateway() string {
	out, err := c.output("gateway", "list")
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		cleaned := ansiRE.ReplaceAllString(line, "")
		if strings.HasPrefix(cleaned, "*") {
			fields := strings.Fields(cleaned)
			if len(fields) > 1 {
				return fields[1]
			}
		}
	}
	return ""
}

func (c *CLI) ProviderGet(name string) error {
	return c.silent("provider", "get", name)
}

func (c *CLI) ProviderCreate(name, providerType string, opts ProviderCreateOpts) error {
	args := []string{"provider", "create", "--name", name, "--type", providerType}
	if opts.FromADC {
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

func (c *CLI) ProviderDelete(name string) error {
	return c.silent("provider", "delete", name)
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

func (c *CLI) ProviderProfileDelete(id string) error {
	return c.silent("provider", "profile", "delete", id)
}

func (c *CLI) ProviderList() ([]string, error) {
	out, err := c.output("provider", "list")
	if err != nil {
		return nil, err
	}
	return parseFirstColumn(out), nil
}

func (c *CLI) InferenceRemove() error {
	return c.silent("inference", "remove")
}

func (c *CLI) InferenceSet(provider, model string) error {
	return c.passthrough("inference", "set", "--provider", provider, "--model", model, "--no-verify")
}

func (c *CLI) SettingsSet(key, value string) error {
	return c.passthrough("settings", "set", "--global", "--key", key, "--value", value, "--yes")
}

func (c *CLI) SandboxList() ([]string, error) {
	out, err := c.output("sandbox", "list")
	if err != nil {
		return nil, err
	}
	return parseFirstColumn(out), nil
}

func (c *CLI) GatewayList() ([]GatewayInfo, error) {
	out, err := c.output("gateway", "list")
	if err != nil {
		return nil, err
	}
	var gateways []GatewayInfo
	for i, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if i == 0 || strings.TrimSpace(line) == "" {
			continue
		}
		cleaned := strings.TrimSpace(ansiRE.ReplaceAllString(line, ""))
		active := strings.HasPrefix(cleaned, "*")
		fields := strings.Fields(strings.TrimPrefix(cleaned, "*"))
		if len(fields) >= 2 {
			gateways = append(gateways, GatewayInfo{
				Name:     fields[0],
				Endpoint: fields[1],
				Active:   active,
			})
		}
	}
	return gateways, nil
}

func (c *CLI) GatewayAdd(endpoint, name string, local, insecure bool) error {
	args := []string{"gateway", "add", endpoint, "--name", name}
	if local {
		args = append(args, "--local")
	}
	if insecure {
		args = append(args, "--insecure")
	}
	return c.silent(args...)
}

func (c *CLI) GatewayRemove(name string) error {
	return c.silent("gateway", "remove", name)
}

func (c *CLI) GatewaySelect(name string) error {
	return c.silent("gateway", "select", name)
}

func (c *CLI) SandboxCreate(opts SandboxCreateOpts) error {
	args := []string{"sandbox", "create", "--name", opts.Name}
	if opts.TTY {
		args = append(args, "--tty")
	} else {
		args = append(args, "--no-tty")
	}
	if opts.From != "" {
		args = append(args, "--from", opts.From)
	}
	for _, p := range opts.Providers {
		args = append(args, "--provider", p)
	}
	if !opts.Keep {
		args = append(args, "--no-keep")
	}
	if opts.UploadSrc != "" {
		args = append(args, "--upload", opts.UploadSrc+":"+opts.UploadDst, "--no-git-ignore")
	}
	if len(opts.Command) > 0 {
		args = append(args, "--")
		args = append(args, opts.Command...)
	}
	return c.passthrough(args...)
}

func (c *CLI) SandboxStatus() ([]SandboxInfo, error) {
	out, err := c.output("sandbox", "list")
	if err != nil {
		return nil, err
	}
	var infos []SandboxInfo
	for i, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if i == 0 || strings.TrimSpace(line) == "" {
			continue
		}
		cleaned := ansiRE.ReplaceAllString(line, "")
		fields := strings.Fields(cleaned)
		if len(fields) >= 2 {
			infos = append(infos, SandboxInfo{Name: fields[0], Phase: fields[1]})
		} else if len(fields) == 1 {
			infos = append(infos, SandboxInfo{Name: fields[0]})
		}
	}
	return infos, nil
}

func (c *CLI) SandboxLogs(name string, follow bool) error {
	args := []string{"sandbox", "logs"}
	if name != "" {
		args = append(args, name)
	}
	if follow {
		args = append(args, "--follow")
	}
	return c.passthrough(args...)
}

func (c *CLI) SandboxStop(name string) error {
	return c.silent("sandbox", "stop", name)
}

func (c *CLI) SandboxStart(name string) error {
	return c.silent("sandbox", "start", name)
}

func (c *CLI) SandboxDelete(name string) error {
	return c.silent("sandbox", "delete", name)
}

func (c *CLI) SandboxConnect(name string) error {
	path, err := exec.LookPath(c.bin)
	if err != nil {
		return err
	}
	args := []string{c.bin, "sandbox", "connect"}
	if name != "" {
		args = append(args, name)
	}
	return syscall.Exec(path, args, os.Environ())
}

func parseFirstColumn(out []byte) []string {
	var names []string
	for i, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if i == 0 || strings.TrimSpace(line) == "" {
			continue
		}
		cleaned := ansiRE.ReplaceAllString(line, "")
		if fields := strings.Fields(cleaned); len(fields) > 0 {
			names = append(names, fields[0])
		}
	}
	return names
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
