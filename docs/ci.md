# HyperShell validation

HyperShell validation runs locally from the Red Hat network because its OIDC
issuer is VPN-only. The Harness workflow connects directly through the
OpenShell Go SDK. It does not persist a gateway registration or use a gateway
administrator account at runtime.

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

The reusable portion is the target block in `test/hypershell-workflow.yaml`.
Its `registration` field supplies non-secret, in-memory connection metadata;
despite the v1alpha1 field name, it does not create persistent CLI state. An
omitted `workspace` selects `default`.
