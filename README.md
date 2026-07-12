# NAS Data Governance

面向个人、家庭和组织数字资产的本地化数据治理与归档工程基座。

本仓库依据《NAS 数据整理与归档概念手册》建立。当前实现安全索引、目录语境、去重计划与受限的安全执行流水线：只读扫描、元数据采集、分层哈希、完全重复报告、可解释的目录角色、草案计划、审批与执行。任何文件写入只能通过显式批准的计划和配置了源根目录的执行器发生。

## 原始资料

- `NAS 数据整理与归档概念手册.docx`：本工程的概念与安全边界来源。
- `文件资料分类大法2025.01.14.xmind`、`飞牛NAS2026.04.06.xmind`：待后续纳入目录语境、分类规则与 NAS 适配设计的补充材料。

## 快速开始

```bash
make test
make build
./bin/nas-governance scan --root /path/to/read-only-sample --out ./var/index.jsonl --db ./var/governance.db --workers 4
./bin/nas-governance analyze --db ./var/governance.db --workers 4
./bin/nas-governance group --db ./var/governance.db
./bin/nas-governance relations --db ./var/governance.db
./bin/nas-governance merge --db ./var/governance.db
./bin/nas-governance plan --index ./var/index.jsonl --out ./var/plan.json
./bin/nas-governance review plans --plan ./var/plan.json
./bin/nas-governance approve --plan ./var/plan.json --out ./var/approved.json --all
./bin/nas-governance execute --plan ./var/approved.json --quarantine /path/to/quarantine --source-root /path/to/data --out ./var/audit.json
./bin/nas-governance recover --db ./var/governance.db
./bin/nas-governance learn --db ./var/governance.db --apply
./bin/nas-governance learn --source=corpus --corpus-dir ./var/corpus --apply
./bin/nas-governance learn --source=feedback --db ./var/governance.db --apply
./bin/nas-governance review rules --db ./var/governance.db
```

扫描默认不跟随符号链接，不跨挂载点，不读取文件内容之外的外部服务，不上传任何数据。完整哈希采用 SHA-256；文件先按大小分组，只有潜在重复项才计算完整哈希。增量扫描复用 size+mtime+inode 三元组匹配的既有哈希，断点续扫通过 `scan_checkpoints` 表记录进度，已删除文件标记为 missing 而非物理删除。

## 仓库结构

```text
cmd/nas-governance/       CLI 入口（scan/analyze/group/relations/merge/plan/approve/execute/recover/learn/review）
internal/domain/          核心对象与安全约束
internal/scanner/         只读文件系统扫描（context-aware BFS、增量哈希复用、断点续扫）
internal/fingerprint/     快速指纹与完整哈希
internal/index/           JSONL 索引适配器
internal/format/          基于 magic bytes 的格式检测与只读元数据提取
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

- 已有：只读扫描（含增量与断点续扫）、JSONL/SQLite 索引、目录语境、格式分析、资产关系、目录合并建议、草案计划、安全执行器（含执行日志与崩溃恢复）、L1–L4 规则学习、人工复核 CLI（plans/rules/merges/conflicts）。`scan --db` 可直接写入后续 `analyze --db` 所需的文件和目录语境记录。
- 执行前置条件：计划须处于 `APPROVED`；每个写操作必须位于显式配置的 `SourceRoots` 内；根目录内任意符号链接会被拒绝；执行失败不会把路径写入审计结果；执行动作通过 `execution_journal` 表落盘，崩溃后 `recover` 可回滚未完成的动作。
- 并发控制：`scan`/`analyze` 支持 `--workers N`，由 `internal/runner` 的 semaphore 通道约束并发度；单任务失败不阻塞其他任务。
- 外部 AI 继续关闭；L2/L3/L4 学习只生成 `status=draft` 规则草案，priority≤60（K-008），不覆盖已审批规则。
- 禁止：扫描阶段直接产生破坏性文件操作；AI 独立决定删除；跨备份域自动去重。

详见 [知识地图](knowledge/maps/knowledge-map.md) 与 [开发路线](knowledge/maps/roadmap.md)。
