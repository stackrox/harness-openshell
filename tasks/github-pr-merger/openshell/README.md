# Native OpenShell inputs

Import the endpointless provider profile into the target gateway and create a
provider instance with a merge-capable GitHub credential. Keep this provider
separate from the review credential.

```bash
openshell provider profile import \
  -f openshell/providers/github-pr-merger.yaml
openshell provider create --name github-pr-merger \
  --type github-pr-merger --credential GITHUB_TOKEN
```

Render `policy.yaml` with `MERGE_REPOSITORY`, `MERGE_PR`, and
`MERGE_HEAD_SHA`, then run the workflow with native OpenShell commands or the
Harness adapter.
