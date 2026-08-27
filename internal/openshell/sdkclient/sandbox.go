package sdkclient

import (
	"context"

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
