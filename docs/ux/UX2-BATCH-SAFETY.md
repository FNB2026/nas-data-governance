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

Schema 12 additive tables/revision/marker commit together or roll back. Unknown
future versions and unidentified nonempty databases are refused by a read-only
preflight before the writable WAL DSN/legacy migrations. Existing WAL version
state is read; immutable=1 is not used to conceal it. The new future-version
regression first failed because the unsupported database changed before refusal,
then passed after preflight. The older link-count fixture now includes the schema
marker from actual Schema001; it does not pretend a foreign markerless DB is NDG.
Owner-lock sidecar acquisition remains a deliberate preflight coordination effect;
this is not a promise of zero filesystem sidecar activity during every refusal.

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
