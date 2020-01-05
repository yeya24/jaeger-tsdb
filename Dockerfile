FROM golang:1.12-alpine AS builder

RUN apk update && apk add --no-cache git \
    ca-certificates

WORKDIR /app
COPY *.go ./
COPY go.* ./

RUN GO111MODULE=on go build -a -tags netgo -o /jaeger-tsdb

FROM jaegertracing/all-in-one:1.14

COPY --from=builder /jaeger-tsdb /go/bin/jaeger-tsdb

ENV SPAN_STORAGE_TYPE=grpc-plugin \
    GRPC_STORAGE_PLUGIN_BINARY="/go/bin/jaeger-tsdb"
