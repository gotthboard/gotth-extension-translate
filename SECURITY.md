# Security policy

This is an untagged v1.0.0 candidate with no deployed supported release. Report vulnerabilities privately through the GOTTH security-reporting channel. Do not open public reports containing source/translated text, credentials, tokens, paths, object keys, private model output, or product identifiers; use synthetic fixtures and redacted stable classes.

The process must run as an unprivileged user with private configuration/token files and 0700 socket directories. Rotate both host and backend tokens and replace the complete manifest/grant/Session binding after suspected compromise. Revoke routing before shutdown and follow docs/release/ROLLBACK.md. Remote provider egress is not part of v1.
