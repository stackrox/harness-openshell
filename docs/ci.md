# CI and live validation

Credential-free PR checks do not establish live inference success.
Image PR checks build without registry login or publication; only
main/tag pushes publish images and update the shared registry cache.

## HyperShell validation

HyperShell is the managed OpenShell environment used by these validation
examples. The existing local smoke tests use the Red Hat network because that
issuer is private. Managed review CI can use a Linux runner with the same network
access; a public gateway alone does not establish issuer reachability. The `harness` CLI connects directly through the
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
`openshell-github-app-client-id`; callers explicitly forward
`OPENSHELL_GITHUB_APP_PRIVATE_KEY` and the credential for their gateway path:
`VERTEX_AI_SERVICE_ACCOUNT_KEY` for local setup, or
`OPENSHELL_OIDC_CLIENT_SECRET` for managed execution. The App token is used on
the trusted host for `gh`. Local setup also registers it as the provider
credential; the managed platform supplies its own scoped provider credential. It is never included in
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
Local runs use the Vertex secret/variables above; managed runs use the connection
configuration below. Summaries show status, head SHA, and
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

The [reusable workflow](../.github/workflows/pr-review-reusable.yml) uses the
caller's repository or organization variables to select the connection. With no
managed connection settings, it invokes [`setup-openshell`](../.github/actions/setup-openshell/action.yml)
and [`pr-review-local.sh`](../scripts/pr-review-local.sh) for temporary local
workspace, providers, and inference setup. With a complete managed connection,
it runs [`pr-review.sh run`](../scripts/pr-review.sh) directly.

`pr-review.sh` handles PR checks, policy rendering, and output validation. The
existing CLI owns sandbox execution and deletion, including normal cancellation.
Generated sandbox and temporary workspace names fit v0.0.109's 19-character limit.

## Managed reviewer transition

This path targets an existing HyperShell OpenShell **v0.0.109** gateway. Keep
`inference.local` for now. Configure these **repository or organization Actions
variables in the caller**, not environment-scoped variables:

| Variable | Value |
|---|---|
| `OPENSHELL_GATEWAY_ENDPOINT` | HTTPS gateway URL; selects managed execution |
| `OPENSHELL_WORKSPACE` | Workspace the service-account subject can access |
| `OPENSHELL_OIDC_ISSUER` | HTTPS issuer from the gateway connection metadata |
| `OPENSHELL_OIDC_CLIENT_ID` | Gateway service-account client ID |
| `OPENSHELL_OIDC_AUDIENCE` | That gateway's audience |
| `OPENSHELL_RUNNER` | Optional Linux runner label, such as a dedicated `hypershell-ci` label; default `ubuntu-latest` |

Store `OPENSHELL_OIDC_CLIENT_SECRET` as an Actions secret and forward it explicitly
from the caller. Any partial managed connection fails validation; it does not
fall back to creating a local gateway. The workflow clears `OPENSHELL_GATEWAY`
so a runner's named CLI registration cannot override the direct connection.
The secret is supplied only to configuration validation and managed execution.

The runner needs Bash, `gh`, `jq`, GNU `timeout`, OpenSSL, and access to both the
gateway and issuer. The workflow installs Go and builds the trusted harness.
The managed path does not install the OpenShell CLI or authenticate to Google.
Use a runner on the Red Hat network for a private issuer. A runner label selects
an existing runner; it does not provision network access.

Before enabling review, the platform owner must:

1. Grant the gateway service-account subject `user` access to the selected
   workspace. Use a repository-specific workspace for this POC's fixed provider
   names and repository-scoped credentials.
2. Import the task's endpointless `github-review` profile and create the provider
   instance named `github-review`, using a token scoped to the target repository
   with `Contents: read` and `Pull requests: read/write`. Own its refresh or
   replacement. The host's newly minted App token is only used for metadata
   checks on this path; it is not uploaded to the managed provider.
3. Configure and verify the workspace's `inference.local` route for Gemini 2.5
   Pro and keep its Vertex credentials usable. The Haiku smoke example below
   exercises a different model and is not proof that this review route works.
4. Verify the effective REST method/path restrictions and credential binding on
   the deployed gateway. HyperShell's reviewed v109 gateway configuration
   disables process-binary-aware network policy, so do not assume the task's
   binary restrictions are enforced there.

Native profile import already accepts a directory in v109:
`openshell provider profile import --from tasks/github-pr-reviewer/openshell/providers`.
Run it under platform bootstrap authority with the intended gateway/workspace.
It imports definitions only and is create-only; it does not refresh credentials
or provision provider instances. Ordinary review jobs do not call it.

### Activate a consuming repository

After publishing this change, update both the reusable workflow `uses` reference
and `harness-ref` to the same full commit SHA containing this integration. Then
forward the gateway secret in that caller's existing `secrets` block:

```yaml
secrets:
  OPENSHELL_GITHUB_APP_PRIVATE_KEY: ${{ secrets.OPENSHELL_GITHUB_APP_PRIVATE_KEY }}
  OPENSHELL_OIDC_CLIENT_SECRET: ${{ secrets.OPENSHELL_OIDC_CLIENT_SECRET }}
```

Keep the existing GitHub App client-ID input. A managed-only caller can omit the
Vertex secret. The repository's checked-in `ai-review.yml` still pins the older
workflow; this change deliberately does not invent a future commit SHA or
activate a deployment. Do not forward the new secret while still calling the
old workflow, which does not declare it.

### Verification and later changes

On the chosen runner, first verify service-account access with the existing
SDK lifecycle smoke, then run the review against a designated test PR. Verify
an allowed inline comment, denial outside the allowed PR/methods, and sandbox
cleanup on success, failure, and cancellation. Confirm authentication still
works after access-token expiry. Static checks and fake commands do not prove
any of those live properties. The existing `test/hypershell-lifecycle.sh` is a
local/VPN helper and skips in CI; it cannot be the CI acceptance check. The
configured managed review path above executes in CI without that helper.

The local CLI/SDK remain at their existing v0.0.110 pins. Qualify the SDK against
the actual vendor v109 gateway image; no server upgrade is required by this
patch. Upstream [#2907](https://github.com/NVIDIA/OpenShell/pull/2907) provides a
future replacement for custom OIDC token plumbing. Upstream
[#3195](https://github.com/NVIDIA/OpenShell/pull/3195) removes `inference.local`;
migrate setup and agent/provider configuration when HyperShell adopts it.
Neither change needs a new task runner interface now. HyperShell
[#267](https://github.com/openshift-online/hypershell/pull/267) provisions and tests
its own platform; the reviewer does not depend on that pipeline or its cluster
administration credentials.

Review remains advisory. A later approval task can submit an explicit review for
the reviewed commit with separately granted permissions. Issue-to-PR can use its
own task policy. Merge decisions stay in repository workflows; a successful
runner result is not an approval.

The following bootstrap examples describe the separate managed smoke-test
environment, not the reviewer workspace or evidence of a completed CI run.

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
