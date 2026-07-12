# Changelog

本项目的显著变更按里程碑组织。每个里程碑对应 [开发路线](knowledge/maps/roadmap.md) 中的章节。

## v1.0.0 — 生产化收束

首个生产就绪版本。完成 M1–M6 全部里程碑，提供可在真实 NAS 上运行的安全索引、分析、计划、执行与学习闭环。

### M6 生产化收束

- **P0-1 持久化执行日志与崩溃恢复**（`633c9ac`）：`execution_journal` 表记录每个写动作生命周期；`Recover()` 在启动时回滚未完成动作或重置中断计划；CLI `recover --db`。
- **P0-2 增量扫描与断点续扫**（`c7a1ddc`）：size+mtime+inode 三元组哈希缓存复用；`scan_checkpoints` 表断点续扫；已删除文件标记为 missing 而非物理删除；自定义 BFS 遍历支持 context 优雅中断。
- **P0-3 真实 NAS 故障演练**（`e3363d9`）：隔离临时目录中 4 个只读场景；报告脱敏，临时目录前缀替换为 `<tmp>`。
- **P1-4 任务队列与资源控制**（`b1667d2`）：`internal/runner` worker pool，semaphore 并发控制；CLI `--workers N`。
- **P1-5 人工复核管理界面**（`fd4124a`）：`review` 子命令覆盖 plans/rules/merges/conflicts 四条线；规则全链路 draft→probation→approved。
- **P1-6 补齐底层测试**（`c81df09`）：6 个包 34 个新测试，覆盖此前未触达分支；全量 `go test ./... -race` 17 包通过。
- **P2-7 文档状态同步**（`2e5eb5d`）：roadmap/README/ADR/知识卡全部与代码实现对齐。

### M5 资产关系与智能整理

- 资产组识别（业务锚点或路径前缀聚类）、版本关系、派生关系、目录合并建议。
- L1 本地规则模型 + 生命周期 + SQLite 持久化。
- L2 本地统计学习：遍历 SQLite 索引统计目录名频次，生成规则草案。
- L3 行业资料学习：读取 TXT/MD/DOCX/PDF 提取术语，CJK n-gram 分词。
- L4 决策反馈学习：统计历史 plan 偏差，生成权重调整建议。

### M4 常见格式分析

- magic bytes 格式检测（图片/视频/音频/PDF/压缩包）；RIFF 子类型区分；OOXML 分流。
- 只读元数据提取（PNG/JPEG/GIF/BMP/WebP 尺寸、PDF 页数、ZIP 条目数、MP4/MOV/MKV 时长、MP3 时长）。

### M3 安全执行

- 执行前 stale 复核（path/size/mtime/inode/hash 五项比对）。
- Plan 状态机（DRAFT→APPROVED→STALE_CHECKED→EXECUTING→VERIFIED/ROLLED_BACK）。
- 隔离区路径管理；跨卷复制-校验-删除源；回滚机制。
- MOVE/COPY/RENAME、DELETE→隔离；CLI approve/execute。

### M2 目录语境与去重计划

- 目录角色分类（敏感/原始/备份/系统/项目/缓存/归档等）；上级目录链（1—6 层）；业务锚点（项目代号、年份目录）。
- 完整保留评分（Authority/Stability/PathDepth/RoleBonus）；冲突复核。

### M1 安全索引

- 只读文件扫描；SHA-256 分层哈希；完全重复报告。
- 不跟随符号链接、不跨挂载点、可处理中断与权限错误。
