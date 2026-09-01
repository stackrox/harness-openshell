# Profiles

`harness-basic.yaml` is the canonical `harness.openshell.dev/v1alpha1`
scaffold embedded by `harness init` and used by `harness doctor` when no local
config exists.

`images/sandbox-default/` contains the default sandbox image inputs. The
workflow refers to the published image; local build contexts are not accepted
by `harness apply`.

`providers/` contains provider-profile examples used by diagnostics and by the
external platform bootstrap process. Applying a workflow never creates a
credentialed provider. A provider named in `spec.providers` or
`spec.sandbox.providers` must already exist on the selected gateway.
