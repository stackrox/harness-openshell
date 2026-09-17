# ACS CI nightly OpenShell inputs

The task policy is an overlay for the shared `sandbox-stackrox-ci` image. The
image supplies the runtime tools; this directory supplies task-specific
network permissions and provider-profile metadata.

Import the Atlassian endpointless provider profile through trusted platform
bootstrap, then create matching read-only provider instances. Create
`prow-gcs-read` from OpenShell's built-in `google-cloud` profile and configure
its gateway-managed service-account JWT refresh. The Harness CLI only verifies
and attaches those instances; it does not provision or manage their
credentials.

The built-in Google Cloud profile supplies the gateway-managed metadata path
that gsutil uses. The workflow's Boto configuration enables gsutil's
`[GoogleCompute]` metadata credential lookup without placing a credential in
the sandbox, and the workflow sets both legacy metadata variables explicitly
for gsutil's metadata client. They point at OpenShell's loopback emulator. The
task policy binds that provider instance only to the read-only
`test-platform-results-public` endpoints.
The task also points Google Cloud CLI tools at OpenShell's combined CA bundle
so `gsutil` verifies the sandbox proxy certificate without disabling TLS.

The `github_git` policy is intentionally unauthenticated and read-only because
the StackRox repositories used by this task are public. The Atlassian and Prow
GCS provider instances remain gateway-owned; repository source being public
does not make those data sources public.
