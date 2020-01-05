FROM jaegertracing/all-in-one:1.14

COPY ./jaeger-tsdb /go/bin/

ENV SPAN_STORAGE_TYPE=grpc-plugin
ENV GRPC_STORAGE_PLUGIN_BINARY="/go/bin/jaeger-tsdb"

VOLUME ["/data"]