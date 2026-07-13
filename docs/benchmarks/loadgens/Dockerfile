ARG GO_VERSION=1.25

FROM golang:${GO_VERSION}-alpine AS builder

ENV GOFLAGS="-mod=readonly"

RUN apk add --update --no-cache ca-certificates make git curl mercurial

RUN mkdir -p /build
WORKDIR /build

RUN git clone https://github.com/jamiealquiza/sangrenel.git

WORKDIR /build/sangrenel

RUN go mod download && go build -o /sangrenel

FROM alpine:3.24@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b

RUN apk add --update --no-cache ca-certificates tzdata curl bash

COPY --from=builder /sangrenel /sangrenel

ENTRYPOINT [ "/sangrenel" ]
