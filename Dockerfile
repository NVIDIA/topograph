FROM golang:1.25.9 AS builder

WORKDIR /go/src/github.com/NVIDIA/topograph
COPY . .

ARG TARGETOS
ARG TARGETARCH

RUN make build-${TARGETOS}-${TARGETARCH}

FROM alpine:3

ARG VERSION=dev
ARG REVISION=unknown

RUN apk add --no-cache rdma-core

COPY --from=builder /go/src/github.com/NVIDIA/topograph/bin/* /usr/local/bin/

LABEL org.opencontainers.image.title="Topograph" \
    org.opencontainers.image.description="Discovers the physical network topology of a cluster and exposes it to schedulers." \
    org.opencontainers.image.url="https://github.com/NVIDIA/topograph" \
    org.opencontainers.image.source="https://github.com/NVIDIA/topograph" \
    org.opencontainers.image.documentation="https://github.com/NVIDIA/topograph/blob/main/docs/overview.md" \
    org.opencontainers.image.authors="NVIDIA CORPORATION" \
    org.opencontainers.image.vendor="NVIDIA" \
    org.opencontainers.image.licenses="Apache-2.0" \
    org.opencontainers.image.version="${VERSION}" \
    org.opencontainers.image.revision="${REVISION}"
