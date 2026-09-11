# Profiles

`harness-basic.yaml` is a canonical version 1 workflow scaffold that can be
copied into a repository-owned workflow package.

`stackrox/image/sandbox-default/` contains the optional StackRox sandbox image
inputs. Generic workflows use the NVIDIA community base image; use this
published image when a workflow needs the StackRox-specific tools it adds.
Local build contexts are not accepted by `harness workflow apply`.

`providers/` contains provider-profile examples used by the external platform
bootstrap process. Applying a workflow never creates a
credentialed provider. Providers named by `inference.provider` or
`sandbox.providers` must already exist on the selected gateway.
