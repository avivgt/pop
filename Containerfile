FROM golang:1.24 AS builder
ARG TARGETOS
ARG TARGETARCH

WORKDIR /workspace
COPY go.mod go.mod
COPY go.sum go.sum
RUN go mod download

COPY cmd/main.go cmd/main.go
COPY api/ api/
COPY internal/controller/ internal/controller/

RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -a -o manager cmd/main.go

FROM registry.access.redhat.com/ubi9/ubi-minimal:latest

LABEL name="pop-operator" \
      vendor="POP Project" \
      version="0.1.0" \
      summary="POP Operator - Platform Outage Provisioner" \
      description="Automates OpenShift control plane backup and graceful cluster shutdown" \
      io.k8s.display-name="POP Operator" \
      io.openshift.tags="backup,shutdown,etcd,maintenance"

WORKDIR /
COPY --from=builder /workspace/manager .
USER 65532:65532

ENTRYPOINT ["/manager"]
