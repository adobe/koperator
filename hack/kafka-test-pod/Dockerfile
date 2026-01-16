FROM golang:1.25@sha256:bc45dfd319e982dffe4de14428c77defe5b938e29d9bc6edfbc0b9a1fc171cb3 as builder

WORKDIR /workspace

# Copy the go code
COPY main.go main.go

# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GO111MODULE=on go build -a -o app main.go


FROM scratch
COPY --from=builder /workspace/app .
ENTRYPOINT ["/app"]
