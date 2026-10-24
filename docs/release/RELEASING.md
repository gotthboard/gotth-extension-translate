# Release procedure

No tag, push, hosted release, or deployment is authorized by this repository state.

A v1.0.0 release candidate must be a clean exact commit and must pass on the designated development host: format, generated-byte comparison, dependency binding, unit/hostile/process tests, race, repeat/shuffle, fuzz smoke, coverage review, vet, build, no-egress scan, allocation/latency evidence, reproducible artifact comparison, clean-clone rerun, vulnerability scan, history-aware secret scan, dependency-license reconstruction, and two cold reviews. The builder rejects dirty trees by default, exports the immutable exact commit, and records commit/tree/toolchain/build parameters plus binary/SBOM/legal-inventory hashes. GOTTH_EXTENSION_ALLOW_DIRTY_EVIDENCE=1 is only for provisional evidence and produces source_dirty=true. Retain the final archive digest externally with its embedded provenance.

Publication separately requires owner authorization, an immutable tag at the admitted commit, exact Forgejo ref/archive verification, and Forgejo hosted release-object verification. GitHub publication is a separate Bryce handoff and is not a Forgejo release blocker. A local archive is not a release. Follow ROLLBACK.md before any deployment.
