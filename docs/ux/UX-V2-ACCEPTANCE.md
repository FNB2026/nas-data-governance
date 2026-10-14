# Governance UX v2 — Phase B acceptance ledger

Date: 2026-10-14. Initial matrix and SQL/store baseline source: main
`5b802233c2a33a61fb57587d92724e70836400a1`, tree
`b46284935177c04af81526041651faf80c99fb85`.
Initial B-1 classification implementation: `c4113e997be3f548ea50f2049ac8334fe169f4b1`,
tree `6e5d3fd332cc48d79be810ab25ac73676026ef0f` in PR #110. Later revisions
clarify documentation and remove a UI claim of a local source when network
identity is unconfirmed; the Go classifier remains unchanged. Final source,
merge/CI and native/signed GUI identities are separately bound in the PR's
exact-HEAD receipts. The initial baseline does not certify those later changes.
Scope: acceptance and confirmed-defect triage; no unrelated feature expansion.
The owner now orders the work as Phase A → B → C → D → E.

## Owner's revised Phase B sequence

### B-1 — Issue #109: diagnosis before revalidation

`UNVERIFIED → evidence-backed cause → explicit revalidation → MATCH / clear refusal`.
No automatic retry loop, weaker reader or platform bypass is introduced.
The current Darwin cause is an unimplemented NDG mount-boundary proof, observed
before catalog, lease, metadata or content access. It is not evidence of inode
replacement, expired Bookmark, changed volume UUID, permission denial or a NAS
outage. The typed refusal is `SOURCE_PLATFORM_PROOF_UNAVAILABLE`; repeated
explicit previews still refuse without issuing tokens or reading source files.

The reason-contract change separately classifies typed permission, cancellation,
deadline, mount/symlink rejection, unavailable kernel method, source content
mismatch and catalog failure. Unknown errors stay unknown and private error
strings are not returned as reasons. Reader identity/metadata/binding changes
remain one honest category because the existing reader does not expose a more
specific observation. Authorization-layer failures do not prove an expired
Bookmark or changed volume UUID; they keep the existing authorization-required
category. A network source's missing/read failure is not proof of a server outage.

All rejection reasons are allowlisted by freshness status in preview and commit.
Only `MATCH` plus existing eligibility/capabilities may record. Commit rechecks
the source and durably preserves rejection reasons across receipt replay/restart.
A new explicit preview may succeed only after a recoverable source cause is
corrected and the trusted reader actually observes MATCH. Platform method
absence cannot be corrected by retrying or reauthorizing.

Under the owner's revised PASS / clear-refusal contract, Issue #109 may close
after the classification and explicit revalidation/refusal code contract passes
independent review, final CI and post-merge checks. The original Mac positive-flow
blocker is preserved separately in [Issue #111](https://github.com/FNB2026/nas-data-governance/issues/111).
B-1 closure does not establish Mac Source Verification PASS or close Phase B;
native/signed GUI evidence remains part of B-2/B-3.

### B-2 — T01–T25 execution groups

| Group | Cases | Evidence needed |
|---|---|---|
| Core chain | T01–T10 | Actual supported user flow, selection, current-source preview, durable recording/receipt and restart; source bytes unchanged |
| Exceptional chain | T11–T18 | Read-only/lock/permission/network/source-change refusal and legal revalidation; no false missing, no automatic unlock |
| Experience chain | T19–T25 | Upgrade/rollback, native accessibility, privacy, GUI/memory performance, channel/ownership limits and nondeveloper comprehension |

Each case retains the layered status below. Nondeveloper evidence is additionally
mandatory, not inferred from frontend tests or CI. Applicability/refusal cases
must show the expected actual refusal; they cannot excuse the required primary
Mac positive flow. No historical failed runtime result is rewritten.

### B-3 — Governance UX v2 RC-1 readiness contract

This is a readiness label, not a created version, Tag, DMG or release asset.

| Required condition | Current status |
|---|---|
| T01–T25 applicable cases closed with source-bound evidence | BLOCKED / partial evidence |
| Mac Source Verification positive flow and negative boundaries PASS | BLOCKED |
| Current native GUI walkthrough PASS, with signed-candidate identity when certifying distribution | NOT RUN |
| Nondeveloper usability PASS (5–8 participants, at least 80% comprehension) | NOT RUN |
| Full CI/Security at the final frozen RC source and post-merge gate PASS | Required for the future RC; current/old green results are not that identity |

Docker, fnOS and MAS channel rollout remain BLOCKED on this RC exit and their
own later runtime gates. No approval for public release or cross-channel writes
is implied. Keep GitHub beta.11 assets immutable.

## Stage exits

| Phase | Exit | Current state |
|---|---|---|
| A | #105/#106/#107 independently reviewed, normally merged, same-source post-merge checks obtained | CLOSED; evidence below |
| B | UX v2 P0 hard gates, actual supported desktop workflow, performance and comprehension evidence | BLOCKED; code evidence is partial runtime coverage |
| C | Reviewed, isolated disposable Docker host trial with actual HTTP/scan/restart/privacy proof | NOT RUN for the next trial; prior fnOS LIMITED BLOCKED preserved |
| D | Installable reviewed fnOS artifact, isolated first install and safe lifecycle evidence | NOT RUN; current scaffold is not installable |
| E | Reassess MAS using actual compatibility/runtime/account evidence | NOT RUN for this reassessment; existing MAS NO-GO unchanged |

Do not start C/D field operations while B is unresolved. Read-only dependency
inventory is allowed; it does not constitute executing or passing those phases.
P1 batch approval/execution and all-filter selection remain deferred and are
not artificially made P0 requirements. beta.11 assets and historical evidence
remain immutable; no new public release is authorized.

## Phase A receipt

#105 merge `20c545b60d88a2ea67beef5be1766cc97ec152d0`, #106 merge
`b686cfe4dabf0f8eaa182c598e34f092474f415d`, #107 merge
`5b802233c2a33a61fb57587d92724e70836400a1`.
The previously missing final main checks are now read successfully:
[CI 38551647447](https://github.com/FNB2026/nas-data-governance/actions/runs/38551647447),
[Security 38551795482](https://github.com/FNB2026/nas-data-governance/actions/runs/38551795482),
[Linux 38576299341](https://github.com/FNB2026/nas-data-governance/actions/runs/38576299341).
All eight standard checks completed SUCCESS at the frozen source. An additional
same-source Linux dispatch was still running at the first successful sample;
it is not represented as completed, and no further repeat is needed.
Local main required checks returned 0; private output SHA-256
`b24768b23f3a5f08aee239f8f40486232b551564f593ec96aa48df47633822d4`.
[Closure receipt](https://github.com/FNB2026/nas-data-governance/pull/107#issuecomment-6170209608).
Original EOF/TLS gaps and failed historical runs are preserved.

## Evidence rules

- A passing test establishes its tested layer only. CI, SQLite/store, mocked
  bridge, rendered browser, native dev App and signed App are distinct layers.
- The matrix is an inventory, not a blanket PASS for all listed tests. Runtime
  rows require current build identity, actual interaction, state and source bytes.
- Do not inject a reader/flag into a signed App, fabricate Journal, alter an old
  database, or call a lower persistence layer to bypass source verification.
- Only newly created disposable files/projects may be used. Raw evidence remains
  private; public records use safe counts, hashes and code/test identifiers.
- Confirmed acceptance defects may receive minimal independently reviewed fixes;
  unrelated features, platform write enablement and broad refactors are frozen.

## Current blocking conditions

1. Darwin source freshness is deliberately unavailable: the production verifier
   returns UNVERIFIED before a source lease/read and issues no new preview token.
   Safe refusal is demonstrated; successful new desktop decisions are not.
   This blocks the primary desktop P0 positive workflow, including a 100-group
   business flow. Store-only successes do not close this gap.
2. No current native/signed UX v2 GUI acceptance exists. The installed public
   beta.11 does not contain these later main changes; old bridge screenshots
   cannot establish the frozen product's behavior.
3. The required 5–8 nondeveloper walkthrough and 80% comprehension evidence are
   NOT RUN. No people are contacted or results inferred automatically.
4. 10k SQL measurements do not cover GUI responsiveness, memory, keyboard/zoom
   or native source I/O. A written performance decision is still required.

Capabilities express permissions, while per-group freshness can independently
block a request. The current capability label and the unavailable Darwin
verifier must be checked together in actual GUI acceptance; do not relabel the
platform as fully usable solely because can_record is true.

## PRD T01–T25 inventory

Statuses below are runtime/full-case status. Existing code evidence is listed
separately and must not be interpreted as a PASS of the whole case.
Independent read-only source auditor `/root/linux_web_implementation` checked
the frozen HEAD/tree and mapped all 25 cases without running tests, GUI or NAS.
The audit specifically confirmed the store-only 100-item boundary, UNKNOWN
risk/role capabilities, network-source test skipping, and missing native AX/
memory/human evidence. It is not independent runtime acceptance.

| ID | Existing evidence / bounded next check | Runtime status |
|---|---|---|
| T01 | Audited six intent enums plus historical PRIMARY_RETENTION; verify real old-decision rendering and audit links | NOT RUN |
| T02 | Disposable store transaction/byte evidence; actual Wails on Darwin refuses new freshness/token | BLOCKED |
| T03 | Component selection tests; current-page scope/count must be observed in actual App | NOT RUN |
| T04 | Cross-page/sort/project-scope component tests; validate current native selection without changing hidden snapshots | NOT RUN |
| T05 | All-filter selection is unavailable by contract; verify truthful native labels and scope | NOT RUN |
| T06 | Protection/critical-plan store guards exist; real mixed risk GUI is not covered, unavailable risk remains UNKNOWN | PARTIAL / NOT RUN |
| T07 | Hundred-item replay/audit dedup test and mock double-click tests; actual App request/receipt still needed | PARTIAL / BLOCKED |
| T08 | Store CAS/concurrent request tests; two real windows and cross-process ownership are distinct tests | NOT RUN |
| T09 | Indexed member/version conflict and Linux physical-stale tests; native scan-update workflow still needed | PARTIAL / BLOCKED |
| T10 | Durable failure/retry/audit and component outcomes; actual cancel/exit/restart/receipt discovery still needed | NOT RUN |
| T11 | Read-only service/store/UI rejection tests; use a new closed disposable read-only project for App refusal | NOT RUN |
| T12 | Recovery-lock guarded writes and read-only history tests; actual lock/GUI evidence must retain source/Journal state | NOT RUN |
| T13 | Critical/protected approval/execution guard tests; no new batch approval/executor path is allowed | PARTIAL / NOT RUN |
| T14 | Existing Beta backend Purge denial tests; verify current App policy without invoking real deletion | PARTIAL / NOT RUN |
| T15 | Existing stale/approval-digest regression suite; distinguish old candidate evidence from current App regression | PARTIAL / NOT RUN |
| T16 | Hardlink-aware query and role evidence; unknown physical savings cannot be presented as certified reclaimable bytes | NOT RUN |
| T17 | Linux openat2 ordinary/injected-error CI; Darwin refuses freshness; actual bind-mount/SMB identity matrix absent | PARTIAL / BLOCKED |
| T18 | Network-source acceptance requires a separately configured disposable source; ordinary CI may skip it | NOT RUN |
| T19 | Old layout/schema/WAL/hot-journal refusal tests; signed upgrade/backup rollback must use new copies, not history | PARTIAL / NOT RUN |
| T20 | Old synthetic browser focus/layout evidence is historical; current native keyboard/Escape/zoom/screen reader absent | NOT RUN |
| T21 | Allowlisted UI/export and synthetic privacy tests; current bounded GUI/process log window still required | NOT RUN |
| T22 | Fresh SQL-only 10k baseline below; no GUI/memory threshold result | PARTIAL |
| T23 | Formal current desktop GUI P0 record-only chain; no need to introduce P1 file writes to test P0 | BLOCKED |
| T24 | HTTP/MAS direct-call denial tests exist; no fnOS installed artifact or complete channel runtime proof | PARTIAL / NOT RUN |
| T25 | Updated store cross-process owner tests; old beta.11/arbitrary SQLite writers and real worktree conflict are not certified | PARTIAL / NOT RUN |

Representative source-confirmed anchors (existence does not imply full-case PASS):

- T01: `TestLegacyReadOnlyDecisionsPreserveUnknownAndPrimary`.
- T06/T12/T13: `TestCriticalProtectedIntentNeverApprovesOrClearsRecoveryLock`.
- T07/T09: `TestBatchHundredItemsReplayRestartAndSourceConservation`,
  `TestBatchPolicyExpiryScopeAndBodyGuards`, `TestBatchVersionsPartialConflictAndUnchanged`.
- T08/T25: `TestBatchDecisionCASRulesProtectionAndGuardChanges`,
  `TestWritableStoreOwnerRejectsSecondHandle`, `TestStoreOwnerAcrossProcessAndCrashExit`.
- T10: `TestBatchDurableFailureResumeAndAtomicAudit`,
  `TestOutcomePersistenceFailureAndCancelledCallNeverReportSaved`.
- T14/T15: `TestPurgeReleasePolicyDeniesBeforeSideEffects`,
  `TestExecutionConsistencyStaleInvalidatesApproval`.
- T17: `TestGuardedContextLinuxOpenat2RequiresNoXdevNoSymlinksAndBeneath`.
- T18: `TestMountedNetworkPauseResumeAcceptance` needs
  `NDG_TEST_NETWORK_SOURCE`; unset is SKIP, not SMB acceptance.
- T19: `TestGovernanceMigrationFailureRollsBackAdditiveIncrement`.
- T22: `TestGovernanceTenThousandGroupPaginationAndFilterCount`.
- T24: `TestSandboxManualDraftAndApprovalAreRejected`,
  `TestAuthenticationCSRFAndExecutionBoundary`.

## Fresh bounded store baseline

Actual command on the frozen source, exit 0:

```text
go test -count=1 -v ./internal/store -run '^(TestGovernanceTenThousandGroupPaginationAndFilterCount|TestBatchHundredItemsReplayRestartAndSourceConservation)$'
```

Environment: Go 1.26.9, darwin/arm64, macOS 26.6.2. Newly disposable SQLite and
synthetic source fixtures only. The 100-item test verifies saved counts,
same-key concurrent replay, audit deduplication, close/reopen receipt identity,
zero created operation plans and source-byte conservation. It calls the store
directly; it is not a Wails freshness or GUI success.

The 10k-group test uses 20k synthetic rows and ten timed
`ListGovernanceGroups` calls of page size 20 (each call contains multiple SQL
queries, not one SQL statement):
P50 42.856375 ms, nearest-rank P95 48.784292 ms. Count/filter assertions passed.
This is an observed SQL baseline, not a preapproved performance PASS, a GUI
measurement or a source scan. Memory and GUI P95 were not measured. Do not set
a retrospective pass threshold to match these numbers.
Private raw output SHA-256:
`6ce8a17c922ae2c22fa0662b695cf94709a33fdeebb124b9ffe3f64dc7d3779f`.

## Native profile preflight

Read-only source inspection found that the ordinary desktop composition uses
`NewAPI()` and the default project service. `DefaultSupportBase()` derives the
NDG application-support root from `os.UserConfigDir()`. This inspection found
no supported ordinary-desktop runtime profile override; the injectable
`project.Options.SupportBase` is not itself an exposed App launch contract.
A different bundle identifier alone therefore does not prove an independent
ordinary desktop data root. No App was launched against the shared default
profile, and no existing App, project registry or historical database was adopted.
Native acceptance needs a proved isolated environment or a separately reviewed
test composition whose differences are explicitly recorded; it must not be
represented as the unchanged official App.

## Next bounded acceptance window

1. Bind a dedicated dev App to the frozen source/build/bundle and a fresh private
   profile/project; never adopt an existing App or historical project.
2. Verify read-only list/detail/history, selection scope, unavailable controls,
   privacy, keyboard/zoom and safe Darwin refusal in the actual GUI. Label dev
   evidence explicitly; it does not certify a signed future candidate.
3. Register the performance environment, sample policy, GUI timing and memory
   thresholds before the measured run; retain the SQL-only baseline separately.
4. Resolve the genuine platform-proof/positive-flow blocker through a narrow
   safety review; no bypass or forced hash-reader success is permitted.
5. Obtain current signed candidate and human walkthrough evidence when the
   supported P0 flow is actually available. Only then decide B's exit.

Current Phase B verdict: BLOCKED, not ACCEPTED_WITH_LIMITS. Unavailable P1
features alone would not block P0, but inability to record safe new decisions
on the primary desktop platform and missing mandatory acceptance evidence do.
