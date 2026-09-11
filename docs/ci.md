# CI and live validation

Credential-free PR checks do not establish live inference success.
Image PR checks build without registry login or publication; only
main/tag pushes publish images and update the shared registry cache.

## HyperShell validation

HyperShell validation runs locally from the Red Hat network because its OIDC
issuer is VPN-only. The Harness workflow connects directly through the
OpenShell Go SDK. It does not persist a gateway registration or use a gateway
administrator account at runtime.

## GitHub-hosted Vertex smoke test

`.github/workflows/vertex-smoke.yml` is manually dispatched only. It starts a
local OpenShell gateway on a GitHub-hosted runner, obtains a short-lived Google
access token from `VERTEX_AI_SERVICE_ACCOUNT_KEY`, and uses it to run OpenCode
with Gemini 2.5 Pro through `inference.local`. It creates and deletes an
isolated workspace, so it does not affect the gateway's default workspace.

Repository configuration:

- secret: `VERTEX_AI_SERVICE_ACCOUNT_KEY` — JSON key for the dedicated Vertex
  service account;
- variables: `VERTEX_AI_PROJECT_ID`, `VERTEX_AI_REGION`.

The test script accepts `GOOGLE_VERTEX_AI_TOKEN`, not a service-account key.
This keeps the OpenShell boundary identical when the key bootstrap is later
replaced with GitHub Workload Identity Federation.

Run the same smoke locally with a service-account file, without changing the
default gcloud identity (do not enable shell tracing around credentials):

```bash
export GOOGLE_APPLICATION_CREDENTIALS=/path/to/service-account.json
export VERTEX_AI_PROJECT_ID=YOUR_VERTEX_PROJECT
export VERTEX_AI_REGION=global
export GOOGLE_VERTEX_AI_TOKEN="$(gcloud auth application-default print-access-token)"
make test-vertex-gemini-opencode
unset GOOGLE_VERTEX_AI_TOKEN
```

The selected Vertex project must grant this service account prediction access;
it need not be the project that owns the account. A successful local run proves
the OpenCode/Gemini runtime path, not the Claude reviewer or review quality.
The smoke validates the final response marker, propagates agent and cleanup
failures, and attempts cleanup on SIGINT/SIGTERM. Forced termination (SIGKILL or
runner loss) cannot execute shell cleanup. Credential-free orchestration tests
run as part of `go test ./...`.

The service-account project must have access to `gemini-2.5-pro` in the
configured Vertex region. A 404 from the inference setup means the model is
unavailable to that project; do not bypass the check with `--no-verify`.

## Label-driven PR review

The review workflow mints a short-lived GitHub App installation token before
fetching the diff or creating the OpenShell workspace. Configure these values
in each consuming repository:

- variable: `OPENSHELL_GITHUB_APP_CLIENT_ID` — the GitHub App Client ID;
- secret: `OPENSHELL_GITHUB_APP_PRIVATE_KEY` — the complete PEM private key.

The App installation must grant `Contents: read` and `Pull requests: read and
write` repository permissions, and include the repository being reviewed. The
workflow requests only those permissions when minting the installation token.

The workflow accepts the App Client ID as
`openshell-github-app-client-id`; callers explicitly forward only
`VERTEX_AI_SERVICE_ACCOUNT_KEY` and `OPENSHELL_GITHUB_APP_PRIVATE_KEY`. The
token is used on the trusted host for `gh` and native provider bootstrap, then
passed to OpenShell as the provider credential. It is never included in
sandbox environment variables, payloads, agent arguments, or artifacts.
Installation tokens expire after one hour and are revoked by the token action
after the job.

The Harness repository's `ai-review.yml` is a thin caller of the pinned GitHub
review workflow, so `pull_request_target` runs use the same path as consuming
repositories. Changes to that caller are exercised after they reach the default
branch; before then, use `actionlint` and the local `scripts/pr-review.sh`
commands below. Normal reviews remain `pull_request_target` runs from the
default branch.

Once `AI review` is on the default branch, add `ai-review` to an open, non-draft
PR. It reviews the full diff on labeling and each pushed head; newer runs cancel
older ones. Removing the label, closing, or drafting the PR disables review.
It uses the Vertex secret/variables above. Summaries show status, head SHA, and
an artifact link. Seven-day artifacts hold input revisions, diff/hash, execution
metadata, raw output/diagnostics, and `review.txt`. Reviews are advisory inline
comments only; they do not approve, request changes, or merge.

The reviewer runs Claude Code through `inference.local` and Google Vertex AI.
The workflow keeps `REVIEW_MODEL` and `REVIEW_CLI_MODEL` explicit; the currently
validated default is `claude-haiku-4-5@20251001` / `haiku`. Vertex identifies
Sonnet 4.5 as `claude-sonnet-4-5@20250929`; switch both values together only
after the CI service account can invoke that model.

Only trusted default-branch code runs on the host. The pinned sandbox receives
the PR diff and a PR-scoped GitHub token; OpenShell permits only inline comment
POSTs to that exact PR. Label/head/base are rechecked before execution and
publication. Diffs over 256 KiB are rejected; execution and diagnostic output
are bounded. The
completion check rejects errors, tool calls, empty or truncated responses—not
incorrect findings. Artifacts remain unvalidated model output. Cleanup covers
success, failure, and normal cancellation, but cannot guarantee runner-loss cleanup.

Locally, use `gh` authentication, `jq`, GNU `timeout` (Homebrew `coreutils` on
macOS), and the Vertex token/project variables above. Use a new absolute artifact
directory each time and an open, non-draft PR carrying `ai-review`:

```bash
make cli
export REVIEW_REPOSITORY=stackrox/harness-openshell REVIEW_PR=123
export REVIEW_DIR="$PWD/review-artifacts-123"
bash scripts/pr-review.sh prepare
bash scripts/pr-review.sh run
```

Unit tests use fake commands, not Vertex. Structured findings and publication
are deferred.

## One-time platform bootstrap

A gateway administrator adds the CI service-account subject to the `default`
workspace with the `user` role. This membership is gateway state and should be
managed centrally, not recreated by each repository run.

The default workspace is intentionally implicit in
`test/hypershell-workflow.yaml`. Use a dedicated workspace when repository
isolation becomes more important than the low-friction shared pilot.

### Vertex inference base layer

`test/hypershell-haiku-workflow.yaml` exercises Claude Haiku through Vertex in
the dedicated `default-inference` workspace. Before an ordinary workspace user
can apply it, a platform administrator must establish three durable resources:

1. add the harness service-account subject to `default-inference` as `user`;
2. create the `vertex-claude-haiku` provider from an identity with Vertex AI
   prediction access; and
3. set `inference.local` to provider `vertex-claude-haiku` and model
   `claude-haiku-4-5@20251001`.

For local development credentials, OpenShell's native bootstrap is:

```bash
openshell provider create --gateway ADMIN_GATEWAY \
  --workspace default-inference \
  --name vertex-claude-haiku \
  --type google-vertex-ai \
  --from-gcloud-adc \
  --config VERTEX_AI_PROJECT_ID=PROJECT_ID \
  --config VERTEX_AI_REGION=us-east5

openshell inference set --gateway ADMIN_GATEWAY \
  --workspace default-inference \
  --provider vertex-claude-haiku \
  --model 'claude-haiku-4-5@20251001'
```

Do not use `--no-verify`: a successful inference write is the base-layer proof
that the ADC principal has `aiplatform.endpoints.predict`. After bootstrap,
ordinary applies only read the matching provider and route; they neither need
workspace-admin permission nor receive the Vertex credential in the sandbox.
If a workflow selects a different provider, model, or route, the compatibility
reconciliation performs an admin-only upsert in that workspace. Treat that as
isolated-workspace setup, not a shared-workspace runtime operation; Harness does
not restore the previous route after the run.

Validate from the VPN with:

```bash
make test-hypershell-haiku HYPERSHELL_SA_ENV=path/to/user-sa.env
```

## Runtime environment

Provide these values to the local harness process:

- `HYPERSHELL_GATEWAY`: HTTPS gateway endpoint
- `OPENSHELL_OIDC_ISSUER`: HTTPS OIDC issuer
- `OPENSHELL_OIDC_AUDIENCE`: gateway token audience
- `OPENSHELL_OIDC_CLIENT_ID`: user service-account client ID
- `OPENSHELL_OIDC_CLIENT_SECRET`: user service-account client secret

`test/hypershell-lifecycle.sh` reads them from the git-excluded file named by
`HYPERSHELL_SA_ENV` and maps the non-secret connection metadata to the names
used by the workflow. The client secret remains in
`OPENSHELL_OIDC_CLIENT_SECRET`; it is not represented in the workflow document,
plan, or command output. Administrator credentials remain outside repository CI
and ordinary validation.

This document is also used to exercise the label-driven artifact-only review
workflow on a small documentation-only change.

## Workflow contract

The reusable portion is the `target` block in `test/hypershell-workflow.yaml`.
Its `registration` field supplies non-secret, in-memory connection metadata and
does not create persistent CLI state. An omitted `workspace` selects `default`.
