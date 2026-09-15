# CI and live validation

Credential-free PR checks do not establish live inference success.
Image PR checks build without registry login or publication; only
main/tag pushes publish images and update the shared registry cache.

## HyperShell validation

HyperShell is the managed OpenShell environment used by these validation
examples. Validation runs locally from the Red Hat network because its OIDC
issuer is VPN-only. The `harness` CLI connects directly through the
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
the OpenCode/Gemini runtime path; review publication and quality need their own
validation.
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

The token is repository-scoped; OpenShell's rendered REST policy further
restricts sandbox requests to the selected PR's allowed read and inline-comment
endpoints. The Actions job's `contents: read` permission is a separate token
boundary from these explicitly requested GitHub App permissions.

The workflow accepts the App Client ID as
`openshell-github-app-client-id`; callers explicitly forward only
`VERTEX_AI_SERVICE_ACCOUNT_KEY` and `OPENSHELL_GITHUB_APP_PRIVATE_KEY`. The
token is used on the trusted host for `gh` and native provider bootstrap, then
passed to OpenShell as the provider credential. It is never included in
sandbox environment variables, payloads, agent arguments, or artifacts.
Installation tokens expire after one hour and are revoked by the token action
after the job.

The `harness-openshell` repository's `ai-review.yml` is a thin caller of the
pinned GitHub review workflow, so `pull_request_target` runs use the same path as consuming
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

The active reviewer runs OpenCode with Gemini 2.5 Pro through `inference.local`
and Google Vertex AI. The model is selected in
[`scripts/pr-review-local.sh`](../scripts/pr-review-local.sh) and the agent
arguments in [`opencode-harness.yaml`](../tasks/github-pr-reviewer/workflow/opencode-harness.yaml).
The task consumes the existing inference route; it does not configure it.
Keep those selections aligned and verify model access with the CI identity
when changing them.

The host uses trusted caller default-branch inputs and the pinned
`harness-openshell` revision. The sandbox receives the PR diff and attaches the
`github-review` provider's masked proxy interface, not the raw token. Its REST
policy allows selected PR reads and inline-comment POSTs to that exact PR.
The instructions request at most three comments; the policy does not enforce
comment count or review quality.

The wrapper rechecks label/head/base before launching the agent and after the
run. Comments can be posted during execution, so the final host check is not
a gate before each comment. Diffs over 256 KiB are rejected; execution and
diagnostic output are bounded. The
[completion check](../scripts/review/README.md) validates the OpenCode event
stream, including text and a terminal stop event, with recognized exceptions
for unresolvable comment positions and shell parser failures. It does not
validate finding correctness.
Artifacts remain unvalidated model output. Cleanup covers success, failure,
and normal cancellation, but cannot guarantee runner-loss cleanup or undo
comments that have already been posted.

Locally, use `gh` authentication, `jq`, GNU `timeout` (Homebrew `coreutils` on
macOS), and the Vertex token/project variables above. Use a new absolute artifact
directory each time and an open, non-draft PR carrying `ai-review`:

```bash
make cli
export REVIEW_REPOSITORY=stackrox/harness-openshell REVIEW_PR=123
export REVIEW_DIR="$PWD/review-artifacts-123"
bash scripts/pr-review.sh prepare
bash scripts/pr-review-local.sh
```

The local wrapper needs a reachable local gateway and a repository-scoped
`GITHUB_TOKEN` for provider bootstrap, in addition to the Vertex variables.
It creates a fresh workspace, runs the review, and removes its setup resources.
A setup or teardown failure fails the command.

For an already-configured target, run `bash scripts/pr-review.sh run` after
preparation instead. Select a registered gateway/workspace with
`OPENSHELL_GATEWAY` and `OPENSHELL_WORKSPACE`, or use the existing
`target.registration` fields in the trusted task document for a direct
connection (see [workflow contract](#workflow-contract)). The review command
uses the selected target and only creates its task sandbox. It needs host `gh`
authentication for PR checks, while the platform supplies the `github-review`
provider with usable credentials and the Gemini 2.5 Pro inference route.

Unit tests use fake commands, not Vertex. The agent can already publish inline
comments directly through the allowed API endpoint. A structured findings
format and a separate publication stage remain deferred.

## Current reviewer setup

The [reusable workflow](../.github/workflows/pr-review-reusable.yml) invokes
[`setup-openshell`](../.github/actions/setup-openshell/action.yml), which installs
the pinned OpenShell CLI and waits for gateway readiness. This CI path uses a
local gateway. [`scripts/pr-review-local.sh`](../scripts/pr-review-local.sh)
creates a temporary workspace, registers `github-review` and `vertex-review`,
and configures inference. It calls [`pr-review.sh run`](../scripts/pr-review.sh),
then removes the providers, any profile it imported, and the workspace.

`pr-review.sh` handles PR checks, diff preparation, policy rendering, and output
validation. It invokes the existing `harness workflow apply` command with a
unique sandbox name. The CLI owns sandbox execution and deletion, including
normal cancellation; the local wrapper waits for it before tearing down setup.

## Managed reviewer transition

Direct managed-gateway connectivity is already implemented in the CLI. The
reusable reviewer still uses the local setup above; moving it requires an
agreed managed integration contract:

- **Runtime access:** a CI identity with the required workspace membership,
  plus network access to the gateway and OIDC issuer. The HyperShell issuer
  described here is reachable only from the Red Hat network/VPN.
- **Workspace isolation:** an explicit choice of shared or dedicated workspace
  and the task's provider names and allowed operations.
- **Provider credential lifecycle:** platform ownership of GitHub App token
  minting or refresh, repository and permission selection, and credential
  replacement or expiry. Pre-provisioning a provider name does not keep an
  expired installation token usable.
- **Inference and policy:** a matching provider/model route and equivalent
  policy enforcement, so ordinary task runs can use existing references.

Once these requirements are met, replace `setup-openshell`, Google bootstrap,
and `pr-review-local.sh` in the job with managed authentication and
`bash scripts/pr-review.sh run`. Keep host GitHub authentication for preparation
and PR checks. The task inputs, review command, and allowed GitHub operations
stay the same. The reusable workflow currently exposes only the local CI path;
the review command provides the execution step for a future managed caller.

The POC deliberately fixes the task document, `github-review` provider name,
and Gemini 2.5 Pro model. Configure a managed target through the existing
workflow format and provide those resources before invoking it.

The following bootstrap examples describe the managed validation environment;
they are not evidence that the reusable reviewer has completed this transition.

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
isolated-workspace setup, not a shared-workspace runtime operation; the CLI does
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

## Workflow contract

The reusable portion is the `target` block in `test/hypershell-workflow.yaml`.
Its `registration` field supplies non-secret, in-memory connection metadata and
does not create persistent CLI state. An omitted `workspace` selects `default`.
