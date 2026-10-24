# Translation interface V1 compatibility

The owned protobuf package is gotth.translation.v1. Its host-facing V1 API contains one unary RPC, Translation.Translate. The same package also defines BackendIdentity.Prove for the private backend connection; the extension does not register BackendIdentity on its host socket. Field numbers, enum values, backend-proof transcript, and RPC paths are frozen for the v1 release line. Removed field names/numbers and enum names/numbers must be reserved and existing paths cannot be reused with different semantics.

V1 accepts exactly control major 1 minor 0 and translation major 1 minor 0. Bounds are 256 segments, 64 KiB per segment, 256 KiB total source text, 320 KiB encoded request, 1 MiB encoded response, 128-byte segment IDs, canonical BCP 47 tags, fourfold output expansion with a 256-byte per-segment and 1024-byte per-request floor, and at most 32 admitted backend executions. The configured semaphore does not claim a process-wide bound on decoded or control RPCs. Remote provider egress, asynchronous jobs, callbacks, HTML, and generic payload fields are absent.

Cache semantics are normative. CACHE_CLASS_NONE forbids the extension and backend from retaining text after the RPC completes or reusing it across calls. CACHE_CLASS_REQUEST permits memoization only within that single RPC; it still forbids cross-RPC retention or reuse. The extension itself implements no cache. A host-owned longer-lived cache remains outside this process and under the host's consent, authorization, expiry, and deletion policy.

Application failures use the following stable status/message pairs:

| Condition | gRPC status | message |
| --- | --- | --- |
| missing deadline | InvalidArgument | deadline_required |
| missing metadata | Unauthenticated | transport_identity_required |
| invalid/duplicate service token | Unauthenticated | invalid_transport_identity |
| invalid correlation ID | InvalidArgument | invalid_correlation_id |
| retained manifest/grant no longer negotiates to retained Session | FailedPrecondition | session_binding_changed |
| Translate metadata Session mismatch | PermissionDenied | session_binding_mismatch |
| Health request Session mismatch | FailedPrecondition | session_binding_mismatch |
| malformed Handshake request | InvalidArgument | invalid_handshake |
| Handshake expected identity/digest mismatch | FailedPrecondition | extension_binding_mismatch |
| malformed Health request | InvalidArgument | invalid_health_request |
| nil or unknown-field Translate request | InvalidArgument | invalid_request |
| request/envelope/segment/source bound exceeded | ResourceExhausted | request_too_large |
| noncanonical or unsupported language | InvalidArgument | unsupported_language |
| unsupported content kind | InvalidArgument | unsupported_content_kind |
| unsupported cache/retention class | InvalidArgument | unsupported_retention |
| empty/malformed/unknown-field segment | InvalidArgument | invalid_segments |
| repeated segment ID | InvalidArgument | duplicate_segment_id |
| admitted backend capacity exhausted | ResourceExhausted | capacity_exhausted |
| caller cancellation | Canceled | request_cancelled |
| caller/backend deadline exhausted | DeadlineExceeded | backend_timeout |
| backend call failure | Unavailable | backend_unavailable |
| malformed/unknown-field/mismatched backend output | DataLoss | malformed_backend_response |
| result/envelope expansion bound exceeded | ResourceExhausted | output_too_large |

Transport failures before the interceptor, including gRPC envelope rejection or connection loss, use gRPC implementation status text and are not part of the application-message contract. The table does not promise one lowercase message for those failures. Application and service-config retries are disabled. gRPC may transparently retry only when transport proves an attempt unprocessed; once backend admission begins, outcome may be unknown and the extension never retries.

Owned Translate request, segment, response, and result messages reject unknown fields and unknown enum values so backend/host semantics cannot widen silently. Imported gotth.extensions.v1 control messages retain normal protobuf forward-compatible unknown-field behavior as required by the released foundation/SDK contract. Minor releases may add separately named APIs but cannot widen v1 authority, weaken validation, change valid ordering, or reinterpret an existing field.
