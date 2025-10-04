FROM golang:1.25@sha256:d7098379b7da665ab25b99795465ec320b1ca9d4addb9f77409c4827dc904211 as builder

WORKDIR /workspace

# Copy the go code
COPY main.go main.go

# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GO111MODULE=on go build -a -o app main.go


FROM scratch
COPY --from=builder /workspace/app .
ENTRYPOINT ["/app"]
