# Issue #62 — 执行状态一致性修复（2026-10-10）

## 冻结证据与发布边界

- 根因与正式 GUI 失败：[Issue #62](https://github.com/FNB2026/nas-data-governance/issues/62)、[PR #63](https://github.com/FNB2026/nas-data-governance/pull/63)。证据文档已合入 main `591861cc870f70b0edae1806dedf62c392d22fab`。
- 正式 beta.6 RC `42397a9196ae5c0621aacec3913970b7269bae24` 的 Disposable Quarantine / Restore 仍为 **FAIL**。文件内容闭环成功不等于状态与审批闭环成功。
- 本修复不修改旧数据库、日志、夹具、Tag、版本、DMG 或发行资产，不操作真实 NAS。Release 保持 Draft，Public Beta 保持 BLOCKED。

## 根因与旧实现失败回归

Executor 在内存内转换 VERIFIED / DRAFT，ExecutionService 没有持久化终态；stale 分支返回 nil error，被汇总为 Executed。服务还使用请求中的批准计划，重复或陈旧请求可能复用执行资格。

先增加三个 service + SQLite 回归，在产品代码变更前运行 `go test ./internal/app -run TestExecutionConsistency -count=1`，实际 exit 1：

| 回归 | 旧实现实际结果 | 必须满足 |
| --- | --- | --- |
| VerifiedReloadAndRepeat | durable APPROVED | durable VERIFIED，重开后旧请求不再执行 |
| StaleInvalidatesApproval | executed=1 / failed=0，DRAFT 仅在内存 | executed=0 / failed=1，durable DRAFT，旧审批失效 |
| TerminalPersistenceFailure | 注入 VERIFIED 写入失败仍被报告成功 | 不计成功，保留 EXECUTING 和 Journal，禁止新写入 |

旧代码证明使用人工生成的临时文件与独立 SQLite，不修改正式失败证据。

## 修复不变量

1. 真实执行持有项目执行 owner，重新读取数据库权威计划。已有计划不能被 CLI 请求中的状态或动作替换。
2. 审批采用 DRAFT → APPROVED 条件更新；执行保留原分层阶段，CAS APPROVED → STALE_CHECKED → EXECUTING。受影响行数必须为 1；已有 Journal 禁止重新取得执行资格。
3. STALE_CHECKED 纳入恢复锁，补齐两次 CAS 之间的进程退出窗口；无 Journal 的遗留 reservation 只回 DRAFT，要求新人工审批。
4. stale 是明确 `stale_detected` 失败。DRAFT 与审批失效审计在同一事务提交，无法写库则不报告审批失效成功。
5. Pending Journal 原子创建且先于文件写入。VERIFIED、Journal 完成性/身份检查、隔离项与 Audit 在同一事务提交后才增加 Executed。
6. 执行、登记、审计、终态持久化或终态读取不能确认时停止剩余批次写入，保留恢复锁；不使用错误的 VERIFIED 反馈代替 durable 状态。
7. Source / Restore / Purge 应用服务共用非阻塞 OS owner lock。新真实执行检查三种恢复记录；查询失败即拒绝。Source 恢复不能与活跃写执行并行。
8. Pending、失败、读取不明或回滚未持久化不等于“没有发生写入”。恢复保留锁并明确要求 reconciliation；不自动回 APPROVED。已确认回滚项重复恢复不再次移动。
9. COPY 恢复在删除复制目标前检查 Journal 的 SHA-256 和大小；变化目标保留内容及锁。既有 MoveFile、SourceRoots、symlink、保护与 Journal-before-write 边界保留。
10. 计划替换禁止删除批准/执行计划或已有 Journal / Audit。旧 APPROVED + Journal 保持恢复阻断，不在本 PR 中迁移或自动修补旧失败数据库。

没有数据库迁移、依赖升级或版本修改。

## 自动化与独立复核

本机 Go 1.26.9：全量 `go test ./...`、全量 `go test -race ./...`、`go vet ./...`、golangci-lint、frontend-check、public-check、version-check、版本映射/macOS脚本/发行工作流结构测试通过。前端测试与构建用于接口回归，不替代正式 App GUI。

新增/强化覆盖：重开与重复执行、陈旧动作请求、stale 拒绝、审批 CAS、missing row、并发请求单赢家、owner 排除恢复、批次遇不确定结果停止、Journal 原子性、reservation/执行抢占/登记/审计/终态写入失败、pending 重启保留锁、STALE_CHECKED 恢复、COPY 变化目标拒绝及回滚落库失败。注入的隐私错误标记不进入 service/recovery 公共错误。

独立审查发现并促成批次停写、COPY 变化目标保护、完整 lifecycle recovery gate 和读回失败停写修订；独立四包 race 测试通过。最终精确提交复核与 GitHub CI / Security 在修复 PR 中留证。

## 限制与下一正式候选

- 本记录仅为源码与自动化证据，beta.6 FAIL 不变。beta.7 正式签名、公证、下载资产与 GUI 复验尚未完成，不得将开发测试认作 PASS。
- OS owner 覆盖同一 canonical 项目路径（父目录 alias 归一化）；硬链接数据库别名不属于支持的项目打开方式。本 PR 不宣称已验证所有平台/任意别名。
- Windows store 交叉构建通过；完整 app/executor 交叉构建仍被既有 syscall.Stat_t 平台实现阻断，不将其记为 Windows运行验收。
- Wails 执行使用 exclusive mutex，长执行期间同 API 的只读查询可能等待；不新增 GUI 进度语义。
- 未知/旧 Journal 保留现场，需恢复协议下明确 reconciliation，不自动重复写入或自动重批。
- Crash Recovery / Recovery Lock 正式运行验收仍 NOT RUN；这里的故障注入仅为回归，不是主动触发正式 App 崩溃验收。
- 下阶段独立冻结 beta.7 最终版本提交 SHA，再生成签名、公证、Staple、Gatekeeper、DMG / SHA-256 / SBOM 与 Draft Release。正式 GUI 使用全新 disposable 文件及项目复验隔离/恢复、终态、stale 审批失效、重复执行、状态一致性及限定窗口隐私。全部通过才进入后续 Crash Recovery gate；不直接发布 Public Beta。
