# Feature plan

## Translation contract

1. Admit PRD, architecture, implementation specification, and workflow state.
2. Decide and publish a repository license.
3. Freeze the protobuf package, field numbers, bounds, stable result classes,
   BCP 47 mapping rules, and compatibility policy.
4. Implement manifest/grant conformance and authenticated local transport.
5. Implement the `gotth-translate` adapter without remote-provider egress.
6. Add hostile tests, content-redaction checks, allocation/latency evidence,
   clean-clone/container proof, and rollback instructions.
7. Prove separate Mail, BB, and Cloud adapter fixtures without merging product
   authority into this repository.
8. Complete two clean reviews, build immutable artifacts, and release only
   after exact Forgejo/GitHub parity.

Remote providers, asynchronous jobs, and product UI are separate future
features. They are not stubs in the first slice.
