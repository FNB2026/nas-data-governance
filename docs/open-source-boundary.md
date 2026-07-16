# Open-Source Boundary and Release Readiness

This document defines what may become public and what must remain outside the repository. It is a publication gate, not permission to change repository visibility.

## Public-Safe Scope

The intended public surface is limited to:

- Go source code and SQLite schemas;
- synthetic tests created under temporary directories;
- architecture decisions, knowledge cards, and operational documentation;
- the sanitized Markdown whitepaper under CC BY 4.0;
- example rules that contain no real paths, filenames, identities, or customer data;
- CI, dependency, security, and contribution policy files.

Public code may describe categories such as contracts, medical records, customers, or backups. It must not contain real instances of those materials.

## Never Publish

- production SQLite databases, WAL/SHM files, JSONL indexes, checkpoints, or audit reports;
- hash-failure manifests, private diagnostics, scan logs, execution journals, or quarantine contents;
- real NAS roots, paths, filenames, directory trees, metadata exports, screenshots, media, or documents;
- credentials, tokens, private keys, cookies, environment files, or workstation-specific paths;
- third-party source material without documented provenance and redistribution rights.
- private DOCX/XMind research material, even when the repository contains a sanitized derivative summary.

Private reports must remain `0600`. Ordinary logs must remain path-free and content-free.

## Private Source Materials

The current public-ready tree does not track the private DOCX concept manual or the two XMind research files. They were copied to a repository-external private directory with owner-only permissions before removal from the tree. Their generalized methods were independently summarized in `docs/whitepaper.md`; real identities, equipment, capacities, business structures, paths, filenames, and directory trees were excluded.

The private pre-rewrite history and release assets have been preserved in an owner-only offline mirror, complete Git bundle, and checksum-verified release backup. All pushable branches were rewritten to remove the three source files and workstation-local author identity. The old `v1.0.0` release and tag were removed rather than reused.

GitHub's read-only pull-request refs still retain earlier commits for PRs created before the rewrite. They cannot be changed by a normal or forced Git push. The repository must remain private until GitHub Support has dereferenced the affected PR refs, removed cached views, and completed server-side garbage collection.

## Git History and Identity Findings

The rewritten pushable history uses platform-neutral `/synthetic/...` fixtures. Synthetic macOS-style user paths and the workstation-local author identity exist only in the private backup and the GitHub PR refs awaiting server-side purge.

Repository-local Git configuration and rewritten commits use the GitHub noreply project identity.

## Mandatory Publication Gates

- [x] License software under Apache License 2.0 in root `LICENSE`.
- [x] License the sanitized whitepaper and original documentation under CC BY 4.0 in `LICENSE-DOCS.md`.
- [x] Remove the private DOCX and XMind source materials from the current tree and prohibit those formats in `make public-check`.
- [x] Back up the original mirror, complete Git bundle, source documents, and old release assets with owner-only permissions.
- [x] Rewrite every pushable branch to remove the private source materials and workstation-local author identity.
- [x] Remove the old `v1.0.0` GitHub Release and tag rather than retagging them.
- [x] Run `make public-check`, race tests, vet, Govulncheck, Gitleaks, and release packaging successfully.
- [x] Review third-party dependency licenses and bundle verbatim upstream license texts.
- [x] Ask GitHub Support to purge the four affected PR refs, cached views, and unreachable objects.
- [ ] Obtain GitHub Support confirmation and independently verify that the old commit and three private paths are inaccessible.
- [x] Require a persistent SQLite journal for real execution and stop on journal write failure.
- [x] Create indexes, plans, audits, diagnostics, and SQLite artifacts with owner-only permissions.
- [x] Reject symlinked or overlapping source and quarantine roots.
- [x] Use the canonical public Go module path and portable release checksums.
- [ ] Add a Code of Conduct after choosing a monitored private conduct-reporting channel.
- [ ] Enable branch protection, secret scanning, Dependabot alerts, and private vulnerability reporting when repository visibility and plan support them.
- [ ] Create a fresh release from the public-ready commit; do not present the older private-preparation tag as the public launch baseline.
- [ ] Change repository visibility only after every blocking item above is resolved.

## Release Procedure

1. Merge the readiness changes through a reviewed pull request.
2. Re-run all local and GitHub checks on the exact release commit.
3. Obtain confirmation that GitHub has purged affected PR refs, cached views, and old objects.
4. Create a new signed or annotated `v1.1.0-beta.1` tag; do not move `v1.0.0`.
5. Build release artifacts from the tagged commit and publish matching SHA-256 checksums.
6. Enable the public repository security and branch settings.
7. Change visibility last, then verify the anonymous public view.
