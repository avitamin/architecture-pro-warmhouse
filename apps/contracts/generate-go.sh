#!/bin/sh
set -eu
mkdir -p gen
protoc -I /workspace/docs/api/proto --go_out=gen --go_opt=module=warmhouse/contracts/gen \
  --go-grpc_out=gen --go-grpc_opt=module=warmhouse/contracts/gen \
  --include_imports --descriptor_set_out=gen/contracts.pb /workspace/docs/api/proto/warmhouse.proto
go mod download
go test -mod=readonly ./...
go vet -mod=readonly ./...
