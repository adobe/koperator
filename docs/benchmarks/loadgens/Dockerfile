ARG GO_VERSION=1.25

FROM golang:${GO_VERSION}-alpine AS builder

ENV GOFLAGS="-mod=readonly"

RUN apk add --update --no-cache ca-certificates make git curl mercurial

RUN mkdir -p /build
WORKDIR /build

RUN git clone https://github.com/jamiealquiza/sangrenel.git

WORKDIR /build/sangrenel

RUN go mod download && go build -o /sangrenel

FROM alpine:3.24@sha256:3f4d6b32e26703c6082839234f43769d5046b3ac7c910033911d1e71d8cd630a

RUN apk add --update --no-cache ca-certificates tzdata curl bash

COPY --from=builder /sangrenel /sangrenel

ENTRYPOINT [ "/sangrenel" ]
