# gotth-extension-translate

Out-of-process, product-neutral translation adapter for GOTTH Mail, GOTTH-BB, and GOTTH Cloud. It implements extension ID gotth.extension.translate and owned interface gotth.interface.translation v1. The host keeps authentication, source authorization, consent, audit, retention, and presentation authority; this process receives only bounded ordered UTF-8 text segments and returns derived display text.

## V1 boundary

- exact github.com/gotthboard/gotth-extensions v1.0.0 manifest/grant/session admission;
- generated control service imported from github.com/gotthboard/gotth-sdk v1.0.0, with no local control copy;
- authenticated 0600 Unix host transport with pre-handler deadline/token admission and per-Translate complete Session binding;
- canonical BCP 47 tags, strict request/output bounds, deadlines, cancellation, bounded backend execution, documented application errors, and bounded asynchronous content-free audit logs;
- one Unix-socket-only gotth-translate adapter. There is no TCP, HTTP, remote-provider, callback, job, database, filesystem-content, or product mutation path.

The supervisor passes one already-connected Unix backend descriptor. Before exposing READY, the extension verifies its kernel PID/UID/GID, completes a token-keyed HMAC readiness proof on that exact connection, and disables pathname redial. Only then may Translate send x-gotth-backend-token and caller ID gotth.extension.translate. Mock backend fixtures are test-only. Production use requires an independently admitted gotth-translate implementation and language/quality evidence for every advertised model identity.

## Runtime

The host provides absolute private files through GOTTH_EXTENSION_CONFIG_FILE, GOTTH_EXTENSION_MANIFEST_FILE, GOTTH_EXTENSION_GRANT_FILE, GOTTH_EXTENSION_SERVICE_TOKEN_FILE, and GOTTH_TRANSLATE_SERVICE_TOKEN_FILE, plus a new socket path in GOTTH_EXTENSION_SOCKET. Files and socket directories must be owned by the process user with no group/world permission. The manifest and grant files must be exact canonical gotth-extensions JSON. GOTTH_TRANSLATE_BACKEND_FD names the inherited descriptor (3 or greater). The configuration has exactly schema, backend_peer_pid, backend_peer_uid, backend_peer_gid, backend_model_identity, backend_quality_tier (experimental or evaluated), backend_timeout_ms (100..10000), and max_concurrent (1..32 admitted backend executions). The host admits a quality tier only when it retains matching evaluation evidence.

Every RPC needs a deadline and printable-ASCII x-gotth-service-token before its handler runs. Translate also checks x-gotth-session-sha256 inside the method. CACHE_CLASS_NONE forbids post-call retention; CACHE_CLASS_REQUEST permits only same-RPC memoization. Source or translated text, raw MIME, attachments, Markdown structure, paths, object keys, and credentials must never be placed in configuration, logs, errors, metrics, or evidence.

Run lightweight checks with make verify. Run make verify-heavy only on the designated development host. Build a deterministic archive with scripts/build-artifact.sh 1.0.0 OUTPUT_DIR. The tree is implemented but untagged, unpublished, and undeployed; see docs/release/RELEASING.md.

Licensed under MIT. Exact dependency licenses/notices are inventoried under docs/release/ and packaged under THIRD_PARTY_LICENSES in every archive.
