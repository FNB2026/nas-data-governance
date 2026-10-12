# Direct-edition supported backup exporter: design only

Baseline `584aafeb4cf1bb68cbda3bde2d26698b05555640`. **NOT IMPLEMENTED / NOT RUN** as a supported product operation. This design does not enable MAS import, share live SQLite between channels, migrate bookmarks, clear Recovery Lock, or operate on user/NAS data. The Phase 1C disposable preflight remains insufficient for production import.

## Existing gaps

`internal/project/lock_unix.go` protects recent-project metadata writes. The Wails API mutex is per instance. `SQLiteStore.AcquireExecutionLock` excludes execution/recovery on a DB but does not exclude every scan, CLI, project open or exporter. None is a whole-project ownership protocol. Sidecar absence and permission 0700 are insufficient proofs of clean shutdown or trusted ownership. `SQLiteStore.Close` alone is not an exporter transaction or a verified checkpoint/close receipt.

## Proposed ownership and snapshot protocol

1. Create a new project-owner lease with no-follow, regular-file, owner/ACL and containing-directory checks, nonblocking OS lock and pinned directory/file identity. Every desktop/CLI writer, migration, project open, recovery and future server adapter must participate. A stale text PID must not override the OS lock. Until all paths participate, supported export is blocked.
2. Keep the lease from quiescing through snapshot validation and publication. Reject active scans/executions, held native I/O, other handles/process writers and unresolved recovery states for a migration-capable export. A future diagnostic export of blocked state must be separately identified and never automatically accepted for migration. Do not repair or reset historical journals just to make export eligible.
3. Drain jobs cooperatively, persist checkpoints/audit, retain source leases until in-flight I/O returns. Close auxiliary handles, verify no uncertain action; preserve original source DB state. Timeout/cancel means export refusal, not force-unlock.
4. Use the [SQLite Online Backup API](https://www.sqlite.org/backup.html) with a pinned source connection and a new private destination. The locked `modernc.org/sqlite v1.56.0` driver exposes `NewBackup`/`Step`/`Finish` through a raw connection; future implementation must test its exact connection lifecycle and completion/error contract. Check step and finish errors, bound busy retries, and retain the lease. Do not copy the main file of live WAL or assume its sidecars are optional.
5. Quiesce and close the destination, explicitly check checkpoint/close results where relevant; validate supported schema/fingerprint, `integrity_check`, `foreign_key_check`, plan/Journal invariants and absence of destination sidecars. A consistent SQLite snapshot is necessary but does not prove application execution-state consistency.
6. Hash and size the final destination only after close. Fsync DB and manifest, then private directory. Publish by an atomic, no-overwrite operation within a pinned parent; revalidate identity and fsync parent. Reject an existing output, links, changed ancestry or insufficient ownership. Read-only mode is defense in depth, not immutability against the owner.
7. On interruption, keep the original DB and any prior exports unchanged. Only remove a positively owned temporary output under its pinned root; cleanup/fsync failure is a failure with private diagnostic receipt, never success. Do not erase uncertain user output.

`VACUUM INTO` is an alternative [documented by SQLite](https://www.sqlite.org/lang_vacuum.html), with different layout and durability characteristics; choosing it requires exact-driver fault testing. Current design prefers the backup API. Neither API by itself establishes trusted provenance or the global owner lease.

## Manifest identity and trust

A future versioned manifest should bind exact source build/SHA/channel/exporter version, original project identity, supported schema fingerprint, snapshot digest/size, creation receipt and policy eligibility. It must not contain bookmark bytes, credentials or filesystem paths. The SQLite backup itself can contain private index paths: private archive, never repository/CI/public attachment. A new MAS project identity is created on future import; source roots require fresh user selection, old grant/checkpoint identities cannot silently authorize a new mount.

**SHA-256 is integrity, not provenance.** A bundle ID string, source version, code signature of the installed exporter, or a public key supplied inside the backup does not authenticate who created the snapshot. Do not embed a signing private key in an App. A per-user Keychain export key could sign canonical manifest bytes, but MAS trust enrollment/pairing, key ownership, revocation and replay controls are still unresolved. Independently verifying a user's enrolled exporter key is a candidate, not an approved trust protocol; no automatic trust-on-first-use import is proposed.

Decision required before implementation: define an independently authenticated key enrollment/provenance contract and import compatibility policy. A backup without that evidence is refused, even if all hashes match. Export generation may be developed separately, but must not be advertised as supported MAS migration until this trust boundary and private staging containment are closed.

## Required deterministic tests before integration

| Fault / conflict | Expected result |
|---|---|
| Second process/store/CLI writer; active scan or recovery | Refuse export without source mutation or lock override |
| WAL data, clean connection close, busy/error on backup step/finish | Snapshot includes committed data or fails; no partial success |
| Unknown source/exporter/schema, forged manifest/key, digest mismatch | Fail before migration or permanent registration |
| Sidecars appear, destination changes, ancestry swapped, symlink/mount crossing | Fail closed; original export/source unchanged |
| Disk full, fsync/publish failure, cancel/crash at each boundary | No published partial backup; cleanup only proven owned output |
| Duplicate export, output already exists, concurrent imports | No overwrite; explicit idempotent identity and ownership handling |
| Unresolved Journal and Recovery Lock | Preserved, not converted to clean/complete by backup validation |
| Bookmark copied / source identity differs | Fresh grant required; no old approval/checkpoint authority inherited |
| Restart after failure/success | Receipt, published bytes and application state agree |
| Public errors/logs | Enum/status only; raw paths, filenames, bookmarks and signing material private |

Next implementation should be a separate direct-edition PR with synthetic fault tests and independent review. Real sandbox import and signed-channel acceptance remain subsequent gates.
