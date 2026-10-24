#!/bin/sh
set -eu
root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root"
selection=$(go list -m -f '{{if or (eq .Path "github.com/gotthboard/gotth-extensions") (eq .Path "github.com/gotthboard/gotth-sdk")}}{{.Path}} {{.Version}}{{with .Replace}} replace={{.Path}}@{{.Version}}{{end}}{{end}}' all | sed '/^$/d' | sort)
expected='github.com/gotthboard/gotth-extensions v1.0.0
github.com/gotthboard/gotth-sdk v1.0.0'
[ "$selection" = "$expected" ] || { printf 'unexpected foundation/SDK selection:\n%s\n' "$selection" >&2; exit 1; }
if go list -m -f '{{if .Replace}}{{.Path}} => {{.Replace.Path}}{{end}}' all | grep -q .; then echo 'module replacement is forbidden' >&2; exit 1; fi
grep -Fqx 'github.com/gotthboard/gotth-extensions v1.0.0 h1:lKXOydN/7+Mq9W2juqnWUXNJiirpYR6F3KPXYV55Hho=' go.sum
grep -Fqx 'github.com/gotthboard/gotth-sdk v1.0.0 h1:RmjRs/l+H3KMDmnimZs2OL8ExCchT/opVO5tDdjb4EA=' go.sum
extensions_dir=$(go list -m -f '{{.Dir}}' github.com/gotthboard/gotth-extensions)
sdk_dir=$(go list -m -f '{{.Dir}}' github.com/gotthboard/gotth-sdk)
source=proto/gotth/extensions/v1/control.proto
source_sha=846dbdd487e9688a338c7885478e1296456f994813330ecccdc3f4b225f10cfe
printf '%s  %s\n' "$source_sha" "$extensions_dir/$source" | sha256sum --check --status
printf '%s  %s\n' "$source_sha" "$sdk_dir/$source" | sha256sum --check --status
cmp "$extensions_dir/$source" "$sdk_dir/$source"
[ "$(cat "$extensions_dir/compat/v1/control-descriptor.sha256")" = 62a549b29a030f0decb46b2b1c2f382bbbc75430ba0b3d7c729bd21528b505cf ]
if find proto/gotth/extensions/v1 -type f -print -quit 2>/dev/null | grep -q .; then echo 'repository-local control copy forbidden' >&2; exit 1; fi
printf '%s\n' 'published foundation and SDK V1 binding verified'
