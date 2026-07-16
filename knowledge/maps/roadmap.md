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
- 已完成 L1：本地规则模型、生命周期、SQLite 表、规则集加载与优先级保护。
- 已完成 L2：本地统计学习（learning 包）。`Learn()` 遍历 SQLite 索引统计目录名频次与共现，跳过敏感目录（K-009）与 builtin 已覆盖术语；`GenerateDrafts()` 生成 status=draft 规则草案并持久化，强制 priority≤60（K-008），保留已审批规则不被覆盖；co-occurrence 平局确定性打破（内置权威优先→术语字典序）；CLI `learn --db [--apply] [--out]` 子命令，默认只读干跑。
- 已完成 L3：行业资料学习（corpus）。`LearnFromCorpus()` 读取受信目录 `var/corpus/` 的 TXT/MD/DOCX/PDF 资料提取文本（PDF 用纯 Go 库 dslipak/pdf，无 OCR/外部服务），CJK 2-4 字 n-gram 分词，跳过 builtin 已覆盖术语；行业敏感词（诊断/处方等）路由到 SensitiveCandidates 生成 RoleSensitive 草案；`GenerateCorpusDrafts()` 持久化 status=draft 规则，priority≤60（K-008），保留已审批规则不被覆盖；CLI `learn --source=corpus --corpus-dir [--apply] [--out]` 子命令，默认只读干跑。
- 已完成 L4：决策反馈学习（feedback）。`LearnFromFeedback()` 遍历历史 operation_plans，统计保留副本与评分排名的偏差，生成权重调整建议（authority/stability/path_depth/role_bonus 四维度，单次 ±3 分上限）；检测 learned 规则在被拒绝 plan 的 evidence 中的出现率，超过阈值（默认 0.5）生成置信度降级建议（单次 ≤0.2）；`GenerateFeedbackDrafts()` 持久化权重调整规则草案 + 直接降级 draft/probation 规则的 confidence（approved 不动）；store 新增 ListTasks + ListAllPlans 跨任务遍历历史；CLI `learn --source=feedback --db [--apply] [--out]` 子命令，默认只读干跑。

## M6 生产化收束（已完成）

本里程碑把 M1–M5 的功能基座推向可在真实 NAS 上运行的生产形态，不改变"默认只读、保护优先"的安全边界。

- P0-1 持久化执行日志与崩溃恢复（commit `633c9ac`，开源前进一步 fail-closed）：新增 `execution_journal` 表（schema 003），记录每个文件系统动作的生命周期（pending→done/failed→rolled_back）与实际目标路径；非 dry-run CLI 强制要求 SQLite `--db`，`BeginJournal` 或 `MarkJournalDone` 失败即停止并在需要时回滚，禁止无持久化日志继续写文件；`Recover()` 在启动时扫描未完成动作。测试覆盖日志幂等、崩溃恢复以及 Journal 失败不写入/回滚场景。
- P0-2 增量扫描与断点续扫（commit `c7a1ddc`）：新增 `scan_checkpoints` 表（schema 004）与 `file_instances.file_status` 列（active/missing）；扫描前从 DB 加载 `ListFileMetadata` 缓存，比较 size+mtime+inode 三元组复用既有 SHA-256（哈希缓存复用）；自定义 BFS 遍历替代 `filepath.WalkDir`，每轮迭代检查 `ctx.Err()` 支持优雅中断；单目录 `os.ReadDir` 失败记入 `Stats.Errors` 并 continue，不终止整个扫描；扫描后 `MarkFilesMissing` 标记已删除文件为 missing（非物理删除）；`ResumePath` 选项支持断点续扫。store 层 6 个测试 + scanner 层 11 个测试。
- P0-3 真实 NAS 故障演练（commit `e3363d9`）：在 `t.TempDir()` 隔离环境中跑 4 个只读场景（崩溃恢复、中断续扫、stale 检测、权限错误容忍）；报告通过 `sanitize()` 函数脱敏，把 `os.TempDir()` 前缀替换为 `<tmp>`，不暴露机器 hash 与绝对路径；placeholder 死代码清理。
- P1-4 任务队列与资源控制（commit `b1667d2`）：新增 `internal/runner` worker pool，semaphore 通道控制并发，`Submit/Wait/Run` API，context-aware，单失败不阻塞其他任务；files 切片 mutex 保护，计数器用 atomic；`domain.TaskState` 枚举（queued/running/completed/failed/cancelled）+ store `UpdateTaskState`；CLI `scan --workers N`、`analyze --workers N`。8 个测试。
- P1-5 人工复核管理界面（commit `fd4124a`）：新增 `review` 子命令，4 个子动作：`plans`（审阅待批准计划）、`rules`（审阅 draft/probation 规则，支持 approve/reject）、`merges`（审阅合并建议）、`conflicts`（审阅冲突计划）；`bufio.Reader` 逐行确认，`filepath.Base()` 路径脱敏，REVIEW→SKIP 转换；填补 probation→approved CLI 缺口，规则全链路 draft→probation→approved 可在 CLI 完成。6 个测试。
- P1-6 补齐底层测试（commit `c81df09`）：覆盖 6 个包此前未测试的分支——internal/index JSONL 往返与错误路径（7 测试）、planner 默认 REVIEW 与 Cache 角色 QUARANTINE（2 测试）、dircontext 六个目录角色正面测试（7 测试）、merge pickTarget 双后缀与中文 _副本/_temp/_old/_new（5 测试）、relations Audio 派生与 _old/_new/_backup/副本 版本标记（6 测试）、format WebP/HEIC/AVI/TAR/RAR/Bzip2 检测（7 测试）。新增 481 行测试，全量 `go test ./... -race` 17 包通过。

验收：执行动作可崩溃恢复；扫描可中断续扫且不丢失既有哈希；并发受 worker pool 约束；人工复核覆盖计划/规则/合并/冲突四条线；底层测试覆盖此前未触达的分支。全部遵守 AGENTS.md 工程护栏（默认只读、不跟随符号链接、保护优先、审计不泄露路径）。
