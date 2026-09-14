# Native OpenShell inputs

Import `providers/github-issue-to-pr.yaml`, create the matching provider
instance, render `policy.yaml`, upload or check out the repository, and launch
the native sandbox. The policy permits one pull-request creation and one issue
comment, plus Git Smart HTTP push for a new branch. It does not permit merges
or label changes.

```bash
openshell provider profile import -f openshell/providers/github-issue-to-pr.yaml
openshell provider create --name github-issue-to-pr \
  --type github-issue-to-pr --credential GITHUB_TOKEN
openshell sandbox create --from "$IMAGE" \
  --policy /tmp/issue-to-pr-policy.yaml \
  --provider github-issue-to-pr -- opencode run --format json
```
