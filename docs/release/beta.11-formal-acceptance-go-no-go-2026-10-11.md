# beta.11 formal artifact acceptance and release decision — 2026-10-11

## Decision and evidence boundary

**Public Beta: NO-GO / BLOCKED. Release: Draft.** No public publication is authorized.
This report closes the affected Purge-disabled / normal Quarantine-Restore
regression following independent read-only evidence review. It does not waive a new-Mac
offline first installation, certify all historical Purge recovery states, or
replace the approved beta.10 Crash Recovery evidence contract.

## Frozen identity and build receipts

- Product RC: `791f19119542e69b62ee62dd358fed188000837e` (PR #83 merge).
- Annotated tag: `v0.5.0-beta.11`; tag object
  `395623c9ff0154d8ba7d0c63a109e1f8df2dccec` peels to that RC.
- PR #83 exact `80ef7846715acf99c3420252079e2e03aae093b2`:
  independent APPROVE; CI 38121655008 / Security 38121654943 SUCCESS.
- RC post-merge CI 38121849268 / Security 38121851128 SUCCESS.
- Formal Release workflow [38122101720](https://github.com/FNB2026/nas-data-governance/actions/runs/38122101720): SUCCESS.
  Environment approval authorized signed Draft production only. Tag push used
  existing administrator bypass of restricted ref creation; protections were
  not changed. This receipt does not imply public release approval.
- Installed official DMG App About: `0.5.0-beta.11`, full RC above,
  `2026-10-11T07:43:25Z`, channel `beta`; bundle `com.fnb.ndg`, build 11.
- Developer ID team `A2DYS82NLA`, hardened runtime and secure timestamp verified.
  App deep/strict codesign and Gatekeeper execute assessment PASS.
- DMG SHA-256/checksum, signature, notarized Gatekeeper open assessment and
  staple validation PASS. Installed App has no independent stapled ticket
  (stapler exit 65); the workflow staples the distribution DMG. Do not describe
  standalone App staple validation as PASS or infer fresh-Mac offline behavior.
- Test machine: Mac15,7 / arm64 / macOS 26.6.2, previously used for acceptance.

| Official asset | Bytes | SHA-256 |
|---|---:|---|
| NDG-0.5.0-beta.11-macos.dmg | 6364250 | aa3d9dada7bd1f670f70e4385e91e16a0927b5a8855656394d99a9e4f1fe5273 |
| DMG checksum file | 94 | 1f4ae3f04d95e59307c95d8e7ede669f0e187af6204e923490798e7b49227e06 |
| CycloneDX SBOM | 45567 | a545ffee13dc57f78123ca0b5634808b6d0b2ebd496347b1fcde1d3023b0c17d |
| SPDX SBOM | 80471 | 837c51e70f8212e3d2312b6922c699cc4a05f31510fbe70e3ffd5c30cf073778 |

Both published SBOMs inventory Go dependencies; neither includes source-map-js.
They cannot substantiate npm coverage. The npm fix is instead tied to the frozen
lockfile, fresh npm ci/audit, frontend build and exact Release workflow identity.
Existing beta.10 tag and all four asset hashes were checked unchanged.

## Fix and security receipts

- [PR #58](https://github.com/FNB2026/nas-data-governance/pull/58), merged
  `f0a7c107c3253746e4f9e955ee98dcfa1d5ac176`: source-map-js 1.2.1 to 1.2.2
  only (version/resolved/integrity); 208-package comparison found no other drift.
  Fresh full npm audit: zero vulnerabilities. Exact and post-merge CI/Security
  and independent review PASS; 258 frontend tests/build/release checks PASS.
  [GHSA-68fv-2mgg-jv7q](https://github.com/advisories/GHSA-68fv-2mgg-jv7q)
  affects indexed source-map offset handling below 1.2.2. Build dependency status
  alone was not treated as a risk exemption. Local Node 25.9/npm 11.12 emitted
  a jsdom engine warning; official CI supplies its configured supported runtime.
- [PR #82](https://github.com/FNB2026/nas-data-governance/pull/82), merged
  `f599cf764641ff970293d1ea46c40c64c8785f0b`: Service/Wails/CLI Purge Create,
  Approve, Dry Run and Execute deny before side effects using fixed release
  policy; no ordinary UI/environment/channel override. Exact independent review,
  CI/Security and post-merge gates PASS. Full Go race/vet/lint, 260 frontend
  tests/build, audit and version/public/release checks PASS (race output can
  include cached results; not represented as a forced fresh count=1 run).
- Historical lists/recovery remain available; ambiguity with both files present
  retains lock in regression tests. Existing historical stage-only recovery
  identity validation is not comprehensively certified by this change. Unknown
  historic Purge scenes require separate review, never automatic cleanup.

## Official GUI and filesystem regression

All files were artificial and disposable, with independent pre-operation byte
backups. No real NAS, million-file project, historical failure DB, product binary,
tag or asset was modified. GUI performed scan, suggestion save, human decision,
approval, Dry Run, Quarantine, independent Restore draft/approval, Dry Run and
Execute. SQLite was read only; no API injection or direct state edits.

Fixture preparation limitations were retained: the first project inherited a
protected source role and executed nothing. The second unknown-role group
generated REVIEW actions, reached VERIFIED but moved no bytes; this is **not**
Quarantine PASS. Resaving a new suggestion batch containing an existing plan
hit a UNIQUE constraint and was refused. A new independent project with an
explicit temporary-role pair provided the actual write regression. Reusing a
saved suggestion batch was not certified by this scoped gate.

| Observation | Actual result |
|---|---|
| New write project normal scan | COMPLETED / FINALIZING; discovered=7, processed=7; checkpoint completed/7 |
| Three duplicate groups | temporary pair KEEP+QUARANTINE; unknown pair REVIEW; protected pair not approved |
| Wrong lexical root alias | scope_validation_failed; no file movement; corrected to stored canonical source root |
| Successful Dry Run | executed=0/skipped=1/failed=0; all seven source bytes unchanged; quarantine empty |
| Actual Quarantine | executed=1/failed=0; one target absent, one quarantine file of 12000 bytes with matching SHA-256 |
| Durable state | operation plan VERIFIED; execution Journal QUARANTINE/done; six non-target files unchanged |
| Restore boundary mismatch | safe scope_validation_failed; no bytes altered; supplied Journal-consistent quarantine root |
| Restore approval/Dry Run/Execute | separate draft and approval; APPROVED Dry Run; natural RESTORED |
| Final bytes | all 7 source hashes/sizes equal baseline and independent backup; quarantine files=0 |
| Final durable state | restore plan RESTORED; restore Journal done; quarantine item RESTORED; active=7, missing/unavailable=0 |
| Audit | stale_check=1, KEEP=1, quarantine=1, restore_execution=1 |
| Capability/lock | GUI lock none, scan and Quarantine available, permanent Purge restricted; Purge plans=0 |
| Purge GUI | creation disabled before and after Restore; explicit release-policy explanation; history/recovery retained |

Final DB integrity_check: ok after normal App quit and absence of WAL/SHM.
Read-only immutable SQLite was used for the closed, fully checkpointed DB;
ordinary read-only open after quit could not open the WAL-mode database and was
not misreported as evidence. Final DB SHA-256:
`187b2cdd0dbd84511ea5e1067718d46c8a58e249acd9605c39d87ea01e0caea7`.

## Limited-window privacy

Collector began before App start: 2026-10-11 07:48:02.088510Z; App start
07:48:03.115052Z; stop 11:07:07.512308Z after normal quit. Window includes a
long idle/user pause, not three hours of sustained execution load. 11,869 health
samples, zero dead collector samples, maximum sampling gap 1.211237 seconds.
App PID stayed bound to the signed official binary. No restart or retrospective
log-show replacement was used to claim continuity.

stdout/stderr and collector stderr: zero bytes. Broad NDG/WebKit unified-log
predicate captured 77,320 lines (77,319 JSON records and one stream header) /
77,609,594 bytes. 76 path, filename and
business-anchor marker variants (plain, case, URL, JSON and base64 forms) had
zero hits in these captured logs, 11 job-event payloads and 4 audit details.
Unified raw file SHA-256:
`66ea6063074e4aa43bba0cf5eae5163eb668ec7ba6cc18f1fd3b06a20d42f2e8`.
Raw GUI, database, manifest, screenshots and logs remain local private evidence;
operational path-bearing DB fields are not ordinary runtime logs and are not
uploaded. Input widgets used to enter approved roots are not asserted to be
masked. This is limited-window evidence, not complete all-process coverage,
network-capture proof, long-load privacy or a whole-system zero-leak guarantee.

## Release Go / No-Go

| Gate | Status | Evidence / restriction |
|---|---|---|
| Immutable candidate / signing / distribution trust | PASS | Frozen identity and formal receipts above; standalone App staple limitation retained |
| source-map-js high advisory remediation | PASS | PR #58, frozen lock, fresh audit=0; SBOM npm gap stated |
| New Purge lifecycle denial / affected QR normal regression | PASS | PR #82 + actual official GUI/bytes/SQLite and independent review |
| Limited captured-window privacy | PASS | Independent reconstruction of marker set and zero hits; defined predicate/window only |
| Limited beta.10 Crash Recovery | PASS by approved combination | Formal B/C permanently INCONCLUSIVE; A unknown-output lock/audit limitation retained; not rerun here |
| New-Mac beta.11 offline first installation | BLOCKED | No unused test Mac; user deferred, not waived |
| Full-process / sustained-load privacy minimum | BLOCKED pending explicit support-scope decision | Current capture cannot establish it; do not automatically lower criteria |
| Manual backup support documentation | PASS documentation only | DB backup is not source backup; independent source copies required; no overwriting unresolved scenes or forced unlock |
| Historical unknown Purge recovery broad support | DEFERRED from certified scope | Preserve existing safe lock/history; separate evidence needed for uncertain stage identity |
| Public publication | BLOCKED / NOT AUTHORIZED | Release remains Draft; obtain final authorization only after gates close |

No Purge execution, new Crash injection, new feature or extra candidate was
introduced. The next concrete dependency is an unused Mac for official DMG
offline first installation and a decision on the remaining privacy support
scope. Neither source tests nor this normal GUI regression replaces those gates.

## Independent review receipt

The independent reader inspected original 7-file source/backup bytes, the closed
SQLite database and Journal/item/plan/root/hash/size associations, scan events,
checkpoint, four audit records, zero unresolved recovery states, retained earlier
fixtures, collector lifecycle and original logs. The 24-token / 76-variant set
was independently reconstructed, not accepted from summary counters. Read-only
DB hash remained unchanged; integrity_check was ok. GUI review used saved AX
transcripts; the reviewer did not replay GUI actions or independently witness
the earlier operation window. Primary GUI observations and local screenshots
remain separate evidence. Limited functional and captured-window privacy PASS;
the exclusions in this report remain binding.
