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
- HOLD / RESTORED / PURGED、只读、恢复锁与正在创建时保持禁写。

不改变后端事务、Journal、审批协议、Purge、恢复锁或文件执行规则，不增加自动删除或部分文件清理。

## 本地自动化证据

- 页面专项 23/23 PASS（新增 13 项）：新草案新审批、两种历史排序、Dry Run 后重新加载、新批准 ID/digest、冲突/未知状态、非 QUARANTINED、恢复锁、只读、busy。
- 全前端 28 文件、252 测试 PASS；TypeScript / Vite build PASS。
- 专项日志 SHA-256 `6f2f4b84d00780df447979c70937f15a2b99a50d8f7c57bb7cbf25b8b604e7ed`；全前端 `e469bf44320b95539975f5722c8fc074660dc1f9bc68808018506a117e1c80ff`；build `6d389e44051d677b7a0835cc72069b34b3b717db442181f7d69407fb14a09c27`。
- 相关 Go race、CI/Security、独立 exact-HEAD review：尚待完成，以后续实际结果为准。

## 正式层仍待验证

本修复的测试/构建不是正式发行物通过。beta.9 D GUI 闭环 FAIL 不变；下一不可变候选需真实 GUI 重跑 D 与 A/B/C 针对性回归。Issue #72 / #75 保持 OPEN，Public Beta BLOCKED，Release Draft，Purge NOT RUN。
