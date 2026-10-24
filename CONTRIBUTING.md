# Contributing

Changes must preserve host authority, exact manifest/grant/Session and transport binding, strict bounds, whole-response validation, content-free errors/logs, Unix-only backend egress, and product isolation. Read docs/prd.md, docs/architecture.md, docs/implementation-spec.md, and docs/compatibility-v1.md before changing code.

Run make verify locally and make verify-heavy only on the designated development host. Wire changes require compatibility review, regenerated-byte proof, reserved removed numbers/names, hostile tests, and consumer impact analysis. Do not add generic payloads, URLs, callbacks, remote-provider stubs, product adapters, databases, broad filesystem access, automatic retries, or copied control code.

Canonical development is Forgejo; GitHub is a distribution mirror. Commit, push, tag, release, credential use, provider access, and deployment require separate authorization. Contributions are accepted under the MIT License.
