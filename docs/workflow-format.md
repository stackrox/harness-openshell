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
- `agent` is the command executed in the sandbox.
- `source` optionally uploads a repository checkout.
- `payloads` uploads host files or inline content to sandbox destinations.

String values may contain `${VAR}` references resolved from the calling
process environment. Harness does not load `.env` files implicitly.

## Security contract

Workflow files contain provider names, never provider credential values. The
gateway owns credentials and exposes masked proxy behavior to authorized
sandbox requests. Raw credentials must not appear in workflow YAML, sandbox
environment values, payloads, agent arguments, logs, artifacts, prompts, or
structured output.

## Compatibility policy

The Go parser in `internal/config` is the executable source of truth. Parser,
plan, apply, and redaction tests are the format contract. A future incompatible
shape increments `version` and fails clearly; there is no migration layer.
