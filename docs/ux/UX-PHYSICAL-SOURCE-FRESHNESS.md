# Governance source freshness observation

Status: implementation slice in review; not a release or runtime acceptance.

## Contract

Preview and Commit re-read the physical bytes of only the selected members of
the selected duplicate groups. The selected subject, storage root, member paths,
and expected indexed hashes come from the open project's store; the frontend
cannot provide an arbitrary file path or root. The Wails composition root
obtains the existing source access lease before opening members. The store
returns metadata only and never opens source files.

Each member is opened through the rooted no-follow reader. For this freshness
entry point, the registered canonical root is reopened by walking every
absolute path component from `/` with directory descriptors and no-follow
opens. A symlink in an ancestor, at the root, or below the root is rejected;
the reader does not resolve a changed path and silently inherit access. A mount
at the registered root is allowed, while descendants on another device are
rejected. Root replacement, path replacement, and size, modification-time, or
reliable device/inode changes are checked before and after hashing, followed by
a reopen of the current path binding. A differing full SHA-256 blocks
recording. A current MATCH is only a point-in-time observation: bytes can
change after verification and before a later executor action. The executor
must retain its independent stale check; this observation is not execution
authority and does not eliminate TOCTOU.

The operation is bounded to at most 100 selected groups, 256 actual active
member rows across those groups, and 128 MiB of indexed member sizes. The
preflight rejects a group with more than 256 members and rejects invalid,
overflowing, or over-budget aggregate sizes before any member is opened. The
reader also caps each individual read at indexed size plus one byte, so a file
that grows during verification cannot cause an unbounded read. Context is
checked before opening and between read calls. Kernel I/O already blocked in
the operating system cannot be interrupted by Go; the access lease remains
held until that call returns. The API has 30-second Preview and 60-second
Commit deadlines, but they do not guarantee an immediate return from blocked
filesystem I/O.

Ordinary singleton files are not read or full-hashed by this feature. The
observation runs only for selected, currently eligible duplicate groups. A
budget refusal is `SOURCE_LIMIT_EXCEEDED`; users must narrow the selection or
record that it is unverified. A single duplicate group larger than the cap
cannot be made verifiable by splitting the selection. The interface must label
that result limited/unverified and must not imply that all source data was
checked.

Preview verification exceptions or blocked source observations must not create
a durable preview token. Commit rechecks the source and records a durable
blocked outcome for changed or unverified members; it does not write file
status, remove group members, or create a governance decision for those items.
Legacy `SaveGroupDecision` remains guarded by the existing service policy and
cannot bypass source verification. These are record-only governance intents;
they do not approve a plan, execute a file action, clear a Recovery Lock, or
change quarantine/restore/purge capabilities.

The UI and its submit handler permit a new records-only batch only when each
`ELIGIBLE` item has `source_freshness=MATCH`. An eligible item with missing,
unknown, or non-MATCH freshness blocks a new batch. Explicitly `BLOCKED` items
may remain in a partial batch with MATCH items. After an ambiguous response,
the UI may retry only the already-created request with its original token and
idempotency key; this is receipt reconciliation, not a newly authorized batch.

## Error and channel limits

Authorization failure is `AUTHORIZATION_REQUIRED`; an explicitly network
classified source lease can report `NETWORK_PAUSED`; a content or identity
mismatch is `SOURCE_CHANGED`; and the bounded-budget refusal is
`SOURCE_LIMIT_EXCEEDED`. For a non-MAS filesystem source whose mounted network
identity is not represented by the existing access lease, a missing path is
reported as `SOURCE_UNAVAILABLE`. This new observation does not infer SMB or
network identity from a path string or guess based on an unverified mount.
Existing scan/resume network classification is not modified by this slice.

An unavailable or unauthorized source is never reconciled as `missing` or
`unavailable` in the project index by this read-only check. No scan is started,
resumed, or changed as a side effect.

## Evidence boundary

Automated tests use disposable local files and SQLite projects. They cover
same-size/same-mtime content drift without reindexing, the check again at
Commit, durable blocking, the legacy decision entry point, symlink rejection,
rooted same-device enforcement in the reader, context cancellation, lease
release, and file/byte budget boundaries. This does not establish native macOS sandbox behavior, a real SMB
mount's network identity, signed-App GUI behavior, responsiveness of blocked
kernel I/O, or usefulness on a large NAS. Phase 1B and physical-source GUI
acceptance remain separate and must not be upgraded based on these tests.
