FROM golang:1.27@sha256:3680233e3204827fbdc66088528ae6d4b3d034f51d03a99d454f6de034888244 as builder

WORKDIR /workspace

# Copy the go code
COPY main.go main.go

# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GO111MODULE=on go build -a -o app main.go


FROM scratch
COPY --from=builder /workspace/app .
ENTRYPOINT ["/app"]
