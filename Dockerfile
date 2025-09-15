FROM golang:1.24.7 AS builder

WORKDIR /go/src/github.com/NVIDIA/topograph
COPY . .

ARG TARGETOS
ARG TARGETARCH

RUN make build-${TARGETOS}-${TARGETARCH}

FROM gcr.io/distroless/static-debian11:nonroot

COPY --from=builder /go/src/github.com/NVIDIA/topograph/bin/* /usr/local/bin/
