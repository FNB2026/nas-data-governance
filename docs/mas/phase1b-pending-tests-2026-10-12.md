# Mac App Store Phase 1B — pending runtime scenarios

This checklist preserves Phase 1B as **INCONCLUSIVE**. It does not supersede the historical CarbonCore strict zero-marker **FAIL** or authorize touching the active desktop environment.

## Environment requirement

Use a dedicated macOS VM or independent Mac with a fresh app container, synthetic-only SMB server, dedicated test share and mount point, and a controlled port. Before any test, record the test App PID, binary SHA-256, Bundle ID, version, test window, server identity, client mount mapping, and synthetic source hashes. Prove the mounted share maps to the test service before selecting files.

If isolation or service-to-mount mapping cannot be established, stop and report BLOCKED. Never inspect or take over QEMU PID 29693, stop an existing App/service, unmount an unverified SMB mount, modify TCC/system privacy databases, or reuse any beta.11/project container or historic failure DB.

## Open scenario matrix

| Scenario | Current state | Evidence required |
|---|---|---|
| CarbonCore 3-window logging controls: service only; App without SMB access; authorized SMB scan | Historical strict zero-marker FAIL is retained; attribution unresolved | Per-window monotonic timestamps; process/binary identity; unified logs separated by NDG/WebKit/Apple process; stdout/stderr; compare exact synthetic markers. Never use a new clean run to erase the historic FAIL. |
| SMB identity change and reauthorization | INCONCLUSIVE | Prove service endpoint and mount mapping; show changed identity refused, explicit new selection recorded, old source remains unavailable, then lawful Resume. |
| Reauthorization while Recovery Lock is active | Runtime INCONCLUSIVE; automated test evidence exists | Produce a real unresolved disposable Journal lock; reauthorize through GUI; verify lock remains until normal recovery closes it. |
| Cancel → orderly quit → relaunch → Resume persistence | INCONCLUSIVE | Observe scan state, cancel, close normally, reopen same disposable project, verify checkpoint/Journal/source hashes, Resume, final DB integrity. |
| Native picker close while active | INCONCLUSIVE | Capture actual GUI and process lifecycle when panel is dismissed or app closes; ensure no background lease/panel survives. |
| Blocked native SMB I/O cancellation bound | INCONCLUSIVE for latency; safe terminal state evidence retained | Exercise only an isolated server. Record cancel request/return/server restoration times and resource release. Do not assert bounded immediate return unless observed. |
| TCC permission revoke/regrant | NOT RUN / currently no demonstrated control | Document exact macOS privacy control and bundle identity. Do not use Full Disk Access or edit the TCC database. If no user-accessible path exists, preserve NOT RUN. |
| Stale Bookmark regeneration | NOT TRIGGERED / INCONCLUSIVE | Induce a legitimate stale condition in a disposable volume and capture bookmark refresh/persistence; otherwise retain status. |

## Safe stop and close criteria

- Stop immediately if test source identity is uncertain, an unrelated mount/session would be affected, source hashes change, DB/checkpoint evidence becomes ambiguous, or raw sensitive data appears in ordinary logs.
- A permission/authorization failure is never translated to `missing`.
- Reauthorization does not resolve an active Recovery Lock.
- Preserve the disposable project until a closed-DB integrity check and before/after source hash comparison are complete.
- Keep raw logs/screenshots in a private local archive; only commit redacted summaries.

No Phase 1B runtime tests were executed as part of Phase 1C. Environment availability remains BLOCKED; all statuses above are carried forward as historical evidence.
