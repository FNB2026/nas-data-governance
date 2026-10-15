# Darwin rooted source reader: proof review for #111

Date: 2026-10-15. Baseline main `e55484ee6c95a73290a2182b66b977ddf4d61dd5`.
Verdict: **BLOCKED — no production reader change justified by this research**.
This document is a reviewable technical proposal and disproof inventory, not an
independent approval, Mac positive-flow certificate or assertion that all public
macOS APIs are incapable. [Issue #111](https://github.com/FNB2026/nas-data-governance/issues/111)
remains open. B-2 may proceed under its independent isolation gate.

## Claim that must be established

For each accepted member hash, bind the registered source root, the current
member name, the opened regular file and its mount ancestry. During resolution,
no symlink (including an ancestor of the source root) and no nested mount may be
traversed. A nested mount on the same device, an uncertain identity or a mount on
a file is not an exception. Mounts *above* the explicitly registered root are
allowed; mounting the source root itself still requires an explicit trusted
source binding. Root replacement must be detected rather than silently adopting
the new directory. Holding the old root alone does not establish the current
registered path. Content, metadata and physical identity must be checked through
the held descriptor, and the complete binding must be revalidated before the
result is used. Commit needs a fresh check, CAS/protection/Recovery Lock and the
existing cancellation/lease lifetime rules.

Distinguish three requirements: (1) lookup cannot cross a forbidden boundary;
(2) the returned descriptor remains the same object while read; (3) the current
path still denotes that object when the decision is accepted. Post-open identity
comparisons may help (2)/(3), but cannot by themselves prove (1). Metadata
timestamps and before/after snapshots do not prove absence of an ABA change
between observations. Whether the required binding is a point-in-time check or
a continuous transaction guarantee must be explicit; do not quietly weaken it.

## Evidence provenance

Local platform: macOS 26.6.2 (25G83), Xcode 26.6 (17F113), arm64. SDK `sys/fcntl.h`
defines `O_NOFOLLOW_ANY`, `O_RESOLVE_BENEATH`, `AT_SYMLINK_NOFOLLOW_ANY` and
`AT_RESOLVE_BENEATH`; no public no-cross-mount open flag was found in this header.
The pinned Go x/sys does not export `O_RESOLVE_BENEATH`, so the disposable research
probe gets its value from the installed C header, not an invented Go constant.
An SDK declaration or this host's success does not prove availability on every
NDG deployment target (minimum macOS 13) or future kernel.

Reviewed Apple OSS XNU revision:
`f6217f891ac0bb64f3d375211650a4c1ff8ca1ea`. It is a pinned source specimen,
**not a verified source match to the running 25G83 kernel**. Conclusions about the
running system require native observations; general API guarantees require
supported documentation or Apple confirmation. Links below pin review targets.

## Candidate mechanisms and remaining obligations

| Mechanism | Established usefulness | Missing proof / disposition |
|---|---|---|
| Whole relative `openat(rootfd, rel, O_NOFOLLOW_ANY | O_RESOLVE_BENEATH | O_RDONLY | O_CLOEXEC)` | Current SDK and native probe reject leaf/intermediate symlinks, absolute and escaping `..` paths | No reviewed public guarantee of no mount crossing; do not enable freshness |
| Component-by-component `openat` + no-follow | Can hold each directory object and reject symlinks | A child moved outside the task root remains usable via its child FD. Per-child BENEATH is relative to that child, not the original root. Full relative lookup is a necessary improvement, not mount proof |
| `fstat`, `fstatfs`, `fgetattrlist` identity/volume fields | Describe an opened object's metadata/filesystem; can detect many substitutions or different-volume cases | `st_dev` is not a mount-instance ID. FSID/volume UUID equality has not been proved to exclude same-device stacked mounts, reuse or transition. Post-open sampling does not constrain prior traversal |
| `ATTR_DIR_MOUNTSTATUS`, `getattrlistbulk` | Can observe mount-point/trigger flags independently of device equality; stronger candidate than `st_dev` alone | Must validate returned-attribute support, root/mounted-on semantics and every intermediate component. Observation and later open have a race; no transaction/FD-opening transfer has been established |
| `getfsstat`/mount table + ancestry/path checks | Useful diagnostic snapshot, including possible mount points omitted by device comparison | No public atomic table-generation/lookup binding demonstrated. Mount/unmount ABA can occur between snapshots; path strings are not authority |
| Foundation volume/resource identifiers, bookmarks/security scopes | Access grants and volume observations; useful for source authorization | Not a documented rooted no-mount-crossing primitive. Bookmark permission does not certify server/share identity or root/member ancestry. Existing Required Reason research remains separate |
| FSEvents/kqueue/NSFileCoordinator or advisory locks | May detect changes or coordinate cooperative actors | Cannot assume all filesystem/mount actors participate, event delivery is atomic with open, or no event means no race. Not a proof substitute |
| Kernel `NOCROSSMOUNT` / mount IDs | The reviewed kernel distinguishes mount traversal from BENEATH/no-follow and has internal mount state | Kernel flags/KPIs must not be passed as undocumented user open flags or private syscall workarounds. Need a supported public route and exact semantics |
| Immutable APFS snapshot / independent VM | Potential separate constrained research environment | Does not automatically give the ordinary user's live registered source the required semantics. Snapshot creation/mounting, permissions, supported FS and commit freshness would need a separately reviewed design |

Apple sources supporting this separation:

- [vn_open_auth](https://github.com/apple-oss-distributions/xnu/blob/f6217f891ac0bb64f3d375211650a4c1ff8ca1ea/bsd/vfs/vfs_vnops.c#L550-L580)
  sets no-follow and BENEATH namei controls separately. Inference: these controls
  should not be equated with the separate no-cross-mount state.
- [lookup_traverse_mountpoints](https://github.com/apple-oss-distributions/xnu/blob/f6217f891ac0bb64f3d375211650a4c1ff8ca1ea/bsd/vfs/vfs_lookup.c#L1792-L1845)
  checks `NOCROSSMOUNT` separately before processing mounted vnodes. This source
  is rationale for further investigation, not a supported userspace API promise.
- [attr_pack_dir](https://github.com/apple-oss-distributions/xnu/blob/f6217f891ac0bb64f3d375211650a4c1ff8ca1ea/bsd/vfs/vfs_attrlist.c#L2132-L2162)
  reports mount state for both root and mounted-on vnodes; the
  [bulk fallback](https://github.com/apple-oss-distributions/xnu/blob/f6217f891ac0bb64f3d375211650a4c1ff8ca1ea/bsd/vfs/vfs_attrlist.c#L4051-L4063)
  uses an internal no-cross-mount lookup. Neither yields a public file-content FD
  atomically tied to the reported no-mount observation.
- [Apple statfs manual](https://developer.apple.com/library/archive/documentation/System/Conceptual/ManPages_iPhoneOS/man2/statfs.2.html)
  documents filesystem information; it does not promise a no-crossing open.
- [Existing Foundation / Required Reason review](../mas/required-reason-volume-identity-2026-10-12.md)
  retains its own identity and distribution questions. Ordinary desktop research
  here grants no MAS permission or reason declaration.

## Fresh real primitive experiments

`go test -count=1 -v ./scripts/research/darwin-root-probe` and
`go run ./scripts/research/darwin-root-probe` create only new disposable local
fixtures. No source arguments, existing database, injected production reader or
preview-token generation exists. The positive case reads known synthetic bytes;
negative cases issue real Darwin syscalls and real symlink/rename operations.
Every diagnostic label is fixed and omits private paths.

Observed on this host:

| Experiment | Result | What it establishes |
|---|---|---|
| Whole-root ordinary read | PASS | These native primitives work for this simple current-host case |
| Source-root, root-ancestor, member-leaf and intermediate symlink | All rejected | Real no-follow negatives, not a mock |
| Escaping `..` and absolute relative-open input | Both rejected | Real current-host BENEATH negatives |
| Hold child FD, move child out of root, open member via child FD | Open still succeeds | Counterexample to treating a chain of held child FDs as proof of current root ancestry |
| Same move, re-open complete member path via original root FD | Rejected | Complete relative lookup detects this particular moved-name case |
| Replace root pathname while holding original root FD | Old FD still opens original object; path identity differs | Holding a root is object stability, not current pathname binding |
| Restore original root after transient replacement | Before/after identities equal | Endpoint sampling cannot establish no intervening replacement; this is not a claim that pinned-FD confinement itself failed |
| Separate read-only disposable HFS+ disk image nested below a fresh root, whole-root open using both flags | Nested file opened and known bytes read; device and FSID differ | Real counterexample to treating these flags as no-cross-mount; not a same-device experiment |
| `fgetattrlist` on that opened mount root | Mount-point flag observed | Promising mount observation; does not atomically bind each component and complete member lookup |
| Same-device / stacked / file mount, mount race, unsupported identity | NOT RUN | No mount-boundary PASS follows from other experiments |

These are controlled deterministic interleavings. They are real filesystem
observations, but not an exhaustive concurrent stress proof, kernel-version
matrix or real source-verification positive test. Production Darwin still
refuses before store/lease/source content access and issues no new token.

The explicit mount experiment is `python3 scripts/research/darwin-mount-probe.py`.
It accepts no paths, creates its own image and known synthetic seed, mounts only
at its own new temporary root and detaches that exact mountpoint without force.
The fixture is removed only after successful detach; failure retains it. It is
not part of ordinary CI and never touches existing mounts. The observed result
was `whole_root_flags_crossed_nested_mount=true`, `known_bytes_read=true`,
`different_device_observed=true`, `different_fsid_observed=true`,
`mountstatus_root_flag_observed=true`, `same_device_nested_mount_tested=false`.

A follow-up candidate worth review is checking each newly opened directory FD's
mount-root/trigger status before descending, combined with a whole-root BENEATH
lookup and final binding checks. It is **not dismissed as impossible** here.
Its proof still must connect those separate observations to the exact complete
lookup under rename and mount/unmount races, define forced-unmount behavior and
returned-attribute support, and demonstrate same-device and file mounts. Merely
adding more checks to the component chain does not establish that connection.

## Required adversarial matrix before any minimal reader implementation

1. Positive: disposable regular files, nested ordinary directories, known SHA-256,
   supported local FS and root grant; no project/index/audit corruption or source
   mutation. Include read budgets and cancellation without early lease release.
2. Boundary: symlink at root ancestor/root/intermediate/leaf, absolute and `..`
   escape, directory moved out during descent, nonregular FIFO/socket/device,
   file growth/identity/metadata/hash mismatch. Unknown identity must refuse.
3. Mount: different-device directory mount; same-device stacked/alias mount;
   nested mount followed by return to original filesystem; mount on a file;
   automount/trigger and APFS firmlink semantics. Record actual device equality
   where claiming the same-device case. No injected table is real-mount evidence.
4. Deterministic synchronization before/after each relevant lookup/attribute
   query/read/rebinding point: root rename/replacement, mount/unmount/remount,
   intermediate moves and ABA. A watcher is not the synchronization authority.
5. Supported OS/FS matrix and API unsupported/denied/error behavior; no fallback
   to unsafe path opens. No blanket inference from APFS to SMB/NFS/FUSE.
6. Integration: preview MATCH/issued token only with actual safe verification;
   commit fresh recheck, expired token, CAS conflict, protection and Recovery Lock;
   no false missing/unavailable reconciliation and no bytes changed. Actual
   current native/signed GUI comes after the backend proof, never by bypass.

## Concrete questions for Apple DTS (prepared, not submitted)

1. Which supported public macOS API atomically resolves a regular member beneath
   a held directory, rejects every symlink and **all nested mounts including
   same-device/stacked/file mounts**, and returns the content descriptor? Is there
   a documented userspace equivalent of no-cross-mount namei semantics distinct
   from `O_RESOLVE_BENEATH`? What minimum OS and filesystem support is guaranteed?
2. Can `ATTR_DIR_MOUNTSTATUS`/`getattrlistbulk` be used in a supported atomic
   observation-to-open protocol? How can userspace bind the mount instance and
   ancestry to the FD without a check/open or transient mount ABA window? What
   support-mask/unknown/trigger semantics must cause refusal?
3. Is a public per-mount instance identifier or monotonic topology generation
   available to ordinary apps? What are FSID/volume-identifier reuse rules, and
   do same-device mount aliases or firmlinks share those values?
4. What guarantees does whole-root BENEATH provide for concurrent ancestor moves
   and root replacement? How can a long-running read prove its current registered
   path remains bound through commit rather than only before/after snapshots?
5. Are these answers valid for APFS/HFS+/SMB/NFS/FUSE and App Sandbox? Which
   information queries require separately declared privacy reasons? Do not infer
   ordinary-desktop success or bookmark access establishes MAS suitability.

## Decision and next implementation boundary

Do not change `guardedContextMountProofAvailable()` or the Darwin adapter's
pre-read refusal. Keep the research executable outside `internal/fingerprint`.
First obtain independent review of the proof obligations and supported mechanism;
if no equivalent proof is available, retain #111 BLOCKED with the questions above.
Only then implement the smallest platform adapter and add the real matrix. An
independent reviewer can reproduce the native experiments and reject individual
claims without accepting a production change or adopting private evidence.
