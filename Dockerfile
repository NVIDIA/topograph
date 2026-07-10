FROM golang:1.25.12 AS builder

WORKDIR /go/src/github.com/NVIDIA/topograph
COPY . .

ARG TARGETOS
ARG TARGETARCH

RUN make build-${TARGETOS}-${TARGETARCH}

FROM alpine:3

RUN apk add --no-cache rdma-core

COPY --from=builder /go/src/github.com/NVIDIA/topograph/bin/* /usr/local/bin/

LABEL org.opencontainers.image.documentation="https://github.com/NVIDIA/topograph/blob/main/docs/overview.md" \
    org.opencontainers.image.authors="NVIDIA CORPORATION" \
    org.opencontainers.image.vendor="NVIDIA"
