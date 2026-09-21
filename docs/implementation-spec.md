# Implementation specification

## Status

This is a planned contract, not implemented code. Exact protobuf field numbers
and release bounds are frozen only by the feature admission process.

## Identity and manifest

- repository: `gotth-extension-translate`
- extension ID: `gotth.extension.translate`
- control protocol: `gotth.extensions.control` major 1
- business interface: `gotth.interface.translation` major 1
- required capability: `translate.text`
- optional capability: `translate.remote-egress`
- optional named secret: `translation.remote-provider.credential`

## Synchronous request

The initial request contains:

- bounded correlation ID;
- source language mode: canonical tag or `auto`;
- one canonical BCP 47 target tag;
- content kind: `plain`, `mail-visible-text`, `markdown-segment`, or
  `document-text`;
- explicit cache class: `none`, `request`, or a future admitted host policy;
- ordered segments with opaque IDs and UTF-8 text.

Initial design ceilings are 256 segments, 64 KiB per segment, and 256 KiB of
source text per synchronous request. They are release constants, not runtime
escape hatches. The encoded envelope and returned expansion receive separate
hard bounds. Larger work fails with a stable `request_too_large` result and may
be routed by the host to a future jobs contract.

The response repeats the admitted target tag and returns ordered segment
results with detected source tag, translated UTF-8 text, quality tier, backend
model identity, and stable result class. It returns no HTML. Missing,
duplicate, reordered, over-expanded, invalid UTF-8, or unknown segment results
invalidate the whole response.

## Transport and egress

The host authenticates a private transport identity bound to the effective
extension session. Every call requires a deadline. The extension authenticates
the exact configured backend service. Arbitrary request-supplied URLs are
forbidden.

Remote provider support is absent from the first implementation unless a
specific provider contract, egress destination, secret slot, redaction test,
consent surface, and retention statement are admitted together.

## Consumer adapters

- Mail parses MIME and selects visible decoded text before calling the
  extension. Attachments and headers do not cross the boundary.
- BB tokenizes canonical Markdown into protected structure and translatable
  text segments. Board reassembles and escapes the result.
- Cloud performs authorization, explicit selection, text extraction, and file
  type/size checks before calling. The extension cannot open a Cloud object.

## Verification

Required coverage includes bounds, invalid UTF-8, BCP 47 mapping, duplicate
segment IDs, timeout/cancellation, backend authentication, denied egress,
malformed and over-expanded output, retry behavior, log/metric redaction,
cross-product grant isolation, local backend integration, and consumer
fixtures proving that raw MIME, Markdown structure, storage paths, encrypted
content, and credentials do not cross the interface.
