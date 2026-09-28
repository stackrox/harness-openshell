# Compatibility

Last reviewed: 2026-09-28.

| Component | Repository baseline | Locally exercised | Latest source reviewed | Status |
|---|---|---|---|---|
| OpenShell CLI and local gateway | `0.1.2` (`.openshell-version`) | static/unit checks; live `local`+`kind` e2e pending | `0.1.2` (release) | upgraded from `0.0.110`; v0.1.x migration applied |
| OpenShell Go SDK (`go.mod`) | `v0.0.0-20260928030816-6648bd0c290e` (= `v0.1.2` source tag) | via unit tests / fake client | matches CLI baseline | aligned with CLI baseline |
| Agent Control Plane | no runtime dependency | not installed | `101c0ec` | manifest schema and `acpctl apply` source reviewed; parser/runtime conformance remains open |
| Go | `1.26.0` (`go.mod`) | `1.26.8` | n/a | build, unit tests, vet, shell/action checks, and config suite pass; golangci-lint is blocked by the installed config parser |

## OpenShell v0.1.x migration

OpenShell `0.1.2` is a breaking release line with coordinated CLI, gateway, and
SDK changes. This repository now pins the CLI/gateway baseline and Go SDK to
`0.1.2`. Existing sandboxes must be recreated when changing from the `0.0.x`
line; the integration tests provision their own sandboxes.

The earlier `0.0.111` CLI change made `sandbox create --upload` incompatible with
a trailing command. The harness's canonical path now creates sandboxes, uploads
files, and executes commands through the Go SDK, so that CLI-only constraint does
not apply to the workflow runner. The CLI remains used for gateway provisioning
and inspection in the integration scripts.

## OpenShell policy compatibility

Harness policy documents stay in the upstream OpenShell policy YAML schema.
OpenShell `0.1.2` baseline includes policy middleware and keeps Z3-backed proving in the
gateway/prover path; the client-side bundled Z3 integration was removed. The
harness does not introduce a policy dialect or its own solver.

ACP's control plane currently vendors an OpenShell policy protobuf that may lag
the newest `openshell-policy` YAML field names. ACP export preserves the source
policy fields under `Policy.spec`; end-to-end policy conformance should remain
open until tested against a running ACP revision.

## ACP manifest compatibility

The reviewed ACP `Resource` schema accepts `Agent` fields for `prompt`,
`providers`, `payloads`, `environment`, `repo_url`, `entrypoint`, and
`sandbox_policy`, plus a `Policy.spec`.

At commit `101c0ec`, `acpctl apply` consumes prompt, providers, payloads,
environment, and sandbox policy. Its create/patch implementation does not
consume the declared `repo_url` or `entrypoint` fields. Harness export retains
those fields so intent is visible and automatically becomes effective when ACP
fixes the consumer.

ACP gateway discovery and OpenShell CLI registration belong to
`acpctl gateway setup-cli`; the harness should not duplicate that behavior.
