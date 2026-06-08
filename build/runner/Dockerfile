FROM registry.access.redhat.com/ubi9/ubi-minimal:latest

RUN microdnf install -y openssh-clients tar gzip --nodocs && microdnf clean all

ARG OPENSHELL_VERSION=0.0.58
RUN curl -LsSf "https://github.com/NVIDIA/OpenShell/releases/download/v${OPENSHELL_VERSION}/openshell-x86_64-unknown-linux-musl.tar.gz" \
    | tar xz -C /usr/local/bin openshell

COPY harness /usr/local/bin/harness

USER 1000

ENTRYPOINT ["harness", "launch"]
