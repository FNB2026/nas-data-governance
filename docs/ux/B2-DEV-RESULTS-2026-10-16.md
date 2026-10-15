# B-2 native keyboard and read-only copy follow-up

Result: bounded development observations; full B-2, signed RC acceptance and
Darwin positive verification remain incomplete. The [2026-10-15 results](B2-DEV-RESULTS-2026-10-15.md)
and their failures remain historical evidence.

## Exact development build and isolation

- Source `952bf5b68f678e43fc1bd269a56ff2e84f534599`, based on main
  `d5f9bca14825e72d752aefc5d5c3fcc52499bb98`. The only product change deduplicates
  identical reason codes in the workbench's presentation; capability and save
  guards are unchanged.
- Native About showed `dev-b2`, that exact source SHA, build
  `2026-10-15T18:38:13Z`, channel `dev`. Executable SHA-256
  `aa4c21f50b04692052539ec630dce937a91328e2dd23f50ec47ea3cdccd498e2`.
- Existing `script/build_and_run.sh --verify` produced an ad hoc signed native
  Wails App with `desktop,production,ndg_devprofile`. Tracked source patch was
  empty at build. Go build info records the tags but omits VCS fields; identity
  here comes from the clean build receipt and native About, not an invented VCS
  attestation or reproducibility claim. It is not a Developer ID/notarized RC.
- Go 1.26.9, Wails 2.13.0 and bundled Node 24.19.0. All 325 frontend tests,
  production frontend build and tagged Wails adapter tests passed.
- Before GUI work, verified a fresh 0700 temporary profile and exact 0600
  marker, live PID and zero open handles under the shared default NDG tree.
  Initial GUI had no adopted recent project. Only the previously generated
  45-group synthetic project was explicitly opened, using the actual GUI's
  **read-only** action. No lower-store fixture, injected reader or token was used.
  A different Bundle ID alone is not the isolation proof.

## Actual native observations

| Slice | Observation | Scope |
|---|---|---|
| B2-F01 duplicate copy | The actual read-only banner displays its explanation once | Fixed in the native development App |
| Browse | Existing synthetic project displays 45 groups / 3 pages, with private aliases and filename search disabled in privacy mode | Read-only browse only; no new scan in this follow-up |
| Tab / Shift+Tab | Focus moves from source selector to decision selector and back | Actual native focus observations |
| Full control navigation | Ordinary Tab skips some buttons/checkboxes in this WebKit configuration; Option+Tab reaches descending checkbox, current-page action and first group checkbox | No global keyboard preference changed; ordinary Tab coverage is not a full PASS |
| Space | Descending checkbox changed 1 → 0 → 1; first-group checkbox changed 0 → 1 and explicit selection became 1 group / 1 page | Actual state changes, not a synthetic key event |
| Refusal dialog | Native preview shows eligible 0, save disabled, and read-only refusal. Tab focuses Return; Shift+Tab stays on that sole enabled dialog action | Expected read-only refusal, not #111 positive verification |
| Escape | Closes the dialog with 1 selection retained | Trigger focus restoration was not certified |
| Conservation | After quitting this exact App, default NDG tree's 941 metadata entries unchanged; synthetic DB has 0 decisions, batches, decision-audit entries and operation plans | Metadata-only default-tree comparison, not a content audit |

Screenshot SHA-256:
`44acd592bd9cce2c9988f25d07212e717c665de4527469331503512b9730372b`.
Private screenshot and raw receipts stay outside public source.

Zoom, VoiceOver, whole-App memory, new performance sampling, signed upgrades,
nondeveloper comprehension and all positive source-bound governance remain NOT
RUN/BLOCKED as applicable. This follow-up does not reuse earlier timing samples
as measurements of this new binary. Public Mac beta.11 and Docker Preview.1
remain unchanged; no public asset or dangerous operation was released.
