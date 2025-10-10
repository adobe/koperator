FROM golang:1.25@sha256:1c91b4f4391774a73d6489576878ad3ff3161ebc8c78466ec26e83474855bfcf as builder

WORKDIR /workspace

# Copy the go code
COPY main.go main.go

# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GO111MODULE=on go build -a -o app main.go


FROM scratch
COPY --from=builder /workspace/app .
ENTRYPOINT ["/app"]
