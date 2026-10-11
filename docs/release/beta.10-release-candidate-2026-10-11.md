# beta.10 Restore 新审批闭环候选准备 — 2026-10-11

## 来源与冻结身份

后端安全修复 PR #73 已合并；beta.9 正式 D 验证保锁与经核验结案，但新审批入口 FAIL，证据 PR #76 已合入 main，历史资产不变。

最小 GUI 修复 [PR #77](https://github.com/FNB2026/nas-data-governance/pull/77)：exact HEAD `45fc0d6a1aa1512f72c2636ebf235172396c0c61` 独立 APPROVE，CI `38089184994` / Security `38089185005` SUCCESS；merge `72e29171ec4f86c4b65bcfa65e0354cd90a8b865`，post-merge 门禁尚待实际完成。

本 PR 只同步 `0.5.0-beta.10` / Bundle build `10`、CHANGELOG 与候选文档。最终 RC 必须取版本 PR 合并后的 SHA，经过精确 HEAD 与合并后 CI/Security，再创建新的 annotated Tag、正式签名公证及 Draft 资产。禁止移动 beta.8/beta.9 Tag、覆盖 DMG 或改写历史失败证据。

## 正式验证

使用新人工 disposable 文件、新 GUI 项目数据库、独立备份及批准根目录；启动前连续采集 stdout/stderr 与选定统一日志，保留 PID、采集健康及明确起止边界。

- D：实际 pending + 部分恢复目标；终止已核验测试 App；重启保持锁，重复 Recover 不结案、不改未知输出。
- D：独立原样保留部分目标，验证完整隔离副本及目标缺失；正常 GUI Recover 安全结案并清除审批。通过新草案、新审批、Dry Run、正式执行，还原完整字节/hash、Journal、计划和审计一致；重载不选历史审批。
- A/B/C：未确认动作保锁；无 Journal 中断退回草案重审；已确认动作安全回滚及解锁。未实际观察到极短窗口时为 INCONCLUSIVE，不把源码/故障分类测试自动替代正式 App。
- 限定日志隐私：测试标记及编码变体零泄漏；不得扩大为全系统、长时或全新 Mac 离线首装 PASS。

正式结果均 NOT RUN。Issue #72 / #75 保持 OPEN，Public Beta BLOCKED，Release Draft，Purge NOT RUN。仅实际证据闭合后才考虑关闭修复任务。
