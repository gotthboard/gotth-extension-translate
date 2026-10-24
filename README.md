# gotth-extension-translate

`gotth-extension-translate` is the planned, product-neutral extension adapter
between GOTTH applications and the standalone `gotth-translate` service.

The first consumers are GOTTH Mail, GOTTH-BB, and GOTTH Cloud. Each host keeps
authentication, authorization, content selection, consent, audit, retention,
and presentation authority. The extension receives only bounded text segments
that the host has already authorized and reduced for translation. It never
receives a product database, administrator session, raw mailbox, storage path,
object key, credential, or broad filesystem mount.

This repository currently contains contracts only. It is planned and
unreleased; it contains no executable, image, tag, release, deployment, or
compatibility claim.

Future implementation is based on the exact published
`github.com/gotthboard/gotth-extensions v1.0.0` module and the exact
published `github.com/gotthboard/gotth-sdk v1.0.0` module. Those versions are
a design baseline, not evidence that implementation has started.
Shared generated control messages and services must be imported from the SDK;
this repository must not copy or vendor generated control code.

Read the documents in this order:

1. [`docs/prd.md`](docs/prd.md)
2. [`docs/architecture.md`](docs/architecture.md)
3. [`docs/implementation-spec.md`](docs/implementation-spec.md)
4. [`docs/feature-plan.md`](docs/feature-plan.md)

The canonical source is Forgejo. GitHub is the public distribution mirror.
No license has been selected for this repository; public visibility is not a
license grant.
