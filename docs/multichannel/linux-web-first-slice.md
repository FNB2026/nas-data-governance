# Linux/Web first read-only slice

This is an experimental **local-source** adapter, not a Docker/NAS release. It
shares ScanService, ScanJobRunner, JobManager, SQLiteStore, duplicate queries,
scanner traversal and quick/full fingerprint algorithms with desktop/CLI.

## Current capabilities and boundaries

- One private server state directory, one per-alias database. Browser requests
  cannot submit host paths. Aliases contain only lowercase ASCII, digits and `_`.
- Local allowlisted filesystems only. SMB/NFS/FUSE/unknown sources are rejected
  until separately verified mount/server identity and outage acceptance exist.
- Linux kernel with `openat2` required. Descriptor reads use BENEATH,
  NO_SYMLINKS and NO_XDEV; the original root binding is checked before reads.
  Child-directory entries carry descriptor-relative, eagerly read metadata.
  Hashing uses the same existing reader algorithms with before/after metadata
  and pathname-binding verification. Unknown source identity never inherits DB
  or checkpoint; source re-registration/migration is NOT implemented.
- Authenticated scan, cancel, legal Resume and aggregate duplicate results;
  source file names/paths/content hashes are excluded from HTTP results. Result
  lists are limited to 200 aggregate groups. Full directory-context and audit
  browsing, review/planning, backup/import and network Resume are NOT RUN or
  not implemented in this slice. No file write executor is reachable.
- Host and Origin exact match, no forwarded-header trust, no CORS, strict
  HttpOnly cookies, CSRF on every POST, idle expiry, login cooldown, bounded
  sessions and per-session requests. Credentials are private operator files,
  not query strings/build args/stdout. TLS is provided by an operator proxy
  with exact configured HTTPS Origin and Secure cookies; proxy acceptance is
  NOT RUN. Initial provisioning uses a strong random credential; account
  management/first-login password change is deferred, not advertised as done.
- The server lock excludes other **server instances** on that private state.
  Desktop/CLI do not participate in this new lock; never point them at server
  databases and never use external existing project DBs. The private state and
  configuration are operator-owned and must remain stable during service
  lifetime; 0700 alone does not establish ACL/ancestor trust. Other processes
  running as the same owner are outside this isolation boundary.
- Close cancels scans and waits for outstanding core I/O before releasing DB
  ownership. Native blocked reads are not forcibly interrupted; stop/recovery
  handling must not start a replacement writer while the process is alive.
- Quarantine, Restore, Purge, arbitrary recovery and plan execution are absent
  from the Backend interface and every authenticated direct API route rejects.
  Existing recovery journals are checked before scan, never automatically
  cleared. First use requires a new server state, not a historical project.

## Local setup

Use only disposable source files during this experimental stage. Create a new
private state directory and private configuration directory outside sources,
owned by the service UID. Both directory mode 0700 and file mode 0600 are required.
Generate a fresh credential locally (`openssl rand -hex 32 > credential` after
`umask 077`); this command writes only the named credential file and never prints
it. Copy `packaging/docker/server.example.json`, adjust approved alias mappings
and exact origin, save as `server.json` mode 0600. Never commit either file.

Build the separate React entry using the existing frontend dependency lock:

```sh
cd cmd/ndg-desktop/frontend
npm ci
npx tsc
npx vite build --config vite.web.config.ts
cp ../../../internal/apihttp/webdist/web.html ../../../internal/apihttp/webdist/index.html
cd ../../..
CGO_ENABLED=0 go build -trimpath -o bin/ndg-server ./cmd/ndg-server
```

The repository placeholder page explicitly says Web assets are not built.
Packaging builds the real React entry and reuses transport/error helpers,
DisabledNotice, LoadingState and shared styling. Wails builds remain separate.

`compose.yaml` binds only localhost on the host, a specifically named local
source read-only, dedicated state read-write, private config read-only and tmpfs.
It has no socket, privileged mode, host network or quarantine mount. Runtime is
non-root with read-only rootfs and all capabilities dropped. It is a local build
template, **not a published verified image digest**. Back up closed server state
independently; upgrade/downgrade and supported backup export are not certified.
Stop before changing images, preserve existing state, never automatically delete
state on uninstall. Removing an experiment container must not remove its volume.

## Evidence layers

- macOS host: HTTP contract/race tests and complete existing source gate.
- Linux amd64/arm64: cross-build, not native runtime proof.
- Dedicated Ubuntu CI: Linux filesystem/SQLite negative tests and disposable
  amd64 Docker build/start/restart/read-only/hash/cookie tests when its exact
  workflow succeeds. Docker is absent on the local host; no local container PASS.
- ARM64 container, NAS mounts, syscall-blocked Cancel, migration rollback,
  proxy/TLS, browser GUI and production deployment: NOT RUN.
- OCI publication, SBOM/provenance/vulnerability gate and fnOS packaging:
  BLOCKED pending a verified container and separate release authorization.

No historical beta.11 asset or MAS acceptance conclusion is changed.

The public-boundary gate prohibits checked-in `.env.*` files; the template is
therefore named `environment.example`. Copy it to a private `.env` locally and
set its only disposable source variable explicitly. No credentials are in it.
