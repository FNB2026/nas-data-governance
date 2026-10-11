# beta.9 Restore Recovery 正式候选准备 — 2026-10-11

## 修复来源与冻结规则

[PR #73](https://github.com/FNB2026/nas-data-governance/pull/73) exact HEAD `ae1644ef88863784238cb3a845dc481ed66f2bc6` 独立 APPROVE；CI `38067959502`、Security `38067959358` SUCCESS。修复 main merge `40cd859afe4bb7eed5e80036fd052ce11bb43ba2`，合并后门禁需另行核验。

本 PR 只同步 VERSION `0.5.0-beta.9`、Bundle build `9`、CHANGELOG 与候选准备。最终 RC 取本版本 PR 合并后的 main SHA；不能把修复 merge SHA 当成最终 RC。创建新 annotated Tag，禁止移动任何既有 Tag 或替换资产。原 [PR #71](https://github.com/FNB2026/nas-data-governance/pull/71) / [Issue #72](https://github.com/FNB2026/nas-data-governance/issues/72) 的 beta.8 D FAIL 保留。

## 正式工作流

版本 PR 精确 CI/Security、独立审查、main 合并后门禁 → 新 Tag → 既有 Release Verify/macOS build/Developer ID/hardened runtime/Apple Notarization/DMG staple/Gatekeeper/DMG SHA-256/CycloneDX与SPDX SBOM/Draft Release。全部产物绑定最终 RC SHA。下载官方 Draft DMG 后核对签名、校验和、Gatekeeper 与 About 完整版本/Commit/构建时间/通道。

## 受影响正式验收

| 场景 | 当前正式状态 | 必需证据 |
| --- | --- | --- |
| D Restore pending/部分目标 | NOT RUN | 实际中断、重启锁、Recover 拒绝、两侧字节保留、pending/APPROVED/脱敏审计一致 |
| D 安全手工结案与新恢复 | NOT RUN | 独立保留部分输出不覆盖，正常 Recover 验证隔离完整/目标不存在，durable ROLLED_BACK/审批清空/审计/解锁；新计划审批 Restore 完整 |
| A/B/C 针对性回归 | NOT RUN | 未确认保锁、无 Journal 重审、确认动作回滚及状态/审计/哈希一致 |
| 限定连续窗口隐私 | NOT RUN | 启动前采集stdout/stderr与选定统一日志，PID与采集存活连续性，实际GUI/通知和私有标记变体检查 |

使用新人工 disposable 项目、独立备份和获批准根目录，不接触历史数据库/部分输出。不能证明身份时正确保锁；手工保留操作与正常 Recover 分开，不伪造 Journal、不强制清锁。极短窗口未观察到时按 INCONCLUSIVE 记录；组合证据标准须另获批准。

原始路径、标记和日志只存本机私有归档。Issue #72 在源码和正式验收均通过前不关闭；Public Beta BLOCKED，Release Draft，Purge NOT RUN。
