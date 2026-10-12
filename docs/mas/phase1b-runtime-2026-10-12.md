# Mac App Store Phase 1B — sandbox authorization runtime evidence

## Decision

Phase 1B remains **INCONCLUSIVE**. The local, ad-hoc sandbox App demonstrated explicit reselection, persistence across an orderly process restart, and a successful scan of a synthetic relocated local source. Earlier disposable SMB runs demonstrated a network pause and same-identity Resume, and rejected automatic reuse after the SMB endpoint changed. The current official signed beta.11 artifact was not used; TCC revocation, exact SMB reauthorization after endpoint change, exact SQLite checkpoint inspection for the final local run, and bounded cancellation of blocked native SMB I/O remain unverified. A strict unified-log privacy check found a synthetic test marker in an Apple CarbonCore message; this is a privacy **FAIL** for the observed log window, without evidence that NDG's own logger emitted it. Do not treat this document as MAS readiness or a PASS for the full runtime gate.

## Release and source identities

- PR #86 Phase 0 was merged before this work. PR #88 Phase 1A and icon PR #89 were independently reviewed, CI/Security passed and both merged. The icon change applies only to future builds; beta.11 assets were left intact.
- Phase 1A's full formal runtime acceptance remains **INCONCLUSIVE**.
- Phase 1B branch: `codex/mas-phase1b`, based on main `a878e04e615439439321198388e6e7e984d0c561`.
- This branch adds an explicit source-authorization panel for an already-open writable project. It uses the existing product Wails picker and `RegisterScanSource` gate; it does not start a scan or Resume, migrate old checkpoint rows, change recovery locks, or enable writes.
- Runtime tests used the actual Wails/React product UI in a local **ad-hoc signed development sandbox build**, bundle `com.fnb.ndg.mas.phase1a`. It was not a Developer ID or MAS-signed distribution, had no MAS provisioning profile, and was not beta.11. The longer GUI runtime sequence used the same source tree before commit, but its About commit field pointed at the clean base; those runtime observations are not attributed to an immutable source SHA. After commit `7fc299b660e27abb08c5178d81a51dd7aa7a7516`, a clean-HEAD rebuild displayed that exact full commit in About and the explicit authorization panel was visible in the disposable project. This later build received identity/UI smoke verification only; it does not retroactively bind the earlier extended runtime sequence to the committed SHA.
- No App Store Connect upload, Quarantine, Restore, or Purge write was performed.

## Runtime result matrix

| Scenario | Result | Evidence and limit |
|---|---|---|
| Disposable local baseline | PASS | Earlier Phase 1B run completed for 7 synthetic files; quick hash covered 7/7, full SHA-256 covered the 6 duplicate candidates, 3 duplicate groups were shown, and the singleton remained outside full hashing. All 7 source files matched the private fixture baseline. |
| POSIX permission denial | PASS, limited | A disposable source directory was made unreadable and restored immediately. The GUI returned an authorization error; the prior completed checkpoint and active file records remained unchanged. This is not a macOS TCC revoke test. |
| macOS TCC permission revoke/regrant | NOT RUN / INCONCLUSIVE | The ad-hoc bundle did not appear in the targeted File & Folders control. Full Disk Access was not granted as a substitute. |
| Stale/moved local root, without reselecting | PASS, limited | The saved root became unavailable; scan start returned an authorization error. The project remained readable, and no indexed rows were marked missing. The UI initially had no reselect entry, which is why this branch adds one. |
| Explicit reselection in the existing project | PASS, limited | Through the native system directory panel, the relocated synthetic root was registered as a second storage. The old root stayed unavailable; the new storage was online; the UI stated that no scan, Resume, checkpoint migration, or recovery-lock clearing occurred. |
| Reselection persistence across orderly restart | PASS, limited | The test App was quit normally and relaunched. The new root remained online and could be selected in the scan page. A manual scan completed with discovered/processed 7/7 and failed 0. Source SHA-256 matched the fixture baseline 7/7. This proves the exercised local path, not native `bookmarkDataIsStale=true` handling. |
| Recovery lock remains uncleared by reauthorization | PASS in automated test; runtime INCONCLUSIVE | `TestReauthorizationNeverClearsRecoveryLock` is included in the full Go race run. The GUI scenario with a live unresolved Journal lock and source reselection was not run. |
| SMB network loss during full hashing | PASS, prior disposable run | The actual UI showed FULL_HASHING before the isolated loopback SMB service was stopped. The GUI exposed Resume and the DB observation recorded `PAUSED_NETWORK`; all indexed file records remained active. The exact runtime receipt is retained privately. |
| Same-identity SMB remount and Resume | PASS, prior disposable run | The same test service identity was restored; GUI Resume reached COMPLETED and the observed project remained intact. Do not generalize to arbitrary NAS or formal distribution. |
| Changed SMB endpoint auto-inheritance | PASS, prior disposable run | Replacing the test service endpoint without explicit reauthorization produced an authorization failure. No new job was started and no files were reconciled as missing. |
| Changed-endpoint SMB explicit reauthorization and Resume | INCONCLUSIVE | A later attempt could not prove that Finder's mounted endpoint corresponded to the isolated test service: the mounted endpoint reported by the OS differed from the service port, and the expected disposable mountpoint was empty. Testing stopped without scanning or changing that project. No other SMB mount/session was altered. |
| Delayed native SMB read and Cancel | PASS for safe final state; INCONCLUSIVE for bounded cancellation | During a disposable full-hash read, the native SMB operation did not return promptly. The GUI entered “cancelling”; after the isolated test server was restored, the job became CANCELLED, the checkpoint was resumable, and file records stayed active. No maximum cancellation latency or lease-release deadline was established. |
| Picker close / app exit | INCONCLUSIVE for close-during-panel | Escape dismissed the native panel. Cmd-Q while the modal panel was active did not establish that the App could orderly quit or cancel a pending native picker. The API shutdown regression tests passed, but this exact GUI close path was not proven. |
| Repeated cancel, process restart, and Resume idempotency | INCONCLUSIVE | A same-identity Resume and an orderly local restart were observed separately. The full repeated cancel → quit → relaunch → Resume sequence was not completed against the SMB checkpoint. |
| SQLite checkpoint for the final relocation scan | INCONCLUSIVE | The product UI showed two scan jobs and the final 7/7 completion. Direct `sqlite3`/Python reads of the App Sandbox container DB were denied by macOS to the external shell process. No bypass or copied/replaced DB was used. Exact final checkpoint and hash columns were therefore not inferred from UI counts. Earlier DB receipts remain separate and private. |
| Quarantine / Restore / Purge writes | NOT RUN | Outside Phase 1B authorization; sandbox build keeps write operations disabled. |

## Log privacy window

- A unified-log collector was active across the disposable Phase 1B window (2026-10-12 18:17:25–19:13:01 +0800). It captured 212,154 JSON records across two local files. The raw files are retained locally with mode `0600`; they are not included in this PR.
- The scan for synthetic root strings returned zero exact root-path matches. A synthetic SMB fixture marker did appear in Apple `com.apple.CarbonCore` log records: 40,398 `eventMessage` records and 8,744 `formatString` entries in the collected window. The sample message concerned system file-server configuration. This fails a strict “no test marker in unified logs” requirement for that window. The record subsystem is Apple CarbonCore; available evidence does not establish that NDG's own logger formatted the marker.
- The process's stdout/stderr were not attached to the collector. Therefore stdout/stderr privacy is **INCONCLUSIVE**, not PASS. The collection is not a full-process or long-duration privacy certification.
- Test paths, file names, bookmarks, credentials and raw log content are intentionally omitted from this repository document. Original evidence remains in the local private disposable-test archive.

## Code and automated verification

### PR #90 independent review and channel correction

PR #90 was reviewed at exact HEAD `f266e0f6836294ce5bbf8c96e8ead1655b3ede7e` against base `a878e04e615439439321198388e6e7e984d0c561`. The review found a channel-specific UX/security defect: the authorization panel was shown to ordinary Developer ID/DMG builds even though those builds do not persist sandbox-scoped directory bookmarks. The UI therefore overstated the authorization guarantee for that distribution.

The correction adds backend-derived `can_reauthorize_sources` capability (`sandboxBuild`), and the Sources page hides the panel by default until that capability is positively returned. The panel copy now names the Mac App Store sandbox build. API lookup failures fail closed. This keeps the ordinary DMG behavior and authorization path unchanged; it does not create a new authorization mechanism.

The source-registration call chain remains separate from scan and recovery actions. `PickDirectory` only obtains the selected root; `RegisterScanSource` acquires and releases the sandbox lease while persisting a distinct storage identity. It does not start a scan, Resume, migrate checkpoint rows, or change Recovery Lock. A new business-call-chain regression registers a second disposable root while a recovery lock is active, verifies no scan job is created, verifies the lock remains active, and verifies Resume remains blocked. A build-capability regression covers both ordinary and sandbox configurations. These tests do not substitute for the still-unrun GUI scenario with a live unresolved Journal lock.

No Quarantine, Restore, or Purge write was enabled or performed. No beta.11 artifact, tag, or release asset was changed.

### Log attribution comparison

The earlier strict unified-log result remains an environment-level privacy **FAIL**: the synthetic fixture marker was present in the observed unified-log window. Records were attributed to the Apple CarbonCore subsystem; this identifies the emitting subsystem, but does not prove whether the marker was derived from NDG activity. It remains a failure of the strict environment-wide zero-marker criterion, and causal product attribution is **INCONCLUSIVE**. It is not waived solely because the subsystem is Apple-owned.

The three requested controlled groups (synthetic SMB service only; NDG idle without SMB access; NDG authorized SMB scan) were **NOT RUN** in this update. At inspection time, the ad-hoc NDG test app was open on a disposable local project, an isolated test SMB listener was still present, and the host had other SMB activity. The available evidence did not prove that the mounted SMB endpoint belonged exclusively to the isolated test service. Starting or stopping a service, closing the app, or changing a mount could have contaminated attribution or affected an unrelated session, so no such action was taken. Consequently, there is no new process-correlated comparison for CarbonCore, NDG, WebKit children, or stdout/stderr. The prior stdout/stderr result remains **INCONCLUSIVE** because those streams were not attached. Raw system logs remain local and private.

### Updated verification

Local verification on the Phase 1B worktree:

- `go test -race ./...` — PASS after the channel-capability and business-side-effect regressions.
- `go vet ./...` — PASS.
- MAS-tagged targeted Wails tests for channel capability and registration side effects — PASS.
- Frontend suite — 29 files, 264 tests PASS.
- Targeted `SourceAuthorizationPanel` tests — 3 tests PASS, including the default-hidden case.
- Frontend production build — PASS.
- Ad-hoc sandbox app build from clean commit `7fc299b660e27abb08c5178d81a51dd7aa7a7516` and `codesign --verify --deep --strict` — PASS for local development validation only. The built executable SHA-256 was `e3b7651bc7a95f10bb7dbd1ea151523e6b18f344291e984ad1fc659b9c3e4c79`; About displayed the same full commit. A GUI smoke check opened the disposable project and visibly exposed the new authorization panel. This is not a formal MAS-signed artifact.
- `npm ci` reported a Node engine warning: installed Node 25.9.0 is outside jsdom's declared supported ranges. Tests and build completed, but the toolchain warning should be removed by running the release CI under a declared supported Node version.

The full Wails test suite is not a supported MAS-tag invocation: legacy tests in that package assume ordinary-build permissions. Running the entire package with `-tags mas` executes those unrelated assumptions and fails; the two MAS-specific tests above were run selectively and passed. This limitation is recorded rather than represented as a successful full MAS suite.

## Remaining Phase 1B work

1. Repeat the SMB changed-identity case with a verified isolated mount-to-server mapping; then explicitly reselect that SMB root through the product panel and test same-checkpoint Resume. Do not alter the currently unidentified SMB sessions.
2. Obtain targeted TCC revoke/regrant evidence on a disposable host where macOS exposes the bundle in the relevant control.
3. Exercise a live recovery lock during source reauthorization and prove it stays locked.
4. Capture a reliable native-panel close/cancel path and measure cancellation/lease drain for stalled SMB reads without force-unmounting or killing the App.
5. Run the full cancel → normal quit → relaunch → Resume sequence; verify SQLite with a supported same-app/export path and compare DB hashes/checkpoint to independent source baselines.
6. Run the three process-attributed log comparison groups in a clean, isolated window with a verified SMB service/mount mapping; capture NDG main process, identifiable WebKit children, CarbonCore, and stdout/stderr separately. Resolve the strict environment marker FAIL using source evidence, not an Apple-process exemption.
7. Rebuild the corrected source at an immutable SHA, verify its About identity/signature, and repeat evidence that must bind to that exact build. Formal MAS-signed GUI acceptance remains a later gate.

**Phase 1A runtime acceptance: INCONCLUSIVE. Phase 1B: INCONCLUSIVE. beta.11 assets unchanged. App Store Connect: NOT USED.**
