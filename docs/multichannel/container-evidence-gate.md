# Experimental OCI evidence gate

Date: 2026-10-13. Implementation baseline:
`15328e0054c1c3e476dd756436ce52d6f3c50c1a`.
This is a G4/G6 evidence-retention experiment, **not a registry publication,
formal image candidate, signed provenance or fnOS readiness certificate**.

| Layer | State at implementation |
|---|---|
| Offline synthetic OCI integrity/refusal tests | PASS (15 tests) |
| Existing fnOS offline safety checks | PASS (24 tests; hard build block retained) |
| Native amd64/arm64 collector execution | NOT RUN; root dispatch after registration/review |
| Real SBOM/npm/Govulncheck/Grype findings | NOT RUN; no simulated all-clear |
| Repeated-build byte reproducibility | NOT RUN |
| Signed attestation / registry digest / release | NOT SIGNED / NOT RUN / NOT AUTHORIZED |
| NAS/network authorization, directory-context audit, trusted backup exporter | BLOCKED / unsupported; not certified by this gate |

## Execution boundary

`.github/workflows/container-evidence.yml` is JSON-formatted YAML and accepts
**workflow_dispatch only**, with no input that selects arbitrary filesystem or
registry destinations. It uses native `ubuntu-24.04` amd64 and
`ubuntu-24.04-arm` arm64 jobs, read-only repository permissions, exact checkout
SHA, no persisted checkout credential, and pinned Action commits. It has no
packages write, OIDC permission, GHCR push, Release, Tag, App Store or fnOS install.
Registering a new dispatch-only workflow may require merge into default main;
the independent reviewer/source checks precede that, and runtime stays NOT RUN
until actual job evidence exists.

The collector refuses non-GitHub/non-Linux/native-architecture mismatch, a dirty
or changed checkout, unexpected source SHA, remote Docker host/context and an
existing temporary evidence namespace. It operates only the fresh runner's
local Docker daemon. BuildKit's disposable build worker is tooling; NDG's tested
runtime remains non-root, read-only rootfs and approved synthetic read-only source.
There is no local-Mac Docker call or production NAS operation in offline tests.

## Frozen content identity

One Buildx invocation exports both a local Docker image for the existing real
synthetic acceptance test and an OCI tar. The config digest must equal the loaded
Docker image ID. OCI archive SHA, index SHA, manifest/config/layer SHA and sizes,
and uncompressed rootfs diff IDs are verified without filesystem extraction.
Links, unexpected payloads, duplicate files, unknown blobs, oversized entries,
invalid JSON, wrong architecture/source labels and damaged descriptors reject.
The narrow format supports one OCI image with gzip or uncompressed tar layers;
it does not claim arbitrary multi-platform indices/attestation graphs support.

Source `git HEAD`, tree, `VERSION`, commit-derived UTC build time and
`web-experimental` channel are bound to image labels and receipt. The existing
HTTP test also verifies the running binary's commit/channel/version/build time.
The core VERSION is not a macOS asset identity: the channel and source SHA remain
independent and beta.11 Tag/DMG are untouched. `SOURCE_DATE_EPOCH` is supplied as
an aid; it does not prove a second build produces identical bytes.

Downloads are fixed in `packaging/docker/evidence-tools.lock.json`: Buildx
v0.38.0, BuildKit v0.34.0 immutable public image digest, Syft v1.54.1 and Grype
v0.120.1. Each native tool binary/archive is checked against the recorded hash
and current official checksum file before running, and reported tool version is
checked. The tool hash is content verification over official HTTPS metadata;
upstream Sigstore signatures are **not** verified here. The runner's Docker Engine
version is observed, not a pinned installation or authenticated engine attestation.
Govulncheck is fixed to the existing `golang.org/x/vuln/...@v1.1.4` module with Go
checksum database verification. npm audit uses the same digest-pinned Node image
as the Web build, not an unpinned local npm executable. Newer official releases
require a reviewed lock update, never a `curl | sh` installer.

## SBOM and vulnerability evidence

Syft generates both CycloneDX and SPDX for the actual final OCI image and for
the public checkout. Runtime image SBOM cannot by itself fully inventory bundled
JavaScript/build dependencies; the source lock SBOM and npm audit cover that
separate layer. Generated SBOM structure/nonempty inventory is checked, not a
formal complete SBOM schema certification or absence-of-unknown-dependencies proof.

- npm audit includes lockfile build dependencies; any high/critical finding or
  failed/insufficient scan blocks.
- Govulncheck scans Linux `cmd/ndg-server` source. All reported findings are
  conservatively blocking, including module-level findings; this is not an
  assertion that every finding is reachable or that all repository entrypoints
  were scanned by this experiment. Pretty JSON events and exit code are retained.
- Grype scans the final OCI archive with `--fail-on high`, retaining DB metadata,
  matches and exit code. Tool failure/missing DB evidence is not all-clear.

Scanner DBs/advisories are dynamic; the resulting findings belong to that run,
tool version, source/image and time, not a timeless security claim. No finding is
waived by a green source CI or by classifying it as a build dependency.

## Retention and independent verification

Raw build/tool stdout/stderr stay under a private runner subdirectory and are
never uploaded. The public sibling allowlist contains only the OCI tar, four
SBOM JSONs, three scan JSONs, runtime/tool JSONs and unsigned receipt. Known
source/runner paths, absolute local path values and secret/fixture markers are
redacted or rejected. Sensitive synthetic config/credentials/DBs and build caches
are not artifacts. On an incomplete build/runtime/tool failure, only a bounded
failure classification and any already-sanitized JSON reports are retained; the
partial image is removed from this fresh CI output and is not claimed accepted.
Vulnerability blocking can retain a complete identity-verified evidence bundle,
but the workflow is still failed. Retention is seven days, not immutable release
storage.

The receipt and artifact hashes are **unsigned CI evidence**. Validation without
external expectations proves self-consistency, not provenance or producer identity.
Independent review must verify GitHub run/attempt/source SHA and job status using
the service API, then provide expected commit/tree/architecture when checking a
downloaded bundle:

```sh
python3 scripts/packaging/container-evidence.py /approved/evidence/public \
  --expected-commit <verified-run-commit> --expected-tree <verified-source-tree> \
  --expected-architecture amd64
```

The placeholder path is documentation only. No file is extracted from an image.
Caller must prove a stable owner/ACL/lease for the downloaded directory throughout
verification. Current owner/mode/link/mount preflight is not protection against
hostile concurrent remounts. Content validation can succeed for a bundle with
BLOCKED scans; inspect scan receipts and actual workflow result separately.

## Primary sources

- [Docker exporters and single-build multiple outputs](https://docs.docker.com/build/exporters/)
- [OCI tar exporter](https://docs.docker.com/build/exporters/oci-docker/)
- [OCI image layout](https://github.com/opencontainers/image-spec/blob/main/image-layout.md)
- [GitHub native hosted runners](https://docs.github.com/en/actions/reference/runners/github-hosted-runners)
- [Buildx v0.38.0 official release/checksums](https://github.com/docker/buildx/releases/tag/v0.38.0)
- [Syft v1.54.1 official release/checksums](https://github.com/anchore/syft/releases/tag/v1.54.1)
- [Grype v0.120.1 official release/checksums](https://github.com/anchore/grype/releases/tag/v0.120.1)
- [Grype threshold and real exit status](https://oss.anchore.com/docs/guides/vulnerability/filter-results/)
- [Govulncheck official command documentation](https://pkg.go.dev/golang.org/x/vuln/cmd/govulncheck)

Only actual runner evidence may update native/runtime/security states. This work
does not relax MAS NO-GO, historical privacy FAIL, fnOS's hard build refusal or any
unresolved backup, Journal, network or release authorization gate.
