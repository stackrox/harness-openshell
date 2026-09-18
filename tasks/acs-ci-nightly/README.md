# ACS CI nightly

This task runs the ACS repository's canonical `scripts/run-triage.sh` inside a
read-only OpenShell sandbox. The ACS repository owns the Prow/GCS lookup and
triage logic; this Harness bundle provides the generic OpenShell task wiring
and provider-backed connections. The task produces the normal triage report
without creating or updating Jira issues or posting to Slack.

The task is intentionally read-only: Jira and community triage may inspect
their sources, while the Jira updater and Slack publication remain disabled.

## Contract

- Trigger: a trusted repository workflow chooses when to run it. The task has
  no scheduler or GitHub Actions trigger of its own.
- Trusted inputs: the workflow document, `ACS_TRIAGE_REF`, `TRIAGE_RUN_URL`,
  the gateway target, provider names, and the pinned `stackrox-ci` image.
- Task input: the checked-out `stackrox/acs-triage-agent` source owns the
  collector and analysis logic. The source and Prow result data cannot change
  the provider identities, policy, image, or workflow wiring defined here.
- External operations: public GitHub clone/fetch and read-only Prow GCS and
  Jira queries. The task cannot push source, create or update Jira issues, or
  publish to Slack.
- Outputs: the normal ACS artifacts, including `ci-triage.json`,
  `triage-report.md`, and `slack-summary.txt`, downloaded to the caller's
  output directory. They are optional so partial diagnostics can still be
  retained when analysis fails.
- Cleanup: the sandbox and host-side source staging are removed after outputs
  are downloaded. Downloaded artifacts and any external reads remain with the
  caller.

## Platform prerequisites

The platform must provision these gateway-owned resources before applying the
workflow:

- `vertex-claude-triage` and the matching `inference.local` route;
- `atlassian-triage-read`, configured for read-only Jira/Confluence access;
- `github-triage-read`, configured for read-only project and issue queries;
- `prow-gcs-read`, created from OpenShell's built-in `google-cloud` provider
  profile and configured with gateway-managed Google service-account JWT
  refresh for read-only access to the `test-platform-results-public` bucket.

The Atlassian profile in `openshell/providers/` contains metadata only. It does
not create providers or contain credentials. Provider credentials must never
be placed in workflow environment variables, payloads, agent arguments, or
artifacts. The Prow provider uses the upstream `google-cloud` profile so its
refresh and `gcloud storage` metadata behavior stay aligned with OpenShell.

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
