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

The script creates a fresh 0700 temporary project and private evidence directory,
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

## Actual checks and evidence scope

The host scenario verifies authentication refusal, every create/approve/dry-run/
execute path under quarantine/restore/purge returning 403, frozen runtime version,
7 active/quick, 6 full hashes, three pairs, completed checkpoint, failed/missing/
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

Nineteen offline tests exercise real synthetic archive/fixture bytes plus
mocked Docker/HTTP: default no side effects, explicit preflight refusal, frozen
source/content mismatch, existing destination preservation, links, tag refusal,
exact-ID cleanup, ownership/mount/limits/foreign-endpoint refusal, source hashing,
all twelve dangerous API denials and negative API/privacy/source-change cases.
The actual frozen amd64 OCI was also converted locally and its unchanged config
and layer identities checked, without calling Docker. Required local Go tests,
vet/build, full race, public/version/release checks passed; this is distinct from
an authorized fnOS host run, which remains NOT RUN until a private actual receipt
exists and an independent reviewer verifies it.
