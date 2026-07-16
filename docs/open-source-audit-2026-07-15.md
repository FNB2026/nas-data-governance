# Open-Source Readiness Audit — 2026-07-15

## Outcome

The repository's current tree and all pushable branches are licensed and sanitized, but it is **not yet ready to change visibility**. GitHub's read-only pull-request refs still retain pre-rewrite objects and require a server-side purge by GitHub Support.

## Verified Repository State

- GitHub visibility: private.
- Default branch: `main`.
- Merge policy: squash merge only; merged branches are deleted automatically.
- Issues: enabled. Wiki and Discussions: disabled.
- Existing releases and tags: none; the pre-sanitization `v1.0.0` release and tag were backed up and removed.
- Root software license: Apache License 2.0.
- Original documentation and sanitized whitepaper license: CC BY 4.0.
- Branch protection: unavailable while the repository remains private on the current GitHub plan; configure it after publication or a plan change.

## Security and Privacy Results

- Gitleaks full-history CI on the sanitized pull request: passed.
- Manual history signature scan: no credential signatures; only synthetic macOS-style user-path fixtures were found.
- `govulncheck 1.1.4 ./...`: zero reachable vulnerabilities.
- Current tracked-artifact and text boundary check: passed.
- Private DOCX/XMind materials were reviewed and found to contain real identities, equipment, capacities, business structures, and directory information; they are excluded from the public-ready tree.
- Every pushable branch was rewritten to remove the three private source paths and workstation-local author identity.
- A fresh mirror clone verifies that branch and tag histories contain none of those paths or the local identity.
- Four GitHub pull-request refs were affected by the rewrite; read-only PR refs still make the old objects reachable until GitHub completes server-side removal.

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
- A complete private mirror, verified Git bundle, and checksum-verified copy of every old release asset.
- Rewritten and force-updated active branches, with the old release and tag removed.
- A private GitHub Support request draft containing the affected PR count and first changed commit mapping.

## Remaining Publication Blockers

1. Obtain confirmation on the submitted GitHub Support request that affected PR refs, cached views, and old objects were purged, then independently verify the old paths are inaccessible.
2. Choose a monitored private contact channel before adding a Code of Conduct.
3. Select a fresh first public release version; do not recreate or reuse `v1.0.0`.

Repository visibility must remain private until these blockers are resolved.
