# Sandbox images

This directory contains reusable OpenShell sandbox image build contexts. An
image supplies a runtime toolchain; it does not own provider credentials,
workflow policy, skills, prompts, or task permissions.

A workload selects an image, and the Harness runner passes that image reference
to OpenShell when it creates the sandbox. Local and managed gateways use the
same contract. Add a separate image only when a workload needs a materially
different system toolchain; otherwise reuse an existing image and keep
task-specific behavior in `workloads/`.

StackRox image profiles and build instructions are documented in
[`stackrox/README.md`](stackrox/README.md).
