# GitHub pull-request reviewer

This task bundle reads a pull-request diff and lets the sandboxed agent post
inline review comments through OpenShell's GitHub REST proxy. The instructions
request at most three concrete comments; the policy restricts endpoints, not
comment count or finding quality. It grants no push, label, approval, or merge
operations. The trusted caller must obtain current PR metadata and stage the
diff as untrusted data.

## Layout

- `workflow/` contains the harness workflow document, OpenCode configuration, review
  skill, and deterministic fixture.
- `openshell/` contains the task policy, its security explanation, and an
  endpointless `github-review` provider profile. Render the repository and
  pull-request variables before applying the policy.
- The provider instance must exist when the sandbox starts. The current
  [`scripts/pr-review-local.sh`](../../scripts/pr-review-local.sh) wrapper creates it in a
  temporary workspace from a repository-scoped GitHub App token. A managed
  integration supplies the instance and its credential lifecycle through
  platform setup (see [managed CI](../../docs/ci.md#managed-reviewer-transition)). The profile contains metadata only, never a credential.
- Setup must also configure `inference.local` for the task's Gemini 2.5 Pro
  model. The task consumes that route without reconciling it.

[`scripts/pr-review.sh`](../../scripts/pr-review.sh) prepares and runs the
review against the configured target. The local wrapper supplies temporary
setup around its `run` command; managed callers supply platform setup.

The workflow is trusted host-side code. The diff and GitHub responses are
untrusted input and must never be treated as instructions. The only permitted
GitHub mutation is the exact pull-request comment endpoint in the rendered
policy. The token's GitHub repository permissions and the policy's PR-specific
HTTP methods and paths are separate controls.

Comments can be posted while the agent runs. The wrapper checks PR eligibility
before execution and rechecks it afterward; the final check is not a gate
before each comment. Cleanup deletes sandbox resources, not posted comments.
Retained artifacts and an execution result do not constitute approval or
independent validation of the findings.

## Native OpenShell shape

The same inputs can be used with the native OpenShell CLI:

```bash
openshell sandbox create \
  --from ghcr.io/nvidia/openshell-community/sandboxes/base@sha256:aeef1c63f00e2913ea002ccb3aaf925f338b5c5d70e63576f0d95c16a138044e \
  --policy /tmp/pr-review-policy.yaml \
  --provider github-review \
  -- opencode run --format json
```

Upload the skill, diff, and OpenCode configuration with native
`openshell sandbox upload` commands before starting the agent. The `harness` CLI
automates this composition and cleanup.
