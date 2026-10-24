# Feature plan

## Translation contract

1. Admit PRD, architecture, implementation specification, and planned workflow
   state, including the future baseline of exact published
   `gotth-extensions v1.0.0` and `gotth-sdk v1.0.0` modules.
2. Keep workflow dependencies empty until implementation is explicitly
   activated; selecting a design baseline does not create an implementation
   dependency or completion evidence.
3. Decide and publish a repository license.
4. Freeze the protobuf package, field numbers, bounds, stable result classes,
   BCP 47 mapping rules, and compatibility policy.
5. Import generated control messages and services from `gotth-sdk v1.0.0`;
   do not copy, vendor, or regenerate shared control code locally.
6. Implement `gotth-extensions v1.0.0` manifest/grant conformance and
   authenticated local transport.
7. Implement the `gotth-translate` adapter without remote-provider egress.
8. Add hostile tests, content-redaction checks, allocation/latency evidence,
   clean-clone/container proof, and rollback instructions.
9. Prove separate Mail, BB, and Cloud adapter fixtures without merging product
   authority into this repository.
10. Complete two clean reviews, build immutable artifacts, and release only
   after exact Forgejo/GitHub parity.

Remote providers, asynchronous jobs, and product UI are separate future
features. They are not stubs in the first slice.
