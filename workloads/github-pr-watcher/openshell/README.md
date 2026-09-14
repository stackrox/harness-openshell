# Native OpenShell inputs

Import `providers/github-pr-watcher.yaml`, create the matching provider
instance, render `policy.yaml` with the selected repository and pull request,
and launch the sandbox with the existing PR branch checked out. The policy
allows Git Smart HTTP receive-pack for that repository so branch protection and
the caller must enforce the branch boundary.

```bash
openshell provider profile import -f openshell/providers/github-pr-watcher.yaml
openshell provider create --name github-pr-watcher \
  --type github-pr-watcher --credential GITHUB_TOKEN
openshell sandbox create --from "$IMAGE" \
  --policy /tmp/pr-watcher-policy.yaml \
  --provider github-pr-watcher -- opencode run --format json
```
