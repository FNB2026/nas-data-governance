# Local MAS Phase 0 reproduction

This is an isolated capability prototype, not a store release build. See [results and limits](phase0-2026-10-12.md).

Prerequisites: macOS Apple Silicon, Xcode/CLT, Go matching go.mod, Wails CLI 2.13.0, Node/npm. No portal/login/upload step.

1. `scripts/mas/build-probe.sh` builds the independent React assets and Wails production-mode bundle and ad hoc signs it with sandbox entitlements. Existing DMG plist/entitlements/scripts are not selected.
2. `python3 scripts/mas/fixture.py prepare --manifest /tmp/ndg-mas-new-private.json --port 2445`. Never reuse a historical manifest. The manifest contains private synthetic paths/hash baselines.
3. Launch `cmd/ndg-mas-probe/build/bin/NDG MAS Probe.app`. Select the manifest's `local` directory using the system Open panel. `NDG_PROBE_PICKER_START` can supply a test-only initial location if Computer Use keyboard focus fails; it never bypasses Open approval.
4. Click scan: expect files=7, quick=7, full=6, groups=3. Quit/relaunch and scan without selecting again. The saved bookmark is resolved with security scope; DB lives in this bundle's container.
5. For SMB use an independent Python environment with pinned `impacket==0.13.1` (the actual run reused a pre-existing private tooling environment). Start `fixture.py serve --manifest …`, then `fixture.py mount --manifest …`. It binds loopback only and serves read-only artificial files. Confirm chosen port is unused before start; never stop another service or eject another mount.
6. Select exactly the manifest's mount directory. Scan, normal quit/relaunch, scan. Unmount exactly this fixture with `fixture.py unmount`; scan must pause with authorization lock. Remount same fixture and scan must regain correct counts. An unavailable bookmark must not auto-mount through credentials or silently index an empty underlying mountpoint.
7. With the grant selected, click the unselected-canary boundary button: permission denial is expected. Missing canary or other errors are INCONCLUSIVE, not PASS. Invalid-bookmark unit tests exercise native resolution; a controlled private grant corruption negative must preserve a backup and keep the DB untouched.
8. `fixture.py verify --manifest …` checks all 14 source hashes/sets. Close App, copy closed synthetic container DBs/evidence to a private archive, unmount test fixture, stop only its service. Retain evidence.

Native tests (build frontend first): `go test -tags 'masprobe,production' ./cmd/ndg-mas-probe`; `go vet -tags 'masprobe,production' ./cmd/ndg-mas-probe`. Core regression: `go test ./internal/scanner ./internal/fingerprint ./internal/store`. Tests run outside App Sandbox; actual GUI observations supply OS authorization evidence.

The probe mutex serializes native scope and scans. Source grants are private 0600 data in a 0700 Foundation Application Support container. It refuses a resolved root relocation; prototype `locked` refers only to authorization. Production Journal/Recovery Lock integration, general migration and every write path are future work. Prototype has no executor/Purge API. Production MAS config is only a proposed read-only target until those changes are reviewed.

## Phase 1A product authorization layer

See [Phase 1A report](phase1a-2026-10-12.md) for the build-time MAS policy, actual product GUI evidence and remaining INCONCLUSIVE scenarios. Build the separate local sandbox product with `scripts/mas/build-phase1a.sh`. This is a dev/ad hoc capability validation App, not an App Store distribution or a replacement for the GitHub DMG.
