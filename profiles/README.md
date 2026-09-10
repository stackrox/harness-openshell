# Profiles

`harness-basic.yaml` is a canonical version 1 workflow scaffold that can be
copied into a repository-owned workflow package.

`images/sandbox-default/` contains the default sandbox image inputs. The
workflow refers to the published image; local build contexts are not accepted
by `harness workflow apply`.

`providers/` contains provider-profile examples used by the external platform
bootstrap process. Applying a workflow never creates a
credentialed provider. A provider named in `providers` or
`sandbox.providers` must already exist on the selected gateway.
