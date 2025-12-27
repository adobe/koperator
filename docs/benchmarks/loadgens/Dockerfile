ARG GO_VERSION=1.25

FROM golang:${GO_VERSION}-alpine AS builder

ENV GOFLAGS="-mod=readonly"

RUN apk add --update --no-cache ca-certificates make git curl mercurial

RUN mkdir -p /build
WORKDIR /build

RUN git clone https://github.com/jamiealquiza/sangrenel.git

WORKDIR /build/sangrenel

RUN go mod download && go build -o /sangrenel

FROM alpine:3.23@sha256:865b95f46d98cf867a156fe4a135ad3fe50d2056aa3f25ed31662dff6da4eb62

RUN apk add --update --no-cache ca-certificates tzdata curl bash

COPY --from=builder /sangrenel /sangrenel

ENTRYPOINT [ "/sangrenel" ]
