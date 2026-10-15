# B-2 ordinary Mac development acceptance plan

Prepared 2026-10-15 before measured GUI runs. Product baseline:
`e55484ee6c95a73290a2182b66b977ddf4d61dd5` (PR #110 merged).
This is a local development composition; it cannot certify a formal signed RC.

## Isolation gate

`ndg_devprofile` replaces only the ordinary desktop composition's project-service
support provider. The explicit environment value must name a newly created
0700 `ndg-b2-*` directory directly under a canonical system temporary directory,
with a private exact marker. Missing/invalid configuration aborts before Wails
startup. No HOME changes, global preference changes or existing profiles are
needed. The support provider rechecks root identity, marker and support directory;
there is no fallback. Tests cover rejection, project/registry placement, restart
and support-directory substitution. This is disposable test configuration, not
an adversarial filesystem security primitive or a new production profile API.

Before GUI actions: verify compiled tags/binary hash/ad hoc signature, live PID,
empty recent list, profile permissions and absence of shared NDG open handles.
Take a metadata-only fingerprint of the default NDG tree and compare after the
run. Paths and raw logs stay in ignored private `var/`, never in this ledger.
Use only newly generated local synthetic sources and projects. No lower-store
decision/token/Journal fabrication, existing project or external service is used.

## Ordered GUI cases

1. Create a project through the native GUI, select a newly generated 45-group
   source using the native picker, scan it normally. Check list/detail/history.
2. Select individual/current-page groups, change page/sort/filter, check explicit
   scope/counts and unavailable all-filter action; clear selection.
3. Preview a decision twice explicitly. Expect the actual platform-proof refusal,
   no token, no record action, unchanged source/decisions/batches and no automatic
   retry. Test keyboard focus, Space, Escape and browser zoom where supported.
4. Toggle path privacy and inspect list, detail, search and errors using synthetic
   canaries. Inspect bounded process logs for path/name/content leaks. A UI text
   check is not whole-OS or third-party framework privacy certification.
5. Close the new project and reopen its DB read-only in the actual GUI. Verify
   browsing and refused write/scan/preview controls. Do not introduce a lock or
   history to manufacture an exceptional success.
6. Generate a separate 10,000-group / 20,000-file source, create/scan through the
   same GUI. Do not use a seeded SQLite database. Check actual page counts and
   collect browsing/memory observations. Positive decisions remain blocked.

## Performance policy registered before measurement

Environment: macOS 26.6.2 / Xcode 26.6 / arm64, Wails 2.13.0,
ordinary local APFS disposable sources, 1280 x 800 initial window. Record actual
Go/Node versions, binary hash, source patch and untracked-source digest.

- SQL baseline remains separate. No retroactive threshold or inferred GUI PASS.
- GUI sample: one untimed warm-up followed by 20 alternating pagination actions,
  each bracketed by a native action and accessibility observation. Record every
  elapsed sample and nearest-rank P50/P95. These durations include automation and
  AX overhead: they are conservative round-trip observations, not isolated paint
  timings. The roadmap's 500 ms first-screen objective cannot be certified by
  this instrument; report that objective NOT MEASURED unless direct evidence
  becomes available.
- Development smoke criterion: no frozen window, wrong page, stale count or
  selection loss; action-to-observed-state P95 <= 2 seconds. This is an explicit
  engineering screening criterion, not an approved product performance waiver.
- Main-process RSS: sample once per second through browsing, report start/peak/end
  and post-idle values. Screening limits: peak <= 512 MiB and post-idle growth
  <= 128 MiB over pre-browse idle. WebKit subprocess attribution/whole-App memory
  remains separate; main RSS alone never closes the complete memory gate.
- CUA overhead, busy shared host, missing subprocess attribution or insufficient
  samples require a qualified observation or NOT MEASURED, never an invented PASS.

Historical signed beta.11, real nested mounts/SMB, legacy decisions, durable
positive receipts, recovery locks, upgrades and human comprehension retain their
own NOT RUN/BLOCKED states. No source writes or channel rollout are part of B-2.
