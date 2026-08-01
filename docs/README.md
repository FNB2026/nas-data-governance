# 文档索引

本目录按“规范基线—架构决策—实施方案—专题提案—历史材料”组织。发生冲突时，先遵守 `AGENTS.md` 与 ADR，再以标记为现行的实施主文档为准。

## 用户文档

- [NDG 用户指南](user-guide/NDG-用户指南.md)：安装、扫描、重复结果、目录语境保护、执行计划与回滚、数据库管理、故障排查、隐私安全声明。

## 规范基线

- [白皮书](whitepaper.md)：领域模型、安全边界与治理方法。
- [桌面端前端架构与后端协同推进方案](desktop-frontend-architecture.md)：七域页面、Binding/DTO、执行安全接线、Phase 0—7、测试矩阵与 DoD；当前桌面实施主文档。
- [桌面信息架构](ui/information-architecture.md)：七域导航与页面职责摘要。

## 架构与边界

- [ADR](adr/)：已接受的架构决策记录。
- [开源边界](open-source-boundary.md)：公开与私有资料边界。
- [公开仓库审计](open-source-audit-2026-07-15.md)：特定日期的发布审计快照。

## 路线与专题

- [NDG 可视化开发路线手册](《NDG%20可视化开发路线手册》.md)及其[校正与修订指南](《NDG%20可视化开发路线手册》校正与修订指南.md)。
- [本地语义学习提案](proposals/local-semantic-learning.md)。

## 历史方案

- [桌面端前端重构执行方案](frontend-execution-plan.md)：早期五页面拆分方案，已被七域主文档取代，仅保留作迁移背景与历史参考。

文档中的“已完成”属于其标注日期的事实快照；实施前仍需以当前代码、生成 Binding、数据库迁移和验证结果重新校准。
