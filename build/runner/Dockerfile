FROM registry.access.redhat.com/ubi9/ubi-minimal:latest

RUN microdnf install -y openssh-clients --nodocs && microdnf clean all

COPY harness /usr/local/bin/harness
COPY openshell /usr/local/bin/openshell

USER 1000

ENTRYPOINT ["harness", "launch"]
