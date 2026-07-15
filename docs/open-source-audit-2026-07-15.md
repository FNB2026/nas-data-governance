# Open-Source Readiness Audit — 2026-07-15

## Outcome

The repository is technically prepared for a public-readiness pull request, but it is **not yet ready to change visibility**. The current tree is licensed and sanitized; historical source documents, old release artifacts, and commit identity history still require a controlled history migration.

## Verified Repository State

- GitHub visibility: private.
- Default branch: `main`.
- Merge policy: squash merge only; merged branches are deleted automatically.
- Issues: enabled. Wiki and Discussions: disabled.
- Existing release: `v1.0.0`, with four platform archives and SHA-256 checksums.
- Root software license: Apache License 2.0.
- Original documentation and sanitized whitepaper license: CC BY 4.0.
- Branch protection: unavailable while the repository remains private on the current GitHub plan; configure it after publication or a plan change.

## Security and Privacy Results

- `gitleaks 8.28.0 git --redact --log-opts=--all`: 60 commits scanned, no leaks found.
- Manual history signature scan: no credential signatures; only synthetic macOS-style user-path fixtures were found.
- `govulncheck 1.1.4 ./...`: zero reachable vulnerabilities.
- Current tracked-artifact and text boundary check: passed.
- Private DOCX/XMind materials were reviewed and found to contain real identities, equipment, capacities, business structures, and directory information; they are excluded from the public-ready tree.
- Historical commits include a workstation-local `.local` author email; future repository-local commits now use the existing GitHub noreply project identity.

Automated scans do not establish copyright ownership, consent, or redistribution rights.

## Dependency and Release Results

- Runtime dependency graph: 12 external Go modules.
- Detected runtime licenses: MIT and BSD-3-Clause.
- Verbatim upstream license collection succeeded for all 12 runtime modules.
- Release packaging requires and bundles the software license, documentation license, README, third-party inventory, and verbatim dependency licenses.

## Implemented Preparation

- Public-boundary policy and automated `make public-check` gate.
- CI enforcement for the public-boundary check.
- Checksum-pinned full-history gitleaks job.
- Private-safe bug and feature request forms.
- Support policy and contribution provenance rules.
- Expanded ignore rules for databases, indexes, logs, environment files, keys, and private reports.
- Platform-neutral synthetic test paths.
- Release-time third-party license collection.
- Apache-2.0 software licensing and CC BY 4.0 documentation licensing.
- A sanitized, source-independent Markdown whitepaper with no real identities, paths, devices, capacities, or production examples.
- Owner-only repository-external backup of the three private source files, followed by removal from the current tree.
- A public-boundary rule that rejects tracked DOCX and XMind files.

## Blocking Owner Decisions

1. Create and verify a private offline bundle, then rewrite public branches and tags to remove the DOCX/XMind files from all reachable history.
2. Remove or replace the existing `v1.0.0` GitHub release artifacts, which were built before the sanitized boundary; do not reuse them as the public launch.
3. Decide whether the workstation-local author email in existing commits is acceptable or should be rewritten during the same migration.
4. Choose a monitored private contact channel before adding a Code of Conduct.
5. Select a fresh first public release version; do not move or reuse `v1.0.0`.

Repository visibility must remain private until these blockers are resolved.
