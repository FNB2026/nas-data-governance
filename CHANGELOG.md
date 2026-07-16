# Changelog

本项目的显著变更按里程碑组织。每个里程碑对应 [开发路线](knowledge/maps/roadmap.md) 中的章节。

## Unreleased

- 开源前安全收口：非 dry-run `execute` 强制要求 SQLite `--db`，Journal 初始化或动作完成记录失败即停止并回滚；核心索引、计划、审计和 SQLite 产物统一为目录 `0700`、文件 `0600`，并收紧既有宽权限文件。
- 源根目录和隔离根目录必须是互不重叠的真实目录且不能是符号链接；根目录自身也纳入执行前符号链接检查。
- Go module 统一为 `github.com/FNB2026/nas-data-governance`；构建写入版本、提交和构建时间，增加 `version` 子命令及 macOS/Linux 兼容的 SHA-256 发布校验。
- README 将首次体验限制为只读分析和 `execute --dry-run`，真实写操作移入高级风险章节并禁止默认使用全量审批。
- 扫描日志安全收口：普通错误输出只保留阶段与聚合数量，SQLite upsert 错误使用行号而非源路径；`scanner.Stats.FormatErrors` 不再输出路径。新增真实 SMB 高位 inode 回归，确认无符号索引与 SQLite 按位存储数量一致。
- 新增 `diagnose-paths` 私有只读兼容性诊断：在任务根目录内检查历史失败路径、NFC/NFD 变体和规范化同名目录项，拒绝符号链接、跨挂载点与越界路径，不读取文件内容、不自动重命名。
- 新增 `diagnose-merges` 私有门控报告：统计兄弟目录对、名称相似对及 Jaccard 0.10/0.25/0.50 分档；保持生产合并阈值 0.5 不变，近似候选只供人工复核。
- P2 审慎治理建议：新增 `diagnose-governance` 私有只读报告，合并完全重复 DRAFT 计划、零字节保守分类和大容量音视频格式/编码/版本/派生/侧车/目录职责证据。报告固定 `execution_authorized=false`，拒绝非 DRAFT 结果，不持久化审批或调用执行器。
- 零字节文件无论命中占位、失败输出、潜在临时产物或无法解释分类，都只生成 `KEEP_AND_REVIEW`；理论重复容量不等于可删除容量。
- P1 分析准确率：新增 `diagnose-formats` 私有复核报告，聚合大型 unknown、扩展名/文件头冲突和媒体元数据缺口；报告强制 `0600`，普通日志不输出路径。
- 媒体只读解析扩展：WAV/AIFF/FLAC 时长与编码，MP4/MOV/M4V 尾部 `moov` 的时长/尺寸/编码，AVI 时长/尺寸/编码，MPEG 帧尺寸；修复 AVI RIFF 子类和 M4A 检测顺序。`analyze --refresh-metadata` 只重试当前能力范围内的缺口。
- 目录职责规则升级为 `builtin-v2`：新增录音/素材/母带、制作/后期/剪辑/设计、成品/播出版等明确语义；音乐/资料等模糊词继续保留 unknown。
- P0 真实数据分析收敛：超大资产组以 10,000 成员为安全上限分层拆分并强制人工复核，evidence 只保留聚合规则说明、不重复成员路径；新增 AIFF/AIFC、OLE DOC/XLS/PPT、PSD 与工程侧车识别；`analyze` 新增断点复用、unknown 定向刷新、SQLite 分批持久化和聚合进度。
- 建立侧车依赖保护：XMP/CPR/SESX/PSD 及 PEAK/PKF/PEK/CFA/MPGINDEX 默认不参与自动清理；可再生成标记不等于可删除。
- 新增哈希失败闭环：扫描有限重试、失败记录保留、`0600` 私有清单，以及带根目录/符号链接/挂载点/stale 校验的 `retry-hashes` 局部补扫；补扫只生成新索引，数据库补录继续由独立的 `import-index` 完成。
- 格式分析数据库告警改为仅输出聚合数量，报告文件强制使用 `0600` 权限，避免普通日志和宽松文件权限暴露敏感文件名与路径。
- 新增 `import-index`：先校验完整 JSONL，再分批幂等导入 SQLite 并重建目录语境，全程不访问 NAS。
- 支持高位 SMB device/inode 在 SQLite 中无损往返。
- 扫描错误日志脱敏，并收紧资产分组与版本关系识别。

## v1.0.0 — 私有内部里程碑（未公开发布）

此版本号对应历史重写前的私有里程碑；Release 与标签已经删除，不得重建或复用。首个公开版本将使用新的预发布版本号。

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
