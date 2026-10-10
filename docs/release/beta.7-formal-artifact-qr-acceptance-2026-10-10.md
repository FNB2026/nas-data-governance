# beta.7 正式发行物与 Disposable Quarantine / Restore 验收

## 冻结身份与边界

- 修复 PR #64 合并 `ed691a15a4d3c2aa951d6df875c9134bd05dfded`，版本 PR #65 合并后最终 RC `b09166f4ef5f7a4c9fa899a3c1370163fa373e62`。
- 新 annotated Tag `v0.5.0-beta.7` 对象 `fd1faf513bbe6bfbb43f8d3d443e67e18ce1ae09`，peel 为最终 RC。没有 force 或移动 beta.6 Tag。
- main CI `37964191884`、显式 main Security `37964236991` 均 SUCCESS，且对应最终 RC。
- beta.6 正式 FAIL 与失败数据库、日志、夹具保留。源码回归通过不替代正式发行物实测。
- Release 仅 Draft，Public Beta BLOCKED。本轮不进入 Purge，不主动触发 Crash Recovery。

## 正式流水线记录

[Release run 37964574677](https://github.com/FNB2026/nas-data-governance/actions/runs/37964574677) 绑定冻结 RC。

- attempt 1：Verify SUCCESS；macOS Build 在 Wails 依赖下载阶段因 `proxy.golang.org` TCP 443 `i/o timeout` 失败，签名及 Draft 作业跳过。不是 GUI 验收结果。
- attempt 2：仅重跑失败作业，冻结 SHA 不变；Build Unsigned SUCCESS。
- 签名部署 `6966845819`：通过既有 `release-macos` required-review 机制批准，本轮用户授权的维护者代理操作；不是独立人工发行批准，没有修改或移除环境保护规则。
- attempt 2 最终 Verify / Build / Sign & Notarize / Draft 全部 SUCCESS。正式资产保持 Draft，未发布。
- 官方 DMG SHA-256 `074b3ef3bd6dbfffd1bc7d59966fcf6ad3b324ee0ddf0a73f389dd7983cae68f` 与校验文件和 GitHub digest 一致，四份资产大小与 digest 均匹配。
- DMG Staple、App codesign deep/strict、Gatekeeper execute PASS；Developer ID Team `A2DYS82NLA`，Hardened Runtime。票据附于 DMG；App 未单独 Staple（返回65），不能声称全新 Mac 离线首装通过。
- GUI About 实测 `0.5.0-beta.7` / 冻结完整 RC / `2026-10-09T17:23:09Z` / beta。macOS ShortVersion `0.5.0`、build `7` 符合版本映射。旧 beta.6 App 已私有备份，未打开历史失败数据库。

## 夹具准备

五组全新人工可丢弃本地夹具：正常闭环、源变化 stale、恢复目标冲突、隔离内容变化、受保护目录。每组四个文件，独立源根、隔离区与根外对照。全部源文件 SHA-256/大小与独立备份一致；隔离区为空；独立项目数据库尚待正式 GUI 创建。没有真实 NAS 写入。

所有原始路径、文件名、业务测试锚点、数据库快照与日志只保留于本机私有归档。公开记录仅含脱敏汇总。

## 正式运行状态

| 门禁 | 当前状态 |
| --- | --- |
| 正式下载 SHA / 签名 / 公证 / Gatekeeper / About | PASS（本机在线基础检查；离线首装未验收） |
| GUI 扫描、目录语境、计划复核审批与 Dry Run | happy PASS，其余案例未执行 |
| 真实 Quarantine / 原路径 Restore | Quarantine PASS，Restore 执行工具阻断 |
| durable VERIFIED、重开及重复执行安全 | VERIFIED PASS，重开与重复操作待验 |
| stale executed=0 / failed=1 / DRAFT / 旧审批失效 | NOT RUN |
| 恢复冲突、隔离变化、保护及源根边界 | NOT RUN |
| 限定运行窗口事件 / stdout / stderr / 统一日志隐私 | NOT RUN |
| Crash Recovery / Recovery Lock 正式故障测试 | NOT RUN |

独立准备复核指出：`operation_plans.state` 为判定依据，`evidence_json` 内状态是旧快照；恢复完成后源计划仍 VERIFIED，隔离项及恢复计划才为 RESTORED；HOLD 限制 Purge，不能虚构后端 Restore 拒绝测试。

## 首组实际 GUI 证据（阶段记录，未最终通过）

- 正式 GUI 创建独立 happy 项目，只读核对登记源根与新夹具一致。
- 扫描自然 COMPLETED，文件 active=4、missing/unavailable=0；重复组1，两份独立物理副本完整 SHA 一致。已查看 temporary/normal 目录语境及保留理由。
- 保存草案、人工记录动作决定后，数据库仍 DRAFT，执行中心可执行数0。这里只证明 GUI 不提供执行资格，不冒称未发起的后端拒绝实测。
- 独立批准与 Dry Run：executed=0/skipped=1/failed=0，durable APPROVED；源4/4原始哈希一致、隔离区空、Journal/隔离项0、根外不变。
- 真实隔离：executed=1/failed=0，durable VERIFIED，唯一 QUARANTINED 项、唯一 done Journal。隔离目标 SHA/大小与初始清单一致，另外3个源文件及根外对照未变；已批准可执行列表归零。
- 恢复草案、审批、Dry Run 分开完成。恢复校验通过，restore plan APPROVED、restore Journal0，文件仍保持隔离后状态。
- Computer Use 坐标输入连续返回 `windowNotFoundAtPosition` / `noWindowsAvailable`；AX 将恢复试运行和执行合并为单元格，只能触发试运行。刷新绑定、Raise 和原生窗口菜单未解除定位问题；已请求用户点击当前唯一测试项的恢复执行按钮。工具阻断不能解释为产品 Restore FAIL 或 PASS。
- 当前采样：54标记/156编码变体；stdout/stderr、约366.8MB统一日志、11条 job_events、3条 operation_logs 均零命中。采集仍进行，未覆盖其余案例和重开后的新进程，不能作为最终隐私PASS。
- 恢复闭环、重开项目、stale、冲突、保护和最终隐私核验未完成，整体 INCONCLUSIVE。
