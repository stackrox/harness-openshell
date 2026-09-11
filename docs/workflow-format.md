# Workflow format

This repository accepts one document shape: a version 1 OpenShell workflow.
The CLI command already identifies the document type, so the format does not
use Kubernetes-style `kind`, `apiVersion`, `metadata`, or `spec` wrappers.

## Minimal shape

```yaml
version: 1
name: pr-review

target:
  gateway: openshell
  workspace: default

inference:
  route: inference.local
  provider: vertex-review
  model: gemini-2.5-pro

sandbox:
  image: quay.io/example/reviewer:v1
  providers: [github-review]

agent:
  type: opencode
  args: [run, --format, json]

outputs:
  - source: /sandbox/artifacts
    destination: artifacts
    required: false
```

`version` must be `1` and `name` is required. All other top-level fields are
optional. Unknown fields are rejected so a typo cannot silently change a run.

## Fields

- `target` selects the gateway and workspace. Explicit CLI flags and
  `OPENSHELL_*` environment variables take precedence over these values.
- `inference` selects the gateway inference route and model when needed. Its
  `provider` must already exist in OpenShell.
- `sandbox` describes the image, policy, environment, provider attachments,
  payload handling, and cleanup behavior for a run.
- `sandbox.providers` names providers that must already exist in OpenShell and
  attaches their masked proxies to the sandbox.
- `sandbox.keep` defaults to `false`; set it to `true` only to retain a sandbox
  for debugging.
- `agent` is the command executed in the sandbox.
- `source` optionally uploads a repository checkout.
- `payloads` uploads host files or inline content to sandbox destinations.
- `outputs` downloads sandbox paths after the agent exits. `source` must be an
  absolute path below `/sandbox`; `destination` is a relative path below the
  host directory passed with `--output-dir`. Outputs default to required;
  `required: false` allows a missing path without failing the run. Existing
  host destinations, traversal paths, links, and archive entries outside the
  requested path are rejected. Sources are exact files or directories; there
  is no glob syntax. Downloads are bounded to protect the host.

Run a workflow with outputs by choosing an explicit host directory:

```bash
harness workflow apply workflow.yaml --output-dir ./workflow-artifacts
```

Downloads happen before the sandbox cleanup step, including when the agent
fails, so a workflow can preserve diagnostics while still returning a failed
run. The current SDK adapter transfers files through OpenShell's authenticated
SSH tunnel; it does not invoke the OpenShell CLI or copy gateway credentials.

String values may contain `${VAR}` references resolved from the calling
process environment. Harness does not load `.env` files implicitly.

## Security contract

Workflow files contain provider names, never provider credential values. The
gateway owns credentials and exposes masked proxy behavior to authorized
sandbox requests. Raw credentials must not appear in workflow YAML, sandbox
environment values, payloads, agent arguments, logs, artifacts, prompts, or
structured output.

Workflow, policy, and payload declarations are trusted host-side inputs. Do not
run an untrusted PR-supplied workflow with a credentialed host context; the
trusted PR-review workflow checks out its workflow from the default branch and
stages the PR diff as data. Interpolated values are redacted from display
projections, but Harness does not attempt to detect credentials embedded as
literal YAML values.

Inference route reconciliation currently writes a changed route and therefore
requires workspace-admin access. Shared workspaces should use a matching
bootstrap-owned route; isolated workspaces may use the compatibility write.

## Compatibility policy

The Go parser in `internal/config` is the executable source of truth. Parser,
plan, apply, and redaction tests are the format contract. A future incompatible
shape increments `version` and fails clearly; there is no migration layer.
