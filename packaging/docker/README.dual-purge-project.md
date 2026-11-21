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
