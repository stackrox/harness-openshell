# syntax=docker/dockerfile:1.4

# OpenShell in-cluster sandbox launcher.
#
# A minimal image containing the Go launcher, openshell CLI, and openssh.
# Runs as a Kubernetes Job triggered by `kubectl apply -f sandbox.yaml`.
# Reads sandbox configuration from a ConfigMap and credentials from
# mounted Secrets — no credentials transit the kubectl client.
#
# Build:
#   GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o launcher .
#   docker build --platform linux/amd64 -t quay.io/rcochran/openshell:launcher .
#
# Push:  docker push quay.io/rcochran/openshell:launcher

FROM registry.access.redhat.com/ubi9/ubi-minimal:latest

RUN microdnf install -y openssh-clients --nodocs && microdnf clean all

COPY launcher /usr/local/bin/launcher
COPY openshell /usr/local/bin/openshell

USER 1000

ENTRYPOINT ["/usr/local/bin/launcher"]
