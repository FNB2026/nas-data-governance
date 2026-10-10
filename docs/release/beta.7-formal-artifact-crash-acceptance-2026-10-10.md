# beta.7 正式发行物 Crash Recovery / Recovery Lock 验收 — 2026-10-10

## 结论与范围

**Crash Recovery 整体：INCONCLUSIVE。** 真实复制中断、重启恢复锁及不确定写入的安全阻断已取得正式 GUI / 文件 / SQLite 证据；回滚、审计闭环及解锁尚未取得实际成功证据。另有已复现的 **GUI 恢复结果反馈 FAIL**：返回 `skipped`、恢复锁仍激活时，仍弹出成功通知「普通执行恢复完成，处理 1 个计划」。不能据此宣称恢复完成，也不能将安全保留 pending 误判为自动回滚失败。

Public Beta 保持 **BLOCKED**，beta.7 Release 保持 **Draft**。本轮不进入 Purge 或最终发布收口。未修改产品代码、发行版本、Tag、DMG、历史失败数据库或真实 NAS。

前置 [PR #66](https://github.com/FNB2026/nas-data-governance/pull/66) 已合并；main `7a041d0b3387ace61c541545ccbbcf3f86c575b6` 的 [CI](https://github.com/FNB2026/nas-data-governance/actions/runs/38035676821) 与 [Security](https://github.com/FNB2026/nas-data-governance/actions/runs/38035731979) 均 SUCCESS。该工程证据不替代本轮正式 App 运行证据。

## 正式发行身份

- 官方签名 App：`0.5.0-beta.7`，channel `beta`。
- GUI About 完整 Commit：`b09166f4ef5f7a4c9fa899a3c1370163fa373e62`。
- About 构建时间：`2026-10-09T17:23:09Z`。
- DMG SHA-256：`074b3ef3bd6dbfffd1bc7d59966fcf6ad3b324ee0ddf0a73f389dd7983cae68f`。
- 本轮重新核验 codesign deep/strict 与 Gatekeeper execute 成功，GUI About 与冻结 RC 一致；没有使用开发构建或注入 App。

## 夹具、边界与基线

全部是新建人工 disposable 数据，独立项目数据库、隔离区、源根与独立备份；没有真实 NAS 会话。写入前逐文件保存 SHA-256 / 大小，所有备份哈希一致。包含两份独立的 512 MiB 重复文件、一个普通文档单例、一个受保护单例及源根外哨兵。

第一次准备的路径语境触发 `raw_source / HOLD`，未批准、未执行，保留该项目证据。随后新建另一独立项目及临时文件语境夹具，没有覆盖或绕过 HOLD。

正式 GUI 正常扫描自然 COMPLETED：4 个文件、1 组重复候选；目录语境确认临时文件，其中一份 KEEP，另一份 QUARANTINE。人工起草、保存决策、独立审批、限定 SourceRoots / QuarantineRoot、Dry Run 均经过正式 GUI。Dry Run `executed=0 / skipped=1 / failed=0`，源文件 4/4 原哈希，隔离区空，Journal 空，计划 APPROVED。

## 实际中断与重启

只读观察器验证精确 App 可执行文件、该 PID 打开的独立数据库、注册源根与唯一 APPROVED 计划。GUI「执行隔离」后，在 **EXECUTING + pending Journal + 隔离目标实际增长至 8 MiB** 时 SIGSTOP 精确 App PID，保存只读 SQLite 一致性快照，再 SIGKILL 该 App。没有中断 NAS、写数据库或伪造 Journal。

观察时目标为 8,388,608 bytes；冻结后稳定部分输出为 8,421,376 bytes（采样与进程停止之间仍有复制进度）。源文件没有删除或变化。正式 App 在同一日志窗口内重新启动，GUI 打开同一个 disposable 项目。

| 项目 | 实测结果 | 判定 |
| --- | --- | --- |
| 真实文件操作中断 | EXECUTING / pending / 部分目标存在 | PASS |
| 重启恢复锁 | GUI「恢复锁激活」，1 条未完成写入 | PASS |
| 新操作阻断 | 扫描及执行中心导航 disabled，审计与恢复可访问 | PASS（可见 GUI 边界） |
| 文件安全 | 4/4 源 SHA-256 / 大小不变；根外哨兵不变；active=4 | PASS |
| 普通执行恢复 | 真实 GUI 点击一次，显示 `skipped`；仍 EXECUTING / pending | 不确定结果安全阻断 PASS |
| 恢复前后字节一致 | 源与部分隔离目标均未改变，未重复写入 | PASS |
| 回滚 / 解锁 | 未发生，pending 现场保留 | NOT PROVEN |
| 审计闭环 | operation_logs=0；持久化 pending Journal=1；quarantine_items=0 | INCONCLUSIVE，不宣称成功审计 |
| GUI 恢复反馈 | `skipped` + 锁激活，仍显示成功「恢复完成」 | FAIL |

## 恢复语义与证据限制

冻结源码 `internal/executor/recovery.go` 明确要求：任一 Journal 非 done 或未确认回滚时，返回 skipped / errors，并保留恢复锁；只有所有动作已明确完成，才允许回滚后 durable ROLLED_BACK。当前 pending 分支符合 fail-closed 设计。**源码预期不代表本轮直接捕获了结构化 errors 字段。** 实测证据为 GUI skipped、恢复锁及只读数据库 / 文件快照。

`AuditRecoveryPage.tsx` 的普通执行恢复把 `results.length` 当作处理数，并无条件成功 toast；页面摘要只展示 action，没有说明 pending 的人工 reconciliation 要求。实际复现的通知不能证明恢复完成。

本轮没有取得 all-done-but-not-finalized 的自然中断窗口，不能用源码测试代替正式 rollback/unlock；没有通过修改数据库、注入已签名 App 或强制删除部分输出制造通过。恢复操作只执行一次，原现场和备份保留。Restore 中断分支也未执行。

## 连续日志与隐私

统一日志采集先于 App 启动，保持跨崩溃和重启的同一采集进程；两段 App stdout/stderr 在各次启动前打开。窗口 UTC `07:51:17.376220` 至 `07:59:46.245957`，约 509 秒，App PID 15984 → 23110。

- 1,010 次存活采样，采集进程死亡样本 0；最大采样间隔 0.512 秒；主动结束采集，exit=0。
- 32,204 条统一日志 JSON 记录；2 行非 JSON 采集头，未作为事件计数。
- 28 个源路径、文件名、业务标记编码 / 大小写变体，在已采集统一日志、采集 stderr 和两段 App stdout/stderr 中均零命中。
- GUI 普通数据行使用掩码路径；用户明确输入的根配置与原始 AX 仅保存在本机私有归档。
- predicate 包含 NDG 与 WebKit 进程 / subsystem，但没有独立完整 WebKit PID 清单。此结果只支持 **该窗口、该 predicate、该测试标记集合的隐私 PASS**，不支持全进程或长时隐私 PASS。

脱敏聚合与原始本机证据 SHA-256 见 [runtime-summary.json](evidence/beta7-crash/runtime-summary.json)。原数据库、文件路径、文件名、测试标记及原始日志只保存在本机私有归档，不上传。

## 后续门禁

1. 独立复核本轮实际状态与字节证据，归档本记录。
2. 独立跟踪 [Issue #67](https://github.com/FNB2026/nas-data-governance/issues/67) 恢复反馈缺陷；不要把 skipped / errors 报成恢复完成。
3. 在既有安全边界内取得可确认的正式 rollback / audit / unlock 证据，以及必要 Restore 中断证据；任何新产品修复必须经独立 PR 和新不可变正式候选复验。
4. 整体 Crash Gate 未关闭前，不进入最终发布判定。全新 Mac 离线首装、npm 告警与剩余隐私门禁仍未关闭。
