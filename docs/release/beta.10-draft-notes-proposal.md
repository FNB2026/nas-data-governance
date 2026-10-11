# beta.10 Draft Notes（审计后提案）

- 正式候选 RC `c1941f84e2d1c836b57cd0750db1c2554ba6c4c9`；仅 macOS 13+ Apple Silicon。
- 修复 Restore 部分目标安全保锁及旧计划回滚后的新草案/新审批入口；不复用失效审批。
- 限定 Crash Recovery Gate PASS；正式 B/C INCONCLUSIVE，采用经批准的冻结源码组合证据；A 未确认动作有锁保护，但缺少新持久拒绝审计。
- beta.8/beta.9 原失败记录和资产保留，用户资料与 Journal 不清洗。
- beta.10 仍不可公开：全新 Mac 离线首装未执行、npm source-map-js 高危未完成风险处置、Purge 未验收但实际可达、首发隐私/备份支持范围证据契约待裁定。
- 当前仅内部 disposable 验收；禁止 Purge，禁止唯一真实资料执行。文档警告不代表 App 已技术禁用 Purge。
- 先冷备份完整项目目录（DB/WAL/SHM），未知输出/恢复锁保留现场人工核对，不删除部分文件。
- 无最终发布授权，Release 保持 Draft。实时决策见 beta.10-public-go-no-go-2026-10-11.md。
