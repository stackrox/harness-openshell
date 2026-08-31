# HyperShell CI

The HyperShell workflow connects directly through the OpenShell Go SDK. It does
not install the OpenShell CLI, persist a gateway registration, or use a gateway
administrator account.

## One-time platform bootstrap

A gateway administrator adds the CI service-account subject to the `default`
workspace with the `user` role. This membership is gateway state and should be
managed centrally, not recreated by each repository run.

The default workspace is intentionally implicit in
`test/hypershell-workflow.yaml`. Use a dedicated workspace when repository
isolation becomes more important than the low-friction shared pilot.

## Repository secrets

Configure these GitHub Actions secrets:

- `HYPERSHELL_GATEWAY`: HTTPS gateway endpoint
- `HYPERSHELL_OIDC_ISSUER`: HTTPS OIDC issuer
- `HYPERSHELL_OIDC_AUDIENCE`: gateway token audience
- `HYPERSHELL_SANDBOX_SA_ID`: user service-account client ID
- `HYPERSHELL_SANDBOX_SA_SECRET`: user service-account client secret

The workflow maps the last value to `OPENSHELL_OIDC_CLIENT_SECRET` only for the
harness process. The secret is not represented in the workflow document, plan,
or command output. `HYPERSHELL_SA_ID` and `HYPERSHELL_SA_SECRET` are not needed
by repository CI and should remain platform-administration credentials.

## Repository contract

The reusable portion is the target block in `test/hypershell-workflow.yaml`.
Its `registration` field supplies non-secret, in-memory connection metadata;
despite the v1alpha1 field name, it does not create persistent CLI state. An
omitted `workspace` selects `default`.
