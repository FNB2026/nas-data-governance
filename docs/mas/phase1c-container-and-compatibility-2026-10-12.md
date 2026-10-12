# Mac App Store Phase 1C — container, closed-backup preflight, and compatibility audit

Date: 2026-10-12 (Asia/Shanghai)
Baseline: `cf520128e29897707832c936c74bf8ee5ea9081c`
Status: **design and local preflight implementation only; not MAS-ready**

## Governance state preserved

This report does not change the accepted release matrix:

| Gate | Preserved state |
|---|---|
| Phase 0 | CLOSED |
| Phase 1A source | Merged; runtime acceptance INCONCLUSIVE |
| Phase 1B source | PR #90 merged; runtime acceptance INCONCLUSIVE |
| Independent macOS/SMB environment | BLOCKED |
| Historical strict CarbonCore zero-marker check | FAIL; evidence retained |
| MAS distribution | NO-GO |
| GitHub beta.11 tag and assets | Immutable; untouched |

Phase 1C tests, source review, and this design cannot promote any of these rows.

## Identity and build separation

- Existing GitHub/Developer ID bundle identity is `com.fnb.ndg` in the current desktop `Info.plist`.
- The Phase 0 design proposes a separate MAS identity `com.fnb.ndg.mas`. It is **not registered or confirmed available**. Do not create the App ID or provisioning profile in this phase.
- A separate bundle ID gives the MAS build a separate macOS sandbox container, preferences, and app-scoped bookmarks. This allows the direct build and future MAS build to coexist without sharing a live project database.
- `cmd/ndg-desktop/wails.json`, Developer ID entitlements, beta.11 tag, and published files remain unchanged. `build/mas/config.json` remains a local design configuration with upload and writes disabled.
- App ID ownership/availability, Apple Distribution identity, installer signing identity, profile/capability match, and current packaging rules remain pending account-holder confirmation and a later authorized signing phase.

## Container data layout and access boundaries

The MAS composition root resolves Foundation's `NSApplicationSupportDirectory` and the sandbox maps it to the bundle-specific container. The current MAS API places its project and grant stores under that base. The safe intended layout is:

```text
<MAS application container>/Library/Application Support/NDG/
  projects/<new-project-id>/
    governance.db                 SQLite main file
    governance.db-wal             only while SQLite needs it
    governance.db-shm             only while SQLite needs it
    project.json                  project identity/display metadata
  capabilities/<root-key>.json    private scoped bookmark + grant identity
  recent.json                     private recent-project references
```

The project database, WAL/SHM, metadata, and capabilities are app-private records. They are not placed on SMB or beside source data. Current `internal/privatefs` applies owner-only `0700` directories and `0600` files; the native grant manager writes capability records under a private directory. Project rows and grant records can contain local source paths or bookmark bytes, so ordinary logs and frontend DTOs must not include them.

The actual MAS API already validates project database paths beneath its injected sandbox support root and rejects symlinks/paths outside `projects`. The added MAS-tagged regression tests exercise that boundary with a temporary support root. This is a host unit test, not proof of a real macOS container path or sandbox enforcement.

An open SQLite project is one live unit: main DB plus its WAL/SHM sidecars and SQLite locking state. Never make an import by copying only the main file while an app may be writing, never point both editions at the same live DB, and never copy a live WAL/SHM set as a portable archive. Future export must first quiesce the project through the owning app's project lock, use SQLite's supported backup/snapshot mechanism, close the snapshot, run integrity checks, and write a closed package. The existing app has no approved closed-backup exporter yet.

## Import policy for a future direct-edition backup

Import is an explicit user operation, separate from scanning, project opening, authorization, and any file operation.

1. The direct edition creates an independent, closed backup package; it does not expose its live project DB to MAS.
2. In MAS, the user selects that package through a system panel. The read scope is held for validation and copying. The original package remains untouched.
3. The app copies the fixed package members into a new `0700` staging directory under the MAS container, with `0600` files. The preflight requires an explicit private staging root and fails closed if it is not a real owner-private directory. It rejects symlinks, unexpected files, cross-device package members, SQLite sidecars, unknown format/schema identities, source bundle mismatches, hash mismatches, failed SQLite integrity checks, and failed foreign-key checks. The prototype has an 8 GiB database limit and checks cancellation between bounded copy reads; a blocked OS read itself cannot be promised to stop immediately.
4. No migration is applied to the selected backup. Any future migration runs only on the staged copy; failure discards only that newly created stage and leaves the selected backup unchanged.
5. The imported project receives a new MAS project identity. Existing source paths remain descriptive database data, never authority. Every source must be reselected with the native panel; old bookmarks and direct-edition authorization are not imported or auto-resolved.
6. If any DB identity, schema, format, or completeness claim is absent or uncertain, reject import and request a newly created supported backup. Never infer schema from a filename or path.
7. A recovery journal/lock imported from a backup remains unresolved until the normal recovery process verifies it. Import or renewed authorization never clears it.

The source database currently does not reliably record an app build and complete schema identity. Therefore ordinary legacy DBs without a trusted export manifest are not importable under this design. A manifest is an interchange contract and a checksum is an integrity check, not a cryptographic signature or proof of source authenticity. A future exporter must produce it while holding the app's project ownership lock. Do not present the local preflight package as a complete import feature.

### Minimal implementation in this phase

`internal/masbackup` adds `ValidateClosedBackup`, a read-only preflight for the fixed two-file package `backup.json` + `governance.db`. It uses a rooted directory handle, rejects links and extra entries, refuses `-wal`, `-shm`, and rollback-journal companions, strictly decodes a path-free manifest, copies the DB into a caller-supplied private staging root, checks its SHA-256, SQLite `integrity_check`, foreign keys, required layout columns, and an exact `sqlite_master` schema fingerprint pinned to this source layout. Package members must be regular files on the selected backup volume (device identity is implemented for Darwin/Linux; unsupported targets fail closed). Copying is cancellation-aware between reads and capped at 8 GiB. A cleanup failure is returned as a rejection and clears the validation result. It does not open a project, run migrations, import bookmarks, write a project, delete/rewrite the source, or expose source paths in the result. The source caller must hold the native read lease for the duration.

The format/schema IDs in this experimental package are not yet emitted by the direct app, and the API is not wired to Wails. This validates fixture packages only; it does not claim that an existing user backup is supported or that a closed DB's provenance is authentic. The 8 GiB package database cap is an implementation limit, not a promise about final product import size. The copy loop honors cancellation between reads but cannot interrupt an OS-level read already blocked in the kernel. Tests inject a cleanup error after removing the fixture stage; they do not simulate an OS that leaves an undeletable residual, which remains an operational failure. The prototype does not prove App Sandbox container placement, POSIX ownership, or ACL behavior at runtime; these remain pre-integration requirements. The manifest and sidecar checks cannot prove the absence of every possible concurrent writer, especially a writer using SQLite DELETE journaling. A future exporter must provide a lock-backed, clean-close contract and provenance that the importer can verify. Device-number checking is appropriate for the macOS target but is not a universal same-mount proof on systems that permit bind mounts sharing a device number.

## Local disposable-data evidence

New tests create fresh local SQLite DBs under test temporary directories. No NAS mount, beta.11 container, historical failure DB, original project, or source-file write is used.

| Scenario | Evidence | Result |
|---|---|---|
| MAS project remains under injected app-support root | MAS-tagged Wails test; DB and metadata are regular owner-only files, external DB path rejected | PASS at unit-test layer |
| Current disposable DB package, digest, SQLite integrity, and FK checks | `internal/masbackup` fixture test | PASS |
| Old/other bundle ID or unsupported schema ID | Strict preflight rejection test | PASS at unit-test layer |
| `-wal`, `-shm`, or `-journal` present | Closed-package rejection test | PASS at unit-test layer |
| Digest mismatch or corrupt SQLite with updated digest | Rejection tests | PASS at unit-test layer |
| Unknown manifest field (including a bookmark field) | Strict decoder rejection test | PASS at unit-test layer |
| Database symlink | Rooted preflight rejection test | PASS at unit-test layer |
| Manifest/staging symlinks, non-private staging, unexpected directory entry | Focused rejection tests | PASS at unit-test layer |
| Full-call cancellation and cleanup failure | Injected validation tests; stage empty afterward and result cleared | PASS at unit-test layer |
| Original backup bytes after successful validation | Before/after equality assertion | PASS in fixture |
| Actual sandbox container location, macOS ACL/TCC, Finder/NSOpenPanel import UX | Not exercised in this phase | NOT RUN |
| Existing direct-edition backups or upgrade migrations | No exporter/manifest in product | BLOCKED / NOT RUN |
| Recovery Lock import and resolution under MAS runtime | No import integration and no independent Mac/SMB test environment | NOT RUN |
| MAS signing, provisioning, App Store package, upload | Explicitly out of scope | NOT RUN |

## Apple API and packaging inventory

This is a source inventory and risk review, not a final binary/privacy-manifest certification. Apple says App Sandbox is required for Mac App Store distribution; persistent security-scoped access requires scoped bookmarks and balanced start/stop calls. A Mac App Store profile associates the explicit App ID, distribution certificate, and enabled capabilities. Re-check these official pages before any future distribution work:

- [App Sandbox](https://developer.apple.com/documentation/security/app-sandbox)
- [Accessing files from the macOS App Sandbox](https://developer.apple.com/documentation/security/accessing-files-from-the-macos-app-sandbox)
- [Describing use of required reason APIs](https://developer.apple.com/documentation/bundleresources/describing-use-of-required-reason-api)
- [Privacy manifests](https://developer.apple.com/documentation/bundleresources/privacy-manifest-files)
- [Register an App ID](https://developer.apple.com/help/account/identifiers/register-an-app-id)
- [Create an App Store provisioning profile](https://developer.apple.com/help/account/provisioning-profiles/create-an-app-store-provisioning-profile)
- [Create distribution-signed macOS code](https://developer.apple.com/documentation/xcode/creating-distribution-signed-code-for-mac)

| Area | Current finding | Status / next action |
|---|---|---|
| Sandbox and source access | MAS entitlements declare app sandbox, app-scoped bookmarks, user-selected read-only files, and network client. No blanket source access or network server entitlement. | Design matches read-only scope; real App Review/profile validation NOT RUN. `network.client` necessity still needs entitlement subtraction and mounted-SMB testing. |
| Bookmark bridge | Objective-C bridge uses NSOpenPanel, scoped read-only bookmark data, start/stop access, no-UI/no-mount resolution and stale detection. Capability JSON remains private. | Source reviewed; real revoke/stale/SMB identity cases remain Phase 1B INCONCLUSIVE. |
| SQLite/container | WAL mode and one write connection; sidecars are secured with the main DB. `project.json`, recents and grant metadata are private container-local records. | Code/static evidence; real sandbox persistence/relaunch NOT RUN in Phase 1C. |
| Required reason: File Timestamp | The scanner and validation code call `os.Stat`/`Lstat` and use size/mtime; UI displays modification time. Apple lists `stat`, `lstat`, and related metadata APIs. | Candidate reasons to assess: user-selected sources (`3B52.1`); displaying modification dates (`DDA9.1`); container metadata (`C617.1`) where actually applicable. A manifest reason must match each true use and derived-data rules. |
| Required reason: Disk Space | Darwin source identification and native authorization currently call `statfs` to inspect mount flags/source identity, not to display free/total capacity. Apple lists `statfs` in Disk Space APIs. | **Compatibility blocker to resolve:** current intended use has no clearly matching listed reason. Do not add an unrelated reason. Evaluate replacing this call with a supported public API or obtain Apple guidance, then test identity and privacy behavior. |
| Required reason: User Defaults | Wails v2.13.0 Darwin code calls `NSUserDefaults standardUserDefaults` to set `NSAutomaticQuoteSubstitutionEnabled` for the app. | Candidate reason `CA92.1` only if binary/app-domain behavior confirms access is limited to this app. Recheck upstream Wails source and final binary. |
| Required reason: System Boot Time | Source search of app and Wails v2.13.0 did not identify `systemUptime`/`mach_absolute_time` use. | No reason declared based on current source search; transitive/binary audit remains open. |
| Other SDK/runtime APIs | Go, modernc SQLite, Wails/WebKit, native C/Objective-C and linked Apple frameworks require archive-level inventory; source `rg` is not exhaustive. Wails source also uses KVC `drawsBackground`. | Run current Xcode privacy scan on every final app/embedded SDK and review each warning; verify private inspector/devtools tags absent. NOT RUN. |
| Privacy manifest | No `PrivacyInfo.xcprivacy` exists in the desktop bundle source today. | Do not add guessed declarations. Complete required-reason/API and collected-data inventory first, then author and validate MAS-only manifest. |
| Network | `network.client` is enabled for mounted SMB; no network server or external upload was observed in these source paths. | A local processing code review is not proof of no network traffic. Keep external AI off and test actual destinations in a later isolated runtime. |
| Distribution | Proposed separate ID is `com.fnb.ndg.mas`, not registered. MAS signing/profile/package pipeline is design-only. | Future list: explicit App ID; matching App Store provisioning profile and Apple Distribution identity; sandbox capability/entitlements; independent MAS Wails config; signed app/package validation. No credentials, profiles, or upload used. |

The API reason strings above are candidates, not a completed declaration. Apple requires reasons to describe actual use accurately; a blanket manifest entry does not fix an unsupported purpose. The `statfs` mount-identification use is the most material compatibility concern found in this pass.

## Phase 1B isolated runtime backlog — separate record

Phase 1B remains INCONCLUSIVE. Its pending scenarios and stop conditions are tracked in [phase1b-pending-tests-2026-10-12.md](phase1b-pending-tests-2026-10-12.md). No attempt was made to attach to, signal, or stop QEMU PID 29693, close an existing App, unmount an SMB session, alter system privacy databases, or use the host's user/project container.

## Test commands and limits

Executed on this checkout:

```text
go test ./internal/masbackup ./internal/adapters/wails
go test -tags mas ./internal/adapters/wails -run '^TestMAS(ProjectDatabaseAndMetadataStayInInjectedContainer|DefaultConstructorAndPickerInjectionCannotBypass)$' -count=1
```

Both commands passed. The first is the ordinary host unit suite. The second is only a targeted MAS-tagged adapter test; running the full generic Wails test suite with `-tags mas` is not an intended gate because those ordinary tests assume the non-MAS write-capable constructor. No Wails GUI, true macOS container, SMB service, MAS signature, PrivacyInfo bundle scan, App Store upload, or App Review validation was run.

## Decision

Phase 1C has a small, non-integrated closed-backup preflight and tests, a container/import contract, and a source-level Apple API inventory. It is not an MAS runtime PASS. Keep MAS distribution NO-GO; retain Phase 1A/1B INCONCLUSIVE, independent test environment BLOCKED, and historical CarbonCore FAIL. Next: independent review of the backup preflight and a supported resolution for the `statfs` required-reason concern before a MAS-only GUI import is designed.
