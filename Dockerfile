# Base images are pinned by digest; Dependabot's docker ecosystem
# (.github/dependabot.yml) keeps the tag and digest pairs current.
FROM golang:1.27.1@sha256:512690a5660563b57d37ecc31129e7f136e831db2aed24a1dbeb8ad7380dc0fa AS builder

WORKDIR /go/src/github.com/NVIDIA/topograph
COPY . .

ARG TARGETOS
ARG TARGETARCH

RUN make build-${TARGETOS}-${TARGETARCH}

FROM alpine:3.24@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b

RUN apk add --no-cache rdma-core

COPY --from=builder /go/src/github.com/NVIDIA/topograph/bin/* /usr/local/bin/

LABEL org.opencontainers.image.documentation="https://github.com/NVIDIA/topograph/blob/main/docs/overview.md" \
    org.opencontainers.image.authors="NVIDIA CORPORATION" \
    org.opencontainers.image.vendor="NVIDIA"
