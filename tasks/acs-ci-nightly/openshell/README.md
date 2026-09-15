# ACS CI nightly OpenShell inputs

The task policy is an overlay for the shared `sandbox-stackrox-ci` image. The
image supplies the runtime tools; this directory supplies task-specific
network permissions and provider-profile metadata.

Import the endpointless provider profiles through trusted platform bootstrap,
then create matching read-only provider instances. The Harness CLI only
verifies and attaches those instances; it does not provision or manage their
credentials.

The `github_git` policy is intentionally unauthenticated and read-only because
the StackRox repositories used by this task are public. The Atlassian and Prow
GCS provider instances remain gateway-owned; repository source being public
does not make those data sources public.
