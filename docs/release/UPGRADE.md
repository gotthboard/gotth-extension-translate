# Upgrade

1. Verify the new archive, its SHA256SUMS, provenance, SBOM, and third-party legal-file inventory before extraction. Never upgrade from a dirty-evidence archive.
2. Allocate a fresh private host socket, service token, canonical manifest, grant, and complete Session. Do not overwrite the running instance's socket or credentials.
3. Have the supervisor connect to the admitted gotth-translate backend and pass that already-connected Unix descriptor to the candidate as GOTTH_TRANSLATE_BACKEND_FD. Pin the observed backend PID, UID, and GID in configuration.
4. Start the candidate in parallel. It may expose READY only after kernel peer-credential validation and the backend HMAC readiness proof succeed.
5. Complete authenticated Handshake and Health calls against the candidate's exact manifest, grant, and Session. Keep user traffic on the old instance until both pass.
6. Route new calls to the candidate, then drain the old instance. Calls admitted before a failure have unknown backend outcome and must not be retried automatically.
7. Send SIGTERM to the old process and verify its socket disappears. Retire its token, descriptor, manifest, grant, and Session together.
8. On any candidate failure, stop new routing and follow ROLLBACK.md; original host content remains canonical and usable.
