# ACS CI nightly

This task runs the read-only CI-failure portion of the ACS triage agent. It
queries the StackRox Prow result bucket, analyzes recent nightly
failures, and writes `ci-triage.json` without creating or updating Jira
issues.

The task is intentionally narrower than the full ACS triage workflow. It is a
first consumer contract for `acs-triage-agent`; Jira/community triage and
publication can be added as separate task bundles after this contract is
validated.

## Contract

- Trigger: a trusted repository workflow chooses when to run it. The task has
  no scheduler or GitHub Actions trigger of its own.
- Trusted inputs: the workflow document, `ACS_TRIAGE_REF`, `TRIAGE_RUN_URL`,
  the gateway target, provider names, and the pinned `stackrox-ci` image.
- Untrusted input: the checked-out `stackrox/acs-triage-agent` source and the
  Prow result data it reads. Neither is allowed to define providers, policy,
  image, or commands.
- External operations: public GitHub clone/fetch and read-only Prow GCS and
  Jira queries. The task cannot push source, create or update Jira issues, or
  publish to Slack.
- Output: `/sandbox/acs-triage-agent/artifacts/ci-triage.json`, downloaded to
  the caller's output directory. The output is optional so partial diagnostics
  can still be retained when analysis fails.
- Cleanup: the sandbox and host-side source staging are removed after outputs
  are downloaded. Downloaded artifacts and any external reads remain with the
  caller.

## Platform prerequisites

The platform must provision these gateway-owned resources before applying the
workflow:

- `vertex-claude-triage` and the matching `inference.local` route;
- `atlassian-triage-read`, configured for read-only Jira/Confluence access;
- `prow-gcs-read`, created from OpenShell's built-in `google-cloud` provider
  profile and configured with gateway-managed Google service-account JWT
  refresh for read-only access to the `test-platform-results-public` bucket.

The Atlassian profile in `openshell/providers/` contains metadata only. It does
not create providers or contain credentials. Provider credentials must never
be placed in workflow environment variables, payloads, agent arguments, or
artifacts. The Prow provider uses the upstream `google-cloud` profile so its
refresh and gsutil-compatible metadata behavior stay aligned with OpenShell.

The workflow uses the shared `sandbox-stackrox-ci` image. Because image
publication is independent of task publication, the trusted caller must set
`ACS_TRIAGE_IMAGE` to the immutable digest of the published image. Do not
accept that value from pull-request data.

## Applying the task

From a trusted caller with a reachable managed gateway:

```bash
export ACS_TRIAGE_IMAGE='quay.io/rcochran/openshell:sandbox-stackrox-ci@sha256:<digest>'
export ACS_TRIAGE_REF='main'
export TRIAGE_RUN_URL="${GITHUB_SERVER_URL}/${GITHUB_REPOSITORY}/actions/runs/${GITHUB_RUN_ID}"
harness workflow apply tasks/acs-ci-nightly/workflow/harness.yaml \
  --output-dir ./triage-artifacts
```

For a pull-request test, `ACS_TRIAGE_REF` may be the public ACS repository
commit under test. The workflow itself remains trusted host-side configuration;
the source checkout is only task input.

## Native OpenShell inputs

`openshell/policy.yaml` allows public Git Smart HTTP reads, the read-only
GitHub API calls used by the agent, read-only Atlassian REST calls, and the
Prow GCS bucket. The policy does not allow Git push, Jira writes, or Slack
webhooks. Render or review this policy as part of platform bootstrap before
using the task.
