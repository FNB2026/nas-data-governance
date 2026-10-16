# fnOS development packaging

当前受限只读入口已在一台 fnOS amd64 上通过真实 HTTPS/Chrome 验收。
核心仍是冻结的公开 Preview.1，网关是独立 cde4c01 开发候选；完整 FPK、
自动安装、治理写入和整机重启尚未验收。已验证的容器重建和冷备份恢复见
[Compose 生命周期记录](../../docs/development/FNOS-READONLY-COMPOSE-2026-10-16.md)。

`compose.lan-readonly.yaml` 和 `scripts/packaging/init-fnos-readonly.py` 为独立的
开发准备工具：仅生成全新私有核心配置，固定镜像、只读来源、无端口、外部专属
internal 网络。准备状态 `PREPARED_NOT_STARTED` 不证明网络、镜像、网关已部署。
工具要求可信管理员维护单个配置根的稳定挂载视图，拒绝该根内的嵌套挂载/别名。
不得把任意 Web 输入直接传给管理员请求文件。通用安装链路仍未交付。

## G5 FPK scaffold — not installable

This separate G5 scaffold uses the structure and fields verified against official
fnpack 1.2.3 on 2026-10-12. It does not package a working NDG service, create an
FPK, install anything, contact Docker, or change the macOS channels. No accepted
OCI digest exists in this scaffold. `image` and acceptance fields are empty by
design. **Research structure PASS is not fnOS runtime PASS.**

```sh
make fnos-test
./scripts/packaging/build-fnos-package.sh
```

The first command performs structural checks and 24 synthetic tests. The second
must exit 1 with `accepted_oci_digest_required` and produces no artifact. Even a
synthetic receipt saying PASS cannot enable building: the final hard stop is
`fnos_runtime_integration_not_implemented`. Future enablement requires a new
reviewed implementation and actual isolation, image and device evidence; editing
the receipt or policy alone cannot enable it.

## Files and explicit limits

- `ndg-readonly-test/manifest`: uniquely scoped research identity, x86 target only;
  no claim of architecture or minimum-system acceptance. Research version is
  unrelated to the immutable macOS beta.11.
- `app/docker/docker-compose.yaml`: **JSON subset of YAML**, intentionally parsed
  by Python standard-library JSON. YAML tags, aliases and duplicate keys are not
  accepted. No additional dependency or implicit string interpolation is used by
  the offline checker. Compose itself has not been run.
- `config/privilege`: package user for hooks; it does not set container identity.
- `config/resource`: one named Docker project. No data-share, root-mode privilege
  or host Docker socket resource is requested.
- `app/ui/config`: administrator-visible direct port entrance. It does not
  authenticate the NDG API. A loopback-only port is intentional: remote browsing
  needs a reviewed isolated proxy or gateway, not an automatic public listener.
- `wizard/`: warning-only forms; no input can authorize a host bind path.
- `cmd/`: start/stop, installation, config and upgrade reject as unsupported. No
  initializer, trusted backup exporter, migration or graceful-stop integration is
  implemented. App Center normally owns Compose start/stop; device sequencing is
  NOT RUN. `status` uses fixed container identity and label/health evidence only.
- uninstall hooks perform **no deletion**. This does not certify fnOS's own
  uninstall retention behavior. Never install this scaffold on production storage.
- package icons are derived from the repository-owned NDG icon; no downloaded
  template icons or binary tool are distributed.

The `healthcheck` command, package UID/GID mapping, private state initialization,
authorization-to-bind mapping and loopback UI access are **proposed contracts**,
not implemented service claims. `${NDG_AUTHORIZED_SOURCE}` must eventually be
generated from a verified administrator authorization and safely serialized,
never trusted from a browser, raw wizard text, shell `eval` or unverified path.
It is bound read-only. DB state must remain private/local under `TRIM_PKGVAR`.
Quarantine, Restore and Purge are not mounted or enabled.

## Offline checks

The checker rejects extra Compose fields/services, root users, public listeners,
writeable sources, rootfs writes, credential environment additions, host socket
mounts, symlinks/special files, unsafe file modes, hidden/generated/private
entries, duplicate JSON keys and image tags instead of digests. It explicitly
walks children only after checking the directory boundary; it does not use
recursive glob. Device changes and nested mount targets are rejected before
descent/content reads, including Linux same-device bind mounts and mount-on-file
entries. Linux uses `/proc/self/mountinfo`; macOS arm64 uses 64-bit `getmntinfo`
with `MNT_NOWAIT`, and x86_64 requires the explicit INODE64 ABI symbol. Unknown
platforms, unavailable mount tables and malformed records fail closed. Local
macOS table reading and deterministic injected boundaries are tested; actual
mount/remount integration and macOS x86_64 are **NOT RUN**.

Errors contain classification only; mount paths and sources are not logged.
Before enumeration, root and ancestor POSIX ownership/mode/link/identity are
checked. A system-owned sticky temporary ancestor is allowed, but the package
root must belong to the caller. This is still a trusted, stable checkout
precondition: callers must prove ACL safety and retain exclusive ownership/lease
while all document reads run. 0700 or owner UID alone is not proof of ACLs or
future mount stability. The checker detects some observed replacements but is
not an adversarial concurrent filesystem snapshotter or a runtime container
ownership/ACL verifier. The receipt is a proposed local schema, **not
cryptographic evidence**. The hard build block prevents it being used to assert
acceptance today. Tests never call a real Docker daemon.

## Official tool and references

The tool observation receipt is [fnpack-tool-receipt.json](fnpack-tool-receipt.json).
The binary was fetched publicly and run only in a disposable directory; it was
not installed, committed or used to build an FPK. Its observed SHA-256 is not an
upstream published signature. Review the [official fnpack commands](https://developer.fnnas.com/docs/cli/fnpack/)
and [Docker case](https://developer.fnnas.com/docs/examples/docker/) before a future
packaging change. Remaining prerequisites and official-field evidence are in the
[G5 report](../../docs/multichannel/FNOS-PACKAGING-RESEARCH.md).
