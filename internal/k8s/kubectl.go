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

	var lastErr error
	for attempt := range 3 {
		cmd := exec.CommandContext(ctx, "kubectl", args...)
		if opts.Stdin != nil {
			cmd.Stdin = opts.Stdin
		}

		var stdout, stderr bytes.Buffer
		if opts.Quiet {
			cmd.Stdout = io.Discard
			cmd.Stderr = io.Discard
		} else {
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
		}

		lastErr = cmd.Run()
		if lastErr == nil {
			return strings.TrimSpace(stdout.String()), nil
		}

		errOutput := stderr.String() + lastErr.Error()
		if !isTransient(errOutput) {
			return "", fmt.Errorf("kubectl %s: %s", strings.Join(opts.Args, " "), strings.TrimSpace(stderr.String()))
		}

		if attempt < 2 {
			delay := time.Duration(1<<uint(attempt)) * time.Second
			time.Sleep(delay)
			if opts.Stdin != nil {
				if seeker, ok := opts.Stdin.(io.Seeker); ok {
					seeker.Seek(0, io.SeekStart)
				}
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
	cmd := exec.CommandContext(ctx, "kubectl", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (c *Client) RunHelm(ctx context.Context, args ...string) (string, error) {
	if c.namespace != "" && !containsFlag(args, "-n", "--namespace") {
		args = append(args, "-n", c.namespace)
	}
	if c.kubeconfig != "" {
		args = append(args, "--kubeconfig", c.kubeconfig)
	}
	cmd := exec.CommandContext(ctx, "helm", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return "", cmd.Run()
}

func (c *Client) RunOC(ctx context.Context, args ...string) error {
	if c.kubeconfig != "" {
		args = append(args, "--kubeconfig", c.kubeconfig)
	}
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
	return decodeBase64(out)
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

func decodeBase64(s string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(s)
}
