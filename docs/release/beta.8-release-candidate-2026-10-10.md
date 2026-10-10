# beta.8 恢复反馈正式候选准备 — 2026-10-10

## 来源与身份冻结

产品修复 [PR #69](https://github.com/FNB2026/nas-data-governance/pull/69)，Issue #67，main merge `b7d40101384778832225c13e8e4e813113a778e3`。精确修复 HEAD `92453e10340eac41ce6f14ed484e08910a2960f9` 已独立 APPROVE，CI `38044521671` / Security `38044521670` SUCCESS。

本 PR 只同步 `0.5.0-beta.8`、Bundle build `8`、CHANGELOG 及候选准备。最终 RC 必须冻结在此版本 PR 合并后的 main SHA，不能使用 #69 的 merge SHA 代替。创建新的 annotated Tag；不移动、覆盖 beta.7 或任何旧 Tag / DMG。

## 发行流程

1. 确认修复 main 与版本 PR 的 exact-SHA CI/Security、独立审查及合并后门禁。
2. 冻结版本合并后的 RC，确认 beta.8 Tag 不存在，创建 annotated `v0.5.0-beta.8`。
3. 运行既有 Release 流水线：Verify、macOS build、Developer ID/hardened runtime、Apple Notarization、staple、Gatekeeper、DMG、SHA-256、CycloneDX/SPDX SBOM、Draft Release。
4. 从 GitHub Draft 下载正式资产，核对哈希、签名、公证、Gatekeeper、About 版本/完整 Commit/构建时间/通道。
5. 只开展受影响恢复场景及必要回归，不发布 Release。

## 正式专项与证据层

| 场景 | 本版本正式状态 | 核验核心 |
| --- | --- | --- |
| A pending 未确认写入 | NOT RUN | 保锁、人工核对提示，无误报成功，不自动删除部分输出 |
| B 无 Journal 中断 | NOT RUN | durable DRAFT、旧审批不可复用、正确重审提示 |
| C done 未结案 | NOT RUN | 字节/哈希、rollback Journal、durable 状态、审计与解锁 |
| D Restore 中断 | NOT RUN | 对应恢复锁、确认回滚与不确定状态保护 |
| 限定窗口隐私 | NOT RUN | GUI/通知/事件、stdout/stderr、统一日志，明确进程覆盖及连续性 |

既有与新增独立源测试已有分类和临时数据库故障注入证据，见 [Issue #67 修复报告](issue-67-recovery-feedback-fix-2026-10-10.md)。它们不是正式签名 App 运行证据。极短窗口无法稳定触发时，正式 Gate 保持 INCONCLUSIVE；组合证据关闭标准必须先获明确批准，本 PR 未改变门禁标准。

原 Crash 数据库、部分隔离输出、日志、截图、独立备份保持不动。测试只使用新人工 disposable 数据与独立项目。Purge NOT RUN，最终发布收口未启动，Public Beta BLOCKED，Release Draft。
