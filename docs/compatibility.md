# Compatibility

Last reviewed: 2026-08-24.

| Component | Repository baseline | Locally exercised | Latest source reviewed | Status |
|---|---|---|---|---|
| OpenShell CLI and local gateway | `0.0.110` (`.openshell-version`) | CI `local`+`kind` e2e | `0.0.111` (release) / `0.0.112` (tag) | re-baselined from `0.0.85` to `0.0.110`; `0.0.111` deferred — see note below |
| OpenShell Go SDK (`go.mod`) | `v0.0.0-20260820101241-7909fb5d0f54` (= `v0.0.110` tag) | via unit tests / fake client | matches CLI baseline | aligned with CLI baseline (skew closed) |
| Agent Control Plane | no runtime dependency | not installed | `101c0ec` | manifest schema and `acpctl apply` source reviewed; parser/runtime conformance remains open |
| Go | `1.25.0` (toolchain `1.26.1`, `go.mod`) | `1.25`/`1.26` | n/a | build, unit tests, vet, lint, and config suite pass |

## OpenShell v0.0.111 deferred (sandbox-create breaking change)

The re-baseline targets `0.0.110`, not the latest `0.0.111`. In `0.0.111`,
`openshell sandbox create` makes `--upload` **mutually exclusive** with a trailing
`-- <command>` (`upload: Vec<String>` gains `conflicts_with = "command"`), and adds
a `--detach` flag. The harness always uploads config/payloads *and* runs a command
(`true` headless, `bash run.sh` for task/attach), so every create path breaks at
`0.0.111`. The break landed in `0.0.111` only — every release `0.0.86`–`0.0.110`
keeps the current idiom working. Adopting `0.0.111` requires reworking the create
flow to create-detached → `sandbox upload` → `sandbox exec`/`connect`; that is a
separate, e2e-validated change, tracked with the modernization re-baseline task.

## OpenShell policy compatibility

Harness policy documents stay in the upstream OpenShell policy YAML schema.
OpenShell `0.0.110` includes policy middleware and keeps Z3-backed proving in the
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
