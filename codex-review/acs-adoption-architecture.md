# ACS-wide adoption architecture

## Thesis

If ACS receives a shared HyperShell service, the simplest repeatable adoption
pattern is:

1. HyperShell provisions and operates one logical ACS OpenShell gateway, with
   workspaces separating repositories or trust zones.
2. Platform owners configure centrally governed identities and provider access.
3. Repository owners add a small, reviewable workflow declaration and CI entry
   point.
4. harness-openshell turns that declaration into an isolated OpenShell run and
   returns evidence to the pull request.

The user-facing promise is not "learn another orchestration platform." It is:

> Add these files to your repository to get a governed security-review agent.

## Proposed topology

```text
ACS repositories
  collector ─┐
  stackrox ──┤
  fact ──────┼── GitHub Actions + harness workflow
  others ────┘              │
                             │ short-lived authenticated connection
                             ▼
                  ACS OpenShell gateway
                  managed by HyperShell
                    ├── collector workspace
                    ├── stackrox workspace
                    ├── fact workspace
                    └── shared pilot workspace
                          ├── GitHub capability
                          ├── inference capability
                          ├── approved sandbox images
                          └── ACS policy baseline
                             │
                             ▼
                    evidence / patch / findings
                             │
                             ▼
                       pull request
```

## Identity and secret model

"One gateway" means one logical ACS service endpoint, not a permanent requirement
for one physical deployment. HyperShell may later partition capacity by region,
environment, or trust level without changing repository workflow intent.

The gateway should have centrally managed access to:

- an inference provider identity with quotas and model policy appropriate for
  ACS; and
- a GitHub App or token broker capable of issuing short-lived, repository-scoped
  credentials with the least privilege needed by the workflow.

Prefer short-lived federation from GitHub Actions to HyperShell/OpenShell. If the
initial integration requires an organization secret, store only the credential
needed to authenticate the CI job to the gateway—not inference credentials or a
long-lived GitHub token copied into every repository.

The gateway should inject capabilities into the sandbox through OpenShell
providers. A repository workflow names required capabilities but never contains
their secret material.

GitHub permissions should be workflow-specific. A review-only workflow should
receive repository-read plus only the check/comment permission needed to publish
results. A remediation workflow should use a separate identity and explicit
approval.

## Authority boundary

The shared service has two distinct authorities:

| Authority | May do | Must not do |
|---|---|---|
| Platform/bootstrap | Create workspaces, providers, inference routes, base policies, approved images, and service accounts | Run with repository write permission by default |
| Workflow/run | Resolve referenced capabilities, create a sandbox, execute a declared workflow, and publish evidence | Create, adopt, update, or delete shared providers and inference routes |

Repository declarations name logical capabilities such as `github-read` and
`inference-default`. The platform maps those names to concrete OpenShell providers
inside the selected workspace. Missing capabilities fail the run before sandbox
creation; they are never silently omitted or auto-created.

## Repository adoption surface

Adoption can intentionally progress from two files to one:

1. **Pilot:** a reusable-workflow caller and a harness declaration.
2. **Mature:** an organization-required workflow discovers the checked-in harness
   declaration, leaving one repository file.
3. **Default-on:** the organization workflow supplies a default review and a
   repository adds a declaration only to opt out or customize it.

The pilot repository change is:

```text
.github/workflows/openshell-security-review.yml
.harness/workflows/security-review.yaml
```

The GitHub workflow should call a centrally versioned reusable workflow. The
repository-local harness declaration contains only repository-specific intent:

```yaml
apiVersion: harness.openshell.dev/v1alpha1
kind: AgentRun
metadata:
  name: security-review
spec:
  target:
    gateway: acs
    workspace: ${GITHUB_REPOSITORY}
  source:
    repo: ${GITHUB_SERVER_URL}/${GITHUB_REPOSITORY}
    ref: ${GITHUB_SHA}
  skill:
    ref: catalog://acs/security-review@v1
  capabilities:
    - github-read
    - inference
  sandbox:
    image: catalog://acs/security-review-sandbox@v1
    policy: catalog://acs/security-review-policy@v1
    keep: false
  outputs:
    - sarif
    - markdown-summary
```

This example represents the recommended contract; it is not the repository's
current schema.

The local file is valuable even when defaults are centralized: it makes agent
execution visible in code review, pins workflow intent, and lets repository owners
control triggers and declared capabilities.

## Central assets

An ACS-owned catalog should version:

- skills;
- policy baselines;
- approved sandbox images;
- reusable GitHub workflows;
- output schemas; and
- example repository configurations.

Catalog references must resolve to immutable versions or digests during a run.
Mutable friendly names may exist for discovery, but the evidence manifest should
record the resolved digest.

The reusable GitHub workflow should own the mechanical setup:

1. exchange GitHub OIDC identity for short-lived gateway authentication;
2. select the already provisioned ACS OpenShell gateway; if the ephemeral runner
   has no local client entry, add that registration metadata first (this stores
   client context only and does not create or modify the gateway);
3. map repository identity to an authorized workspace;
4. execute the pinned harness workflow;
5. publish checks, SARIF, and the evidence artifact; and
6. remove transient local credentials and context files.

Neither the reusable workflow nor harness-openshell provisions or tears down the
gateway. HyperShell retains gateway lifecycle ownership.

## First viral application: security review

Security review is a strong first application because ACS has relevant internal
expertise and the output can be bounded and visible.

The first version should:

1. trigger on a pull request or manual dispatch;
2. read the exact pull-request diff and selected repository context;
3. run with GitHub read access and inference access only;
4. apply a centrally maintained ACS review skill;
5. produce structured findings with file, line, severity, confidence, and
   rationale;
6. publish a concise check result or PR summary; and
7. attach the complete evidence manifest as a CI artifact.

Do not begin with automatic repository writes. Establish precision, useful signal,
and trust first. Remediation can become a second workflow with separately approved
permissions.

## Viral loop

The adoption loop should be deliberately small:

```text
central team publishes workflow
  → repository adds two files
  → PR receives useful, attributable result
  → neighboring repository copies the pattern
  → feedback improves the central skill once
  → all pinned consumers deliberately upgrade
```

The features most likely to drive propagation are:

- under-ten-minute first successful run;
- no repository-level inference credentials;
- a copyable example pull request;
- findings directly on normal developer surfaces;
- deterministic reruns against the same commit;
- an obvious permission and policy declaration; and
- centrally fixed skills without bespoke per-repository scripts.

The key demonstration should be a pull request adding only the reusable workflow
caller and a short declaration, with no inference secret, provider setup script,
custom image build, or direct HyperShell API call in the repository.

## Guardrails against becoming another platform layer

1. Keep native OpenShell concepts visible: gateway, provider, sandbox, and policy.
2. Do not duplicate HyperShell fleet, identity, or gateway lifecycle APIs.
3. Do not mirror the full OpenShell CLI.
4. Let the workflow declaration express outcomes and capabilities, not imperative
   OpenShell command sequences.
5. Treat reconciliation as a replaceable backend: CLI/SDK today, operator custom
   resources later.
6. Make every abstraction earn its place by enabling portability, governance, or
   evidence.

## Pilot plan

Before the pilot, harness execution must support the canonical schema, explicit
workspaces, referenced-only capabilities, strict validation, and a structured run
result. Provider management by ordinary apply is a blocker, not pilot scope.

1. Select one representative repository and one read-only security review.
2. Provision the logical ACS gateway, pilot workspace, and read-only
   GitHub/inference capabilities.
3. Publish one pinned skill, image, policy, and reusable GitHub workflow.
4. Demonstrate adoption in a small pull request containing only the local workflow
   declaration and CI caller.
5. Measure setup time, successful-run rate, useful-findings rate, false positives,
   runtime, and inference cost.
6. Add two repositories with different build systems and separate workspaces.
7. Move discovery into an organization-required workflow to prove the one-file
   adoption path.
8. Only then introduce write-capable remediation or scheduled autonomous work.
