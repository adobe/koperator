FROM golang:1.25@sha256:6bac879c5b77e0fc9c556a5ed8920e89dab1709bd510a854903509c828f67f96 as builder

WORKDIR /workspace

# Copy the go code
COPY main.go main.go

# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GO111MODULE=on go build -a -o app main.go


FROM scratch
COPY --from=builder /workspace/app .
ENTRYPOINT ["/app"]
