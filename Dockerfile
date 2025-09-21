# Build the manager binary
FROM --platform=${BUILDPLATFORM:-linux/amd64} golang:1.25 AS builder

ARG BUILDPLATFORM
ARG TARGETPLATFORM
ARG TARGETOS
ARG TARGETARCH
ARG VERSION
ARG BUILT_AT
ARG GIT_SHA

RUN echo "BUILDPLATFORM: ${BUILDPLATFORM}, TARGETPLATFORM: ${TARGETPLATFORM}, TARGETOS: ${TARGETOS}, TARGETARCH: ${TARGETARCH}"

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# Copy third_party directory which contains local replacements referenced in go.mod
COPY third_party third_party
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
COPY api api
COPY properties properties
RUN go mod download

# Copy the go source
COPY main.go main.go
COPY controllers/ controllers/
COPY internal/ internal/
COPY pkg/ pkg/

# Build
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} GO111MODULE=on go build -a -o manager main.go

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/static-debian11:nonroot

# Redeclare ARG variables for the second stage
ARG VERSION
ARG BUILT_AT
ARG GIT_SHA

# Add metadata labels
LABEL org.opencontainers.image.title="Kafka Operator"
LABEL org.opencontainers.image.description="Kafka Operator for Kubernetes"
LABEL org.opencontainers.image.vendor="Adobe"
LABEL org.opencontainers.image.licenses="Apache-2.0"
LABEL org.opencontainers.image.source="https://github.com/adobe/koperator"
LABEL org.opencontainers.image.documentation="https://github.com/adobe/koperator/blob/main/README.md"
LABEL org.opencontainers.image.version="${VERSION}"
LABEL org.opencontainers.image.created="${BUILT_AT}"
LABEL org.opencontainers.image.revision="${GIT_SHA}"

WORKDIR /
COPY --from=builder /workspace/manager .
ENTRYPOINT ["/manager"]
