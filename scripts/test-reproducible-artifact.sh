#!/bin/sh
set -eu
root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
left=$(mktemp -d); right=$(mktemp -d); extract=$(mktemp -d)
trap 'rm -rf "$left" "$right" "$extract"' EXIT HUP INT TERM
epoch=$(git -C "$root" show -s --format=%ct HEAD)
dirty=false
[ -z "$(git -C "$root" status --porcelain=v1 --untracked-files=all)" ] || dirty=true
build() {
  destination=$1
  if [ "$dirty" = true ]; then
    GOTTH_EXTENSION_ALLOW_DIRTY_EVIDENCE=1 SOURCE_DATE_EPOCH="$epoch" "$root/scripts/build-artifact.sh" 1.0.0 "$destination"
  else
    SOURCE_DATE_EPOCH="$epoch" "$root/scripts/build-artifact.sh" 1.0.0 "$destination"
  fi
}
if [ "$dirty" = true ] && SOURCE_DATE_EPOCH="$epoch" "$root/scripts/build-artifact.sh" 1.0.0 "$left/default-must-fail" >/dev/null 2>&1; then
  echo 'dirty release build accepted without evidence override' >&2
  exit 1
fi
build "$left" >/dev/null
build "$right" >/dev/null
name=gotth-extension-translate-1.0.0-linux-amd64
cmp "$left/$name.tar.gz" "$right/$name.tar.gz"
(cd "$left" && sha256sum -c "$name.tar.gz.sha256")
cmp "$left/$name.tar.gz.sha256" "$right/$name.tar.gz.sha256"
test "$(awk '{print $2}' "$left/$name.tar.gz.sha256")" = "$name.tar.gz"
tar -xzf "$left/$name.tar.gz" -C "$extract"
test "$(stat -c %a "$extract/$name/gotth-extension-translate")" = 755
test "$("$extract/$name/gotth-extension-translate" --version)" = 1.0.0
(cd "$extract/$name" && sha256sum -c SHA256SUMS)
python3 - "$extract/$name" "$dirty" <<'PY'
import hashlib, json, pathlib, re, sys
root, dirty = pathlib.Path(sys.argv[1]), sys.argv[2] == "true"
inventory = json.loads((root / "dependency-licenses.json").read_text())
expected = set()
for package in inventory["packages"]:
    for item in package["legal_files"]:
        path = pathlib.Path("THIRD_PARTY_LICENSES") / f"{package['module']}@{package['version']}" / item["path"]
        expected.add(path.as_posix())
        if hashlib.sha256((root / path).read_bytes()).hexdigest() != item["sha256"]:
            raise SystemExit(f"legal file hash mismatch: {path}")
actual = {p.relative_to(root).as_posix() for p in (root / "THIRD_PARTY_LICENSES").rglob("*") if p.is_file()}
if actual != expected:
    raise SystemExit(f"legal file inventory mismatch: {actual ^ expected}")
provenance = json.loads((root / "provenance.json").read_text())
if provenance["source_dirty"] is not dirty:
    raise SystemExit("provenance dirty state mismatch")
if dirty and provenance["source_tree"] != "dirty-worktree":
    raise SystemExit("dirty evidence tree marker mismatch")
if not dirty and not re.fullmatch(r"[0-9a-f]{40}", provenance["source_tree"]):
    raise SystemExit("clean source tree missing")
if provenance["build_parameters"].get("GOAMD64") != "v1":
    raise SystemExit("GOAMD64 provenance missing")
for field, file in [("sbom_sha256", "sbom.spdx.json"), ("toolchain_sha256", "toolchain.json"), ("dependency_licenses_sha256", "dependency-licenses.json")]:
    if provenance[field] != hashlib.sha256((root / file).read_bytes()).hexdigest():
        raise SystemExit(f"provenance digest mismatch: {field}")
PY
before=$(sha256sum "$right/$name.tar.gz" "$right/$name.tar.gz.sha256")
if build "$right" >/dev/null 2>&1; then echo 'existing artifact replaced' >&2; exit 1; fi
test "$before" = "$(sha256sum "$right/$name.tar.gz" "$right/$name.tar.gz.sha256")"
if SOURCE_DATE_EPOCH="$epoch" "$root/scripts/build-artifact.sh" 1.0.0-alpha.1 "$right/alpha" >/dev/null 2>&1; then echo 'unsupported prerelease artifact accepted' >&2; exit 1; fi
if [ "$dirty" = true ]; then override=GOTTH_EXTENSION_ALLOW_DIRTY_EVIDENCE=1; else override=; fi
if env $override SOURCE_DATE_EPOCH="$epoch" "$root/scripts/build-artifact.sh" 1.0.0 "$root/.artifact-output-test" >/dev/null 2>&1; then echo 'in-tree artifact output accepted' >&2; exit 1; fi
test ! -e "$root/.artifact-output-test"
real_tar=$(command -v tar)
failbin="$left/failbin"
failure_output="$left/injected-archive-failure"
mkdir -p "$failbin"
cat > "$failbin/tar" <<'SH'
#!/bin/sh
for argument do
  [ "$argument" != --format=ustar ] || exit 97
done
exec "$REAL_TAR" "$@"
SH
chmod 0755 "$failbin/tar"
if env $override REAL_TAR="$real_tar" PATH="$failbin:$PATH" SOURCE_DATE_EPOCH="$epoch" "$root/scripts/build-artifact.sh" 1.0.0 "$failure_output" >/dev/null 2>&1; then
  echo 'injected archive-creation failure was accepted' >&2
  exit 1
fi
test ! -e "$failure_output/$name.tar.gz"
test ! -e "$failure_output/$name.tar.gz.sha256"
