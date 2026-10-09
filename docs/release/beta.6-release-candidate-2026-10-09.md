# NDG beta.6 正式发行候选 — 身份与验收计划

## 冻结边界

- 修复基线：main `2d360ca40fd9e4a2ef26f89df9a836e1d8c7aba8`，[PR #57](https://github.com/FNB2026/nas-data-governance/pull/57)。这不是最终发行 SHA。
- 新身份：`VERSION=0.5.0-beta.6`，`BUNDLE_BUILD_NUMBER=6`；CFBundleShortVersionString `0.5.0`，CFBundleVersion `6`；channel `beta`；新的 annotated Tag `v0.5.0-beta.6`。
- 最终 RC SHA 必须在版本 PR 经 CI、独立复核并合并后冻结；正式 App About 必须是完整的该 SHA，不能使用修复 PR 的 merge SHA。
- 仅使用既有 Release 工作流创建新的 Draft/pre-release。禁止发布、修改 beta.5 Tag 或资产、覆盖历史失败记录。
- 当前候选准备状态不是发行物验收通过。各实际结果由版本 PR / 后续发行审计记录注明 SHA、workflow run、资产 ID 和证据层。

## 变更影响与必需复验

本次含扫描恢复代码、Go 1.26.9 工具链和重建/重新签名产物。不是仅文档发布；之前 beta.5 的安装、签名和隐私结果不能自动转成 beta.6 PASS。

| Gate | beta.6 要求 | 当前状态 |
|---|---|---|
| 版本、最终 SHA、构建号与通道 | PR/CI/独立复核；Tag peel、About 完整 Commit 一致 | PASS — PR #59 / RC 42397a9 / About 实测 |
| GitHub 最终 DMG | 从新 Draft 下载，SHA256、Developer ID、Notarization Accepted、DMG staple、Gatekeeper | PASS — [正式发行物验收](beta.6-formal-artifact-smb-acceptance-2026-10-09.md) |
| 实际安装启动 | 新发行物安装、GUI About、最小只读项目验证 | PASS — 正式 App 安装启动及 About |
| 全新 Mac 离线首次安装启动 | 新 DMG 在未运行该候选的 Mac 上，无“仍要打开”/清除 quarantine 的绕过；需要独立机器实际证据 | BLOCKED — 缺 beta.6 独立机器实际证据 |
| SMB 基线/网络暂停/恢复 | 独立 disposable 共享、新 DB，实际状态链、候选覆盖、计数与源一致性 | PASS — 实际 FULL_HASHING / PAUSED_NETWORK / Resume COMPLETED |
| 异常运行日志隐私 | GUI events、stdout/stderr、统一日志；合成路径/文件名/业务锚点零泄漏 | PASS — 本轮限定异常窗口；长时门禁另列 |
| Disposable Quarantine / Restore | SMB 与必要隐私 PASS 后进入 | NOT RUN |
| Crash Recovery / Recovery Lock | 前置 Gate 完成后进入 | NOT RUN |
| 长时隐私与其他发布门禁 | 独立保留逐项实际结论 | NOT RUN |
| Public Beta | 全部剩余 Gate 完成后独立判定 | BLOCKED |

未有本次实际机器/发行物证据的项目保持 NOT RUN/BLOCKED；历史 beta.5 首装证据可作对照，不能作为本次零接触离线首装证据。DMG 已 staple 不等于 nested App 已 staple；按既有资产事实分别记录，不编造 App ticket。

## 正式发行工作流

1. 洁净 checkout + 最终 main Gate → 冻结 RC SHA → 新 annotated Tag，核对远端 Tag object / peel；旧 beta.5 object / peel 保持不变。
2. Release Verify → Build Unsigned .app（注入完整 SHA、VERSION、BuildTime、Channel）→ Developer ID Application + Hardened Runtime → signed DMG → Apple Notarization Accepted → staple → Gatekeeper 与 checksum。
3. CycloneDX / SPDX SBOM 和 CHANGELOG-derived Draft notes；记录 workflow ID、signed artifact ID、最终资产 ID/size/SHA256。
4. 最终 GUI 验收只使用 GitHub Draft 下载的最终 DMG。临时 Actions 目录或本地 dev build 不可替代。

## Disposable SMB 正式复验协议

- 只允许独立测试服务/共享，确认只影响该服务会话；不关闭整机网络，不卸载真实 NAS 共享。
- 全新项目 DB，与 beta.5 失败项目、百万级 NAS 原项目及验证副本分开。
- 七个合成文件：三对重复内容 + 一个普通单例。记录只含匿名标签和数量，普通日志不得含源路径、文件名、业务锚点或凭据。
- 正常 baseline；在实际 GUI FULL_HASHING 时中断专用 SMB 服务；实际观察 PAUSED_NETWORK 和 durable checkpoint。停止/重启测试服务与重新挂载仅作用于专用测试挂载。
- 正式 GUI Resume，不以重新全量扫描掩盖问题；等待自然 COMPLETED。
- 要求六个候选 full 6/6、重复组 3/3、单例不强制 full，七个文件覆盖与 durable prefix 不减少；missing/unavailable=0、源内容前后 SHA256一致；checkpoint completed、Resume 入口消失、无未解释候选待办。
- Capture GUI event、stdout/stderr、统一日志的完整异常窗口；保存原始私有证据和脱敏报告，核对合成隐私锚点命中数。
- 没实际触发中断/没观察到暂停/未自然完成 → INCONCLUSIVE 或 BLOCKED；失败 → FAIL；不通过计数增长推断 PASS。

## 历史记录

beta.5 发行物 `8473dd630c4165389b32ffc85ae062145546a766` 的 SMB Resume **FAIL** 永久保留；源保护/网络状态链 PASS 与 full 2/6、重复组 1/3 的内容完成缺陷分开。根因与修复：[修复报告](resume-full-hash-recovery-fix-2026-10-09.md)。原数据库、日志和夹具不作为本次执行对象。

## 已执行记录

正式 RC SHA `42397a9196ae5c0621aacec3913970b7269bae24`；[不可变发行物与真实 SMB 验收记录](beta.6-formal-artifact-smb-acceptance-2026-10-09.md)列出签名、公证、资产身份、两次 INCONCLUSIVE 尝试、最终 PASS 链和未完成门禁。本记录只更新文档，不改变 Tag 或资产。
