# 文档索引

当前公开版本：**[macOS beta.11](https://github.com/FNB2026/nas-data-governance/releases/tag/v0.5.0-beta.11)**、**[Linux Docker 只读 Preview.1](https://github.com/FNB2026/nas-data-governance/releases/tag/v0.5.0-docker-preview.1)**。Docker 暂不支持隔离、恢复或永久删除，fnOS 客户端入口仍 BLOCKED。详情见 [仓库首页](../README.md)。

文档按规范基线、架构决策、实施方案、专题提案及历史材料组织。遵守 `AGENTS.md` 与 ADR。旧文档内的 Draft/未发布状态属于当时的历史快照，不是当前发行裁定。

## 用户文档

- [NDG 用户指南](user-guide/NDG-用户指南.md)：安装、扫描、重复结果、目录语境保护、执行计划与回滚、数据库管理、故障排查、隐私安全声明。

## 多端产品和发行状态

- [公开入口、品牌与自动检查核查](development/PUBLIC-ENTRY-AUDIT-2026-10-15.md)。

- [Docker 预览版安装手册](multichannel/DOCKER-READONLY-PREVIEW.md) 与 [真实主机验收](multichannel/DOCKER-PREVIEW-RESULTS-2026-10-15.md)。
- [多渠道最新发布台账](multichannel/RELEASE-STATUS.md)、[能力矩阵](multichannel/CAPABILITY-MATRIX.md)及[UX v2 验收矩阵](ux/UX-V2-ACCEPTANCE.md)。
- [工程多端主路线](multichannel/NDG-MULTICHANNEL-MASTER.md)。
- [双端完整隔离／恢复／永久删除实施手册](development/NDG-DUAL-CHANNEL-FULL-GOVERNANCE-EXECUTION-2026-10-15.md)：新增的长期开发目标，目前**不是已发布功能**。

## 规范基线

- [白皮书](whitepaper.md)：领域模型、安全边界与治理方法。
- [NDG v0.5 Beta RC / Release Execution 执行操作手册](release/NDG-v0.5-Beta-RC-Release-Execution-Manual.md)：历史 RC 发行操作手册，不再作为实时进度或当前公开版本判断依据；真实发行身份以 GitHub Releases 和多渠道台账为准。
- [桌面端前端架构与后端协同推进方案](desktop-frontend-architecture.md)：七域页面、Binding/DTO、执行安全接线、Phase 0—7、测试矩阵与 DoD；当前桌面实施主文档。
- [桌面信息架构](ui/information-architecture.md)：七域导航与页面职责摘要。

## 架构与边界

- [ADR](adr/)：已接受的架构决策记录。
- [开源边界](open-source-boundary.md)：公开与私有资料边界。
- [公开仓库审计](open-source-audit-2026-07-15.md)：特定日期的发布审计快照。

## 路线与专题

- [NDG 可视化开发路线手册](《NDG%20可视化开发路线手册》.md)及其[校正与修订指南](《NDG%20可视化开发路线手册》校正与修订指南.md)。
- [本地语义学习提案](proposals/local-semantic-learning.md)。

## 发布验收证据

- [Resume 修复证据（2026-09-29）](release/resume-prefix-repair-2026-09-29.md)：开发修复及早期验证快照。
- [Resume 完成验收（2026-10-02）](release/resume-fix-validation-2026-10-02.md)：开发版自然完成、前缀保留、失败与隐私边界。

## 历史方案

- [桌面端前端重构执行方案](frontend-execution-plan.md)：早期五页面拆分方案，已被七域主文档取代，仅保留作迁移背景与历史参考。
- [v0.5 发布准备现状审计](release/readiness-audit-v0.5.md)与[发布门槛清单](release/release-gates-v0.5.md)：2026-07-30 历史审计依据，不承担实时进度管理。

文档中的“已完成”属于其标注日期的事实快照；实施前仍需以当前代码、生成 Binding、数据库迁移和验证结果重新校准。
