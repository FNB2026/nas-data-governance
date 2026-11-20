# fnOS 升降级链路验收（Linux/Docker 自动化）

- 日期：2026-11-19
- 分支：`codex/fnos-upgrade-acceptance`
- 起点：`4e222fb37b29d5a4816298f6fdc8614bb8aa3953`（`codex/linux-governance-executor` 头，已并入 main 安全检查基线）
- 被测提交：`329f8b5dcce5f8d46994d098859dafe164c8a4aa`
- 环境：GitHub 托管 `ubuntu-latest`（amd64）、真实 Docker、一次性本地 registry

## 判定

| 范围 | 判定 | 依据 |
| --- | --- | --- |
| Linux/Docker 自动化升降级闭环 | **PASS** | 23 项检查全部通过，见下 |
| 真实 fnOS 主机升级与回滚 | **PENDING** | 本项未触碰 NAS 主机，无现场授权 |
| 产品正式发布资格 | **不由此放行** | 仍需独立发布验收，含 SQLite 1.62.0 真实库兼容性 |

## 镜像身份

### 原版（生产 pin，未改动）

| 项 | 值 |
| --- | --- |
| 引用 | `ghcr.io/fnb2026/ndg-readonly-preview@sha256:49dada73…` |
| index | `sha256:49dada73f1eb9cc74485ed5d0b92023d7cb4c3df462d03e105f00158b91668e1` |
| amd64 manifest | `sha256:bb6db808bc5a17cf27b951b07406136c6bba3056c89d2aee0268042a683f5be4` |
| amd64 config | `sha256:2c38ca9493ccebc74cdf0cc84fb23e8a4c59babb1660d4892a2f9a50c12a96d4` |

index → manifest → config 三段关系以注册表回读方式独立核对，与 `PINS` 完全一致。

### 第二候选（本次构建，仅存在于一次性 registry）

| 项 | 值 |
| --- | --- |
| 源码 | `329f8b5dcce5f8d46994d098859dafe164c8a4aa` |
| 构建参数 | `SOURCE_TREE=upgrade-acceptance-candidate`、`CORE_VERSION=0.5.0-upgrade-candidate` |
| 仓库 | `127.0.0.1:5000/ndg-readonly-candidate`（进程内一次性，运行后销毁） |
| index | `sha256:4af8ccc1388b4606acf85b0bf9c82d4365a5e49540b64774a8ad3f636a2ec7a1` |
| amd64 manifest | `sha256:0b821e79241b06c2c0a89803ba289bfe2fdfc198f2b899ce85f0a62a836561ea` |
| amd64 config | `sha256:fb27b28f38189d4bac1b16bf0983b02bf3b90cc681f2c8e49a282be764945323` |
| 平台 | `linux/amd64`（另有 buildx 证明用的 `unknown/unknown` 描述符） |

两个 index 与两个 config 互不相同，满足"同架构、不同实际 image ID、可按 digest 解析"。

## 三处缺陷与修复

1. **裸 digest 不可 inspect**：`step_upgrade` 把 `sha256:…` 直接交给 `docker image inspect`。Docker 按仓库解析 digest，多架构 index digest 不能单独作为目标。统一改走 `candidate_reference()`，始终输出 `repo@digest`。
2. **身份语义被写死**：原 `step_rollback` 用 config digest 常量比较容器，而宿主实际报告的是 index digest（本次实测 `image_id = sha256:49dada73f1eb…`）。正确回滚会被误判为失败。改为与本安装记录的身份比较，与 `verify_rollback` 保持一致；`verify_image_record` 接受 index 或 config 两种派生并记录实际类型。
3. **仓库址硬编码**：候选仓库改为从 `PINS['image']` 派生。新增 `candidate_repo` 字段，规则不对称：生产 scope 必须等于 pin 仓库；只有一次性验收 scope 可指向别的仓库，且必须同时给出候选 digest。

生产 pin、Preview.1 标签、公开 GHCR 均未改动。

## 23 项检查

| # | 检查 | 说明 |
| --- | --- | --- |
| 1 | `github_hosted_linux_amd64_real_docker` | 真实托管 amd64 + Docker |
| 2 | `throwaway_local_registry_running` | 一次性 registry |
| 3 | `second_candidate_index_built_and_read_back` | 构建并从注册表回读 index/manifest/config |
| 4 | `candidate_absent_before_the_refusal_probe` | 删除本地引用并证明已不存在 |
| 5 | `fixture_created_with_known_source_bytes` | 人工源基线 |
| 6 | `installation_only_internal_bridge_created` | 安装专用内网桥 |
| 7 | `pinned_preview_image_pulled_by_digest` | 原件按 digest 拉取 |
| 8 | `candidate_source_rules_refuse_before_execution` | 生产 scope 拒绝非 pin 仓库、无候选拒绝、带 digest 的仓库名拒绝 |
| 9 | `plan_frozen_and_twelve_steps_authorized` | 计划冻结与显式授权 |
| 10 | `original_pinned_image_installed_and_scanned` | 原版安装至 GUI |
| 11 | `original_install_bound_to_recorded_identity` | 容器身份 == 记录身份 |
| 12 | `cold_backup_taken_before_upgrade` | 升级前冷备份 |
| 13 | `missing_candidate_refused_without_touching_the_install` | 候选缺失时拒绝，compose 与容器均不变 |
| 14 | `upgrade_moved_container_to_candidate_config` | 升级到候选，只读根、运行身份、无端口保持 |
| 15 | `upgrade_kept_database_quick_check_and_source_bytes` | 数据库 quick_check 与源字节 |
| 16 | `login_and_state_read_back_after_upgrade` | 升级后重新登录读回状态 |
| 17 | `second_upgrade_is_idempotent` | 重复升级走校验并跳过 |
| 18 | `interrupted_upgrade_reconciled_from_postcheck` | 残留 `start` 记录按后置校验对账 |
| 19 | `rollback_restored_every_backed_up_file` | 回滚后备份文件全部还原（运行时重写另行计数） |
| 20 | `login_and_state_read_back_after_rollback` | 回滚后重新登录读回状态 |
| 21 | `sources_unchanged_across_upgrade_and_rollback` | 源字节全程不变 |
| 22 | `identity_scoped_uninstall_removed_only_this_install` | 按身份卸载 |
| 23 | `throwaway_registry_removed` | 一次性 registry 清理 |

## 证据

- CI 运行：[fnOS Web candidate 44541975655](https://github.com/FNB2026/nas-data-governance/actions/runs/44541975655)，`native-gateway (ubuntu-latest, amd64)` 成功
- 制品 `ndg-cgi-candidate-amd64` 内 `upgrade-chain-proof.json`
  - SHA-256 `1db9591902a9da94fb21fb5612a36373d36a16ab699c43a53b5afdbfba7633b4`
- 同制品 `install-chain-proof.json`
  - SHA-256 `5ee1c43c004368847ba2159f111ab19b353c74e765be18e9592716cd4851f290`

## 复跑

```bash
# 需要 root、GitHub 托管 Linux、daemon 信任 127.0.0.1:5000
gh workflow run fnos-web-candidate.yml --ref codex/fnos-upgrade-acceptance
```

本地无法复跑：脚本要求 `GITHUB_ACTIONS=true`、`RUNNER_ENVIRONMENT=github-hosted`、amd64、root，并显式拒绝在 fnOS 主机上运行。

## 未覆盖

- fnOS 主机上的升级、回滚、开机启动、整机重启
- 新候选的正式渠道声明与发布
- SQLite 1.62.0 既有真实数据库的迁移兼容性（单独发布验收）
- 治理执行、源文件写入、受控永久删除
