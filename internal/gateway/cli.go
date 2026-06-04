package gateway

import (
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

// CLI implements Gateway by shelling out to the openshell binary.
type CLI struct {
	bin string // path or name of the openshell binary
}

func NewCLI(bin string) *CLI {
	return &CLI{bin: bin}
}

func (c *CLI) InferenceGet() error {
	return c.silent("inference", "get")
}

func (c *CLI) ProviderGet(name string) error {
	return c.silent("provider", "get", name)
}

func (c *CLI) ProviderList() ([]string, error) {
	out, err := c.output("provider", "list")
	if err != nil {
		return nil, err
	}
	var names []string
	for i, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if i == 0 || strings.TrimSpace(line) == "" {
			continue // skip header
		}
		fields := strings.Fields(line)
		if len(fields) > 0 {
			names = append(names, fields[0])
		}
	}
	return names, nil
}

func (c *CLI) SandboxCreate(opts SandboxCreateOpts) error {
	args := []string{"sandbox", "create", "--name", opts.Name}
	if opts.TTY {
		args = append(args, "--tty")
	} else {
		args = append(args, "--no-tty")
	}
	if opts.Image != "" {
		args = append(args, "--from", opts.Image)
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

func (c *CLI) SandboxUpload(name, localDir, remotePath string) error {
	return c.passthrough("sandbox", "upload", name, localDir, remotePath, "--no-git-ignore")
}

func (c *CLI) SandboxExec(name string, command ...string) error {
	args := []string{"sandbox", "exec", "--name", name, "--"}
	args = append(args, command...)
	return c.passthrough(args...)
}

// passthrough runs the CLI with stdin/stdout/stderr connected.
func (c *CLI) passthrough(args ...string) error {
	cmd := exec.Command(c.bin, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// silent runs the CLI with all output discarded.
func (c *CLI) silent(args ...string) error {
	cmd := exec.Command(c.bin, args...)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return cmd.Run()
}

// output runs the CLI and returns stdout.
func (c *CLI) output(args ...string) ([]byte, error) {
	cmd := exec.Command(c.bin, args...)
	cmd.Stderr = io.Discard
	return cmd.Output()
}
