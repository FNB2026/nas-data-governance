# Resume repair evidence — 2026-09-29

## Scope

Development branch only. The beta.4 tag, installed acceptance bundle, original project database and draft Release remain unchanged. Source operations are read-only. Development validation is not beta.4 release acceptance.

## Findings and changes

1. Resume enumerated completed directory subtrees and called entry metadata before the checkpoint comparison. Prune wholly completed subtrees before metadata I/O, preserving checkpoint ancestors and lexical DFS ordering (including `a.txt` versus `a/child`).
2. Resume preparation and checkpoint seek now have explicit stages and UI explanations; new jobs immediately refresh task history.
3. A network-classified quick-hash failure blocked checkpoint advancement but did not stop traversal. Stop submitting additional traversal work, drain in-flight workers, preserve the safe checkpoint and finalize `PAUSED_NETWORK`. No relaxed missing reconciliation or artificial checkpoint advancement.

## Evidence

- Scanner tests assert exact suffix coverage at each boundary, absent boundary paths, Unicode names, no metadata reads on skipped entries and no reads of pruned subtrees.
- Service tests cover resume stage transitions, preservation of the durable prefix, bounded work after quick-hash source loss and successful retry with the same checkpoint.
- Frontend: 203 tests, TypeScript and Vite passed. Full Go race suite passed for the prefix/stage changes; app/scanner race tests and go vet passed after the network-pause change.
- Wails desktop build passed for the prefix/stage changes with dev identity; generated models did not drift.
- Actual registered NAS root, original checkpoint 1 (1,308,951): read-only scanner probe reached the first post-checkpoint file in 9,787 ms, visiting 10 directories. This probe did not hash or write project data.
- Independent dev desktop, cloned local project: at 43 seconds UI showed discovered 724, processed 722, failed 1. Later UI showed discovered 879,298, processed 879,295, failed 1; checkpoint still 1,308,951. Both original and clone had 2,956,569 active rows and no missing/unavailable rows at the recorded sample.
- A separate bounded read-only quick-hash probe reproduced failure on post-checkpoint file ordinal 16: errno 2 (ENOENT), network-classified=true, permission=false. Source path and filename intentionally omitted.

## Remaining boundary

The missing/unreadable NAS directory entry is a real unresolved input error. This patch preserves it for recovery; it neither deletes it nor silently skips it. The currently running dev app predates the new fail-fast change and is preserved without interruption. Final release acceptance remains blocked until source readability is restored and a complete scan, reconciliation, duplicate review and privacy verification succeed. No claim of COMPLETED is made.
