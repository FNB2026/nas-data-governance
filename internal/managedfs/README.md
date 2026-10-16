# Private managed-object reader — unintegrated prototype

This package currently provides **read-only inspection only**. It is not imported
by the app, Web API or legacy executor. It cannot create a root, seal/copy an
object, acquire execution authority, quarantine, restore, purge, clear recovery
state or mint a governance preview token. `PurgeReleaseAllowed()` stays false.
The root/catalog initialization and durable Writer protocol are not implemented.

The caller must already possess a persisted root/object proof from a trusted
catalog and independently enforce catalog epoch/pending/recovery gates. A fresh
observation is not a replacement for approved stored evidence. Test fixtures
construct artificial proofs; they are not approval or production-store evidence.

The bounded deployment contract is an exclusive private namespace owned by a
dedicated non-root UID/GID, no untrusted same-UID writer or inherited writable
object FDs, trusted ancestors and trusted administrators without concurrent
privileged remounts. The library verifies owner/mode/ACL/link/FD metadata but
cannot infer that global ownership contract from POSIX modes alone. Whole-root
backup rebind, source deletion, network filesystems and Darwin proof are outside
this prototype. A later Writer must enforce its own privilege and store binding.

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
Until that exact source executes successfully, native kernel proof is NOT RUN.
Even PASS is private-reader evidence, not a Writer, SQLite, NAS, full Q/R/P or
release acceptance.
