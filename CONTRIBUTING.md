# Contribution Guide

## Change Flow

1. Branch from the latest `main` branch.
2. Keep each pull request focused on one auditable concern.
3. Do not push feature or safety-boundary changes directly to `main`.
4. Complete the pull request safety checklist and wait for required checks.
5. Prefer squash merging so each pull request has one clear rollback point.

## Required Local Verification

Run the following before opening or updating a pull request:

```bash
make test
make vet
go test -race -count=1 ./...
make build
```

Go source files under `cmd/` and `internal/` must be formatted with `gofmt`. Changes to dependencies must leave `go.mod` and `go.sum` consistent after `go mod tidy`.

## Safety-Critical Changes

Changes involving scanning boundaries, filesystem writes, execution plans, quarantine, recovery, persistence, or path handling must preserve the rules in `AGENTS.md`.

At minimum, destructive or state-changing behavior must include tests for:

- source-root containment;
- symbolic-link rejection;
- mount-point boundaries where applicable;
- stale source detection;
- write verification;
- durable audit or execution-journal recording;
- rollback and interrupted-process recovery;
- sanitized error and log output.

No new write capability may be introduced inside scanning, analysis, grouping, relation detection, learning, or planning code.

## Test Data and Privacy

Use `t.TempDir()` and synthetic fixtures. Do not commit or paste:

- real NAS paths or filenames;
- production SQLite databases or JSONL indexes;
- private reports, audit logs, or scan checkpoints;
- user documents, media, metadata exports, credentials, or tokens.

Any diagnostic output intended to contain sensitive paths must remain explicitly private, access-controlled, and excluded from normal logs.

## Documentation

Update the relevant README section, ADR, schema, knowledge card, or roadmap entry when a change modifies architecture, data formats, safety guarantees, command behavior, or operational recovery procedures.
