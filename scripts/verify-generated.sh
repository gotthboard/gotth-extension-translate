#!/bin/sh
set -eu
PATH="$(go env GOPATH)/bin:$PATH"; export PATH
root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
[ "$(protoc --version)" = 'libprotoc 36.1' ]
[ "$(protoc-gen-go --version)" = 'protoc-gen-go v1.36.11' ]
[ "$(protoc-gen-go-grpc --version)" = 'protoc-gen-go-grpc 1.6.2' ]
[ "$(sha256sum "$(command -v go)" | cut -d' ' -f1)" = 21401c8d2dae2e6db0d756f7b9ae5a885ac6c14882cbbe521c6f49b184da8402 ]
[ "$(sha256sum "$(command -v protoc)" | cut -d' ' -f1)" = 4b7137d2236700cd9a6b92898ccbe3dd869917912e05017591af811c8387bfbf ]
[ "$(sha256sum "$(command -v protoc-gen-go)" | cut -d' ' -f1)" = 4be7d281ba025a9c3091ec3e2d43e11b87ea102c4617601792642faf8fd479da ]
[ "$(sha256sum "$(command -v protoc-gen-go-grpc)" | cut -d' ' -f1)" = 42d84fd65a6fddaf21319a6ec7275f568007921d14860f2728056ee900d5a25c ]
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT HUP INT TERM
cd "$root"
protoc --go_out="$tmp" --go_opt=paths=source_relative --go-grpc_out="$tmp" --go-grpc_opt=paths=source_relative proto/gotth/translation/v1/translation.proto
cmp proto/gotth/translation/v1/translation.pb.go "$tmp/proto/gotth/translation/v1/translation.pb.go"
cmp proto/gotth/translation/v1/translation_grpc.pb.go "$tmp/proto/gotth/translation/v1/translation_grpc.pb.go"
protoc --descriptor_set_out="$tmp/translation-descriptor.pb" --include_imports proto/gotth/translation/v1/translation.proto
[ "$(sha256sum "$tmp/translation-descriptor.pb" | cut -d' ' -f1)" = "$(cat compat/v1/translation-descriptor.sha256)" ]
