# Native OpenShell inputs

The ACS proof uses the provider profiles in `providers/` as platform bootstrap
inputs. Import only the profiles required by the workload; credentials remain
in the gateway. The current read-only proof uses `atlassian` and relies on the
published StackRox sandbox image's baseline policy.
