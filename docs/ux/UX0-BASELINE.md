# Governance UX v2 — UX0 audited baseline

Date: 2026-10-13. Initial product baseline: main
`356383115f43c5d75ed2827cf7520d3172ae055f` (PR #93).
Owner-supplied PRD: 679 lines, fully read in bounded chunks; SHA-256
`136aeb9e9092781f55ca57b6499ffba3e7634870e063bfa614ff68a1da9cc3fa`.
This document is an original implementation/audit summary, not an unqualified
copy of historical product claims. Existing NAS, databases, failure fixtures,
other writers and beta.11 assets are protected. New work uses distinct worktrees,
exclusive private leases and synthetic data only.

## Source-confirmed decision meanings

| Persisted enum | User wording | Actual behavior at baseline |
|---|---|---|
| KEEP_ALL | 保留所有文件 | Records intent in group_decisions; no file operation |
| DRAFT_ACTION | 制定处理方案 | Records intent only; does not itself create, approve or execute a plan |
| DEFERRED | 稍后再处理 | Records deferral; not execution completion |
| REJECTED_SUGGESTION | 不采纳此建议 | Records disagreement; future suggestion suppression is not proven |
| CROSS_ARCHIVE | 标记不同用途的归档副本 | Existing mutually exclusive intent enum; no physical archive/move |
| BACKUP_RELATION | 标记为备份关系 | Existing intent enum; does not certify backup integrity |
| PRIMARY_RETENTION | 历史决定：指定主要保留项 | Seventh historical enum; requires retained_file_id, must remain readable |

Evidence: internal/domain/model.go; internal/app/review.go;
internal/store/decisions.go; migration 008; Wails SaveGroupDecision;
internal/app/plan.go and BuildDraftPlans/SaveDraftPlans call sites.
Domain comments describe additional intent (close/exclude groups), but a search
of application/planner/report/learning consumers did not prove those effects.
Display copy therefore makes no such promise. Cross/archive and backup are not
split into new fields or migrated based on their Chinese labels.

## Safety and API gaps

- Legacy persistence deletes/reinserts the group's decision. It has no version
  CAS, durable batch receipt/idempotency key or same-transaction review audit.
  It is not safe to implement a hidden frontend loop over this mutation.
- Wails legacy SaveGroupDecision checks read-write service presence but does not
  itself recheck sandbox policy or Recovery Lock. Synthetic baseline regressions
  reproduced writes during both restrictions. New writes must close this gap
  before being enabled; UI capabilities alone are insufficient.
- SQLite uses per-handle serialization; it has no shared project writer lease
  across every current writable store.Open. A second synthetic writable handle
  was accepted. A narrow owner lease is being implemented separately; old beta.11
  does not participate and cannot be retroactively certified by a new lock.
- ApprovePlans separately rejects critical/non-DRAFT plans and recovery lock,
  but baseline MAS policy is not rechecked there or in manual SaveDraftPlans.
  Those confirmed bypasses require backend regressions and guards;
  executor stale/approval/Journal rules remain separate. P0 exposes no approval,
  source execution, HOLD release or Purge shortcut. All new-channel writes remain
  denied. Purge release policy remains backend-enforced.
- Legacy group pagination only supports storage/minimum hardlink-aware estimate.
  New query count/page must use identical server filters. Unproven risk, role,
  protection, time, physical-space or executable filters remain unavailable.
  Unknown is not LOW risk. Context detail can display actual protected flags and
  retention explanations without converting them into execution permission.

## Implementation contract

1. UX1 combines list, evidence, intent form, explicit selection and preview.
   Old route identifiers and plan/approval/recovery pages remain accessible.
2. UX2 provides bounded explicit batches (maximum 100), subject/decision version
   verification, project-scoped expiring previews, idempotency and durable
   per-item outcomes. Decision + review audit + receipt use a proven transaction
   boundary. No group-file operation is introduced.
3. RW desktop may record only when backend guards pass. RO/MAS previews are pure
   reads, issue no commit token and write no database. Web/fnOS adapter does not
   expose decision writes. The UI must fail closed if the new contract is absent.
4. Page selection retains exact observed IDs/versions across pagination and
   sorting. Filter/data changes retain but warn about stale selections; recheck
   before commit. No implicit all-filter selection. No source paths in URL or
   persistent frontend selection storage.
5. Transport errors are ambiguous: retain the exact request/key and reconcile
   its durable receipt. Never relabel an IPC success as entire-batch success;
   validate unique IDs, terminal outcomes and counts. Preserve failed selections.
6. P1 batch approval/execution, full-query selections and unsupported platform
   capabilities remain DEFERRED/BLOCKED. No new public release is authorized.

## Evidence status

UX0 static source audit: performed; independent review pending.
UX1 implementation/component tests and synthetic browser QA: see
[UX1 implementation evidence](UX1-IMPLEMENTATION.md); not formal App PASS.
UX2 persistence/security: in progress in a separate backend worktree.
P0 bytes/concurrency/old-schema/100-group/10k-query and actual GUI: NOT RUN until
recorded at the tested HEAD. Human comprehension walk-through (5–8 people) and
future signed-candidate acceptance: NOT RUN; never inferred from unit tests.
Phase1A/B INCONCLUSIVE, CarbonCore historical FAIL, MAS NO-GO remain unchanged.

## Affected boundaries

Frontend: App/navigation, high-level typed governance adapter, workbench/evidence/
preview, selection/outcome state, vocabulary, tests and isolated CSS. Backend:
review query/decision transaction use case, Wails guards/DTOs and minimal
compatible persistence/owner locking. Scanner/hash/planner/executor algorithms,
old enum values, old beta.11 release/version/signing assets are not UX refactors.
