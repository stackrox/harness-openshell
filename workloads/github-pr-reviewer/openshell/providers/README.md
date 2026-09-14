# Provider boundary

The production reviewer uses the gateway's native GitHub provider instance
named `github-review`. It is intentionally not created by Harness and its
credential is never checked into this workload.

The rendered workload policy is the narrow task boundary: read the current
pull request and post inline comments only. If a platform needs a custom
provider profile, keep that profile in this directory and use an endpointless
profile plus explicit `credential_binding.provider` entries in `policy.yaml`.
