# beta.7 正式发行候选准备 — 2026-10-10

## 身份与发布边界

- 产品修复：[PR #64](https://github.com/FNB2026/nas-data-governance/pull/64)，Issue #62。main 修复合并提交 `ed691a15a4d3c2aa951d6df875c9134bd05dfded`。
- 新版本 `0.5.0-beta.7`，Bundle build `7`，通道 `beta`。本 PR 仅同步版本、CHANGELOG 和发行准备文档。
- 最终 RC SHA 必须在版本 PR 独立复核、CI / Security、合并后 Gate 通过后冻结，不能把 #64 的 merge SHA 当正式发行身份。
- 本阶段未创建 Tag、DMG 或 Release。签名 App、GUI Quarantine/Restore、隐私与 Crash 验收不能由源码测试替代。
- beta.6 正式 Quarantine / Restore **FAIL** 保留。历史 Tag / DMG / 失败数据库 / 日志不修改。Public Beta **BLOCKED**，所有候选 **Draft**，不执行公开发布。

## 工程证据

修复精确 HEAD `94461a55a66cc092f38eee19d1f5edb0725db558` 已独立 APPROVE；CI Verify / macOS Desktop Build 与 Security Gitleaks / Govulncheck 均 SUCCESS。全量本机 Go race / vet / lint、203 前端测试与生产构建、public/version 和发行脚本结构测试通过。main post-merge CI 与 Security 分别核对，不借用 PR 结果作为 main Gate。

## 正式流水线

1. 核对版本提交、main、工作树、独立审查与 exact-SHA CI / Security。
2. 检查远端没有 beta.7 Tag/Release；创建新 annotated `v0.5.0-beta.7`，禁止 force 或移动旧 Tag。
3. 使用既有 Release：Verify → Build Unsigned → Developer ID 签名 / Hardened Runtime → Apple Notarization → Staple → Gatekeeper → Draft Release。
4. DMG、SHA-256、CycloneDX/SPDX SBOM 均绑定同一冻结 RC SHA。
5. 下载 GitHub Draft 正式 DMG，校验 SHA-256、签名、公证、Gatekeeper 与 GUI About（版本 / 完整 Commit / 时间 / channel）。新候选基础证据另记，不覆盖旧报告。

## 必须执行的正式 GUI 复验

全新人工 disposable 源、独立项目数据库、独立隔离区与逐文件 SHA-256/大小/相对路径清单及备份。明确 SourceRoots 与 QuarantineRoot；任何真实 NAS、历史百万级项目和原失败数据库均不作为执行对象。

| 门禁 | 本 PR 状态 | 要求 |
| --- | --- | --- |
| 签名发行物基础检查 | NOT RUN | 官方 Draft 下载资产、签名/公证/About 一致 |
| 隔离与原路径恢复 | NOT RUN | 正式 GUI 人工复核/审批/Dry Run/执行；SHA/大小、非目标与根外字节不变 |
| 计划终态持久化 | NOT RUN | GUI/DB/Journal/Audit VERIFIED 一致，重开后不可再执行 |
| stale 与审批失效 | NOT RUN | executed=0、failed=1、durable DRAFT，旧批准请求无执行资格 |
| 重复执行安全 | NOT RUN | 没有第二次移动、重复隔离项或错误删除 |
| 冲突/变化/HOLD/源根保护 | NOT RUN | 独立夹具保持安全拒绝与内容保护 |
| 限定窗口隐私 | NOT RUN | GUI事件、stdout/stderr、统一日志中路径/文件名/业务测试锚点零泄漏 |
| Crash Recovery / Recovery Lock | NOT RUN | 上述正式门禁通过后单独执行，不在本轮主动崩溃 |
| 全新 Mac 离线首装 | BLOCKED | 需要未运行 beta.7 的独立设备证据 |
| 长时隐私与依赖安全项 | 未关闭 | 不以总体 CI 绿代替 source-map-js 告警及更广进程证据 |

未实际执行、未观察到要求状态或证据不可读取时记录 BLOCKED / INCONCLUSIVE，不宣称 PASS。本轮不进入 Purge，不发布 Release。
