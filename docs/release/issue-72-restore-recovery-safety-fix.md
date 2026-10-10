# Issue #72 — Restore Recovery safety fix

## Frozen failure and scope

Formal beta.8 RC `6644c2ba520173be27a33c53c241e95241563c25`, [PR #71](https://github.com/FNB2026/nas-data-governance/pull/71), [Issue #72](https://github.com/FNB2026/nas-data-governance/issues/72). The full quarantine copy survived, but a 7,634,944-byte partial Restore destination also survived while the backend persisted ROLLED_BACK and removed pending recovery; the official GUI consequently reported success and unlocked. No evidence establishes loss of the original content.

Historical databases, journals, partial output, quarantine copies, backups, screenshots and logs remain unchanged. This fix uses a separate managed worktree and fresh test databases/synthetic files. A live open-handle inventory found no other IDE using the new worktree. Product scope is Restore execution/recovery safety only; no migration, force-unlock API, new feature, Purge or modification of beta.8 assets.

## RED before implementation

`go test -count=1 -run '^TestRestorePartialDestinationRetainsRecoveryLock$' ./internal/app` failed on the frozen implementation:

```text
partial destination incorrectly terminalized: status=ok final=ROLLED_BACK durable=ROLLED_BACK pending=0
```

The RED log is preserved locally. The regression asserts refusal, pending reservation, recovery lock and preservation of both partial output and complete quarantine bytes.

## Minimal safety contract

| Observed state | Result |
| --- | --- |
| Verified complete quarantine, destination conclusively absent | Confirm rollback and atomically persist terminal plan, Journal and audit |
| Destination exists: partial, changed, full matching or full different | Refuse; preserve both instances and pending recovery |
| Quarantine missing/changed/unreadable, destination full or absent | Refuse; preserve pending; content equality does not establish created-output identity |
| Unreadable path, symlink, unavailable ancestor, out-of-root path or different-device descendant | Refuse; absence cannot be inferred from a read error |
| Terminal persistence or audit fails | No success; transaction rolls back and pending remains |

Recovery performs no filesystem writes or automatic deletion. The old Journal has no durable output inode proving that a surviving destination was created by this attempt. Therefore even a full, matching destination alone is not authority for automatic reverse movement.

Read-only probes use root/ancestor directory descriptors, reject symlinks and different devices, require regular files, hash an opened descriptor, check metadata before/after, and reopen the current root/ancestor/leaf binding. Only a leaf ENOENT under successfully checked existing parents proves absence. Unsupported platforms fail closed. Approved current root directories remain a trust boundary: the existing schema does not retain historical root inode identity across application restarts; this change does not claim to detect an arbitrary cross-restart replacement of an approved root with identical content.

Normal ExecuteRestore failure uses the same strict reconciliation after a failed move. Completion persistence failure leaves the full destination and pending Journal intact instead of blindly reversing/deleting an output of uncertain identity. Structured recovery errors and audit details use static classifications, never raw OS/SQLite errors or source paths.

BeginRestore validates canonical plan/item paths, hash, size, approval digest, state and same-item pending reservations in its transaction. Terminal updates consume exactly one consistent pending/APPROVED reservation with checked affected rows; lifecycle, plan, Journal and sanitized audit commit together. ROLLED_BACK clears the old approval. Service execution/recovery retain the existing shared cross-process owner lock and pending recovery gate. Audit records use the originating source plan foreign key; they do not invent a restore-plan FK in operation_logs.

## Explicit manual reconciliation path

This is an operator workflow, not an App force-unlock feature. Do not apply it to historical failure scenes as part of this task.

1. Keep the lock. Verify the complete quarantine item against its approved SHA-256/size, verify an independent backup, and preserve the current database/Journal and partial target evidence privately.
2. In a separately authorized disposable/manual execution step, move the partial/conflicting target into a new independent retention location without overwriting or deleting any instance. Verify the retained bytes and that the original approved destination is absent. Do not change the database or Journal.
3. Use the normal formal GUI Recover Quarantine Restore with the same approved current roots. It rechecks the full quarantine and actual destination absence before terminalizing. Verify plan ROLLED_BACK, Journal rolled_back, successful sanitized audit and lock removal, including after reopening.
4. A new Restore requires a new plan and approval; the old approval cannot be replayed. Keep the retained partial evidence until an explicit later cleanup authorization.

If only a full destination survives and quarantine is missing, remain locked. Any manual return of that copy to quarantine needs its own authorized, backed-up, verified non-overwriting operation; the backend will not infer authority from SHA equality. If evidence or boundaries cannot be established, manual investigation remains required.

## Verification layers

New tests cover deterministic same-content root/ancestor/leaf replacement during descriptor reading, missing/unreadable approved roots with redacted errors, partial/changed/full conflicts, missing/changed/unreadable quarantine, absent/unreadable destination, leaf/root/ancestor symlinks, unavailable ancestors, out-of-scope paths, different-device mount metadata where present (same-device bind mounts are not identified by Dev alone), repeated recovery/reopen, persistence/audit failure, identity/CAS conflicts, owner-lock exclusion, normal execution failures, safe manual preservation and a fresh approved Restore. Wails integration checks actual SQLite pending state propagates into failed structured results and a locked DTO, then safe closure and audit propagate correctly.

Existing tests that encoded unsafe partial/restore-only success were corrected; completion persistence failure now asserts preservation and pending rather than unproven automatic rollback. No development test substitutes for the new signed candidate. Independent review, exact-head CI/Security, merge/post-merge gates and beta.9 formal D plus targeted A/B/C remain required. Public Beta BLOCKED; Release Draft; Issue #72 stays open until repair and formal acceptance are complete.

## Local verification evidence

- Original RED log SHA-256: `68e87ac144b080722a7c5c8b4722a3a90a7bf6d51e168fcd25dd2c78aa7cfa0e`.
- Full `go test -race -count=1 ./...`: PASS; log SHA-256 `e7899f9634c76c893011de7f3067a81a9e362c2228a30a544264c1a94118f9b0`.
- Final Restore-focused race after dynamic-binding/root tests: PASS (app 8.932s, executor 2.468s, Wails 2.233s).
- `go vet ./...`, `make public-check`, formatting/diff checks: PASS.
- No formal beta.9 runtime evidence exists at this source-fix stage.
