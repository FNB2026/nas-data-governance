# Resume Fix validation — 2026-10-02

## Build and scope

Observed desktop: independent NDG Resume Fix, `dev`, About commit
`31d693857939`. Local validation database copied from the original project;
registered Storage ID `src_3913f03885fa`. NAS checks were read-only.
This is development validation, not acceptance of the immutable beta.4 artifact.
No original project, source data, tag or draft Release was changed.

## Natural completion

Job `job-6027ae1dde4b4eb87a5316b2ef255434` resumed on
2026-09-30 18:25:21 CST and completed on 2026-10-02 08:33:09 CST.
UI and read-only SQLite agree on `COMPLETED / FINALIZING`, discovered and
processed `1,648,814`, failed `3`.

- Checkpoint 1 is `completed`, scanned_count `1,308,951`, updated_at
  `2026-10-02T00:33:09.249256Z`. Its count was not artificially advanced.
- All `1,308,951` persisted records at or before the checkpoint boundary remain
  `active`. The boundary was compared privately and is omitted from this report.
- Final file status: active `2,957,765`, unavailable `26`, missing `0`.
- Resume entry is absent on the completed scan page.
- Duplicate Results loads `52,319` groups. One two-copy group was opened:
  content hash, file size, physical identity uncertainty, directory context,
  retention explanation and manual-review advice were visible.
- Path masking is enabled; paths, filenames and business anchors in the checked
  group are masked. External AI, telemetry and cloud upload display disabled.
- Operation audit contains no execution records. This check initiated no NAS
  write, governance, restore or purge operation.

## Error evidence and limits

The UI failed counter counts hash failures, not traversal errors. The historical
job persisted no summary identifying those three hash failures; their individual
causes cannot be reconstructed from its counters.

The 26 unavailable rows belong to three parent groups (1, 10 and 15 rows).
A bounded, read-only probe of one representative per group returned `ENOENT`
after refusing symlink traversal. This establishes current unreadability, not
the exact error at the original scan time, and does not prove every row was
independently probed. No missing status was inferred from these probes.

The scanner freezes its contiguous checkpoint after a traversal error. Therefore
growing scan counters with an unchanged checkpoint are not, alone, evidence of a
persistence defect. Completion here preserved the prefix and retained partial
coverage evidence as unavailable rather than bulk missing.

## Privacy finding and follow-up code

The historical job contains 136,354 structured events. One `job:created` event
contains the full local project database path in `project_id`; no raw path is
included in this report. Other inspected payloads contain aggregate fields.
No readable stdout/stderr log file was available, so runtime-log privacy is not
claimed as verified.

Follow-up source changes remove project identity from creation-event payloads
and forbid `project_id` at the event persistence sanitizer. Private project
identity remains in the job record for project queries. Existing event history
is preserved unchanged.

Scans with errors or partial coverage now persist one path-free summary warning:
traversal error count, quick/full hash failure counts, coverage state, missing
and unavailable counts. Summary persistence failure prevents an unqualified
successful job result. This is future observability; it does not retroactively
recover historical error causes. Targeted regression tests verify a private
database path never enters creation events and a synthetic hash failure remains
counted without exposing its source path or filename.

Follow-up verification: targeted events/jobs/app race tests, full
`go test -race -count=1 ./...`, `go vet ./...` and `git diff --check` passed.

The follow-up source is not deployed into the observed completed dev app.
Resume natural-completion and prefix-preservation checks passed. Full acceptance
remains open for historical failure classification, unreadable source entries
and runtime privacy verification. The beta.4 draft remains unpublished.
