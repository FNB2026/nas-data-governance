# Governance UX v2 — bounded record-only backend

Date: 2026-10-13. Baseline main `15328e0054c1c3e476dd756436ce52d6f3c50c1a`.
Original independent implementation author `/root/linux_web_implementation`;
source snapshot `b4c43d6` was frozen after the agent stopped on a usage limit.
The supervisor preserved that tree, merged current main normally, and added
migration preflight and real adapter JSON/source-conservation regressions.
Those supervisor additions require a different independent reviewer before merge.

## Scope and invariants

Five high-level Wails methods: GetGovernanceCapabilities, ListGovernanceGroups,
PreviewBatchDecisions, CommitBatchDecisions, GetBatchDecisionResult. No executor,
approval, source filesystem write or recovery-lock clear API is part of this port.

- Whole-dataset filter/count/page in one read snapshot. Stable deterministic
  tie order and bounded page/batch. Risk/role remain unknown; unsupported filters
  and all-filter selection explicitly unavailable.
- Subject SHA covers indexed members, metadata, context/protection/rule versions,
  related plans and product identity. Decision revision is a JSON number/int64,
  not an opaque string; the frontend must reject unsafe numeric revisions.
- Persistent project identifier plus canonical DB path/device/inode binding.
  Copied backups may expose historical receipts read-only but cannot replay
  the original project's token. Expiring scoped previews and same-key body conflict.
- Per-item immediate transaction commits decision + audit + outcome atomically.
  A failure leaves only completed prefix items; IN_PROGRESS is never whole-batch
  success. Same-key replay continues only missing item transactions.
- Known `batch_id` or `key:<idempotency-key>` can query without replay, including
  read-only mode and after reopening. Unknown-batch discovery/task UI is not
  implemented in this slice and remains a UX3 dependency.
- RO/MAS preview issues no commit token or persistent preview row. MAS/Web/fnOS,
  Recovery Lock and active jobs deny recording at service/store boundaries.
  Legacy decision/Approve/manual SaveDraft entry guard gaps are also closed;
  scanner's lawful index persistence remains separate.

## Owner and database boundary

All updated writable store.Open entrypoints acquire one nonblocking OS owner
lease before migrations. Regular caller-owned database and owner sidecar, 0600,
no symlink/hardlink and known local filesystem required. Read-only handles remain
available. Close/OS process exit releases ownership; the separate execution lock
is not replaced. Unsupported platforms/filesystems/network state are refused.
Operator must establish stable ownership/ancestry/ACL for the entire operation;
concurrent hostile same-UID/privileged replacement/remount is not certified.
Old beta.11 or arbitrary SQLite clients do not participate; no claim of retroactive
cross-binary mutual exclusion. No historical project was opened or chmodded.

Schema 12 additive tables/revision/marker commit together or roll back. The
independent exact-HEAD review found that the earlier common-marker/MAX-version
preflight also accepted foreign SQLite databases. Four genuine RED cases used
foreign application tables with empty/1/11/12 markers: each was accepted and
received NDG schema/writable SQLite changes. These original RED outputs remain
in the local private archive; this correction does not rewrite them.

The corrected preflight accepts only empty new databases, the full frozen legacy
layout with zero marker rows, or the full governance layout with exactly marker
12. It reads one transaction snapshot and fingerprints all stored tables,
indexes, views and triggers; `sqliteX...` is not hidden by a LIKE wildcard.
Extra/missing objects and mismatched markers refuse before writable initialization.
The complete `sqlite_master` hashes are:

- Legacy: `e177d2a1ec16d33cd96eed49a70082c8b25a10c5b6b72ca7e4c7e5138f811af5`.
- Governance: `2cdb8fa859ae5f680af9cb6ed2455422946e317381ab93a4118077d3e34ccf84`.

An isolated archive of beta.11 RC `791f19119542e69b62ee62dd358fed188000837e`
ran the actual frozen store.Open initializer on a new disposable database and
produced the legacy hash and zero markers. All beta.1–beta.11 tags and baseline
main have byte-identical sorted schema files plus migrations.go (aggregate SHA
`8a7e73d85562eab89512beffec28df5a13a2d10e472d99fd4f4ef900bf235f6c`)
and the same modernc SQLite version. Thus earlier release initializer layouts
are represented without fabricating a minimal legacy schema. This identifies
supported structure, not provenance, authenticity, content integrity or a
certified backup. Partial/tampered/unregistered layouts require read-only/manual
assessment; they are not automatically repaired.

A closed WAL-mode database can create sidecars even with mode=ro. Under the
existing exclusive-owner/stable-local-ancestry contract, preflight uses immutable
only when both WAL and SHM are absent. Present regular WAL/SHM are read normally
in the same snapshot; an incomplete/nonregular pair refuses before opening.
A real uncheckpointed marker change is tested and cannot be hidden by immutable.
Unsupported-layout refusals preserve main DB and existing WAL/SHM/journal bytes
and create no SQLite sidecars. Owner-lock acquisition is a separate deliberate
coordination side effect. Older binaries/uncoordinated writers and hostile path
replacement remain outside that cooperative ownership guarantee.

The former minimal link-count fixture now explicitly tests Init's legacy column
migration as a unit. Public Open compatibility is separately tested against the
complete release layout. Audit-failure tests suspend only synthetic extra schema
triggers during preflight, then re-inject failures after reopening; durable plan,
journal, audit, receipt and byte outcomes are preserved. No production database
or historical failure scene was altered, and no unknown trigger is whitelisted.

## Actual evidence and limits

- Original baseline meaningfully reproduced a second writable handle and legacy
  decision writes under MAS/Recovery restriction. Updated regressions use valid
  scanned subjects, so a missing/nonexistent group cannot fake a guard PASS.
- 100 duplicate groups/200 physical disposable files: record/replay/concurrent
  same-key/reopen, source SHA conservation, no generated execution plans, audits
  and exact counts. Source permissions, decisions, stale revision/rule/protection,
  partial conflict, changed body, expiry/foreign scope, atomic audit/outcome failure,
  timeout prefix and schema migration failures tested.
- Real Go Wails adapter/service/scanner/store/SQLite integration: 7 disposable
  local source files, natural COMPLETED, 3 groups, 6 full-hashed candidates and
  singleton without full hash; JSON numeric DTO roundtrip; scoped preview/commit,
  unchanged plan list, key query, close/reopen read-only receipt, no readonly commit
  token, direct readonly mutation refused, all source SHA-256 unchanged.
- 10k groups/20k indexed rows: server count/page/filter consistency and recorded
  local P50/P95. This is SQL-only, not a 10k-file scan or browser latency proof.

Actual test outputs, source identities and full gates are recorded in the PR;
CI, independent review and native GUI must be bound to the final exact HEAD.
No signed App, MAS/SMB environment, public candidate or real user database was used.
Subject fingerprints compare indexed metadata; they do not certify detection of
an external source edit that has not been re-indexed. Execution still needs its
independent filesystem stale check. Full UX v2 P0 acceptance remains open pending
explicit source-stale semantics/integration, UX3 discovery and human walkthrough.

Six enums and old/unknown historical values remain readable; no reinterpretation,
automatic source migration, backup certification, HOLD release or Purge enabling.
beta.11 assets, MAS INCONCLUSIVE/NO-GO and historical privacy FAIL remain intact.

## CI follow-up and owner identity refusal

The first PR desktop job built the development App but correctly failed its
tracked-binding drift check. Wails v2.13.0 SDK classes/stubs are now regenerated
and committed; a deterministic whitespace-only-line normalization step keeps
regeneration and whitespace verification consistent. No hand-written DTO output
or signed artifact is substituted. Final desktop/gates must be re-bound to HEAD.

A percent-encoded database filename reproduced an owner/SQLite mismatch: SQLite
opened a decoded sibling rather than the literal file owned by the lease.
Default refusal now covers percent escapes as well as the existing question/hash
URI delimiters in both read/write and readonly entrypoints. The RED reproduced
mismatched database identity; the GREEN checks refusal has no directory effect.
This does not add a URI filename feature or modify a user's existing database.

Strict history scanning found additional artificial idempotency fixture constants.
Each known historical finding has an exact commit/file/rule/line exception only;
new tests generate runtime random keys while preserving same-key equality/replay
semantics. Original Security failures remain recorded. No real credential was
found and no path/pattern exemption or force rewrite was introduced.

The next exact-HEAD CI preserved a distinct MAS guard fixture failure: the dev
App and regenerated SDK built successfully, then a new test used default MAS
composition to open an external temporary DB, correctly receiving authorization
refusal before reaching the intended approval check. The regression now uses the
existing sandbox test composition, a synthetic native grant and the real managed
project-creation API inside its private container. Both manual draft and approval
must return the explicit governance refusal and retain the durable DRAFT state.
No production authorization rule was weakened, and no user's container was used.
The original remote failure and local RED remain; targeted MAS/race and vet and
fresh default full gates are rerun separately before the update.

## Independent review boundary and MAS matrix

The independent reviewer reproduced default store/app/Wails and fresh race PASS,
but a broad MAS-tagged suite failed ordinary DMG fixtures that open external
project paths without grants. That FAIL is retained, not relabeled as MAS runtime
PASS. The new DMG integration fixture now explicitly selects DMG test composition;
genuine MAS manual-draft/approval tests continue to use authorized disposable
native-grant/container composition. Targeted MAS guard/race, default full gates,
and native/signed GUI acceptance remain separate claims.
