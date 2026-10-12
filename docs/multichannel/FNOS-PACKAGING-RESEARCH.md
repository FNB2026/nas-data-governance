# G5 fnOS packaging research

Date: 2026-10-12. Source baseline:
`584aafeb4cf1bb68cbda3bde2d26698b05555640`. Work is independent of MAS and the
immutable macOS beta.11. No real NAS, mount, existing service, historical DB or
QEMU process was accessed. No FPK was built or installed.

| Evidence layer | State | Meaning |
|---|---|---|
| Official public tool download and help/template observation | PASS | fnpack 1.2.3; receipt with observed hash, no publisher signature assertion |
| Research structure and synthetic lifecycle/config tests | PASS | Offline only; not runtime/source permission acceptance |
| Pre-descent mount rejection | PASS (injected boundaries) | Device changes, same-device directory/file mounts; no actual mount was created |
| Actual mount/remount integration | NOT RUN | Local table metadata only; no unrelated mounts or NAS contents accessed |
| Accepted G4 image digest and receipt | BLOCKED | Empty identity; image cannot be packed |
| App Center lifecycle/SQLite drain/export/upgrade | NOT RUN | Hooks deliberately reject unsupported operations |
| Isolated fnOS device/UI/ACL/uninstall retention | BLOCKED | No dedicated administrator-authorized test namespace |
| Review/signing/submission/publication | NOT RUN | Platform terms and owner approval required |

## Verified framework

[Official fnpack](https://developer.fnnas.com/docs/cli/fnpack/) supports
`create <appname> --template docker`, boolean `--without-ui`, and
`build --directory <path>`. The live CLI agrees. No account is needed to fetch
its [macOS arm64 download](https://static2.fnnas.com/fnpack/fnpack-1.2.3-darwin-arm64).
We do not ship the downloaded binary; its official URL, observed size/hash and
ad-hoc signing limitation are recorded in the packaging receipt.

The generated template is not safe to ship unchanged: it contains an example
image, unresolved port/UI values, an implicit read-write data-share bind, empty
hooks, five `.DS_Store` entries and a status function that can return success
when no container_name was found. None of these template defaults are accepted
as NDG runtime evidence. Our original scripts use exact fixed names and sanitize
status output, and integration remains blocked.

The [current manifest spec](https://developer.fnnas.com/docs/core-concepts/manifest/)
uses `platform=x86|arm|all`, not obsolete `arch`, and `changelog`, not a historical
`change_log` example. Identity, support range, entry IDs, `service_port`,
`checkport`, `ctl_stop`, and directory authorization must match real operation.
No untested minimum OS or ARM support is declared by this research report.

[Resources](https://developer.fnnas.com/docs/core-concepts/resource/) define
`docker-project.projects[].name/path`, with path relative to `app/`.
[Privilege](https://developer.fnnas.com/docs/core-concepts/privilege/) defines
`defaults.run-as=package` for the package user. Docker container privileges are
independent; they are explicitly constrained in Compose.

[Runtime paths](https://developer.fnnas.com/docs/core-concepts/environment-variables/)
are `TRIM_APPDEST`, `TRIM_PKGETC`, `TRIM_PKGVAR`, `TRIM_PKGTMP`, `TRIM_UID/GID` and
`TRIM_DATA_ACCESSIBLE_PATHS`. The last is an application authorization list,
not sufficient proof of current-user access or safe unchanged mount identity.
DB state is not exposed as data-share and must never be within a scanned root.

[Framework hooks](https://developer.fnnas.com/docs/core-concepts/framework/) are
install/config/upgrade/uninstall init/callback plus main start/stop/status.
Status uses 0 running, 3 stopped and 1 error. App Center manages Docker start/stop
via the Docker resource; future NDG shutdown needs SIGTERM drain/checkpoint and
clean DB close, with device-observed ordering. Upgrade cannot claim success
before a supported consistent backup/export and schema check exist. Uninstall
must not silently remove application or source data; platform retention still
requires an isolated actual install/uninstall test.

## Offline checker boundary

The explicit package walker checks device and mount-table membership before
enumerating a child directory or opening configuration contents. Linux mountinfo
also detects same-device bind mounts and mounts on files; macOS uses 64-bit
`getmntinfo` with `MNT_NOWAIT` to read already-known mount targets only. Mount
paths and source names remain private and never enter checker errors. Current
local macOS arm64 metadata reads and deterministic boundary injection pass;
actual mounted-directory tests and x86_64 runtime are NOT RUN. Unsupported
platforms or unreadable/malformed tables reject.

Darwin layout/symbol selection was checked against the installed Apple SDK
`sys/mount.h` and `sys/cdefs.h` (64-bit statfs size 2168, mount-target offset 88;
arm64 plain symbol versus x86_64 INODE64 alias). This build-time Python checker
is not shipped inside the MAS App, and introduces no new MAS native API or
Privacy Manifest reason. It does not certify the Phase 1A/1B runtime boundary.

Root/ancestor ownership, POSIX modes, symlink rejection and observed identities
are checked first. ACL safety and stability for the complete check remain caller
responsibilities under an exclusive checkout lease: permissions alone do not
prove ownership, container identity or immunity to concurrent remount/replacement.
No hostile/concurrent mount-race certification is claimed.

## Access model decision still required

[Port entrances](https://developer.fnnas.com/docs/core-concepts/app-entry/) do not
reuse NAS login sessions. Administrator-visible icons alone are not API
authentication. The scaffold retains loopback-only binding, so remote GUI access
is intentionally unresolved. Do not solve that by exposing an anonymous LAN
listener.

The [unified gateway](https://developer.fnnas.com/docs/core-concepts/gateway-registration/)
supports Docker through `gatewayPrefix` and `gatewaySocket`, routes HTTP/WebSocket
to an application-target Unix Socket, and supplies `X-Trim-Userid`,
`X-Trim-Isadmin`, `X-Trim-Username`. The application still performs business
authorization. A future adapter must prove only the trusted local gateway can
assert these headers, constrain socket ownership, support its URL prefix and avoid
giving the container a read-write application asset directory merely to create
the socket. None of that has been implemented or certified here.

[Shared directory authorization](https://developer.fnnas.com/api/authorization/shared-access/)
can use administrator settings or `pickSharedFile` plus
`trim.file.getSharedAccessibleFolders` (system 1.2.0401/App 1.34.0 for these APIs).
Generated binds must be from verified authorization, not arbitrary wizard paths.
[ACL checks](https://developer.fnnas.com/api/authorization/file-acl/) use
`trim.file.checkUserACL`; all-false can mean no permission or nonexistence.
It must never be treated as missing by the scanner. Authorization changes,
cross-mount traversal and current-user result isolation require true device tests.
[Wizard values](https://developer.fnnas.com/docs/core-concepts/wizard/) become
environment variables and must be validated/quoted again in lifecycle code.

## Prerequisites and next safe work

1. G4 freezes an actually accepted OCI digest, source SHA, architecture, build/SBOM
   and test receipts. No invented digest or untrusted JSON PASS may enable G5.
2. Implement/review initializer, state ownership, authentication/proxy or gateway,
   directory authorization, health and SIGTERM contracts in an isolated worktree.
   Add a supported snapshot/export path before upgrade; do not use live DB copy.
3. Owner identifies an independent fnOS device/test namespace with OS/CPU,
   administrator, synthetic roots, state volume, port, container and App Center
   scope. Do not borrow production services/mounts or infer such permission.
4. Perform real install/start/stop/restart, pull failure, ACL changes, low-space,
   upgrade compatibility and uninstall/reinstall retention checks. Hash sources
   before/after, verify non-root/read-only container and persistent SQLite.
5. Ask the platform for current submission qualifications, FPK signing and Docker
   digest distribution requirements. The official [publish procedure](https://developer.fnnas.com/docs/quick-started/publish-application/)
   currently directs developers to its community contact/group and says future
   backend procedures supersede that. It does not document a signing command,
   automatic submission API, or guaranteed review timeline.
6. [Official local test guidance](https://developer.fnnas.com/docs/quick-started/test-application/)
   says manual installation is for local testing, not public distribution. Confirm
   self-install Beta policy before publishing FPK outside the App Center.

Do not change Phase 1A/1B INCONCLUSIVE, historical CarbonCore FAIL, MAS NO-GO,
beta.11 assets or any other channel's acceptance from these offline results.
Final submission/public release is a separate owner authorization.
