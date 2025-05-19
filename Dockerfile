FROM golang:1.23.3 AS builder

WORKDIR /go/src/github.com/NVIDIA/topograph
COPY . .

ARG TARGETOS
ARG TARGETARCH

RUN make build-${TARGETOS}-${TARGETARCH}

FROM gcr.io/distroless/static-debian11:nonroot

COPY --from=builder /go/src/github.com/NVIDIA/topograph/bin/topograph /usr/local/bin/topograph
COPY --from=builder /go/src/github.com/NVIDIA/topograph/bin/node-observer /usr/local/bin/node-observer
