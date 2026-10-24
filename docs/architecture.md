# Architecture

## Data flow

    authorized product request
      -> product-owned reduction and consent
      -> authenticated gotth.translation.v1 RPC on private Unix socket
      -> strict manifest/grant/Session and request validation
      -> authenticated gotth-translate RPC on one configured private Unix socket
      -> strict whole-response validation
      -> bounded derived segment results
      -> product-owned escaping, display, audit, and expiry

The host owns product authority. gotth-translate owns language/model execution and evaluation evidence. This repository owns the translation seam and adapter validation. The extension has no product database, administrator session, general filesystem access, remote URL, or callback.

## Trust boundaries

Startup strictly parses canonical manifest and grant bytes with gotth-extensions v1.0.0, negotiates against control 1.0 and translation 1.0, and requires exactly translate.text with no secrets. Every call re-negotiates from authoritative values and compares the complete Session. A private host token authenticates transport; Translate also binds the Session fingerprint. The supervisor supplies one connected Unix descriptor. Startup verifies its SO_PEERCRED PID/UID/GID and a token-keyed HMAC readiness proof before READY. The single-use dialer cannot reopen a replaced pathname. Translate then uses the private backend token, caller ID, timeout, exact model identity, and exact admitted quality tier.

The host-facing and local-backend RPC use the owned gotth.translation.v1 shape. This avoids inventing a generic backend payload while allowing gotth-translate to implement the seam directly. Authentication metadata differs by boundary and never enters protobuf messages.

## Failure, privacy, and cost

Every host RPC passes deadline/token interception before its handler. Translate then validates Session and input before capacity admission or backend work. Admitted backend execution is bounded by a nonblocking semaphore and configured timeout; this is not a process-wide decoded-RPC limit. Cancellation propagates. Service-config and application retries are disabled; gRPC may transparently retry only an attempt proven unprocessed by the transport. Backend output is accepted only when target, count, order, IDs, languages, tier, class, model identity, UTF-8, expansion, and envelope size all match. Any defect rejects the whole response.

Text crosses two protobuf boundaries and one detached response slice. That copy prevents backend-owned result slices from becoming host-visible mutable state. Audit events contain no text or full correlation ID and enter a bounded nonblocking queue; blocked output can drop events but cannot stall RPC or shutdown. V1 stores no cache. NONE forbids backend post-call retention; REQUEST allows same-RPC memoization only. Host-owned longer retention is outside the process. Large or long-running documents fail and may later use a separately admitted jobs contract.
