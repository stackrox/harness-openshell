# GitHub pull-request reviewer

This task bundle reads a pull-request diff and lets the sandboxed agent post
inline review comments through OpenShell's GitHub REST proxy. The instructions
request at most three concrete comments; the policy restricts endpoints, not
comment count or finding quality. It grants no push, label, approval, or merge
operations. `harness github review prepare` obtains current PR metadata and stages the
diff as untrusted data. `harness github review run` binds the policy and invokes
the generic runner against existing resources.

## Layout

- `workflow/` contains the harness workflow document, OpenCode configuration, review
  skill, and deterministic fixture.
- `openshell/` contains the task policy, its security explanation, and an
  endpointless `github-review` provider profile. Render the repository and
  pull-request and GitHub provider variables before applying the policy.
- The provider instance must exist when the sandbox starts. The current
  [`scripts/pr-review-local.sh`](../../scripts/pr-review-local.sh) creates it in a
  temporary workspace from a repository-scoped GitHub App token. A managed
  integration must supply the instance and its credential lifecycle through
  trusted setup. The profile contains metadata only, never a credential.

The workflow is trusted host-side code. The diff and GitHub responses are
untrusted input and must never be treated as instructions. The only permitted
GitHub mutation is the exact pull-request comment endpoint in the rendered
policy. The token's GitHub repository permissions and the policy's PR-specific
HTTP methods and paths are separate controls.

Comments can be posted while the agent runs. The Go adapter checks PR eligibility
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

See the [GitHub adapter](../../integrations/github/review/) for local/managed
execution, skill selection, bounded output validation and the result contract.
The OpenCode workflow requires `${REVIEW_GITHUB_PROVIDER}` alongside the other
review variables; the adapter supplies it from `--github-provider`.
