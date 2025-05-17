FROM golang:1.23.3 AS builder

WORKDIR /go/src/github.com/NVIDIA/topograph
COPY . .

ARG TARGETOS
ARG TARGETARCH

RUN make build-${TARGETOS}-${TARGETARCH}

FROM gcr.io/distroless/static-debian11:nonroot

ARG TARGETOS
ARG TARGETARCH

COPY --from=builder /go/src/github.com/NVIDIA/topograph/bin/topograph-${TARGETOS}-${TARGETARCH} /bin/topograph
COPY --from=builder /go/src/github.com/NVIDIA/topograph/bin/node-observer-${TARGETOS}-${TARGETARCH} /bin/node-observer
