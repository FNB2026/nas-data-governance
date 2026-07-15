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

Earlier commits and the existing `v1.0.0` tag/release still reference or contain these files. Removing them from the latest commit is not sufficient for a public repository. Before changing visibility:

1. create and verify an offline private bundle or mirror of the original history;
2. rewrite every public branch and tag to remove the three source files and any private data;
3. replace the old GitHub release with a new release built from the sanitized history, without reusing old artifacts;
4. run full-history secret and privacy scans again before changing visibility.

## Git History and Identity Findings

The current history contains synthetic macOS-style user-path test fixtures. They are not real workstation paths, and the current tree replaces them with `/synthetic/...`, but the old strings will remain visible in Git history unless history is rewritten.

Some earlier commits also use a workstation-local `.local` author email. Repository-local Git configuration now uses the existing GitHub noreply project identity for future commits, but configuration does not alter existing objects. Before publication, the owner must explicitly decide whether preserving the original history is acceptable or whether author metadata should be rewritten.

## Mandatory Publication Gates

- [x] License software under Apache License 2.0 in root `LICENSE`.
- [x] License the sanitized whitepaper and original documentation under CC BY 4.0 in `LICENSE-DOCS.md`.
- [x] Remove the private DOCX and XMind source materials from the current tree and prohibit those formats in `make public-check`.
- [ ] Back up and rewrite Git history and tags so private source materials are absent from every public ref.
- [ ] Run `make public-check` with no findings.
- [ ] Run `go test -race -count=1 ./...`, `go vet ./...`, and `govulncheck ./...`.
- [ ] Review third-party dependency licenses and binary redistribution obligations.
- [ ] Confirm the complete Git history contains no secrets or private data.
- [ ] Decide whether to preserve or rewrite workstation-local author metadata in existing commits.
- [ ] Review existing GitHub Releases; never retag an existing release.
- [ ] Add a Code of Conduct after choosing a monitored private conduct-reporting channel.
- [ ] Enable branch protection, secret scanning, Dependabot alerts, and private vulnerability reporting when repository visibility and plan support them.
- [ ] Create a fresh release from the public-ready commit; do not present the older private-preparation tag as the public launch baseline.
- [ ] Change repository visibility only after every blocking item above is resolved.

## Release Procedure

1. Merge the readiness changes through a reviewed pull request.
2. Re-run all local and GitHub checks on the exact release commit.
3. Verify the private history backup, then remove source materials from every public branch and tag.
4. Create a new signed or annotated version tag; do not move `v1.0.0`.
5. Build release artifacts from the tagged commit and publish matching SHA-256 checksums.
6. Enable the public repository security and branch settings.
7. Change visibility last, then verify the anonymous public view.
