FROM golang:1.27@sha256:2920bb124f1a2e12a1d39f0cab756ae0068441ee1da7655dec7f5e374944095b AS builder

WORKDIR /workspace

# Build from the repository root to use the root Go module.
COPY go.mod go.sum ./
COPY hack/kafka-test-pod/main.go ./
RUN CGO_ENABLED=0 go build -o app main.go

FROM scratch
COPY --from=builder /workspace/app .
ENTRYPOINT ["/app"]
