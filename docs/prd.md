# Product requirements

## Problem and boundary

Mail messages, discussion posts, and explicitly selected Cloud text need optional machine translation without embedding a model in each product or granting product authority to a translation process. gotth-extension-translate is one out-of-process adapter. The host authenticates users, authorizes and reduces content, obtains consent, owns audit/retention/presentation, and pins artifact, manifest, grant, Session, and transport identity. The extension validates that boundary, calls only an authenticated local gotth-translate service, and returns bounded derived text. It never mutates canonical content.

## V1 requirements

- TRX-001: advertise gotth.extension.translate and gotth.interface.translation major 1 minor 0 through the exact canonical manifest.
- TRX-002: accept only deadline-bound authenticated Unix-socket RPCs bound to the admitted manifest, grant, complete Session, and service token.
- TRX-003: accept ordered UTF-8 segments with opaque IDs, source tag or auto, target tag, content kind, and cache class; return exactly one ordered result per segment.
- TRX-004: use canonical BCP 47 at the public boundary.
- TRX-005: enforce 320 KiB request envelope, 256 segments, 64 KiB per segment, 256 KiB total source, bounded expansion, 1 MiB response, UUID correlation IDs, deadlines, and concurrency before backend work.
- TRX-006: return derived display data only; never rewrite, delete, sign, send, publish, or persist host source.
- TRX-007: expose no raw MIME, attachment, hidden header, rendered HTML, product database/session, storage path/key, credential, URL, or callback field.
- TRX-008: permit only one supervisor-connected Unix backend authenticated by pinned kernel peer credentials, a pre-token HMAC readiness proof, a private client token, and exact model identity. V1 has no remote egress capability or secret grant.
- TRX-009: never log or evidence source/translated text; record only stable classes, sizes, duration buckets, and correlation digests.
- TRX-010: return only experimental or evaluated quality tiers supplied by the admitted backend. Hosts must not advertise quality without retained evaluation evidence.
- TRX-011: perform no application or service-config retry. gRPC may transparently retry only when transport proves the backend did not process the attempt. Any failure after backend admission may have unknown outcome and must not be retried; asynchronous work belongs to gotth-jobs.
- TRX-012: fail closed on timeout, cancellation, language ambiguity, unsupported retention, malformed/reordered backend output, identity mismatch, or expansion.
- TRX-013: bind each product instance to its own grant, Session, and token; no product's authority admits another.
- TRX-014: consume gotth-extensions v1.0.0 and gotth-sdk v1.0.0 directly without replacements or copied control code.

## Consumer requirements

Mail sends decoded/sanitized visible text, never MIME/headers/attachments. Board tokenizes Markdown and protects code, links, mentions, and quote structure before translation. Cloud sends only explicit authorized extracted text, never encrypted/binary content or storage identifiers. The synthetic fixtures under testdata/consumers illustrate the intended reduced request shape and forbidden-field vocabulary; they do not prove any external consumer implementation. Consumer-owned integration remains a separate gate.

## Non-goals

UI localization, model training/registry/evaluation, remote providers, jobs, generic plugin/webhook/event APIs, automatic bulk translation, canonical-content mutation, product adapters, and deployment are outside v1.

## Acceptance

Source acceptance requires exact dependency binding, owned descriptor compatibility, hostile bounds/output tests, privacy proof, authenticated local-backend/process integration, negative egress proof, consumer fixtures, allocation/latency evidence, clean clone, race/repeat/fuzz/coverage/vet/build, deterministic artifact with license/SBOM/provenance/rollback, and two clean reviews. Publication additionally requires an exact clean commit and separately authorized immutable Forgejo tag and hosted release. GitHub mirror handoff is outside this release gate.
