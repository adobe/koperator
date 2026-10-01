ARG GO_VERSION=1.25

FROM golang:${GO_VERSION}-alpine AS builder

ENV GOFLAGS="-mod=readonly"

RUN apk add --update --no-cache ca-certificates make git curl mercurial

RUN mkdir -p /build
WORKDIR /build

RUN git clone https://github.com/jamiealquiza/sangrenel.git

WORKDIR /build/sangrenel

RUN go mod download && go build -o /sangrenel

FROM alpine:3.24@sha256:294b683cb724975bec92580e1e685676bd4b50bda910ddb8c51d4cabeaec77e6

RUN apk add --update --no-cache ca-certificates tzdata curl bash

COPY --from=builder /sangrenel /sangrenel

ENTRYPOINT [ "/sangrenel" ]
