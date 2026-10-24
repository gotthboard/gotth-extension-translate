# Changelog

## Unreleased

### 2026-10-23 — Set the future extensions v1 implementation baseline

Update `README.md`, the product requirements, architecture, implementation
specification, feature plan, and workflow records to select the exact published
`gotth-extensions v1.0.0` and `gotth-sdk v1.0.0` modules as the future
implementation baseline. Require generated extension-control messages and
services to come from the SDK rather than a repository-local copied or vendored
control implementation.

The canonical feature remains `planned` with `depends_on = []` until an
explicit implementation-activation change establishes real prerequisites.
This documentation change creates no executable, dependency selection,
runtime/test evidence, generated artifact, tag, release, deployment, or
compatibility claim. Verification is limited to documentation, link, format,
workflow-state, and scope checks.

### 2026-09-21 — Establish the translation-extension contract

Create the contract-only repository for the product-neutral translation
adapter shared by GOTTH Mail, GOTTH-BB, and GOTTH Cloud. Define host-owned
authorization, consent, content reduction, audit, retention, and presentation;
the extension owns only bounded translation adaptation to `gotth-translate`.

No executable, license grant, tag, release, deployment, or compatibility claim
is created.
