#!/bin/sh
set -eu
PATH="$(go env GOPATH)/bin:$PATH"; export PATH
root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
actual=$(mktemp)
trap 'rm -f "$actual"' EXIT HUP INT TERM
cd "$root"
go list -deps -f '{{with .Module}}{{if ne .Path "github.com/gotthboard/gotth-extension-translate"}}{{.Path}} {{.Version}}{{end}}{{end}}' ./cmd/... ./internal/... | sed '/^$/d' | sort -u > "$actual"
diff -u docs/release/production-modules.txt "$actual"
python3 - "$root/docs/release/production-modules.txt" "$root/docs/release/dependency-licenses.json" "$root/docs/release/sbom.spdx.json" "$root/docs/release/toolchain.json" "$(go env GOMODCACHE)" <<'PY'
import hashlib, json, pathlib, re, shutil, subprocess, sys
modules_path, licenses_path, sbom_path, toolchain_path, cache = sys.argv[1:]
expected = dict(line.split(" ", 1) for line in pathlib.Path(modules_path).read_text().splitlines())
expected_licenses = {
 "github.com/gotthboard/gotth-extensions":"MIT", "github.com/gotthboard/gotth-sdk":"MIT",
 "golang.org/x/net":"BSD-3-Clause", "golang.org/x/sys":"BSD-3-Clause", "golang.org/x/text":"BSD-3-Clause",
 "google.golang.org/genproto/googleapis/rpc":"Apache-2.0", "google.golang.org/grpc":"Apache-2.0",
 "google.golang.org/protobuf":"BSD-3-Clause",
}
licenses = json.loads(pathlib.Path(licenses_path).read_text())
if licenses.get("schema") != "gotth.release.dependency-licenses.v1" or licenses.get("generated_for") != "gotth-extension-translate v1.0.0":
    raise SystemExit("license inventory identity mismatch")
records = {item["module"]: item for item in licenses["packages"]}
if len(records) != len(licenses["packages"]) or {name: item["version"] for name, item in records.items()} != expected:
    raise SystemExit("license inventory module set mismatch")
if {name: item.get("license") for name, item in records.items()} != expected_licenses:
    raise SystemExit("license identifiers mismatch")
for name, item in records.items():
    module_root = pathlib.Path(cache) / f"{name}@{item['version']}"
    files = item.get("legal_files", [])
    if not files or len({x.get("path") for x in files}) != len(files):
        raise SystemExit(f"legal file inventory invalid: {name}")
    for record in files:
        relative = record.get("path", "")
        if not re.fullmatch(r"(?:LICENSE(?:\.txt)?|NOTICE(?:\.txt)?|AUTHORS|PATENTS)", relative):
            raise SystemExit(f"unexpected legal file path: {name}/{relative}")
        source = module_root / relative
        if not source.is_file() or hashlib.sha256(source.read_bytes()).hexdigest() != record.get("sha256"):
            raise SystemExit(f"legal file hash mismatch: {name}/{relative}")
    if not any(x["path"].startswith("LICENSE") for x in files):
        raise SystemExit(f"license text absent: {name}")
    if (module_root / "NOTICE.txt").is_file() and not any(x["path"] == "NOTICE.txt" for x in files):
        raise SystemExit(f"notice omitted: {name}")
sbom = json.loads(pathlib.Path(sbom_path).read_text())
if sbom.get("spdxVersion") != "SPDX-2.3" or sbom.get("dataLicense") != "CC0-1.0" or sbom.get("name") != "gotth-extension-translate-1.0.0" or sbom.get("documentNamespace") != "https://git.dannyhunn.com/gotthboard/gotth-extension-translate/sbom/v1.0.0":
    raise SystemExit("SPDX document identity mismatch")
packages = sbom["packages"]
ids = [item["SPDXID"] for item in packages]
if len(ids) != len(set(ids)):
    raise SystemExit("duplicate SPDX package ID")
root_package = next((item for item in packages if item["SPDXID"] == "SPDXRef-Package-root"), None)
if not root_package or root_package.get("name") != "gotth-extension-translate" or root_package.get("versionInfo") != "1.0.0" or root_package.get("licenseDeclared") != "MIT" or root_package.get("licenseConcluded") != "MIT":
    raise SystemExit("SPDX root package mismatch")
refs = {(x.get("referenceType"), x.get("referenceLocator")) for x in root_package.get("externalRefs", [])}
if ("purl", "pkg:golang/github.com/gotthboard/gotth-extension-translate@v1.0.0") not in refs:
    raise SystemExit("SPDX root purl mismatch")
by_name = {item["name"]: item for item in packages if item["SPDXID"] != "SPDXRef-Package-root"}
if {name: item["versionInfo"] for name, item in by_name.items()} != expected:
    raise SystemExit("SPDX production module set mismatch")
for name, item in by_name.items():
    if item.get("licenseDeclared") != expected_licenses[name] or item.get("licenseConcluded") != expected_licenses[name]:
        raise SystemExit(f"SPDX license mismatch: {name}")
describes = {(item["spdxElementId"], item["relationshipType"], item["relatedSpdxElement"]) for item in sbom["relationships"] if item["relationshipType"] == "DESCRIBES"}
if describes != {("SPDXRef-DOCUMENT", "DESCRIBES", "SPDXRef-Package-root")}:
    raise SystemExit("SPDX document subject relationship mismatch")
related = {item["relatedSpdxElement"] for item in sbom["relationships"] if item["spdxElementId"] == "SPDXRef-Package-root" and item["relationshipType"] == "DEPENDS_ON"}
if related != {item["SPDXID"] for item in by_name.values()}:
    raise SystemExit("SPDX dependency relationships incomplete")
toolchain = json.loads(pathlib.Path(toolchain_path).read_text())
if toolchain.get("schema") != "gotth.release.toolchain.v1" or toolchain.get("platform") != "linux/amd64" or toolchain.get("goexperiment") != "nodwarf5":
    raise SystemExit("toolchain metadata identity mismatch")
commands = {"go":["go","version"], "protoc":["protoc","--version"], "protoc-gen-go":["protoc-gen-go","--version"], "protoc-gen-go-grpc":["protoc-gen-go-grpc","--version"]}
expected_versions = {"go":"go version go1.27.3-X:nodwarf5 linux/amd64", "protoc":"libprotoc 36.1", "protoc-gen-go":"protoc-gen-go v1.36.11", "protoc-gen-go-grpc":"protoc-gen-go-grpc 1.6.2"}
tools = {item["name"]: item for item in toolchain.get("tools", [])}
if set(tools) != set(commands) or len(tools) != len(toolchain.get("tools", [])):
    raise SystemExit("toolchain metadata set mismatch")
for name, command in commands.items():
    executable = shutil.which(command[0])
    if not executable:
        raise SystemExit(f"tool missing: {name}")
    version = subprocess.run(command, check=True, text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT).stdout.strip()
    digest = hashlib.sha256(pathlib.Path(executable).read_bytes()).hexdigest()
    if version != expected_versions[name] or tools[name].get("version_command_output") != version or tools[name].get("sha256") != digest:
        raise SystemExit(f"toolchain tuple mismatch: {name}")
PY
echo 'production dependency, legal text, SPDX, and exact toolchain metadata verified'
