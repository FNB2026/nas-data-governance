# Issue #67 — 恢复结果反馈最小修复

## 历史证据与归档

[PR #68](https://github.com/FNB2026/nas-data-governance/pull/68) exact HEAD `a2329b277fcdd4e95be4f34e20f999b754db0776` 已独立 APPROVE 文档；CI `38036571961` / Security `38036572011` SUCCESS，合并为 `a1a142cc577203bb0272656fc7cafc94017e3efb`。main CI `38043932896` / Security `38043959076` 均 SUCCESS。归档不代表 Crash Gate PASS，旧数据库、部分隔离文件、截图、日志未修改。

## 根因与修复范围

AuditRecoveryPage 的三个恢复按钮无条件 success toast，以结果数组长度表示处理成功。ExecutionCenterPage 的三个入口有同类行为。DTO 已包含分类字段，无需变更 executor、数据库或 Wails 接口。

共享前端分类规则：

- 普通执行 `rolled_back` 且无 errors：确认回滚；`reset_to_draft` 且无 errors：退回草案，提示旧审批不可复用并要求重审。
- 隔离还原只接受 `ok / ROLLED_BACK` 且无 error / error_type。
- Purge 只接受 `ok / ROLLED_BACK` 或 `ok / PURGED` 且无错误；`PURGED` 是既有 `PurgeCommitted` 的实际值。只验证反馈，未启动 Purge。
- skipped、未知结果、非空错误、失败或字段冲突均不显示恢复成功；检查数、确认回滚数、草案数、提交数、未完成数分开。
- 空结果只表示本类型无任务，显示无需恢复。
- 返回后与请求异常分支均刷新全局恢复锁；未解除、无法刷新或计数矛盾均禁止新的写入。不重试恢复 mutation。
- 恢复结果在解除锁后仍保留可见。错误提示只使用固定安全文案，不展开原始 errors、error、error_type 或计划 ID；防止路径和业务标记进入通知。

## 实际自动化证据

新增按钮回归在旧实现稳定失败：普通执行、隔离还原、Purge 拒绝三个用例都出现错误 success（3 failed / 13 passed）。修复后完整前端 **239 tests / 28 files PASS**，生产 TypeScript/Vite 构建 PASS。包含单次调用、混合结果、草案重审、空任务、未知锁、请求/锁刷新异常、矛盾锁计数和隐私 canary。

本机 `go1.26.9` 全量 `go test -race -count=1 ./...` 与 `go vet ./...` PASS。既有源执行恢复测试覆盖：pending 保锁、无 Journal 回 DRAFT、不复用旧审批、done 回滚与持久化、重复恢复、回滚写库失败保锁。

新增 `TestRestoreRecoveryCrashContract` 在全新临时 SQLite / disposable 源 / 独立备份上注入 **测试层**中断状态，5 种形态全部 PASS：仅隔离完整、仅恢复目标完整、两处均完整、两处都不匹配、完整隔离及部分恢复目标。明确检查结果、durable Restore 状态、pending 锁依据、内容哈希及备份不变。测试层使用既有正常计划/审批/Journal API 登记夹具，不读取或改动原崩溃项目，不是正式签名 App 的中断证据。

**重要限制**：部分恢复目标用例按当前后端规则保留部分文件，并登记 ROLLED_BACK；不宣称部分输出已清理，也不证明该形态的整体正式恢复闭环。未知双副本 / 内容变化保持 pending，文件不被删除。恢复审计闭环和全部正式崩溃窗口仍需新候选证据。

日志哈希与测试层标记见 [test-summary.json](evidence/recovery-feedback-67/test-summary.json)。本记录不上传原始日志或夹具路径。

## 新候选与门禁

产品 GUI 已变化，beta.7 Tag / DMG / 历史 FAIL 与 INCONCLUSIVE 保留。修复经精确 HEAD 独立审查、CI / Security、合并后 Gate 通过，才能生成新的 beta.8 不可变 Draft 候选，核验签名、公证、Gatekeeper 与 About。

正式专项 A/B/C/D 另取证；测试层证据不自动关闭正式 Gate。若极短窗口无法稳定触发，组合证据是否关闭 Gate 仍需用户事先明确批准。本次未降低标准。Release Draft，Public Beta BLOCKED。
