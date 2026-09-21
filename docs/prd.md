# Product requirements

## Problem

Mail messages, discussion posts, and selected Cloud documents need optional
machine translation without embedding a model in every product or giving a
translation process product authority. A generic webhook is the wrong
mechanism: translation has a typed request/response contract, strict content
bounds, privacy consequences, and language-quality claims.

## Product boundary

`gotth-extension-translate` is one out-of-process extension. It implements one
versioned translation interface and delegates translation to
`gotth-translate`. The shared `gotth-extensions` protocol supplies only
manifest, grant, negotiation, handshake, health, and lifecycle control.

The host:

- authenticates the user and authorizes the source resource;
- chooses and reduces the exact text segments;
- obtains any required consent and records audit;
- owns original content, translated presentation, retention, and deletion;
- pins the extension artifact, manifest digest, grant, and transport identity.

The extension:

- validates the effective grant and bounded translation request;
- calls only the configured translation backend allowed by the grant;
- returns derived text and bounded model/language metadata;
- retains no product authority and never mutates canonical content.

## Requirements

- `TRX-001`: Advertise extension ID `gotth.extension.translate` and interface
  `gotth.interface.translation` through an exact `gotth-extensions` manifest.
- `TRX-002`: Accept only host-called, authenticated RPCs bound to an admitted
  manifest digest, grant, interface version, and transport identity.
- `TRX-003`: Accept ordered UTF-8 text segments with caller-generated opaque
  segment IDs, source-language mode, target language, content kind, and an
  explicit cache/retention class. Return one result per admitted segment in
  the same order.
- `TRX-004`: Use canonical BCP 47 language tags at the product boundary. A
  backend-specific language ID is an internal mapping, not a public contract.
- `TRX-005`: Bound serialized request bytes, segment count, per-segment bytes,
  output expansion, deadlines, correlation IDs, and concurrent work. Reject
  partial, duplicate, malformed, or over-limit input before backend work.
- `TRX-006`: Treat translation as derived display data. Never rewrite, delete,
  sign, send, publish, or otherwise mutate the host's canonical source.
- `TRX-007`: Receive no raw MIME, attachments, hidden mail headers, rendered
  HTML, product database connection, storage path, object key, authentication
  token, administrator session, or arbitrary host callback.
- `TRX-008`: Make network egress deny-by-default. A local `gotth-translate`
  backend requires only its exact service identity. Any remote provider needs
  a separate named egress/secret grant and visible host-owned disclosure and
  consent.
- `TRX-009`: Never log or place source or translated text in health, control,
  audit, metrics, errors, traces, workflow evidence, or crash reports.
- `TRX-010`: Advertise only languages and quality tiers backed by retained
  evaluation evidence. Experimental languages remain visibly experimental.
- `TRX-011`: Make synchronous requests read-only and safe to retry only before
  an admitted backend result. Long-running work uses a separately admitted
  `gotth-jobs` boundary rather than an invented extension callback.
- `TRX-012`: Fail closed on backend timeout, ambiguous language mapping,
  unsupported retention policy, malformed output, output expansion, or
  incompatible interface version. The host keeps the original usable.
- `TRX-013`: Keep Mail, BB, and Cloud adapters product-owned. One product's
  grant, secret, cache, request, or audit state never authorizes another.

## Consumer requirements

### GOTTH Mail

Mail may send bounded decoded and sanitized visible body text only after
mailbox authorization. The original message remains untouched. The extension
receives no raw MIME, attachment bytes, credential, hidden header, or remote
image URL.

### GOTTH-BB

Board authorizes the post before translation and protects Markdown structure,
code blocks, links, mentions, and quote boundaries. Original Markdown remains
canonical. Returned text is escaped and rendered by Board; the extension never
returns executable HTML or frontend code.

### GOTTH Cloud

Cloud sends only explicitly selected, authorized, extractable text. It never
silently uploads an encrypted/private file, full account tree, binary object,
storage path, or object key. Large documents require the future jobs boundary.

## Non-goals

- UI localization, locale catalogs, date/number formatting, or RTL layout.
- A translation model, model registry, or evaluation suite; those belong to
  `gotth-translate`.
- A generic plugin platform, webhook, event bus, workflow engine, or content
  ingestion service.
- Automatic translation of all mail, posts, or files.
- Editing canonical content, sending mail, publishing posts, or writing files.
- A release, deployment, or compatibility promise from these documents.

## Acceptance

The first alpha requires admitted translation and control contracts, exact
artifact/manifest/grant identity, hostile parser and output tests, privacy and
redaction proof, local-backend integration, negative egress tests, one
consumer adapter fixture for each declared product, rollback evidence, and
exact Forgejo/GitHub tag parity. Documentation alone satisfies none of those
runtime gates.
