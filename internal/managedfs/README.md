# Private managed objects — unintegrated execution prototype

This package is not imported by the app, Web API or legacy executor. Its Reader
is read-only; its separate explicit Initialize/Writer prototype creates a NEW
private root, copies bytes into managed objects and purges registered private
objects. It cannot authorize source removal, restore a source pathname, clear
recovery state or mint a governance preview token. `PurgeReleaseAllowed()` stays
false. No existing application/source/database is adopted or migrated.

The Reader caller must already possess a persisted root/object proof from a trusted
catalog and independently enforce catalog epoch/pending/recovery gates. A fresh
observation is not a replacement for approved stored evidence. Test fixtures
construct artificial proofs; they are not approval or production-store evidence.

The bounded deployment contract is an exclusive private namespace owned by a
dedicated non-root UID/GID, no untrusted same-UID writer or inherited writable
object FDs, trusted ancestors and trusted administrators without concurrent
privileged remounts. The library verifies owner/mode/ACL/link/FD metadata but
cannot infer that global ownership contract from POSIX modes alone. Whole-root
backup rebind, source deletion, network filesystems and Darwin proof are outside
this prototype. Deployment/account isolation remains a separate uncompleted gate.

`OpenReader` refuses root UID/GID, symbolic ancestors, unexpected physical root
identity, private root/object directory metadata, malformed canonical root
marker, missing statx mount evidence and unsupported filesystem profiles. The
configured root's own mount is explicit configuration; every descendant open
uses kernel BENEATH/NO_SYMLINKS/NO_XDEV. Equal `st_dev` is never sufficient.
Filesystem profile checks currently recognize ext-family, Btrfs and XFS statfs
values; each execution filesystem still needs separate real evidence and no
power-loss/durability conclusion follows from this reader.

The root and objects-directory descriptors are retained privately. Inspection
revalidates path/root/marker and directory mount binding before reading, matches
stored single-link regular-file metadata, hashes the complete bounded object,
then rechecks FD metadata, the current name and root binding. Observed stability
checks do not claim atomic immunity to excluded privileged writers. Errors are
fixed categories with no paths, names or contents. The public interface hides
concrete descriptors and mutex-bearing values; Inspect/Close are non-reentrant,
a busy Close refuses, and a closed interface cannot operate on reused FDs.

Local macOS tests establish only unsupported-platform refusal. Cross compilation
establishes ABI/build compatibility only. Linux fixture tests exercise actual
staleness/types/hardlinks/root replacement, interface copying and forced busy
Close/FD allocation. The separate GitHub-hosted driver first checks a valid
non-root match, then real same-device/same-inode directory/file/root bind mounts,
including mounts after the cached directory FD is open, and root replacement.
It checks foreign-UID write refusal and unchanged synthetic object bytes. It
uses its own mount namespace, no network/Docker/services, and refuses fnOS.
The earlier Reader at `0884e8bc27432d328de98b433afeea1aaa95e0a6` passed that
driver on both native architectures in Linux Web boundary run 38912435812,
with source/binary/build/kernel sidecars. This prototype revision changes the
shared file verifier and adds a Writer: earlier PASS does not certify it.

## Writer protocol and evidence limits

Initialize requires a private existing parent and exclusively creates a new
root, directories, lock, canonical marker/control and fixed SQLite catalog.
Root proof, physical catalog/lock/objects/staging identities, random root/store
IDs, epoch and pending state bind reopening. No adoption, repair or missing-DB
creation occurs. A persistent flock and non-reentrant instance gate cover every
method and Close. Interface copies share lifetime. Busy Close never releases FDs.

SQLite uses its ordinary Unix VFS within the exclusive namespace contract,
not a claimed rooted-FD VFS. Pre-existing main/WAL/SHM and namespace entries are
admitted through kernel-rooted checks before opening. The entire session holds
one pinned physical connection; operations never reacquire from the pool.
WAL/FULL and other fixed settings are read back. PERSIST_WAL is set and queried
before changing journal mode; Close may checkpoint/truncate retained files,
but must retain WAL/SHM names. Whole-root and stale-catalog restore are not an
approved backup-import protocol. [SQLite WAL](https://www.sqlite.org/wal.html)
and [file controls](https://www.sqlite.org/c3ref/c_fcntl_begin_atomic_write.html#sqlitefcntlpersistwal)
define those engine settings; they do not certify device power-loss behavior.

Writer admission additionally matches the root FD's current STATX_MNT_ID and
device to the unique record in bounded, verified procfs mountinfo, requiring
the exact ext4/btrfs/xfs type to agree with fstatfs magic. The ext-family magic
alone also identifies ext2/ext3 and is insufficient for Writer admission.
Missing, truncated, ambiguous or mismatched kernel metadata refuses; the
Reader's broader read-only ext-family profile is unchanged. The native proof
reports actual filesystem types exercised. This is type admission, not
per-filesystem or mount-option durability certification.

Every mutator first marks its session blocked and durably writes root Pending
at epoch N+1, then commits SQLite's matching intent before object mutation.
SealCopy uses fresh staging, bounded full hash, file/directory fsync and
NOREPLACE publication before catalog completion. Purge first verifies the stored
catalog proof/full bytes, renames only that private object into staging, syncs
both directories, verifies/records the staged identity (including changed
ctime), commits irreversible intent, rechecks, unlinks and syncs staging. A
completion transaction updates object/result state atomically. Only acknowledged
catalog completion followed by durable root Ready lets that session proceed.

Any uncertain attempt leaves the current session blocked. A new opener requires
matching Ready root/catalog epochs, no pending catalog operation, no unknown root
entries/control temporaries, empty staging and an exact registered-object name
inventory. It cannot infer completion merely from flock availability. Failure
before the first file creation can leave a provably clean pre-operation state;
failure after complete Ready publication can leave a complete state. Reopening
may accept those consistent endpoints, but the failed session never self-clears.

AuthorizationSHA256 records a caller-supplied decision digest only. It does not
check approval, retention, keeper/protection rules, Dry Run or high-risk user
confirmation; product integration must independently enforce all of them.
No public command/API exposes this primitive or its unexported test fault hook.

## Bootstrap partial roots

`Initialize` creates `ndg-bootstrap-partial` immediately after the private root
and removes it only after the whole protocol is durable. It is the sole reason
an incomplete root is identifiable: a root carrying it is refused by
`OpenReader`, and a completed root never carries it.

Every bootstrap step has a fault point, and every fault leaves a state this
package can positively refuse. A failure before the root exists leaves the
private parent untouched and reusable; a later failure leaves a marked partial
root that is never adopted, migrated, clobbered or self-cleared. Only the two
final removal steps may leave a structurally complete root.

These are protocol fault cases, not physical power-loss behavior. They do not
cover a privileged concurrent remount, a foreign-UID writer inside the private
namespace, other filesystems, or a real crash while the SQLite catalog is being
written. Partial-root removal stays a deliberate, separately authorized act:
no API or command deletes a bootstrap residual, so a failed bootstrap leaves
its path permanently unusable rather than silently reusable.

Writer tests cover fixed-store binding, stored child identity, same-inode stale
catalog rollback, real SQLite read-only transaction failure, every persistence
fault/acknowledgement boundary, cancellation, callback reentry and concurrent
Close/FD pressure. The new private GitHub-hosted driver covers process lease,
foreign-UID denial, same-inode bind mounts after cached Writer/SQLite FDs,
replacement-root Close keepers, and actual SIGKILL/fresh-process reopen around
root, SQLite commit, rename/unlink/fsync boundaries for SealCopy and Purge.
These Writer/native cases are NOT RUN for this revision until exact-source CI
evidence exists. Bootstrap crash/fault completeness, per-filesystem durability,
deployment isolation, backup import, source unlink/Restore, NAS, GUI, full Q/R/P
and new release acceptance remain uncompleted independent obligations.
