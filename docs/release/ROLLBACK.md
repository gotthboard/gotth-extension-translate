# Rollback

1. Stop routing new calls to the candidate session before stopping its process.
2. Keep original product content usable; translated text is derived and may be discarded.
3. Stop the candidate with SIGTERM and wait for its Unix socket to disappear.
4. Restore the previously admitted artifact, canonical manifest, grant, complete Session, host token, connected backend descriptor, pinned peer identity, and backend token as one unit.
5. Start the prior artifact, verify its socket mode is 0600, then complete an authenticated handshake and health call against the restored digests before restoring routing.
6. Delete request-cache translations under the host's retention policy. The extension stores no translation cache.

Never reuse a candidate manifest digest, grant, Session fingerprint, or transport token with the prior artifact. If the backend outcome of an interrupted call is unknown, do not retry that call automatically; only gRPC attempts proven unprocessed may be replayed transparently. The host keeps the original source usable.
