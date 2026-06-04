# syntax=docker/dockerfile:1.4

# OpenShell in-cluster sandbox launcher.
#
# A minimal image containing the openshell CLI and supporting tools.
# Runs as a Kubernetes Job triggered by `kubectl apply -f sandbox.yaml`.
# Reads sandbox configuration from a ConfigMap and credentials from
# mounted Secrets — no credentials transit the kubectl client.
#
# Build:  docker build --platform linux/amd64 -t quay.io/rcochran/openshell:launcher .
# Push:   docker push quay.io/rcochran/openshell:launcher

FROM registry.access.redhat.com/ubi9/ubi-minimal:latest AS base

# Install bash, python3, and openssh-clients (required by openshell CLI for SSH sessions)
RUN microdnf install -y bash python3 python3-pip openssh-clients --nodocs && \
    pip3 install --no-cache-dir tomli && \
    microdnf clean all

# Copy the openshell CLI binary (must be pre-built for linux/amd64)
# Build with: cargo zigbuild --release --target x86_64-unknown-linux-gnu -p openshell-cli
COPY openshell /usr/local/bin/openshell
RUN chmod 755 /usr/local/bin/openshell

COPY entrypoint.sh /entrypoint.sh
RUN chmod 755 /entrypoint.sh

USER 1000

ENTRYPOINT ["/entrypoint.sh"]
