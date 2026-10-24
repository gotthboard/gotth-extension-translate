# Release procedure

This repository state is the committed v1.0.0 release candidate. It is not a release: no tag, push, hosted release, or deployment is authorized by this state.

The admitted candidate is a clean exact commit and has passed on the designated development host: format, generated-byte comparison, dependency binding, unit/hostile/process tests, race, repeat/shuffle, fuzz smoke, coverage review, vet, build, no-egress scan, allocation/latency evidence, reproducible artifact comparison, an actual clean-clone rerun of the exact commit, vulnerability scan, history-aware full-repository secret scan, dependency-license reconstruction, and two cold reviews. The revision-matched external evidence bundle records the exact commit and tree. The builder rejects dirty trees by default, exports the immutable exact commit, and records commit/tree/toolchain/build parameters plus binary/SBOM/legal-inventory hashes. GOTTH_EXTENSION_ALLOW_DIRTY_EVIDENCE=1 is only for provisional evidence and produces source_dirty=true.

The builder creates tar and gzip stages separately, validates gzip, extraction, the executable version, and all embedded checksums before making output files visible, and removes any partially published pair on failure. It emits the archive plus a basename-only <archive>.sha256 sidecar; retain and verify both externally.

Publication separately requires owner authorization, an immutable v1.0.0 tag at the admitted commit, exact Forgejo ref/archive verification, and Forgejo hosted release-object verification. GitHub publication is a separate Bryce handoff and is not a Forgejo release blocker. A committed candidate and local archive are not a release. Follow ROLLBACK.md before any deployment.
