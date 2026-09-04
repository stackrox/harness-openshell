# Plan: Switch a Harness between OpenShell and HyperShell

Status: proposed

## Goal

Run the same Harness file with three operating contexts:

```text
local OpenShell
HyperShell with personally managed access
HyperShell with a constrained service account
```

The first version selects only the gateway, workspace, and connection method.
It does not move credentials or define security policy. HyperShell guardrails
come from the service account's workspace membership and gateway-managed
resources, not from Context behavior.

CI is an execution environment. It may use the local Context with an ephemeral
gateway, or the service-account Context when a runner can reach HyperShell.

## User experience

```bash
harness apply -f test/ci-workflow.yaml --context contexts/local.yaml
harness apply -f test/ci-workflow.yaml --context contexts/hypershell-personal.yaml
harness apply -f test/ci-workflow.yaml --context contexts/hypershell-service-account.yaml
```

The local Context uses the active OpenShell gateway. The personal HyperShell
Context uses a gateway registration authenticated by the user through the
normal OpenShell login flow. The service-account Context uses the existing
direct OIDC client-credentials path with secret material supplied externally.

Existing commands without `--context` continue to work.

## Context file

A Context contains one existing Harness `Target`. No new target model or
templating language is introduced.

Personal HyperShell uses a normal named gateway registration:

```yaml
apiVersion: harness.openshell.dev/v1alpha1
kind: Context
metadata:
  name: hypershell-personal
spec:
  target:
    gateway: hypershell
    workspace: personal
```

The service-account Context uses direct, non-persistent connection metadata:

```yaml
apiVersion: harness.openshell.dev/v1alpha1
kind: Context
metadata:
  name: hypershell-service-account
spec:
  target:
    workspace: controlled
    registration:
      endpoint: ${HYPERSHELL_GATEWAY}
      oidc:
        issuer: ${HYPERSHELL_OIDC_ISSUER}
        clientId: ${HYPERSHELL_SANDBOX_SA_ID}
        audience: ${HYPERSHELL_OIDC_AUDIENCE}
```

The workspace names above are examples. They are the main place to distinguish
personally managed provider state from a centrally managed, narrower service
account environment.

`contexts/local.yaml` contains an empty target and therefore uses normal
OpenShell active-gateway resolution:

```yaml
apiVersion: harness.openshell.dev/v1alpha1
kind: Context
metadata:
  name: local
spec:
  target: {}
```

Environment interpolation uses the resolver already used by Harness files.
`OPENSHELL_OIDC_CLIENT_SECRET` stays in the process environment and is never
part of a Context, rendered Harness, or structured output.

## Resolution

When `--context` is present:

1. Parse the Harness and Context strictly.
2. Replace the Harness `spec.target` with the Context target.
3. Expand environment references.
4. Apply existing target precedence: explicit CLI flag, then `OPENSHELL_*`
   environment, then the resolved target, then the active/default gateway.
5. Use that one resolved object for dry-run output and execution.

The Context target is replaced as a whole. Field-by-field merging is not
supported.

## Implementation

1. Add a strict `Context` config type containing only metadata and target.
2. Add `--context FILE` to `harness apply`.
3. Load the Context in `loadWorkflow` and replace `spec.target` before normal
   environment and target resolution.
4. Add the three example Context files.
5. Run the existing smoke Harness through:

   - a developer's active local gateway;
   - a user-authenticated HyperShell registration;
   - HyperShell using the existing service-account VPN/OIDC setup.

## Acceptance

- The unchanged `test/ci-workflow.yaml` returns exactly
  `canonical-sdk-ok` in all three contexts.
- `keep: false` removes the sandbox after every run.
- `harness apply -f file.yaml` behaves exactly as before.
- Invalid or incomplete Context files fail before gateway access.
- Dry-run structured output contains the resolved non-secret target and no
  credential values.
- The personal Context relies on OpenShell's existing login and token refresh.
- The service-account Context contains no client secret and fails clearly
  before sandbox creation when its OIDC issuer is unreachable off VPN.
- The selected service account cannot exceed the workspace role and gateway
  resources assigned to it; Context does not claim to enforce those controls.

## Not in this version

- Provider or model aliases.
- Provider creation or credential bootstrap.
- Inference, image, policy, or agent overlays.
- Context discovery, inheritance, merging, or conditionals.
- Inline secrets.
- Managed HyperShell execution from public GitHub runners until network access
  exists.
- NemoClaw integration.

Those should be considered only after this target-only switch is useful in
practice.
