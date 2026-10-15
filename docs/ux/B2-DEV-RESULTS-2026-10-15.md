# B-2 isolated native development observations

Date: 2026-10-15. Result: **bounded development evidence obtained; Phase B and
RC-1 remain BLOCKED**. See the [pre-measurement plan](B2-DEV-TEST-PLAN.md) and
[Darwin proof review](DARWIN-ROOTED-READER-PROOF.md). No public release, channel
rollout, historical database/asset change or product source-file write was performed.

## Build and isolation identity

Observed native Wails App, not a browser mock or installed historical beta.11:

- Base main: `e55484ee6c95a73290a2182b66b977ddf4d61dd5`.
- App About: `dev-b2`, `e55484ee6c95a73290a2182b66b977ddf4d61dd5-dirty`,
  build `2026-10-15T08:12:56Z`, channel `dev`.
- Executable SHA-256:
  `c4fd53d57029928b3e8d3d9be97db310bf9fb65e14773b894a8fab20a756931a`.
- Bundle `com.fnb.ndg.b2.dev`, ad hoc signature verified locally, initial window
  1280 x 800 points (retina screenshot 2560 x 1600 pixels). This is not a Developer
  ID/notarized distribution candidate or independently reviewed composition.
- Go selected toolchain 1.26.9; Wails 2.13.0; Node 25.9.0 / npm 11.12.1; macOS
  26.6.2 (25G83), Xcode 26.6 (17F113), arm64. npm reported jsdom's Node engine
  mismatch; production TypeScript/Vite build succeeded. No frontend unit-test
  result on that unsupported Node version is claimed.
- Only the build-tagged project-service support provider changes composition.
  Native main, business UI, capabilities, readers, freshness and persistence
  guards use the baseline implementations. Ordinary/MAS builds exclude this
  provider. No HOME override, injected reader, fake token or lower-store seeding.
- Build-time tracked patch SHA-256:
  `cd930295d0060b6fdf2cd35316d1a1399e86e08fb7456cf1071bcfb83817bf86`;
  aggregate untracked-source digest:
  `c1e8019bc6bb7ce823718bd70c54a0eac2b080954460b91cb6a04ce2472ff32b`.
  Private receipts are retained. Later research/documentation/test-only additions
  do not turn this dirty development binary into a frozen signed RC.

Before meaningful GUI actions, the live PID, 0700 fresh profile and exact private
marker were verified. Only the marker existed before startup; startup's support
directory and empty first-use GUI contained no adopted recent project. After
GUI creation, both new databases and recent registry were under that profile.
At sampled startup/final handle checks, shared default NDG handles were zero.
The default NDG tree's **940 metadata entries were unchanged** after the tests;
only metadata was enumerated, no historical DB content was read. A bundle ID
change alone was not used as the data-isolation proof.

## Actual GUI results and case boundaries

| Case/slice | Observation | Bounded result |
|---|---|---|
| Native create/scan/browse | Native folder picker selected the new 90-file source; normal GUI scan produced 45 groups and 3 pages. List and two-member evidence showed directory roles/protection and honest physical-identity limits | PASS for new synthetic read/browse flow |
| T03 current-page selection | Individual selection, page-20 selection, clear-to-zero, and single-after-clear had correct counts | PASS in this dev App |
| T04 page/sort/filter scope | 20 selections remained on page 2; adding one showed 21 across 2 pages. Stable sort preserved the selection. A global name filter reduced matches to one while retaining 21 explicit subjects and warning that 21 came from another filter scope | PASS for these operations; not a positive decision/CAS result |
| T03/T04 cap on 10k dataset | 20 → 40 → 60 → 80 → 100 subjects across 5 pages. On the next unselected page all additional checkboxes were disabled | PASS for 100-group selection cap |
| T05 all-filter action | Explicit unavailable label and disabled action; no claimed server snapshot | PASS for contracted unavailability |
| T06/T16 evidence wording | Protected/raw-source members required manual review; scan-time physical identity was not presented as current proof, and reclaimable physical bytes were not certified | PASS for observed wording; mixed-role/real hardlink matrix remains NOT RUN |
| Actual Darwin refusal | Repeated explicit single previews and one 100-subject preview showed platform-proof absence. All 100 subjects refused; eligible count 0 and Save disabled. Escape returned with 100 selections intact | PASS for expected refusal, **not** Mac positive verification |
| T11 read-only reopen | Closed the newly created project and opened that exact DB read-only through the advanced GUI. Lists/evidence and selection remained usable; preview could not save; task-center writes disabled; new scan form replaced by a read-only refusal | PASS for this newly created project's read-only UI behavior |
| Empty durable history | Native history query reported 0 batches and explicitly did not imply file processing | PASS for empty-history behavior; historical decisions/positive receipts NOT RUN |
| Normal App restart | Quit and relaunched the same binary with the same explicit private profile. Recent list contained only the two new projects; reopening the 10k project showed 10,000 groups / 500 pages, zero selected subjects and zero persisted batches | PASS for this restart/readback; not a durable positive decision receipt |
| T14 policy surface | Settings showed permanent cleanup restricted; workbench stated this version does not support it | PASS for observed policy wording; no deletion invoked |
| T20 keyboard | Command+3 switched to governance; Escape closed normal and 100-item refusal dialogs. Native Go To Folder path selection worked after rebinding the sheet | PARTIAL; Tab/Shift+Tab/Space focus behavior not reliably certified by this CUA run, zoom/VoiceOver NOT RUN |
| T21 privacy | Privacy mode hid list/detail/recent-project source and filename canaries, disabled filename search and hid notes; refusal modal had no canaries | PASS for observed surfaces; whole-system privacy remains separate |
| T22 responsiveness/memory | Real normally scanned 10k groups, 20 timed pagination operations and two main-RSS windows below | PARTIAL, screening observations only |

Read-only UI displayed the sentence explaining read-only preview twice in the
same banner. This is a confirmed copy duplication to triage (B2-F01); the guard
and disabled save behavior were preserved. It is not silently repaired or
represented as polished error-copy acceptance here.

CUA intermittently rejected stale app focus and timed out; the connection was
refreshed/reset before continuing. A sampled native main thread was in the
normal AppKit event loop, which does not establish a product hang. Offscreen AX
activation and keyboard focus are distinct; unsuccessful Tab/Space attempts are
not promoted to an application defect without a reproducible focused test.

## Performance observations under the registered policy

The separate 20,000-file source contained 10,000 unique 128-byte payloads, each
present twice. It was selected, registered, scanned and queried through the
actual App. GUI showed 20,000 discovered/processed, completed, 10,000 groups,
500 pages. No direct SQLite fixture was used to obtain that list.

After one warm-up, 20 alternating page 1/2 actions each returned the correct
500-page count on the first AX observation. Elapsed milliseconds:

```text
641 626 632 628 646 635 644 623 633 649
633 630 643 644 629 625 627 638 618 628
```

Nearest-rank P50 **632 ms**, P95 **646 ms**, maximum **649 ms**. These include CUA
action and AX round trips; they are not direct backend/paint timings. They meet
the plan's 2-second development screening limit. The roadmap's 500-ms first-screen
goal is **NOT MEASURED**, not waived or certified by these values.

Main-process RSS (1 Hz target; actual timestamps retained):

| Window | Samples / elapsed | Start | Peak | End | Last-10 mean |
|---|---|---|---|---|---|
| Scan and following idle | 240 / 242.11 s | 85.06 MiB | 135.42 MiB | 84.33 MiB | 84.33 MiB |
| Pagination, 100-subject selection/refusal and idle | 100 / 100.23 s | 88.45 MiB | 92.11 MiB | 85.55 MiB | 85.57 MiB |

These meet the predeclared main-process screening limits (peak <=512 MiB,
post-idle growth <=128 MiB). WebKit subprocess attribution/whole-App footprint,
long-session leak behavior and nondeveloper usability are **NOT MEASURED**.
No entire performance/memory gate PASS or approved performance decision follows.

## Conservation and bounded log privacy

Both fixture digests still matched: **90/90** and **20,000/20,000** files. The
45-group source also matched the exact original generated bytes. Both DBs had
zero group decisions, governance batches, governance decision-audit entries and
operation plans. Refusal did not manufacture durable success.

One initial aggregate comparison used sorted filename order instead of the
generator's group/copy order and reported false. That collection error is
preserved; a corrected same-order comparison and exact bytes independently
matched. No fixture or expected digest was changed to obtain success.

stdout/stderr had zero hits for fixture roots, filenames/content canaries and DB
anchors. Unified-log query for the actual App PID, with info/debug, captured
**10,735 entries**, 16:12:59–16:51:00 Asia/Shanghai. Source/name/content/DB canaries
were also zero. One AppleEvents system `eventMessage` included a home path in
automation identity metadata; no fixture/default NDG data reference occurred.
This is explicitly not a claim that every OS/framework field is path-free or
that other processes/network traffic were fully audited. Raw logs remain private.

Selected private evidence SHA-256 values:

| Evidence | SHA-256 |
|---|---|
| Scan/idle RSS | `ed5878fd6ac0bc886b26156ce0a33aad48007a4bf76a14774592dadc76bf697e` |
| Browse RSS | `4bb9000d715c21676336d8cdf94b3ea79412f3bce7f379f54c1a9e124d20378b` |
| Conservation checks | `e5c67942c4532d22dff51f34eee292faf848a8350aff3751969833239d06d96c` |
| Pagination observations | `ea65945edd15220d07dcd9615f64c73515ad467cf9861e22a0789c20af9eaa6a` |
| Unified logs | `198b262b0024a79b7e76649e13def95498346d679bf388e63fc35b6f3363504c` |

## Local code verification

Normal `go test -count=1 ./...`, full `go test -race -count=1 ./...`, `go vet ./...`,
`go mod tidy -diff`, `make build` and `make public-check` passed. Separate tagged
race tests cover the isolated support provider and the Darwin pre-read/no-token
refusal. Native research race tests passed on this host. Targeted golangci-lint
2.12.2 reported zero issues for the research probe and tagged Wails adapter.
TypeScript/Vite production build passed; shell syntax and Python compilation
checks passed. macOS CI also exercises the tagged isolation tests; unavailable
SDK/kernel research flags yield explicit SKIP, never a weaker lookup or mount
proof. Remote checks and independent reviews belong to their final PR source
and are not inferred from these local results.

## Remaining gate and review boundary

These observations add dev-native evidence without depending on positive source
verification. They do not close full T01–T25, #111, B-3 or Phase B. Existing
legacy/upgrade, Recovery Lock, real SMB/mount-race, cross-process acceptance and
durable positive decisions/receipts retain their separate NOT RUN/BLOCKED states.
No Journal/history was fabricated to make those cases runnable. The 5–8 human
walkthrough and >=80% comprehension requirement remains NOT RUN.

Required next reviews: independently assess the Darwin obligations and candidate
protocol, independently review the development isolation composition, triage
B2-F01 and finish a reliable focused keyboard/zoom/VoiceOver test. Once a supported
reader has a real proof and adversarial matrix, freeze a new source/build and
perform the positive native/signed RC acceptance. No weaker reader or release
approval is substituted for those steps.
