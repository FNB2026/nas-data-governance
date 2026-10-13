# Governance UX v2 — initial UX1 implementation evidence

Date: 2026-10-13. Parent main: `15328e0054c1c3e476dd756436ce52d6f3c50c1a`.
The author works in an exclusive frontend worktree. Backend UX2 is a separate
worktree/PR and is not implied to exist in this frontend parent.

## Implemented scope

- A six-entry navigation rail combines duplicate list, evidence, records-only
  intent form, preview and item outcomes. Previous route IDs/shortcuts and
  single-plan review/recovery remain accessible. No executor/approval shortcut.
- Server query/count/page contract, explicit cross-page selection (max 100),
  stale/filter-scope warnings, unavailable all-filter selection. Unknown risk
  remains unknown; no physical-space savings or executable-safety claim.
- New typed high-level Wails contract only. Older builds fall back to a read-only
  legacy list and cannot mutate through a loop of old single-group APIs.
- Preview and commit guards, exact unique outcome accounting, partial-success
  selection preservation, one in-flight mutation, retained idempotency key.
  Lost responses can be queried by `key:` without replay; IN_PROGRESS remains
  unresolved. The session-only reconciliation cache survives route changes and
  stores no selection/notes/paths in localStorage or URLs.
- Missing project-open recovery-lock evidence now blocks write capability.
  A new regression failed on the unchanged baseline (1 failed / 7 passed) and
  passed after the fail-closed fix. No automatic recovery/lock clearance.
- Privacy mode hides paths, detailed roles/reasons, content/group identifiers,
  notes and search inputs. Export is an allowlisted ordinal/outcome/count summary
  without raw IDs, source paths, user notes or unknown backend error text.

## Evidence layers

| Layer | Actual evidence | Limit |
|---|---|---|
| Component/API contract | Partial success, double click, lost response/key reuse, readonly receipt under subsequent lock, IN_PROGRESS, foreign batch, route remount, stale selection, late preview, malformed receipt, RO/lock/absent API, privacy, invalid filter | Mocked bridge; not Go/SQLite/runtime authorization proof |
| Baseline frontend suite | 282 tests passed before receipt/query and lock additions; final exact-HEAD suite is recorded in PR | Earlier result is not reused as final identity |
| Browser rendered page | 1440x1080 evidence/preview, focus trapped by repeated Tab, records-only partial result (saved 1/conflict 1; conflict stays selected), 390x844 privacy mode | Synthetic Wails bridge; no real database or file operation |
| Browser privacy/layout | Synthetic filename marker absent from visible text; document had no horizontal overflow at 390px | Limited page/window, not entire-system privacy |
| Native Wails + UX2 database | NOT RUN in this frontend slice | Requires reviewed backend and independent disposable integration |
| 5–8 nondeveloper walkthrough | NOT RUN | Cannot infer user comprehension from test greenery |
| Future signed release candidate | NOT RUN | Existing beta.11 assets unchanged |

The Browser plugin was not available; the documented Playwright CLI fallback was
used with a separate browser session and a localhost Vite port proven unused.
Only synthetic bridge data were injected into this development browser, never
into a signed App. Existing browsers/App/NAS sessions were not adopted or closed.
One console error was a missing development-server favicon (404); this is
recorded rather than claiming zero browser errors. No application exception was
observed in this window. Raw fixture, screenshots, snapshots and console records
stay in a local private archive, outside the repository.

Private evidence hashes (SHA-256):

- Synthetic bridge fixture: `689613a64babf3c77512481eeb87a9b685a9490f8c4aaa24ae420629b07631c5`.
- Desktop evidence viewport: `c8adce2b6f913d684f6edf2ecd1ce20101e0d58c1749d7d43441d8a0223497f6`.
- Preview: `8647c32282c6eff3ac158ab609dc954bccc35b078ca2cf05d9a3e6b4cf3c6c20`.
- Mobile privacy viewport: `c81e33c689ef9e3e0fa4e04ec8b2df565a8e78ee68d5a4031772ea93cffde937`.

## Remaining UX2 / UX3 dependencies

The backend must independently establish query snapshots, transaction/CAS,
project owner locking, channel/recovery guards, durable audits and receipts,
source byte conservation, old schema compatibility and concurrency. Frontend
copy and capability checks do not establish those guarantees.

The current backend contract can query known batch IDs or keys after restart,
but does not list/discover unknown batches. A product task/history discovery
entry is still required before certifying the complete UX3 restart workflow.
Closing the App loses the client session-only key; do not create a replacement
batch to resolve uncertainty. Audit/database reconciliation remains a separate
qualified operation until the GUI discovery contract is implemented.

P1 batch approval/execution, implicit full-query selection, unsupported filters,
MAS/Web/fnOS decision writes remain disabled/deferred. This is an initial UX1
slice, not full P0 acceptance, a release permission, or a change to MAS NO-GO,
Phase1A/1B INCONCLUSIVE or historical CarbonCore FAIL.
