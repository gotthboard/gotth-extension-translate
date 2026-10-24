# Implementation specification

## Status and dependencies

V1 is implemented in Go and remains an untagged release candidate. go.mod selects github.com/gotthboard/gotth-extensions v1.0.0 and github.com/gotthboard/gotth-sdk v1.0.0 without replace directives. Shared control messages/services are imported from the SDK. This repository owns only proto/gotth/translation/v1/translation.proto and its generated bindings.

## Wire contract

Package gotth.translation.v1 exposes one host-facing unary method, Translation.Translate. It also defines private-backend-only BackendIdentity.Prove, which is never registered on the host socket. TranslateRequest fields are correlation_id=1, source_language=2, target_language=3, content_kind=4, cache_class=5, segments=6. Segment is id=1, text=2. TranslateResponse is target_language=1, results=2. SegmentResult is id=1, detected_source_language=2, text=3, quality_tier=4, model_identity=5, result_class=6. Exact enums and compatibility rules are in docs/compatibility-v1.md.

Source language is auto or canonical BCP 47; target is a non-und canonical tag. Content kinds are plain, mail-visible-text, markdown-segment, and document-text. Cache classes are none and request. Segment IDs are 1..128-byte UTF-8 opaque non-space/non-control values. Text is nonempty UTF-8 without NUL.

## Admission and transport

Runtime inputs are private owned regular files and private directories. Symlinks, permissive modes, noncanonical paths, duplicate/unknown JSON fields, noncanonical manifest/grant JSON, stale digests, extra capability/interface/secret authority, existing host socket paths, invalid inherited descriptors, and backend peer/proof mismatches fail startup. A unary interceptor requires a deadline and printable-ASCII service token before every host handler; handlers repeat transport checks defensively. Translate additionally requires the exact Session fingerprint inside the method. Control handshake/health use the published SDK.

Only adoption of a connected Unix descriptor is compiled into the backend adapter. It consumes one supervisor-connected Unix descriptor, verifies SO_PEERCRED PID/UID/GID, completes a challenge HMAC before sending any token, and never redials a pathname. It then sends a backend token and exact caller ID, caps message sizes, propagates cancellation, and disables service-config/application retries. gRPC transparent retry remains possible only for attempts the transport proves unprocessed. READY follows this proof. Configuration pins the exact model identity and exactly one experimental or evaluated quality tier; results that claim another tier fail closed. No net/http, TLS, URL, subprocess, plugin, remote provider, or arbitrary network selection exists.

## Bounds and errors

The constants and stable lowercase error messages are in internal/translate/service.go and frozen by tests. Invalid requests never invoke the backend. Unknown enums/fields in owned Translate request, segment, response, and result messages fail. Imported SDK control messages retain protobuf unknown fields normally. Backend failure maps to backend_unavailable, timeout to backend_timeout, cancellation to request_cancelled, malformed output to malformed_backend_response, expansion to output_too_large, and saturation to capacity_exhausted. Errors contain no content or rejected value.

## Verification and artifacts

Unit tests cover grant/session mutation, transport authentication, BCP 47, critical and exact request/result bounds, duplicates, timeout/cancellation, admitted backend concurrency, hostile output, privacy, descriptors, no-egress source, allocation/latency, local backend, and synthetic consumer-boundary fixtures. The compiled process test covers startup, socket permissions, unauthenticated rejection, handshake, translation, SIGTERM, socket cleanup, and stderr redaction. Scripts pin dependencies/generators, regenerate bindings, and build a deterministic archive containing binary, canonical manifest, project license, exact third-party license/notice texts, SPDX SBOM, exact provenance, checksums, upgrade, and rollback instructions.
