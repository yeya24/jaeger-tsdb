FROM jaegertracing/all-in-one:1.14

COPY jaeger-tsdb /

ENV SPAN_STORAGE_TYPE grpc-plugin



VOLUME ["/data"]
ENTRYPOINT ["/go/bin/all-in-one-linux"]

CMD ["--grpc-storage-plugin.binary=/jaeger-tsdb"]