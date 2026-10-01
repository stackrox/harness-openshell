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
`MERGE_HEAD_SHA`. Set `VERTEX_AI_PROJECT_ID` to the project configured for the
attached `vertex-review` provider, with that provider configured for `global`.
The workflow constructs the global Vertex
OpenAI-compatible endpoint from that project before the sandbox starts. Then
run it with native OpenShell commands or the Harness adapter.
