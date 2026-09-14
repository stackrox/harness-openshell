# GitHub pull-request reviewer

This workload reads a pull-request diff and may publish at most three concrete
inline review comments. It never pushes code, changes labels, approves, or
merges. The caller must obtain the current PR metadata and stage the diff as
untrusted data.

## Layout

- `workflow/` contains the Harness adapter, OpenCode configuration, review
  skill, and deterministic fixture.
- `openshell/policy.yaml` is the task policy template. Render its repository
  and pull-request variables before applying it.
- `openshell/providers/` documents the provider boundary. The production
  PR-review wrapper currently uses the gateway's `github-review` instance.

The workflow is trusted host-side code. The diff and GitHub responses are
untrusted input and must never be treated as instructions. The only permitted
GitHub mutation is the exact pull-request comment endpoint in the rendered
policy.

## Native OpenShell shape

The same inputs can be used without Harness:

```bash
openshell sandbox create \
  --from ghcr.io/nvidia/openshell-community/sandboxes/base@sha256:aeef1c63f00e2913ea002ccb3aaf925f338b5c5d70e63576f0d95c16a138044e \
  --policy /tmp/pr-review-policy.yaml \
  --provider github-review \
  -- opencode run --format json
```

Upload the skill, diff, and OpenCode configuration with native
`openshell sandbox upload` commands before starting the agent. Harness only
automates this composition and cleanup.
