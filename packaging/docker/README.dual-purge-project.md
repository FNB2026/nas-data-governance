# Prepare a Linux purge candidate project

The private Linux candidate reads the same SQLite project that NDG's
`nas-governance scan` and `nas-governance analyze` write. Use a dedicated
Linux filesystem within the candidate's Worker ownership boundary. The scanner,
analyzer, and Worker must see the same source mount at `/ndg-volume`. The
analyzer supplies the format records required by the execution policy.

Use an isolated project with a new state directory. Keep the candidate
stopped throughout project preparation. Finish both commands before starting
the Worker. For a rescan, stop the Worker first and check that no plan needs
recovery before opening the database for indexing again.

Build the ingestion image at the same source commit as the candidate.

```sh
docker build --target scanner \
  -f packaging/docker/Dockerfile.quarantine-candidate \
  -t ndg-project-ingest:local .
```

Mount the source read-only and mount the private state directory writable.
The Worker UID and GID are both 10002. Keep the stage under
`/ndg-volume/.ndg-purge-stage` private to the Worker. The scanner excludes it.

```sh
docker run --rm --network none --read-only --cap-drop ALL \
  --security-opt no-new-privileges \
  --mount "type=bind,src=$NDG_CANDIDATE_SOURCE,dst=/ndg-volume,readonly" \
  --mount "type=bind,src=$NDG_CANDIDATE_STATE,dst=/state" \
  ndg-project-ingest:local scan --root /ndg-volume --storage candidate-volume \
  --db /state/project.db --out /state/index.jsonl

docker run --rm --network none --read-only --cap-drop ALL \
  --security-opt no-new-privileges \
  --mount "type=bind,src=$NDG_CANDIDATE_SOURCE,dst=/ndg-volume,readonly" \
  --mount "type=bind,src=$NDG_CANDIDATE_STATE,dst=/state" \
  ndg-project-ingest:local analyze --storage-id candidate-volume \
  --db /state/project.db --index /state/index.jsonl --out /state/formats.json
```

Inspect the scan and format summaries before starting the candidate Compose
stack. A clean scan registers the storage, completes its scan checkpoint, and
persists indexed identities and directory classifications. The Worker still
checks the live file and keeper under the configured mount before planning,
Dry Run, and execution. A file with missing format data, protected context,
unreliable identity, or no independently verified keeper is ineligible.

Existing projects with file records tied to another absolute path require a
scan through the candidate path. Preserve the previous project before any
migration. This private workflow is limited to isolated Linux deployment
validation.

## Project upgrade and rollback

Normal NDG project opens use Schema013 for new projects. Schema014 is
introduced only by starting the private Worker, after indexing and analysis
have stopped. The Worker takes the project owner lock, checks for unresolved
work, and creates a verified, synced `project.db.before-schema014` snapshot
before changing the database. The snapshot includes committed WAL data. A
pre-existing snapshot is preserved if migration needs to be retried.

Keep the snapshot private and back it up off the candidate volume. To roll back
the software, stop every process that can open this project. Preserve the entire
upgraded state directory for inspection. Copy the backup as `project.db` into a
new private state directory with no SQLite sidecars, verify SQLite
`PRAGMA quick_check`, and open that copy with the baseline client. Restoring
SQLite does not undo a file deletion. Review any actions since the snapshot
against the filesystem before resuming project operations.

## Operator reconciliation

If an execution reports `RECOVERY_REQUIRED`, keep the original source and
stage mounts attached. Use the original plan ID and digest to query Receipt.
If its state is `EXECUTING`, run Recover for that exact plan in the private
console or via `ndg-private-purge -action recover` with the TLS, token,
mode, plan ID and digest arguments. Recover never deletes source content.
It restores a provably staged precommit file or reconciles a verified disk
receipt into SQLite. A proven claim that never touched the filesystem is
closed as rolled back. A repeat request returns the same terminal result.

If recovery still refuses, keep the project locked and retain the private
database, stage records and logs for review. Do not remove the stage, restore
an older database over the only copy, or change the existing mounts to bypass
the check. A replacement mount can have a different kernel identity even when
its device and inode match.

The confined Analyzer reads only verified Linux descriptors and binds its
format record to the indexed size, identity, hash and discovery generation.
Run it after every rescan before authorizing direct purge. Unknown or legacy
unbound format records cannot authorize deletion.

On Linux the Analyzer takes the analysis root from the registered storage for
each selected file when using a project database. For JSON-only analysis,
provide the authorized source with `--confined-root`. The index does not set
the source boundary. The Worker also requires a kernel file identity handle
for a recoverable purge, checked before claiming the execution.
