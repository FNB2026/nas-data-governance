# Issue #80 — Purge release policy

## Scope and identity

Baseline: main `f0a7c107c3253746e4f9e955ee98dcfa1d5ac176`.
Product fix only: permanently destructive Purge is disabled in the compiled
release policy. No channel, UI input, environment variable or public Service
constructor enables it. All public Service, Wails and CLI Create / Approve /
Dry Run / Execute entry points reject before store access or filesystem effects.
Historical lists and conservative exceptional recovery remain available.
Quarantine, Restore and Recovery retain independent capabilities.

This is source/test evidence; beta.10 remains immutable and is not repaired by
this PR. Formal signed beta.11 verification is still required. Release remains
Draft; Public Beta remains BLOCKED. Fresh Mac offline first install is deferred
for lack of equipment, still BLOCKED, neither waived nor passed.

## Regression evidence

Before the fix, new real SQLite/public API regressions failed on the previous
main: Purge plans were created and API calls were not denied. The frontend
isolation regression also failed because a false Purge capability disabled
Restore draft creation. After the fix:

- Full `go test -race ./...` and `go vet ./...`: PASS.
- Public APIs: closed, read-only, read-write and active recovery lock; fixed
  refusal contains no supplied path canary. Normal Quarantine/scan capabilities
  remain enabled when unlocked.
- Service denial with nil store proves policy evaluation precedes store access;
  eligible temporary quarantine bytes and absence of new plans are checked.
- CLI refusal precedes opening even a nonexistent database; no database created.
- Historical ambiguous pending Purge journal: repeat recovery preserves both
  files, pending journal and recovery lock. No real user files are deleted.
- Test-only private lifecycle fixture preserves legacy Purge safety regressions;
  it is not compiled into the production application.
- Frontend: 260 tests / 28 files PASS; TypeScript + Vite build PASS.
- `npm ci`, full `npm audit`: zero vulnerabilities; version/public/release
  checks PASS. Local Node 25.9.0/npm 11.12.1 emits jsdom engine warning; CI uses
  the repository-pinned toolchain, and is independently required.
- Beta ldflags policy regressions PASS; signed artifact identity is NOT RUN.

Raw local command outputs are retained privately, not published with fixture
paths. Runtime GUI and journal evidence from historical versions is unchanged.
The current B/C regression expects Purge disabled even when unlocked. The
historical beta.10 replay runner now explicitly loads its original test carriers
from main `70de4667a3f73719f7ac885da41f6c0cdba22c35`; its original result/log hashes
remain historical receipts, not receipts for this updated runner.

## PR #58 dependency fix

Updated exact head: `e3144eaa0b73a09c25f267c77f9566c41d743425`.
Only three package-lock fields changed: source-map-js version 1.2.1 → 1.2.2,
resolved URL and integrity. Independent review found no other package drift.
Full audit reports zero vulnerabilities, 258 frontend tests and build PASS;
version/public/release gates PASS. Exact-head CI 38120717063 and Security
38120717070 succeeded. Merged as `f0a7c107c3253746e4f9e955ee98dcfa1d5ac176`;
main CI 38120923212 and Security 38120924183 succeeded.

The old beta.10 DMG is unchanged. Dependency remediation belongs to beta.11.
The formal SBOMs cover Go only, so npm remediation is verified through the
frozen lockfile, fresh install/audit and formal build identity, not claimed as
an npm SBOM receipt.

## Remaining gates

PR #82 exact independent review and CI/Security passed and merged as
`f599cf764641ff970293d1ea46c40c64c8785f0b`; post-merge CI 38121602451 and
Security 38121604165 succeeded. Immutable beta.11 RC is
`791f19119542e69b62ee62dd358fed188000837e`; formal Release 38122101720 succeeded.
Actual Purge-disabled / Quarantine-Restore evidence and distribution/SBOM
limits are recorded in [beta.11 formal acceptance](beta.11-formal-acceptance-go-no-go-2026-10-11.md).
Fresh-Mac offline installation remains BLOCKED. The user explicitly approved
limited first-Beta support on 2026-10-11; broader privacy and unknown historical
Purge recovery remain outside certified scope, not silently marked PASS.
No public release authorization is implied by any source or test PASS.
