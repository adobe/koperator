FROM golang:1.25@sha256:a22b2e6c5e753345b9759fba9e5c1731ebe28af506745e98f406cc85d50c828e as builder

WORKDIR /workspace

# Copy the go code
COPY main.go main.go

# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GO111MODULE=on go build -a -o app main.go


FROM scratch
COPY --from=builder /workspace/app .
ENTRYPOINT ["/app"]
