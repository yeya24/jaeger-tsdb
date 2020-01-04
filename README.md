# jaeger-tsdb

## About the project

Jaeger-tsdb is a Jaeger storage plugin based on the grpc-plugin mechanism. It stores Jaeger's data using badger and Promemtheus TSDB (actually modified version).

This project is for fun and practise. Please don't use it because the high cardinality data model is not suitable for Prometheus TSDB.

## How to use

1. Build the plugin binary

```
GO111MODULE=on go build -o jaeger-tsdb 
```

2. Start Jaeger

```
SPAN_STORAGE_TYPE=grpc-plugin ./all-in-one --grpc-storage-plugin.binary=jaeger-tsdb
```

## Example query

1. Get Services

```
curl "localhost:16686/api/services" | jq
{
  "data": [
    "jaeger-query",
    "thanos-querier"
  ],
  "total": 2,
  "limit": 0,
  "offset": 0,
  "errors": null
}
```

2. Get Operations

```
curl "localhost:16686/api/operations?service=thanos-querier" | jq

{
  "data": [
    "/query HTTP[server]",
    "/thanos.Store/Info",
    "/thanos.Store/Series",
    "promqlEval",
    "promqlExec",
    "promqlExecQueue",
    "promqlInnerEval",
    "promqlPrepare",
    "promql_instant_query",
    "querier_select",
    "store_matches"
  ],
  "total": 11,
  "limit": 0,
  "offset": 0,
  "errors": null
}
```

3. Get Traces

```
curl "localhost:16686/api/traces?service=jaeger-query" | jq
{
  "data": [
    {
      "traceID": "59ce4948bbc263f5",
      "spans": [
        {
          "traceID": "59ce4948bbc263f5",
          "spanID": "59ce4948bbc263f5",
          "flags": 1,
          "operationName": "/api/operations",
          "references": [],
          "startTime": 1578101732193403,
          "duration": 2104,
          "tags": [
            {
              "key": "sampler.type",
              "type": "string",
              "value": "const"
            },
            {
              "key": "sampler.param",
              "type": "bool",
              "value": true
            },
    TL;DR
```