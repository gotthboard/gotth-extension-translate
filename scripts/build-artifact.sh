#!/bin/sh
set -eu
if [ "$#" -ne 2 ]; then echo "usage: $0 <version> <output-directory>" >&2; exit 2; fi
version=$1
output=$2
[ "$version" = 1.0.0 ] || { echo 'only v1.0.0 is supported by this release builder' >&2; exit 2; }
root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
output=$(realpath -m -- "$output")
case "$output/" in "$root/"*) echo 'artifact output must be outside the source tree' >&2; exit 2;; esac
mkdir -p "$output"
allow_dirty=${GOTTH_EXTENSION_ALLOW_DIRTY_EVIDENCE:-0}
[ "$allow_dirty" = 0 ] || [ "$allow_dirty" = 1 ] || { echo 'GOTTH_EXTENSION_ALLOW_DIRTY_EVIDENCE must be 0 or 1' >&2; exit 2; }
dirty=false
if [ -n "$(git -C "$root" status --porcelain=v1 --untracked-files=all)" ]; then dirty=true; fi
if [ "$dirty" = true ] && [ "$allow_dirty" != 1 ]; then
  echo 'release artifact requires a clean source tree; set GOTTH_EXTENSION_ALLOW_DIRTY_EVIDENCE=1 only for provisional evidence' >&2
  exit 1
fi
source_commit=$(git -C "$root" rev-parse HEAD)
source_tree=$(git -C "$root" rev-parse "$source_commit^{tree}")
commit_epoch=$(git -C "$root" show -s --format=%ct "$source_commit")
epoch=${SOURCE_DATE_EPOCH:-$commit_epoch}
[ "$epoch" = "$commit_epoch" ] || { echo 'SOURCE_DATE_EPOCH must equal source commit time' >&2; exit 1; }
[ "$(go version)" = 'go version go1.27.3-X:nodwarf5 linux/amd64' ] || { echo 'unexpected Go toolchain' >&2; exit 1; }
[ "$(sha256sum "$(command -v go)" | cut -d' ' -f1)" = 21401c8d2dae2e6db0d756f7b9ae5a885ac6c14882cbbe521c6f49b184da8402 ] || { echo 'unexpected Go executable' >&2; exit 1; }
export GOENV=off GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOFLAGS=-mod=readonly GOEXPERIMENT=nodwarf5 CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GOAMD64=v1
go mod verify
"$root/scripts/verify-release-metadata.sh" >/dev/null
stage=$(mktemp -d "$output/.gotth-extension-translate.XXXXXX")
published_archive=
published_sidecar=
archive=
sidecar=
cleanup() {
  status=$?
  trap - EXIT
  if [ "$status" -ne 0 ]; then
    [ -z "$published_sidecar" ] || rm -f -- "$sidecar"
    [ -z "$published_archive" ] || rm -f -- "$archive"
  fi
  rm -rf -- "$stage"
  exit "$status"
}
abort() { exit 1; }
trap cleanup EXIT
trap abort HUP INT TERM
source_root=$root
if [ "$dirty" = false ]; then
  source_root="$stage/source"
  mkdir -p "$source_root"
  git -C "$root" archive --format=tar "$source_commit" | tar -xf - -C "$source_root"
  [ "$(git -C "$root" rev-parse "$source_commit^{tree}")" = "$source_tree" ]
fi
name="gotth-extension-translate-$version-linux-amd64"
mkdir -p "$stage/$name/THIRD_PARTY_LICENSES"
(
  cd "$source_root"
  go build -trimpath -buildvcs=false -ldflags "-s -w -buildid= -X github.com/gotthboard/gotth-extension-translate/internal/translate.BuildVersion=$version" -o "$stage/$name/gotth-extension-translate" ./cmd/gotth-extension-translate
  go run ./cmd/gotth-extension-translate-manifest "$version" > "$stage/$name/manifest.json"
)
test "$("$stage/$name/gotth-extension-translate" --version)" = "$version"
cp "$source_root/LICENSE" "$stage/$name/LICENSE"
cp "$source_root/docs/release/sbom.spdx.json" "$stage/$name/sbom.spdx.json"
cp "$source_root/docs/release/ROLLBACK.md" "$stage/$name/ROLLBACK.md"
cp "$source_root/docs/release/UPGRADE.md" "$stage/$name/UPGRADE.md"
cp "$source_root/docs/release/toolchain.json" "$stage/$name/toolchain.json"
cp "$source_root/docs/release/dependency-licenses.json" "$stage/$name/dependency-licenses.json"
python3 - "$source_root/docs/release/dependency-licenses.json" "$(go env GOMODCACHE)" "$stage/$name/THIRD_PARTY_LICENSES" <<'PY'
import json, pathlib, shutil, sys
inventory, cache, output = map(pathlib.Path, sys.argv[1:])
for package in json.loads(inventory.read_text())["packages"]:
    source = cache / f"{package['module']}@{package['version']}"
    destination = output / f"{package['module']}@{package['version']}"
    destination.mkdir(parents=True, exist_ok=True)
    for record in package["legal_files"]:
        shutil.copyfile(source / record["path"], destination / record["path"])
PY
if [ "$dirty" = false ]; then
  source_state=$(git -C "$root" ls-tree -r --full-tree "$source_commit" | sha256sum | cut -d' ' -f1)
else
  source_tree="dirty-worktree"
  source_state=$(cd "$root" && git ls-files --cached --others --exclude-standard | LC_ALL=C sort | while IFS= read -r file; do sha256sum "$file"; done | sha256sum | cut -d' ' -f1)
fi
binary_sha=$(sha256sum "$stage/$name/gotth-extension-translate" | cut -d' ' -f1)
sbom_sha=$(sha256sum "$stage/$name/sbom.spdx.json" | cut -d' ' -f1)
toolchain_sha=$(sha256sum "$stage/$name/toolchain.json" | cut -d' ' -f1)
licenses_sha=$(sha256sum "$stage/$name/dependency-licenses.json" | cut -d' ' -f1)
python3 - "$stage/$name/provenance.json" "$version" "$source_commit" "$source_tree" "$source_state" "$dirty" "$binary_sha" "$sbom_sha" "$toolchain_sha" "$licenses_sha" <<'PY'
import json, pathlib, sys
(path, version, commit, tree, state, dirty, binary, sbom, toolchain, licenses) = sys.argv[1:]
data = {"schema":"gotth.release.provenance.v1","version":version,"platform":"linux/amd64","source_commit":commit,"source_tree":tree,"source_state_sha256":state,"source_dirty":dirty == "true","binary_sha256":binary,"sbom_sha256":sbom,"toolchain_sha256":toolchain,"dependency_licenses_sha256":licenses,"builder":"scripts/build-artifact.sh","build_parameters":{"CGO_ENABLED":"0","GOOS":"linux","GOARCH":"amd64","GOAMD64":"v1","GOENV":"off","GOTOOLCHAIN":"local","GOPROXY":"off","GOSUMDB":"off","GOFLAGS":"-mod=readonly","GOEXPERIMENT":"nodwarf5","trimpath":True,"buildvcs":False,"buildid":""}}
pathlib.Path(path).write_text(json.dumps(data, sort_keys=True, separators=(",", ":")) + "\n")
PY
(cd "$stage/$name" && find . -type f ! -name SHA256SUMS -print0 | LC_ALL=C sort -z | xargs -0 sha256sum > SHA256SUMS)
chmod 00755 "$stage/$name"
find "$stage/$name" -type d -exec chmod 00755 {} +
find "$stage/$name" -type f -exec chmod 0644 {} +
chmod 0755 "$stage/$name/gotth-extension-translate"
temporary_tar="$stage/$name.tar"
temporary="$stage/$name.tar.gz"
tar --format=ustar --sort=name --mtime="@$epoch" --owner=0 --group=0 --numeric-owner -C "$stage" -cf "$temporary_tar" "$name"
gzip -n < "$temporary_tar" > "$temporary"
rm -f -- "$temporary_tar"
gzip -t "$temporary"
validation="$stage/validation"
mkdir -p "$validation"
tar -xzf "$temporary" -C "$validation"
test "$("$validation/$name/gotth-extension-translate" --version)" = "$version"
(cd "$validation/$name" && sha256sum -c SHA256SUMS >/dev/null)
archive="$output/$name.tar.gz"
sidecar="$archive.sha256"
[ ! -e "$archive" ] && [ ! -e "$sidecar" ] || { echo 'artifact or checksum sidecar already exists' >&2; exit 1; }
digest=$(sha256sum "$temporary" | cut -d' ' -f1)
printf '%s  %s\n' "$digest" "$name.tar.gz" > "$temporary.sha256"
(cd "$stage" && sha256sum -c "$name.tar.gz.sha256" >/dev/null)
trap '' HUP INT TERM
ln "$temporary" "$archive"
published_archive=1
ln "$temporary.sha256" "$sidecar"
published_sidecar=1
trap abort HUP INT TERM
sync -d "$archive" "$sidecar" "$output"
published_archive= published_sidecar=
cat "$sidecar"
