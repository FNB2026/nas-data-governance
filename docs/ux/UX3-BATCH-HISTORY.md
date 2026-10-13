# UX3 — Read-only durable batch discovery

## Scope and source dependencies

This incremental implementation closes the discovery gap left by known-key
lookup: after losing in-memory client keys, a reopened project can list its
durable governance batches and query their receipts by opaque batch ID.

The original development snapshot was stacked on experimental integration commit
`77d38efd36e1eb31915c5dcffad2bf4d0a8aadfb`, tree
`5edcf344e0d89a709c3c7c864930d99648913b49`, which combined PR #96 frontend
`8e6d1d1e69ed3b57a5edcd94fc0066221e103dc3` and PR #98 backend
`e4dfaaadccb94ebf33e9d1be2fc48ba3e060f5e8` from main
`15328e0054c1c3e476dd756436ce52d6f3c50c1a`. That stack was not merged to main
as a shortcut. The dependencies have since merged through their own PRs. This
PR now normally merges current main and is based on main; its only feature diff
is the UX3 read-only history slice documented here. No change belongs to frozen
beta.11.

## Read contract and safety invariants

- `ListGovernanceBatches` is a sixth high-level **read-only** Wails use case;
  the existing five contracts remain unchanged. Service/store/API contain no
  commit, approval, recovery, source-read or filesystem-write fallback.
- Query defaults to page 1 / 20, allows 1–50 rows and a bounded positive page;
  the GUI requests 10. Count and page use one SQLite read snapshot. No schema
  migration, retention deletion or new database table is needed.
- Only the current database's project namespace is enumerated. Historical
  physical scope bindings remain readable after a managed move/backup reopen,
  as in existing receipt lookup; they grant no replay permission.
- Summary fields are batch ID, actual persisted status, requested count,
  durable outcome count and creation time. No idempotency key, preview token,
  note, request digest/payload, source path or filename crosses the history DTO.
- Durable outcome count includes conflicts/refusals, not just successes.
  `IN_PROGRESS` remains pending even if an operator queries repeatedly.
  Unknown state, unknown outcome, impossible counts or malformed timestamps
  fail closed. COMPLETE with missing durable outcomes is refused.
- Writers' UTC RFC3339Nano times are fraction-padded in ordering; raw strings
  would put a trailing whole-second Z after a later fractional timestamp.
  Equal instants use a stable ID tie-breaker.
- RO project, channel policy and active Recovery Lock allow queries but acquire
  no new recording/execution authority. The lock and plans are never repaired,
  finalized or cleared by this feature.
- The GUI checks the independently loaded governance project scope and strict
  page/receipt accounting. Project replacement, scope/path change, component
  unmount and refreshed requests invalidate late responses. Raw API errors and
  unknown reason codes are replaced by existing safe labels.
- Historical receipt viewing is separate from the active submit controller:
  it never clears retained client requests/selections, creates a new retry key,
  commits an old batch, or calls legacy mutations. Older APIs without discovery
  display an explicit unsupported state and retain existing known-key lookup.

## Actual automated evidence

The baseline build had no history method/type; the new store regression failed
to compile until the query contract was added. This demonstrates missing
discovery, not a newly discovered release defect.

Real store/SQLite fixtures cover three completed batches, bounded count/pages,
loss of keys, close/reopen RO discovery, byte-identical database reads, source
SHA conservation, project isolation and fractional timestamp ordering. A
deterministic transaction failure leaves a real batch IN_PROGRESS with zero
outcomes; query does not finalize it or insert audit rows. A separate inconsistent
synthetic terminal fixture is refused. All files and databases are disposable;
historical failure evidence is not opened or changed.

Real Wails adapter/service/scanner/store tests cover seven temporary files,
natural scan completion, numeric JSON summary roundtrip and RO rediscovery.
Another actual executing-plan fixture establishes Recovery Lock; a history
query and receipt lookup keep both lock and durable EXECUTING state unchanged.
The channel switch in this test is adapter policy evidence, not a native
macOS Sandbox permission certificate.

Frontend component tests cover remount without keys, RO/recovery-lock lookup,
partial outcomes and pending status, false COMPLETE accounting, unknown older
API, scope mismatch, late list/detail after project replacement, safe errors,
pagination and refresh. The integrated full suite has 33 files / 310 tests PASS.
Component mocks are distinct from real Go/SQLite tests; neither certifies the
native GUI. Full required Go/race and frontend build evidence is collected
separately from Wails SDK generation and development App build.

## Remaining boundaries

- No automatic recovery of a lost original request, new-key replay, cancellation
  of an in-progress batch or mutation of its final state is implemented here.
  A process-interrupted batch is exposed for explicit read-only/manual review;
  discovery must not be interpreted as complete background-task recovery.
- Physical source edits not reindexed still lack the full P0 stale contract.
- Genuine isolated native GUI, formal signed candidate and 5–8 nondeveloper
  comprehension walkthrough remain NOT RUN. Full UX v2 PASS is not claimed.
- New source is root-authored and needs independent exact-HEAD review. Old
  dependency reviews and green CI cannot substitute; retain Draft until closed.
- MAS Phase1A/1B INCONCLUSIVE, CarbonCore historical FAIL, MAS NO-GO and
  forbidden MAS/Web/fnOS file writes are unchanged. beta.11 assets immutable.

Raw outputs and screenshots remain local private evidence. Public receipts name
the exact source/tree, scope and return codes without personal paths or markers.

## Independent exact review follow-up

The independent reviewer at 4436342766477c492cdc7efcadd6269e4ad7a425 found two P2
issues despite green baseline tests. A real SQLite negative replaced a COMPLETE
outcome's group ID with an unrelated ID while retaining row count; history still
reported COMPLETE. Separate actual JS probes found inherited reason properties
(constructor/toString/__proto__) resolving to functions/objects instead of safe
text. Both newly committed negative regressions first failed on that snapshot.

The read-only history transaction now validates durable preview scope/token,
unique members, original request member identities and all outcome identities;
COMPLETE requires exact coverage, never pseudo NOT_COMMITTED. Unknown rows, wrong
scope/token, duplicate preview, request mismatch and missing/uncommitted outcomes
refuse without repairing state or writing audit/source data. Reason labels use
own-property lookup; rendered history tests cover prototype names and never call
commit. This remains bounded discovery, not automatic recovery of lost requests.

Original green and RED records remain. The final integrated source must receive
an exact review and fresh gates. The original experimental base is preserved as
provenance only; the PR's current base is main, and the experimental integration
branch was not merged into main as a shortcut.

## Main compatibility snapshot (2026-10-14)

Current main `ed7287c1842aec2dd0608895cd027fc502990864` was merged normally into
the history branch at `660ed7c121ec66a5b4a5e38fbe97bbb50f86d1c2`. The current
17-file diff from main contains only UX3 history UI, its read-only Wails/service/
store contract, generated bindings, tests and this document. Main's SDK/API,
chronological decision timestamp and governance safety implementations are
retained. Fresh local required tests, vet, build, release/public checks, full Go
race, frontend tests/build passed at that snapshot; remote CI, Security and
Linux Web boundary runs were then dispatched for the same SHA. This does not
upgrade native/signed GUI, human walkthrough, physical unindexed-stale, automatic
lost-request recovery or overall P0 acceptance; those remain NOT RUN/open.
