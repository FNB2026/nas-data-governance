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
- 已实现：executor 是唯一写入用户文件系统的包，所有写操作集中在 ops.go，其他包保持只读；DELETE 语义落到可恢复隔离，不提供不可回滚的永久清除。
- 验收：目标校验失败不删除源；状态变化使旧计划失效；回滚恢复原路径；不跟随符号链接；审计日志不含敏感路径。
- 已完成：MOVE/COPY/RENAME、DELETE→隔离、受根目录约束的空目录清理，以及 CLI approve/execute（含真实只读 dry-run 预检）。

## M4 常见格式分析（已完成）

- 已实现：基于 magic bytes 的格式检测（图片/视频/音频/PDF/压缩包；RIFF 子类型区分 WebP vs WAV；OOXML docx/xlsx/pptx 经 [Content_Types].xml 分流）、只读元数据提取（PNG/JPEG/GIF/BMP/WebP 尺寸、PDF 页数、ZIP 条目数、MP4/MOV/MKV 时长+编码、MP3 时长）、SQLite file_formats 持久化、CLI analyze 子命令（可选 --db 持久化、--storage-id/--limit 过滤）。
- 验收：遵循 K-006 渐进式分析，只读文件头，不解码媒体内容、不调用 OCR/AI；28 个测试覆盖所有检测器、元数据提取器、OOXML 分类与边界情况。

## M5 资产关系与智能整理（已完成）

- 已实现：资产组识别（按业务锚点或路径前缀聚类）、版本关系识别（同目录、同扩展名、版本标记剥离后基础名相同）、原始/派生关系识别（同目录、同基础名、同格式类别、不同扩展名；仅基于文件名+FormatInfo 类别，不做像素解码）、目录合并建议（兄弟目录名去 backup/copy 后缀后相同 + 文件名 Jaccard 重叠度 ≥ 0.5）、CLI group/relations/merge 子命令。
- 验收：全部只读，符合 K-002（近似、版本、派生、备份和交叉归档默认不得自动删除）和 K-008（合并建议需人工复核）；测试覆盖聚类、版本标记、派生对、合并建议、边界情况和确定性排序。
- 已完成 L1：本地规则模型、生命周期、SQLite 表、规则集加载与优先级保护；L2—L4 学习管线仍待实施。
