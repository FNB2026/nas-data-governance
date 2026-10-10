# Issue #75 — verified Restore rollback 后新审批入口

## 正式失败与边界

beta.9 RC `af4c5902eb431833e72d9024ccdee6e6ea678f97` 的真实 D 场景已证明后端保锁与安全结案；但隔离项仍 QUARANTINED、旧 Restore 已 ROLLED_BACK 时，正式 GUI 只有终态标记，无法创建新恢复草案。证据归档 [PR #76](https://github.com/FNB2026/nas-data-governance/pull/76)，任务 [Issue #75](https://github.com/FNB2026/nas-data-governance/issues/75)。不修改 beta.8/beta.9 Tag、DMG 或失败现场。

## RED → 最小修复

先添加真实页面渲染回归：旧终态计划存在、隔离项仍 QUARANTINED 时，要求创建**新**草案并以新 ID / digest 审批。原代码 10 个现有测试通过、新测试失败，原因是找不到“创建恢复草案”；RED 日志 SHA-256 `5c3b74a2713a0e25051762f90832a1903f6446711d2b2fd267424fff972fdf03`。

仅修改 ExecutionCenterPage 的隔离还原动作选择：

- 只有无未结案恢复计划、隔离项仍 QUARANTINED 时提供创建入口。
- 已结案 ROLLED_BACK 不再占用活动操作位置；新草案不替换历史计划，也不复用历史审批。
- 活动 DRAFT / APPROVED 与历史记录顺序无关，所有动作绑定活动计划 ID / digest。
- 多个未结案记录或未知状态不提供写动作，要求人工核对。
- 独立审查发现读取未完成/失败时的空列表与旧缓存放行风险，先补两例 RED（旧修复 23 PASS / 新增 2 FAIL）再修复：每次读取立即失去写入资格，只有最新请求成功后恢复；旧请求晚到不能恢复资格。失败提示固定且不含原始 error。刷新同步读取隔离项与恢复计划。后续独立审查又指出读取/写入交错与共享错误提示问题；创建、审批和执行开始即废弃旧读资格，结束后重读 canonical 列表，写入期间禁用刷新。共享读取错误改为固定文本，清理标签也不显示原始错误。
- HOLD / RESTORED / PURGED、只读、恢复锁与正在创建时保持禁写。

不改变后端事务、Journal、审批协议、Purge、恢复锁或文件执行规则，不增加自动删除或部分文件清理。

## 本地自动化证据

- 页面专项 29/29 PASS（新增 19 项）：新草案新审批、两种历史排序、Dry Run 后重新加载、新批准 ID/digest、冲突/未知状态、非 QUARANTINED、恢复锁、只读、busy、读取延迟/失败/刷新失败/旧请求晚到、读取与创建交错、清理页错误脱敏。
- 全前端 28 文件、258 测试 PASS；TypeScript / Vite build PASS。
- 专项日志 SHA-256 `c43e01fee744f6b7a6b96727807527f22a4f7829518a92591c3f6e99fb10e098`；全前端 `0ea23c3cfdbf842e5d0a5000bb5ac5e8139b135e020f7171e27d76139d90a41b`；build `17a5d0c562620c1f570b0caf91c77129580851202a38377f5c58b5ff049121fd`。
- 相关 Go race（app / executor / Wails / store）PASS，日志 SHA-256 `c8f3c0887b8f25db81fa8d98bf603b64a333d5843937163c5a667418c8f71d1a`。
- CI/Security、独立 exact-HEAD review：尚待完成，以后续实际结果为准。

## 正式层仍待验证

本修复的测试/构建不是正式发行物通过。beta.9 D GUI 闭环 FAIL 不变；下一不可变候选需真实 GUI 重跑 D 与 A/B/C 针对性回归。Issue #72 / #75 保持 OPEN，Public Beta BLOCKED，Release Draft，Purge NOT RUN。
