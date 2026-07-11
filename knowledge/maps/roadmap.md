# 开发路线

## M1 安全索引（当前基座）

- 只读文件扫描；SQLite 目标模式；快速指纹与 SHA-256；完全重复报告。
- 验收：不跟随符号链接、不跨挂载点、可处理中断与权限错误、测试不删除任何文件。

## M2 目录语境与去重计划（已完成）

- 已实现：基于路径信号的目录角色、敏感/原始/备份/系统保护、基于完整哈希的草案计划、冲突复核。
- 已完成：上级目录链结构化（1—6 层 ParentChain 与 BranchPoint）、业务锚点识别（项目代号、年份目录）、完整保留评分（Authority/Stability/PathDepth/RoleBonus 四项可解释分数）、SQLite 持久化层（storages/file_instances/directory_contexts/operation_tasks/operation_plans/operation_logs 读写）。
- 验收：内容相同但职责不同的文件默认保留；冲突进入人工复核；业务锚点不同的副本进入人工复核；保留项有可解释的评分理由。

## M3 安全执行（已完成）

- 已实现：执行前 stale 复核（path/size/mtime/inode/hash 五项比对，变化即退回 DRAFT）、Plan 状态机（DRAFT→APPROVED→STALE_CHECKED→EXECUTING→VERIFIED/ROLLED_BACK，禁止跳步）、隔离区路径管理（flat/dated 结构，冲突自动加序号）、跨卷复制-校验-删除源（复制成功不代表移动成功，校验失败不删源）、回滚机制（失败时逆序回滚已执行动作）、审计日志步骤（不记录路径/内容，仅记录字节数/stale 原因/错误类型）、显式 SourceRoots 边界与根目录内符号链接拒绝。
- 已实现：executor 是唯一写入用户文件系统的包，所有写操作集中在 ops.go，其他包保持只读。
- 验收：目标校验失败不删除源；状态变化使旧计划失效；回滚恢复原路径；不跟随符号链接；审计日志不含敏感路径。
- 未完成：CLI 接入（execute 子命令）、MOVE/COPY/DELETE/RENAME 动作实现、空目录清理。

## M4 常见格式分析

- 图片、视频、音频、PDF、Office、压缩包分析器。

## M5 资产关系与智能整理

- 原始/派生、版本、资产组、目录合并建议、本地语义辅助与规则学习。
