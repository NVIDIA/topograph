ARG RUNTIME_IMAGE=alpine:3

FROM golang:1.26.5 AS builder

WORKDIR /go/src/github.com/NVIDIA/topograph
COPY . .

ARG TARGETOS
ARG TARGETARCH

RUN make build-${TARGETOS}-${TARGETARCH}

FROM ${RUNTIME_IMAGE}

RUN if command -v apk >/dev/null 2>&1; then \
        apk add --no-cache rdma-core; \
    elif command -v apt-get >/dev/null 2>&1; then \
        apt-get update \
        && apt-get install -y --no-install-recommends rdma-core infiniband-diags \
        && rm -rf /var/lib/apt/lists/*; \
    else \
        echo "unsupported runtime image: expected apk or apt-get" >&2; \
        exit 1; \
    fi

COPY --from=builder /go/src/github.com/NVIDIA/topograph/bin/* /usr/local/bin/

LABEL org.opencontainers.image.documentation="https://github.com/NVIDIA/topograph/blob/main/docs/overview.md" \
    org.opencontainers.image.authors="NVIDIA CORPORATION" \
    org.opencontainers.image.vendor="NVIDIA"
