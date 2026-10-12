# MAS Required Reason API and volume identity audit

Status: **BLOCKED for a production replacement / MAS distribution**. This is research, not an App Review ruling. Source baseline: `584aafeb4cf1bb68cbda3bde2d26698b05555640` (PR #91 merged). Phase 1A/1B remain INCONCLUSIVE, their independent environment remains BLOCKED, and the historical strict CarbonCore log test remains FAIL. No manifest reason or product identity algorithm was changed.

## Phase 1C closeout

PR #91 was independently reviewed at `b368d2a1ebeb40545bfb5780a29a66fed815cc19`, including the comment/documentation delta from requested `34066905100f80f9915d1959494662327ad3e040`. Reviewer: independent Codex agent `phase1c_review`; patch SHA-256 `3df8c05b64272fd44e800afc340ff596944cfeeece3717c1ccb8a49063f74820`. This is an AI report, not an author GitHub APPROVE. Current rules require zero GitHub approvals and four checks; no bypass was used.

Merged by squash to the baseline above. [Main CI](https://github.com/FNB2026/nas-data-governance/actions/runs/38269775992) and [main Security](https://github.com/FNB2026/nas-data-governance/actions/runs/38269821359) both succeeded. On that exact merge commit, `make test vet build public-check release-check version-check` and `go test -race -count=1 ./...` passed. **Only the unused preflight prototype is CLOSED.** Supported export, trusted provenance, runtime staging containment and import are NOT RUN / BLOCKED. Mode 0700 alone proves neither ownership, ACL isolation nor MAS container membership. The caller must prove those properties and stable ancestry; the current path-based staging creation must not be integrated before rooting and runtime containment are implemented.

## First-party inventory

Reproduce with `rg -n 'statfs|fstatfs|statvfs|fstatvfs|getattrlist' internal cmd`. Inspect Darwin-selected dependencies using `go list -deps -tags 'mas,production' -json ./cmd/ndg-desktop`; declarations and comments are not linked runtime calls.

| Location | Call / purpose | Shipping scope |
|---|---|---|
| `internal/macaccess/bridge_darwin.m:27` | `statfs`, local/network flags and mount source plus root inode for grants | MAS product native bridge |
| Same file, lines 65, 73 | `statfs`, absent network volume / underlying local mountpoint checks | MAS bookmark resolution |
| `internal/scanner/identity_darwin.go:17` | `syscall.Statfs`, filesystem name and physical-identity reliability profile | Both Darwin product channels, including MAS |
| `cmd/ndg-mas-probe/bridge_darwin.m:36` | `statfs`, network classification | Separate Phase 0 probe, not product release |
| `internal/scanner/identity_linux.go` | `syscall.Statfs`, Linux filesystem profile | Linux only; excluded from Darwin |

No direct first-party calls to `fstatfs`, `statvfs`, `fstatvfs`, `getattrlist`, `fgetattrlist` or `getattrlistat` were found at baseline. Related `lstat`/Go Stat operations exist and must also enter the metadata inventory. The selected Darwin/arm64 dependency source includes syscall/x/sys wrappers and modernc libc `Xstatfs`/`Xfstatfs` implementations and an actual `Xgetattrlist` syscall wrapper. Most statvfs mentions are type comments, not calls. In locked SQLite v1.56.0's selected `lib/sqlite_g_0000000000000003.go`, `Xstatfs` is called by `_autolockIoFinderImpl` (line 13594) and proxy locking (43411); `Xfstatfs` is called in Unix VFS open (96127). These inspect filesystem flags/type, so removing first-party statfs alone does not remove the dependency concern. No SQLite `Xgetattrlist` invocation was found in its selected generated source. Presence is source-call evidence, not proof of invocation in every runtime operation. Wails Objective-C and system-framework internals require a separately identified final binary/framework audit. No formal MAS archive exists, so archive/transitive completeness remains NOT RUN.

## Apple rules checked on 2026-10-12

Limited link evidence: `go build -tags 'mas,production' -o <private-output> ./cmd/ndg-desktop` passed (linker warned about duplicate `-lobjc`). The unpackaged local development binary SHA-256 was `121599f76898c218a15af4f6e26459ab91b1d5298f2effa77e19554d40c84de0`. `nm -u` showed `_statfs` and `_fstatfs`; `go tool nm` showed `modernc.org/libc.Xstatfs`, `Xfstatfs` and NDG's scanner profile. These calls are not merely unselected module text in this local link. The binary was not launched, packaged, signed for MAS or uploaded; this is not a complete formal archive or runtime trace.

[Apple's API list](https://developer.apple.com/documentation/bundleresources/app-privacy-configuration/nsprivacyaccessedapitypes/nsprivacyaccessedapitype) includes the filesystem calls and capacity resource keys under Disk Space. Listed reasons concern capacity display, observable space-dependent writes/deletion, opt-in diagnostics, or health research. NDG's mount identity use fits none clearly; we did not declare `85F4.1`, `E174.1`, `7D9E.1` or `B728.1`. Replacing with `getattrlist` is not an automatic exemption. For file metadata, `C617.1` (private container) and `3B52.1` (explicit user grant) are candidates only after all call sites are classified.

[The general policy](https://developer.apple.com/documentation/bundleresources/describing-use-of-required-reason-api) names mobile/watch/TV platforms in its discussion, while manifest key availability includes macOS. That mismatch prevents us from asserting a proven macOS upload rejection or blanket exemption. Applicability to this macOS use is an explicit Apple consultation question. No inaccurate manifest has been created.

## Foundation candidate assessment

| Public resource key | Documented information | Missing security guarantee |
|---|---|---|
| [volumeIsLocalKey](https://developer.apple.com/documentation/foundation/urlresourcekey/volumeislocalkey) | Whether volume is local | Neither filesystem kind nor authenticated SMB identity |
| [volumeIdentifierKey](https://developer.apple.com/documentation/foundation/urlresourcekey/volumeidentifierkey) | Opaque same-volume equality, not persistent across system restart | Cannot serialize object description as durable identity |
| [volumeUUIDStringKey](https://developer.apple.com/documentation/foundation/urlresourcekey/volumeuuidstringkey) | Persistent UUID, possibly absent | No promised SMB endpoint/share uniqueness or spoof resistance |
| [volumeURLKey](https://developer.apple.com/documentation/foundation/urlresourcekey/volumeurlkey) | Volume root URL | Mount path is not server authority |
| [volumeURLForRemountingKey](https://developer.apple.com/documentation/foundation/urlresourcekey/volumeurlforremountingkey) | Remount URL, possibly absent | No promised stable canonical endpoint or authenticated service identity; may contain private information |

These keys can help classify a live volume. They do not jointly prove equivalence to the existing mount-source/root binding across network reconnect, replacement or restart. `isLocal` also cannot replace the scanner's filesystem-specific inode/hardlink reliability profile. An unknown profile must remain conservative, and unknown network identity must deny reuse, rather than silently inherit a grant or checkpoint.

### Actual local research

`scripts/mas/volume-resource-probe.m` is standalone, accepts no source arguments, creates only disposable directories, queries Foundation and outputs booleans/status only. It does not call `statfs`, query capacity, launch NDG, alter bookmarks/mounts/TCC, or enter the signed App. Source SHA-256 `f68f8a04903e7b04945c718f114cc85bd2be73515260b6be6b172da5bea965e5`; local binary SHA-256 `8a66c450d0a8ec60634fee31af0a6076276f2cd5f5a548458d8ac63f24ba0cfc`. Environment: macOS 26.6.2 (25G83), arm64, Xcode 26.6 (17F113). Compile with `clang -Wall -Wextra -Werror -Wno-unused-parameter -framework Foundation scripts/mas/volume-resource-probe.m -o <private-output>`; run without arguments.

Observed: metadata read/local flag/identifier/UUID/volume root present; remount URL absent; two different synthetic directories have equal volume identifiers, and rename retains equality; cleanup succeeded. Scope: **local metadata query PASS**, no scope/bookmark or SMB acceptance. A volume identifier alone demonstrably cannot distinguish two authorized roots on the same local volume.

| Required scenario | Result | Safe requirement for any later implementation |
|---|---|---|
| Local resource read / same-volume distinction | PASS, bounded local probe | Bind a root identity as well as volume; no opaque description serialization |
| SMB vs local classification | NOT RUN | Fresh scoped read must positively identify nonlocal volume; unknown denies reuse |
| SMB unmounted, underlying local directory remains | NOT RUN | Saved network grant + local/unknown now => SOURCE_UNAVAILABLE or authorization refusal; never reconcile missing |
| Same SMB remount / app restart | NOT RUN | Verify endpoint/share/root with supported stable semantics before checkpoint reuse |
| Different endpoint/share at same mount path | NOT RUN | Explicit reauthorization; no inherited approval/checkpoint from path equality |
| Missing/unstable UUID, nil remount URL, resource read fails | BLOCKED for automatic reuse | Preserve saved state and Recovery Lock; refuse unsafe inference |
| Grant identity format migration | NOT RUN | Version scheme; old/unknown schemes demand user reselection, no automatic rebinding |

## Questions ready for Apple DTS

No consultation was submitted. Use [Developer Technical Support](https://developer.apple.com/support/technical/) or [request a new reason](https://developer.apple.com/contact/request/privacy-manifest-reason/) with a synthetic example only:

1. For macOS MAS, does `statfs` restricted to `f_flags`, `f_fstypename` and `f_mntfromname`, never reading capacity, require a Disk Space reason? Is enforcement symbol-based or dependent on accessed fields/purpose? None of the existing capacity reasons describes this purpose.
2. Which public Foundation API can bind a user-selected SMB directory to authenticated server/share/root across restart/remount, and reject a different share at the same path? Does a remount URL or volume UUID provide any such documented identity guarantee?
3. How should an app distinguish an absent granted network volume from its local underlying mountpoint using `WithoutMounting` bookmark resolution, without probing ungranted arbitrary paths?
4. Must framework-internal Required Reason calls caused by public volume resource queries be declared by the app? How should linked but unused Go/libc wrappers be treated in a macOS archive? How should SQLite's Darwin VFS `fstatfs`/`statfs` type/locking checks be declared when they do not read capacity? Removing only NDG's call does not remove this dependency use.
5. What stable root/file identifier and filesystem-reliability API is supported for this use? Can absent/unstable network UUIDs be resolved safely without granting broad filesystem access?

**Decision:** do not replace the production bridge solely on this probe, do not add false reasons, and do not enable MAS release/import/write operations. Continue unrelated Linux work while waiting for isolated runtime evidence and Apple clarification.
