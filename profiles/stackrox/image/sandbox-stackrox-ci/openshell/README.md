# Vendored OpenShell base assets

This directory contains the small set of files copied from the NVIDIA
OpenShell Community base sandbox that this profile uses at runtime:

- the GitHub REST-only agent skill;
- shell initialization adapted from the upstream base image for the StackRox
  Go toolchain and writable caches.

Source: `NVIDIA/OpenShell-Community/sandboxes/base` at commit
`fffb6b2248ff6ba585f50517f3711b08122089f2`.

The source repository is Apache-2.0 licensed. Keep this copy synchronized with
the pinned source commit when the upstream base contract changes. The upstream
network policy is intentionally not copied; this profile owns its minimal
policy and provider profiles supply integration egress at runtime.
