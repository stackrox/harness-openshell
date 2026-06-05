package k8s

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/robbycochran/harness-openshell/internal/status"
	"gopkg.in/yaml.v3"
)

var transientErrors = []string{
	"connection refused",
	"connection reset",
	"timeout",
	"etcd leader changed",
	"the object has been modified",
	"unable to connect to the server",
	"TLS handshake timeout",
	"i/o timeout",
}

// Runner abstracts kubectl/helm/oc operations for testing.
type Runner interface {
	RunKubectl(ctx context.Context, args ...string) (string, error)
	RunKubectlOpts(ctx context.Context, opts KubectlOpts) (string, error)
	RunKubectlQuiet(ctx context.Context, args ...string) error
	RunKubectlPassthrough(ctx context.Context, args ...string) error
	RunHelm(ctx context.Context, args ...string) error
	RunOC(ctx context.Context, args ...string) error
	ApplyYAML(ctx context.Context, resources ...map[string]any) error
	SecretExists(ctx context.Context, name string) bool
	GetSecretField(ctx context.Context, secretName, field string) ([]byte, error)
	GetJSONPath(ctx context.Context, resource, jsonpath string) (string, error)
	NamespaceExists(ctx context.Context, ns string) bool
}

type Client struct {
	kubeconfig string
	namespace  string
}

func New(kubeconfig, namespace string) *Client {
	return &Client{kubeconfig: kubeconfig, namespace: namespace}
}

type KubectlOpts struct {
	Args  []string
	Stdin io.Reader
	Quiet bool
}

func (c *Client) RunKubectl(ctx context.Context, args ...string) (string, error) {
	return c.RunKubectlOpts(ctx, KubectlOpts{Args: args})
}

func (c *Client) RunKubectlOpts(ctx context.Context, opts KubectlOpts) (string, error) {
	args := opts.Args
	if c.namespace != "" && !containsFlag(args, "-n", "--namespace") {
		args = append([]string{"-n", c.namespace}, args...)
	}
	if c.kubeconfig != "" {
		args = append([]string{"--kubeconfig", c.kubeconfig}, args...)
	}

	status.Cmd("kubectl", args...)

	var lastErr error
	for attempt := range 3 {
		cmd := exec.CommandContext(ctx, "kubectl", args...)
		if opts.Stdin != nil {
			cmd.Stdin = opts.Stdin
		}

		var stdout, stderr bytes.Buffer
		cmd.Stderr = &stderr
		if opts.Quiet {
			cmd.Stdout = io.Discard
		} else {
			cmd.Stdout = &stdout
		}

		lastErr = cmd.Run()
		if lastErr == nil {
			return strings.TrimSpace(stdout.String()), nil
		}

		errOutput := stderr.String() + " " + lastErr.Error()
		if !isTransient(errOutput) {
			return "", fmt.Errorf("kubectl %s: %s", strings.Join(args, " "), strings.TrimSpace(stderr.String()))
		}

		if attempt < 2 {
			delay := time.Duration(1<<uint(attempt)) * time.Second
			time.Sleep(delay)
			if opts.Stdin != nil {
				seeker, ok := opts.Stdin.(io.Seeker)
				if !ok {
					return "", fmt.Errorf("kubectl %s: transient error but stdin is not seekable for retry: %s", strings.Join(opts.Args, " "), strings.TrimSpace(stderr.String()))
				}
				seeker.Seek(0, io.SeekStart)
			}
		}
	}
	return "", lastErr
}

func (c *Client) RunKubectlQuiet(ctx context.Context, args ...string) error {
	_, err := c.RunKubectlOpts(ctx, KubectlOpts{Args: args, Quiet: true})
	return err
}

func (c *Client) RunKubectlPassthrough(ctx context.Context, args ...string) error {
	if c.namespace != "" && !containsFlag(args, "-n", "--namespace") {
		args = append([]string{"-n", c.namespace}, args...)
	}
	if c.kubeconfig != "" {
		args = append([]string{"--kubeconfig", c.kubeconfig}, args...)
	}
	status.Cmd("kubectl", args...)
	cmd := exec.CommandContext(ctx, "kubectl", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (c *Client) RunHelm(ctx context.Context, args ...string) error {
	if c.namespace != "" && !containsFlag(args, "-n", "--namespace") {
		args = append(args, "-n", c.namespace)
	}
	if c.kubeconfig != "" {
		args = append(args, "--kubeconfig", c.kubeconfig)
	}
	status.Cmd("helm", args...)
	cmd := exec.CommandContext(ctx, "helm", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (c *Client) RunOC(ctx context.Context, args ...string) error {
	if c.kubeconfig != "" {
		args = append(args, "--kubeconfig", c.kubeconfig)
	}
	status.Cmd("oc", args...)
	cmd := exec.CommandContext(ctx, "oc", args...)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return cmd.Run()
}

func (c *Client) ApplyYAML(ctx context.Context, resources ...map[string]any) error {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	for _, r := range resources {
		if err := enc.Encode(r); err != nil {
			return fmt.Errorf("marshaling YAML: %w", err)
		}
	}
	enc.Close()

	_, err := c.RunKubectlOpts(ctx, KubectlOpts{
		Args:  []string{"apply", "-f", "-"},
		Stdin: bytes.NewReader(buf.Bytes()),
	})
	return err
}

func (c *Client) SecretExists(ctx context.Context, name string) bool {
	return c.RunKubectlQuiet(ctx, "get", "secret", name) == nil
}

func (c *Client) GetSecretField(ctx context.Context, secretName, field string) ([]byte, error) {
	jsonpath := fmt.Sprintf("{.data.%s}", strings.ReplaceAll(field, ".", "\\."))
	out, err := c.RunKubectl(ctx, "get", "secret", secretName, "-o", "jsonpath="+jsonpath)
	if err != nil {
		return nil, err
	}
	return base64.StdEncoding.DecodeString(out)
}

func (c *Client) GetJSONPath(ctx context.Context, resource, jsonpath string) (string, error) {
	return c.RunKubectl(ctx, "get", resource, "-o", "jsonpath="+jsonpath)
}

func (c *Client) NamespaceExists(ctx context.Context, ns string) bool {
	return c.RunKubectlQuiet(ctx, "get", "ns", ns) == nil
}

func isTransient(output string) bool {
	lower := strings.ToLower(output)
	for _, pattern := range transientErrors {
		if strings.Contains(lower, pattern) {
			return true
		}
	}
	return false
}

func containsFlag(args []string, flags ...string) bool {
	for _, a := range args {
		for _, f := range flags {
			if a == f {
				return true
			}
		}
	}
	return false
}

func DefaultNamespace() string {
	if ns := os.Getenv("OPENSHELL_NAMESPACE"); ns != "" {
		return ns
	}
	return "openshell"
}

