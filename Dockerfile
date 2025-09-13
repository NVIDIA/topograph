# syntax=docker/dockerfile:1

FROM --platform=${TARGETOS}/${TARGETARCH} golang:1.23.3 AS builder

WORKDIR /go/src/github.com/NVIDIA/topograph
COPY . .

ARG TARGETOS
ARG TARGETARCH

RUN make build-${TARGETOS}-${TARGETARCH}

FROM --platform=${TARGETOS}/${TARGETARCH} gcr.io/distroless/static-debian11:nonroot

COPY --from=builder /go/src/github.com/NVIDIA/topograph/bin/* /usr/local/bin/
