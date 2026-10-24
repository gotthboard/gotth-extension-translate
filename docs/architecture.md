# Architecture

## Data flow

```text
authorized product request
  -> product-owned content reduction and consent
  -> authenticated gotth.interface.translation RPC
  -> gotth-extension-translate
  -> admitted gotth-translate backend
  -> bounded derived segment results
  -> product-owned escaping, presentation, audit, and expiry
```

The extension is not an authorization service and is not a content store. The
host reduces product objects to an ordered set of text segments. Product IDs
remain opaque and optional; the extension does not fetch source content.

## Authority split

| Authority | Owner |
| --- | --- |
| User authentication and source authorization | consuming product |
| Consent, disclosure, audit, retention, and presentation | consuming product |
| Extension manifest/grant/session admission | consuming product using `gotth-extensions` |
| Language registry, model adapters, quality evidence, batching, and backend cache | `gotth-translate` |
| Translation-interface validation and backend adaptation | `gotth-extension-translate` |
| Long-running job lifecycle | future admitted `gotth-jobs` integration |

The extension gets no database or administrator session. A self-asserted
extension ID is not authentication; transport identity is checked separately.

## Interface ownership

This repository owns `gotth.interface.translation` because it is the first
concrete implementation of the shared translation seam. The generic
`gotth-extensions` control protocol remains business-operation free.

Future implementation must consume foundation control behavior from the exact
published `github.com/gotthboard/gotth-extensions v1.0.0` module and generated
control messages and services from the exact published
`github.com/gotthboard/gotth-sdk v1.0.0` module. This repository owns only its
translation interface and adapter behavior; it must not copy, vendor, or
regenerate a repository-local control implementation. Naming that baseline does
not activate implementation or establish runtime compatibility.

The synchronous interface is request/response and host-called. It does not
create callbacks. A future jobs adapter must be specified as a separate
contract rather than smuggling a queue into the control protocol.

## Privacy and failure

Requests are data-minimized before crossing the process boundary. Logs and
metrics contain stable classes, sizes, language tags, duration buckets, and
correlation digests—not content. A backend outage or malformed response
returns a fixed failure and leaves the source unchanged.

Local `gotth-translate` is the default topology. Remote providers are not
enabled by a URL field alone: they require an exact provider configuration,
named secret slot, egress grant, product disclosure, and user consent.

## Cost model

Text crosses one product-to-extension serialization boundary and one
extension-to-service boundary. Segments are bounded to avoid whole-mailbox or
whole-drive copies. Batch size trades model throughput against latency and
memory; it is explicit and measured. Long documents leave the synchronous
path instead of extending request deadlines until failure is unobservable.
