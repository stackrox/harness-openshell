# Native OpenShell inputs

Import `providers/github-issue-reviewer.yaml` into the target gateway, create
the matching provider instance, render `policy.yaml` with the selected
repository and issue, then launch the native sandbox:

```bash
openshell provider profile import \
  -f openshell/providers/github-issue-reviewer.yaml
openshell provider create --name github-issue-reviewer \
  --type github-issue-reviewer --credential GITHUB_TOKEN
openshell sandbox create --from "$IMAGE" \
  --policy /tmp/issue-review-policy.yaml \
  --provider github-issue-reviewer -- opencode run --format json
```

Harness is optional; it only uploads the skill and manages the one-shot
lifecycle around these same OpenShell inputs.
