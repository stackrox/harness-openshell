package sdkclient

import (
	"context"
	"fmt"
	"io"

	v1 "github.com/NVIDIA/OpenShell/sdk/go/openshell/v1"

	"github.com/stackrox/harness-openshell/internal/openshell"
)

// fromSDKSandbox maps the SDK sandbox view to the harness Sandbox. It reads the
// top-level Name (the resource name, always populated) rather than
// Status.SandboxName (a status echo that can be empty before Ready), and carries
// the lifecycle phase through as a string. Everything else the SDK holds (Spec,
// Labels, Conditions, ResourceVersion, ...) is dropped at this boundary
// (least-exposure firewall — see openshell.Sandbox).
func fromSDKSandbox(s *v1.Sandbox) openshell.Sandbox {
	return openshell.Sandbox{
		Name:  s.Name,
		Phase: string(s.Status.Phase),
	}
}

// Sandboxes lists the sandboxes in the bound workspace.
func (c *client) Sandboxes(ctx context.Context) ([]openshell.Sandbox, error) {
	raw, err := c.raw.Sandboxes().List(ctx, c.workspace)
	if err != nil {
		return nil, translate(err)
	}
	out := make([]openshell.Sandbox, 0, len(raw))
	for _, s := range raw {
		out = append(out, fromSDKSandbox(s))
	}
	return out, nil
}

// GetSandbox reads the named sandbox in the bound workspace, mapping a missing
// sandbox to openshell.ErrNotFound (via translate).
func (c *client) GetSandbox(ctx context.Context, name string) (openshell.Sandbox, error) {
	s, err := c.raw.Sandboxes().Get(ctx, c.workspace, name)
	if err != nil {
		return openshell.Sandbox{}, translate(err)
	}
	return fromSDKSandbox(s), nil
}

// DeleteSandbox removes the named sandbox in the bound workspace.
func (c *client) DeleteSandbox(ctx context.Context, name string) error {
	return translate(c.raw.Sandboxes().Delete(ctx, c.workspace, name))
}

// CreateSandbox maps the harness-owned SDK-native creation subset to the
// upstream SDK. Direct SDK calls do not perform CLI-side provider discovery;
// only the explicitly declared providers are sent.
func (c *client) CreateSandbox(ctx context.Context, desired openshell.SandboxCreate) (openshell.Sandbox, error) {
	s, err := c.raw.Sandboxes().Create(ctx, c.workspace, desired.Name, &v1.SandboxSpec{
		Environment: copySandboxStrings(desired.Env),
		Template: &v1.SandboxTemplate{
			Image: desired.Image,
		},
		Providers: append([]string(nil), desired.Providers...),
	}, copySandboxStrings(desired.Labels))
	if err != nil {
		return openshell.Sandbox{}, translate(err)
	}
	return fromSDKSandbox(s), nil
}

// WaitSandboxReady blocks on the SDK lifecycle helper and maps the result.
func (c *client) WaitSandboxReady(ctx context.Context, name string) (openshell.Sandbox, error) {
	s, err := c.raw.Sandboxes().WaitReady(ctx, c.workspace, name)
	if err != nil {
		return openshell.Sandbox{}, translate(err)
	}
	return fromSDKSandbox(s), nil
}

// ExecSandbox streams a non-interactive command through the SDK, preserving
// stdout and stderr separation without buffering the complete agent run.
func (c *client) ExecSandbox(ctx context.Context, name string, command []string, stdout, stderr io.Writer) (int, error) {
	stream, err := c.raw.Exec().Stream(ctx, c.workspace, name, append([]string(nil), command...))
	if err != nil {
		return -1, translate(err)
	}
	defer stream.Close()

	for {
		chunk, err := stream.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return -1, translate(err)
		}
		var dst io.Writer
		switch chunk.Stream {
		case v1.StreamStdout:
			dst = stdout
		case v1.StreamStderr:
			dst = stderr
		default:
			return -1, fmt.Errorf("unsupported sandbox output stream %q", chunk.Stream)
		}
		if _, err := dst.Write(chunk.Data); err != nil {
			return -1, fmt.Errorf("writing sandbox %s: %w", chunk.Stream, err)
		}
	}
	exitCode, err := stream.ExitCode()
	if err != nil {
		return -1, translate(err)
	}
	return exitCode, nil
}

func copySandboxStrings(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}
