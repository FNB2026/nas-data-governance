# Disposable NAS Docker host smoke protocol

This protocol is **not an FPK installation, NAS source scan, SMB acceptance,
signed release or public deployment**. It consumes the already checked,
unsigned experimental amd64 image from source
`063d0ee980ae8115cc92c0c9df0c5c43e540f14b`, tree
`f56e54626b060bb05b0c66b21e473f754a63e201`, Actions run `38391959321`, attempt 1.
Core version `0.5.0-beta.11` with channel `web-experimental` does not identify
the immutable macOS beta.11 signed DMG.

Frozen identities:

| Content | SHA-256 |
|---|---|
| OCI archive | `6d59909f00e540d93d5fc60a36754f08692f4dcecd251aaa5843b6d1e9999336` |
| Config / Docker image ID | `17286831a665b3d8470db80063014cc8ab96815a12e2a1dbfa1c03f43f8eb7f0` |
| Unsigned evidence receipt | `63be102ba9339d3060643f60790b7cd282e95e2bce2a303e3440f3bd213f491a` |

## Authorization and default

Without `--execute-disposable`, the script emits a fixed protocol-only message:
no Docker calls, path reads, credentials or file creation. Host execution needs
the explicit flag, Linux x86_64, local root privileges and the approved private
artifact bundle. There is no SSH, credential handling, arbitrary image selector,
registry pull/push, image build, host package installation or FPK action.

```sh
python3 scripts/packaging/nas-disposable-smoke.py
# Only after independent review and explicit disposable host authorization:
python3 scripts/packaging/nas-disposable-smoke.py --execute-disposable \
  --bundle /approved/private/bundle
```

The path above is a placeholder. The operator retains the input bundle's stable
ownership/ACLs and an exclusive testing window; checks do not establish safety
against hostile administrators or concurrent daemon/remount replacement.
Unknown ownership or incomplete evidence blocks. No existing NAS source,
database, SMB mount or service configuration is a protocol input.

An operator may pass `--operator-receipt-fd N`, an exclusively owned inherited
writable FIFO descriptor >= 3. Default protocol mode never reads or writes it.
In execution mode the descriptor is checked before fixture creation. An
`initial` JSON line is sent after the fresh resource journal is durable and
before any Docker mutation; `final` follows durable result persistence. Each
line uses one `os.write`, bounded by the descriptor's actual PIPE_BUF and 4096
bytes. The shared fields are `phase`, `pid`, `root`, `device`, `inode`, `nonce`;
final adds `status`, `test_status`, `receipt_sha256`. The root identity must stay
unchanged. A preflight final can have null root/identity/nonce/receipt hash.

The caller must continuously drain a private pipe, bind PID to its child,
validate root device/inode/ownership and the actual receipt SHA, and retain the
initial owned journal on timeout. It must never discover evidence by filename
prefix or scan other private directories. Invalid pipe or failed initial
handoff prevents daemon mutations. A final handoff failure returns BLOCKED and
preserves the already fsynced test receipt without rewriting it: a test PASS is
not proof of successful operator evidence transfer. Private paths travel only
through this opt-in pipe, not stdout/stderr. No secret or source filename is
included. This is an operator evidence protocol, not a product API or permission
grant. An exclusive operator window is still required.

## Content and resource boundaries

The existing OCI verifier checks external frozen source/tree/amd64 expectations,
receipt/files, blob sizes/digests, diff IDs, allowed image contents and scan
evidence. The converter then reads one bounded, no-follow, single-link archive
into memory and checks its frozen SHA again. It constructs a Docker-load archive
from unchanged config bytes and validated uncompressed layer bytes. It never
extracts an archive pathname onto the host. The converter's nonce tag changes
the load manifest only, not the config digest or original artifact.

An already present config ID is reused read-only: no load, tag or image removal.
A new image is loaded only under a fresh random tag. Its actual ID, architecture,
source/tree labels and rootfs diff IDs must match the frozen evidence. Cleanup
may remove only this new nonce reference, after checking no other tags, registry
digests or containers use it. Uncertain ownership retains the image.

Before creating any fixture, the script fixes its parent to `/tmp`, ignoring
TMPDIR/TEMP/TMP. It checks root ownership, sticky-directory protection, trusted
ancestry and current mount/device identity. Network, unknown or ambiguous mount
types reject; only tmpfs/ext4/xfs/btrfs/overlay/zfs are accepted. It never adjusts
mounts or falls back to a caller-supplied location. The script creates a fresh
0700 temporary project and private evidence directory,
seven artificial source files (three identical-content pairs and a singleton),
state and config. It requires at least 1 GiB MemAvailable and 256 MiB temporary
free space; these are checks at that moment, not resource reservations. Only
these disposable fixture directories/files are assigned to container UID 10001.
Source and config binds are read-only; only this fresh state is writable.

Each container has 384 MiB, one CPU, 128 PIDs, no capabilities, no-new-privileges,
read-only rootfs and a bounded noexec tmpfs. One newly created internal bridge
network has an opaque nonce label. A random kernel-selected unused loopback port
is prechecked; a later competing bind causes safe startup failure, not a retry
on an external interface. Actual published IP/port, network ID, mounts and
resource limits are checked. A failed internal-network loopback combination is
BLOCKED; the script does not weaken isolation to pass.

Network/container mutations use only IDs returned by this invocation's create
calls, rechecked against the nonce label and expected identity before operations.
An fsynced private resource journal retains known IDs. There is no prefix-based
discovery/removal, prune, other-service stop/restart or existing image/tag change.
An uncertain create response may leave a resource for manual review; it never
authorizes guessing an ID for cleanup. Private fixture/state/evidence files stay
in place; there is no recursive data cleanup or purge.

The Docker daemon's own storage has an independent check: read-only info returns
only its ID, storage driver and data directory, never existing container env or
logs. This host protocol accepts overlay2 only, checks the canonical root-owned
protected directory on an allowed local filesystem, and requires at least 1 GiB
free there. Driver/daemon/directory identity and free space are rechecked before
each daemon mutation, including cleanup; a failed check retains known resources
for manual review rather than acting on an unconfirmed daemon. The artifact is
fixed and small; this threshold is a conservative current-space guard, not a
reservation against other services' simultaneous writes. No daemon settings,
paths, ownership or production data are altered to pass. Actual container UID,
no-new-privileges and exact bounded noexec tmpfs must also match create options.

Mount identity is proved using a held Linux O_PATH|O_DIRECTORY|O_NOFOLLOW|
O_CLOEXEC descriptor, not by assuming a directory's stat device equals a
mountinfo major:minor field. Pre/post path lstat and held-FD fstat must agree on
device, inode, mode and owner. The FD's unique `mnt_id` must match the unique
nearest covering mountinfo entry, with allowed filesystem and unchanged mapping
before/after the check. Trusted ancestor device/inode and mount namespace
identity must also stay fixed; ancestry ownership/mode policy is checked again.
Missing
kernel handle/fdinfo, wrong or duplicate IDs, unknown/network filesystems,
unmount/remapping, changed path/FD or namespace all reject; the descriptor is
closed on success and refusal. Each daemon mutation repeats this check and
compares mount ID, filesystem, mapping fingerprint and namespace as well as the
original directory identity. An exclusive operator window remains required;
this does not certify protection against hostile root remounts after a check.

The Linux kernel's [proc documentation, sections 3.8 and 3.5](https://www.kernel.org/doc/html/latest/filesystems/proc.html#proc-pid-fdinfo-fd-information-about-opened-file)
defines FD mount IDs and their mountinfo correspondence. The stable synthetic
Btrfs subvolume case first reproduced the old device-equality refusal, then
passed with matching kernel FD identity. This is a proof replacement preserving
path-to-FD device checks, not an allowlist exemption. The earlier exact a4e10ae
device preflight remains BLOCKED and its private evidence is retained; it does
not become host scan PASS because of this compatibility change.

## Actual checks and evidence scope

The host scenario verifies authentication refusal, every create/approve/dry-run/
execute path under quarantine/restore/purge returning 403, frozen runtime version,
7 discovered/processed/scanned/active/quick, 6 full hashes, three pairs, completed checkpoint, failed/missing/
unavailable zero and unchanged source SHA. A second identical writer must fail
while the first holds its state; the same second container must run once the
first is stopped, separating owner refusal from an unrelated startup error.
Only those known containers are legally stopped/restarted. A restart must retain
the durable coverage. This is not cancel/SMB Resume or crash-window proof.

All received HTTP bodies and both test containers' stdout/stderr are checked
for fresh source-name marker, credential, fixture root and internal source path.
This is a bounded test-process privacy window, not all host processes, continuous
system logging or production load. Raw HTTP responses (including session tokens),
Docker diagnostics, source hash manifests and result/resource records remain
under the new private directory. No raw evidence or credentials go to stdout.

Existing containers are compared before/after by ID and stable state timestamps,
running/paused/restarting state, restart count and image ID. Their environment,
mounts and logs are never read. A change by another actor produces INCONCLUSIVE
and is not repaired; uncertain resource ownership produces BLOCKED. Only an
actual authorized run can update host acceptance. Offline mocks and byte
conversion cannot certify Docker or fnOS runtime.

The original OCI/receipt remain unsigned; repeated-build reproducibility and
registry provenance are not added. Historical privacy FAIL, MAS NO-GO, fnOS
package hard block and immutable beta.11 assets remain unchanged.

## Implementation verification (not host acceptance)

Thirty-nine offline tests exercise real synthetic archive/fixture bytes plus
mocked Docker/HTTP: default no side effects, explicit preflight refusal, frozen
source/content mismatch, existing destination preservation, links, tag refusal,
exact-ID cleanup, ownership/mount/limits/foreign-endpoint refusal, source hashing,
all twelve dangerous API denials and negative API/privacy/source-change cases.
Additional deterministic tests cover ignored production TMPDIR, remote/unknown/
ambiguous mounts, device/ancestry replacement, root/sticky/disk refusals, distinct
daemon capacity/identity and guard-before-every-mutation, plus actual security
option mismatches. Default/preflight failure does not claim a nonexistent private
receipt; raw paths never appear in rejection messages.
FIFO tests cover descriptor/type/direction refusal, atomic stable identity,
preflight null-root handoff, default no writes and initial-handoff failure before
any mutation. A completed response with inconsistent counts is a test FAIL.
The prior 31 tests remain, with seven kernel-metadata regressions for stable Btrfs
different devices/matching mount ID, wrong/missing/malformed fdinfo, duplicate/
changing mount mappings, changed FD/path/namespace, missing/open-failed kernel
handle, replaced ancestry and daemon fingerprint changes. These are deterministic mocks; real
operator metadata evidence is separate from a successful disposable host run.
The actual frozen amd64 OCI was also converted locally and its unchanged config
and layer identities checked, without calling Docker. Required local Go tests,
vet/build, full race, public/version/release checks passed. Successful fnOS host
scan/restart acceptance remains NOT RUN; attempted protocol blocks below are
separate from successful runtime evidence and require independent closure.

## Preserved operator attempts (2026-10-14)

The two original a4e10ae attempts remain **BLOCKED**, with private raw evidence
intact. The first wrapper did not receive an initial handoff: its underlying
failure cause, reached phase and resource closure are unknown, and later
Btrfs metadata must not be used to assign it a cause retroactively. The second
attempt explicitly returned `directory_mount_identity_changed` at the old
device-equality check. That refusal is not evidence that a mount actually
changed. Its classification is kept separate from later kernel-FD compatibility
tests and read-only metadata evidence. No original record is overwritten.

At exact harness `92fd581b6e9b20ed7e686dd1b3a61d041e448eb2`, the authorized
operator's 03:36:56–03:36:59 UTC attempt returned **BLOCKED** /
`validation_or_tool_failed`. Both inherited-pipe handoffs and child exit 1 were
observed. Private result SHA-256 is
`6bd918358d054af301b507f8ea5220d9f652f6cc4ba87dd361878e01ea4968c9`.
The owned container was running, but actual NetworkSettings.Ports was empty
despite the requested loopback publication. Direct indexing raised KeyError
before the HTTP/scan acceptance sequence. This is a protocol/environment block,
not evidence of a product functional FAIL or successful scan. Source/runtime
identity remains frozen experimental `063d0ee`, separate from harness `92fd581`
and the immutable signed beta.11 macOS artifact.

The protocol now checks network-settings/ports/binding types before indexing.
Missing or malformed actual ports explicitly return `loopback_binding_changed`;
only the exact requested 127.0.0.1 binding is accepted. The new deterministic
negative first reproduced the old KeyError, then passed with safe classification
and no HTTP calls. Internal bridge, loopback publication and all existing
isolation requirements remain strict; no daemon or network topology is changed
to make this attempt pass. No automatic host retry is authorized by this fix.
Raw own-container diagnostics and private source/status closure remain with the
operator. Independent closure for this exact third attempt verified 15 existing
containers (12 running, 3 stopped): 30 minimal stable-state JSON records form
15 equal before/after pairs. The seven disposable source files remain present;
their before/after hash-list bytes are identical, SHA-256
`11ea3bdcd0fc27acac4cf6ce9a004dba295714b955cfd4fc5549999a2a6365d6`.
Read-only exact-ID queries found this attempt's one container, one network and
one nonce image absent at that query time. This establishes bounded closure,
not cleanup of all earlier experiments or any production directory.

The independent reviewer checked read-only export revision `90ae1033` and the
three actual stderr responses. The original diagnostic rule checked only
`NoSuch` and incorrectly interpreted the network's `not found` response; that
false rule result is retained as a diagnostic limitation, not proof of a
remaining resource. Neither SSH/wrapper return 0 nor unchanged source bytes
changes the child exit 1 and **LIMITED BLOCKED** result. HTTP acceptance was not
executed. Real host scan/restart/single-writer/privacy acceptance remains NOT RUN;
no further device retry or topology weakening is authorized by this record.
