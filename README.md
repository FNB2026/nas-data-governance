# NAS Data Governance

面向个人、家庭和组织数字资产的本地化数据治理与归档工程基座。

本仓库实现安全索引、目录语境、去重计划与受限的安全执行流水线：只读扫描、元数据采集、分层哈希、完全重复报告、可解释的目录角色、草案计划、审批与执行。任何文件写入只能通过显式批准的计划和配置了源根目录的执行器发生。

方法论与安全边界见脱敏后的 [NAS 数据治理与安全归档白皮书](docs/whitepaper.md)。公开范围、禁止提交的资料及发布检查清单见 [开源边界](docs/open-source-boundary.md)。

## 许可

- 软件代码、构建文件和实现类 schema： [Apache License 2.0](LICENSE)。
- 原创文档与白皮书： [CC BY 4.0](LICENSE-DOCS.md)。
- 第三方组件：按 [第三方声明](THIRD_PARTY_NOTICES.md) 及随发布包提供的上游许可文本执行。

用于内部研究的原始 DOCX、XMind、真实目录树和扫描资料不属于公开仓库，也不受上述文档许可授权。

## 快速开始：安全体验

以下命令只读取样本数据并生成项目私有产物，不执行移动、隔离、重命名或删除动作。首次使用请只针对复制出来的合成测试目录。

```bash
make test
make build
./bin/nas-governance scan --root /path/to/read-only-sample --out ./var/index.jsonl --db ./var/governance.db --workers 4
./bin/nas-governance retry-hashes --root /path/to/read-only-sample --failures ./var/index.jsonl.hash-failures.jsonl --index ./var/index.jsonl --out ./var/index-recovered.jsonl
./bin/nas-governance import-index --index ./var/index.jsonl --db ./var/governance.db --batch-size 5000
./bin/nas-governance analyze --index ./var/index.jsonl --out ./var/formats.json --db ./var/governance.db --workers 4 --batch-size 500
# 中断后复用已落库记录；规则升级后可只重新分析旧 unknown
./bin/nas-governance analyze --index ./var/index.jsonl --out ./var/formats.json --db ./var/governance.db --workers 4 --resume --refresh-unknown
./bin/nas-governance analyze --index ./var/index.jsonl --out ./var/formats.json --db ./var/governance.db --workers 8 --resume --refresh-metadata
./bin/nas-governance diagnose-formats --db ./var/governance.db --out ./var/format-review-private.json
./bin/nas-governance diagnose-governance --db ./var/governance.db --out ./var/governance-review-private.json
./bin/nas-governance diagnose-paths --root /path/to/read-only-sample --legacy-log ./var/private-scan.log --out ./var/path-compat-private.json
./bin/nas-governance diagnose-paths --root /path/to/read-only-sample --failures-manifest ./var/index.jsonl.hash-failures.jsonl --out ./var/path-compat-private.json
./bin/nas-governance diagnose-merges --index ./var/index.jsonl --out ./var/merge-review-private.json
./bin/nas-governance group --index ./var/index.jsonl
./bin/nas-governance relations --index ./var/index.jsonl
./bin/nas-governance merge --index ./var/index.jsonl
./bin/nas-governance plan --index ./var/index.jsonl --out ./var/plan.json
./bin/nas-governance review plans --plan ./var/plan.json
./bin/nas-governance review rules --db ./var/governance.db
./bin/nas-governance version
```

## 高级风险操作

文件写操作不属于快速开始。执行前必须同时满足：已有独立备份；先在复制的测试目录验证；隔离区位于所有源根目录之外；人工逐项批准计划；持久化 SQLite Journal 可用。首次使用不推荐 `approve --all`。

先批准明确选中的计划，并执行完全只读的预检：

```bash
./bin/nas-governance approve --plan ./var/plan.json --out ./var/approved.json --plan-id PLAN_ID
./bin/nas-governance execute --dry-run --plan ./var/approved.json --quarantine /path/outside/source/quarantine --source-root /path/to/copied-test-data --out ./var/audit-dry-run.json
```

人工复核 dry-run 审计结果后，真实执行必须提供 `--db`。Journal 初始化或动作完成记录失败时，执行器会在下一次文件写入前停止并回滚已完成动作：

```bash
./bin/nas-governance execute --plan ./var/approved.json --quarantine /path/outside/source/quarantine --source-root /path/to/copied-test-data --db ./var/governance.db --out ./var/audit.json
./bin/nas-governance recover --db ./var/governance.db
```

## 支持平台

发布包覆盖 `darwin/arm64`、`darwin/amd64`、`linux/arm64` 和 `linux/amd64`。其他平台可从源码构建，但不属于首个公开版本的发布矩阵。

扫描默认不跟随符号链接，不跨挂载点，不读取文件内容之外的外部服务，不上传任何数据。完整哈希采用 SHA-256；文件先按大小分组，只有潜在重复项才计算完整哈希。增量扫描复用 size+mtime+inode 三元组匹配的既有哈希，断点续扫通过 `scan_checkpoints` 表记录进度，已删除文件标记为 missing 而非物理删除。

哈希读取默认最多尝试 3 次。仍失败的文件会保留在索引中（哈希字段为空），并写入默认权限为 `0600` 的 `<index>.hash-failures.jsonl` 私有清单；普通日志只报告数量，不输出路径或文件名。`retry-hashes` 会再次核对任务根目录、符号链接、挂载点以及 size+mtime+inode stale 状态，只读取通过检查的目标，并生成新的补录索引和剩余失败清单。它拒绝原地覆盖源索引，也不直接修改数据库；复核新索引后，再单独运行 `import-index` 幂等补录。

`import-index` 先完整校验 JSONL，再分批幂等导入 SQLite 并重建目录语境。该命令只读取本地索引，不访问 NAS，适合在扫描已完成但数据库持久化失败时恢复。

`analyze --resume` 以 SQLite 中已持久化的格式记录为检查点，跳过已完成项的 NAS 读取；`--refresh-unknown` 仅重新分析旧版本未识别项，`--refresh-metadata` 仅重新分析当前解析器能补齐的媒体元数据缺口。结果按 `--batch-size` 分批事务落库，进度日志只输出聚合数量。中断时保留已落库进度，不覆盖完整报告。

`diagnose-formats` 只读项目 SQLite，生成默认 `0600` 的私有人工复核报告：列出大型 unknown、扩展名/文件头冲突和按格式聚合的元数据缺口。终端只显示数量，路径仅保留在私有报告中；诊断不会自动重命名文件。

`diagnose-governance` 也只读项目 SQLite：对完全重复组生成仅 DRAFT 的复核计划，对零字节文件区分占位标记/失败产物/潜在临时产物/无法解释空文件，并聚合大容量媒体的格式、编码、时长、尺寸、目录职责及版本/派生/侧车关系。该命令不写审批状态，不保存可执行任务，不调用执行器；任何非 DRAFT 结果都会被拒绝写入。

`diagnose-paths` 可读取私有历史扫描日志或扫描产生的 `0600` 哈希失败清单，在显式任务根目录内检查原路径、NFC/NFD 文件名变体以及“目录项可见但无法只读打开”的状态。它只会为可访问性诊断尝试只读 Open 后立即关闭，不读取文件内容；不跟随符号链接、不跨挂载点、不越过任务根目录，结果固定为不可执行的 `0600` 私有报告。规范化特征只作为相关性证据，不能触发自动重命名或归因客户端/服务端根因。

`diagnose-merges` 只读取索引并解释合并门控：兄弟目录总对数、有限后缀归一后的名称相似对，以及文件名 Jaccard 0.10/0.25/0.50 分档。生产 `merge` 阈值仍为 0.5；诊断中的近似目录对不会生成审批或执行任务。

媒体元数据仍坚持只读、不解码：WAV/AIFF/FLAC 从有界文件头计算时长与编码；MP4/MOV/M4V 按 atom 边界读取 `moov`，支持尾部 `moov`；AVI 读取 `avih`；MPEG 仅提取可验证的帧尺寸，不使用不可靠码率估算时长。

资产组超过 10,000 个成员时会按更深相对路径拆分，无法继续分辨时才使用稳定分片，所有拆分结果都标记为人工复核。XMP、CPR、SESX、PSD 及 PEAK/PKF/PEK/CFA/MPGINDEX 等项目源文件与侧车依赖默认受保护；“可再生成缓存”只是分类，不授予删除权限。

## 仓库结构

```text
cmd/nas-governance/       CLI 入口（scan/retry-hashes/import-index/analyze/group/relations/merge/plan/approve/execute/recover/learn/review）
internal/domain/          核心对象与安全约束
internal/scanner/         只读文件系统扫描（context-aware BFS、增量哈希复用、断点续扫）
internal/fingerprint/     快速指纹与完整哈希
internal/index/           JSONL 索引适配器
internal/format/          基于 magic bytes 的格式检测与只读元数据提取
internal/formatdiag/      私有大型 unknown、扩展名冲突与元数据缺口诊断
internal/governancediag/  私有重复/零字节/大媒体 DRAFT 治理复核
internal/pathdiag/        私有 CJK/特殊字符路径兼容性诊断
internal/filepolicy/      项目源文件、侧车和可再生缓存的保护策略
internal/dircontext/      目录角色分类、上级目录链与业务锚点
internal/relations/       版本与派生关系识别
internal/assets/          资产组识别
internal/merge/           目录合并建议
internal/planner/         草案计划与可解释保留评分
internal/learning/        L2 统计学习、L3 行业资料学习、L4 决策反馈学习
internal/store/           SQLite 持久化层（项目自身数据库）
internal/executor/        安全执行器（stale 复核、隔离、跨卷复制-校验-删除源、回滚、执行日志）
internal/runner/          worker pool（semaphore 并发控制）
internal/drill/           NAS 故障演练（隔离只读场景）
internal/report/          完全重复报告
knowledge/cards/          白皮书知识卡
knowledge/maps/           概念关系与开发路线
schemas/                  SQLite 目标模式与规则示例
docs/adr/                 架构决策记录
```

## 当前边界

- 已有：只读扫描（含增量、断点续扫、哈希有限重试与失败补扫）、JSONL/SQLite 索引、目录语境、格式分析、资产关系、目录合并建议、草案计划、安全执行器（含执行日志与崩溃恢复）、L1–L4 规则学习、人工复核 CLI（plans/rules/merges/conflicts）。`scan --db` 可直接写入后续 `analyze --db` 所需的文件和目录语境记录。
- 执行前置条件：计划须处于 `APPROVED`；非 dry-run 必须提供 SQLite `--db`；每个写操作必须位于显式配置的 `SourceRoots` 内；源根目录与隔离根目录必须是互不重叠的真实目录且不能是符号链接；执行失败不会把路径写入普通日志；执行动作通过 `execution_journal` 表同步落盘，Journal 失败即停止，崩溃后 `recover` 可回滚未完成的动作。
- 并发控制：`scan`/`analyze` 支持 `--workers N`，由 `internal/runner` 的 semaphore 通道约束并发度；`analyze` 支持断点复用、unknown/媒体元数据定向刷新、SQLite 分批持久化和脱敏聚合进度。
- 外部 AI 继续关闭；L2/L3/L4 学习只生成 `status=draft` 规则草案，priority≤60（K-008），不覆盖已审批规则。
- 禁止：扫描阶段直接产生破坏性文件操作；AI 独立决定删除；跨备份域自动去重。

详见 [知识地图](knowledge/maps/knowledge-map.md) 与 [开发路线](knowledge/maps/roadmap.md)。

## 参与和支持

- 提交改动前请阅读 [贡献指南](CONTRIBUTING.md) 和 [工程护栏](AGENTS.md)。
- 普通使用问题和已脱敏的缺陷请按 [支持说明](SUPPORT.md) 处理。
- 安全问题必须遵循 [安全政策](SECURITY.md)，不要在公开 Issue 中提交真实路径、文件名、数据库、索引或日志。
- 本地及 CI 的公开边界检查：`make public-check`。
