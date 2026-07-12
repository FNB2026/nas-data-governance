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
./bin/nas-governance scan --root /path/to/read-only-sample --out ./var/index.jsonl --db ./var/governance.db
./bin/nas-governance duplicates --index ./var/index.jsonl
./bin/nas-governance plan --index ./var/index.jsonl --out ./var/plan.json
./bin/nas-governance approve --plan ./var/plan.json --out ./var/approved.json --all
./bin/nas-governance execute --plan ./var/approved.json --quarantine /path/to/quarantine --source-root /path/to/data --out ./var/audit.json
```

扫描默认不跟随符号链接，不跨挂载点，不读取文件内容之外的外部服务，不上传任何数据。完整哈希采用 SHA-256；文件先按大小分组，只有潜在重复项才计算完整哈希。

## 仓库结构

```text
cmd/nas-governance/       CLI 入口
internal/domain/          核心对象与安全约束
internal/scanner/         只读文件系统扫描
internal/fingerprint/     快速指纹与完整哈希
internal/index/           JSONL 索引适配器
internal/dircontext/      目录角色分类、上级目录链与业务锚点
internal/planner/         草案计划与可解释保留评分
internal/store/           SQLite 持久化层（项目自身数据库）
internal/executor/        安全执行器（stale 复核、隔离、跨卷复制-校验-删除源、回滚）
internal/report/          完全重复报告
knowledge/cards/          白皮书知识卡
knowledge/maps/           概念关系与开发路线
schemas/                  SQLite 目标模式与规则示例
docs/adr/                 架构决策记录
```

## 当前边界

- 已有：只读扫描、JSONL/SQLite 索引、目录语境、完全重复报告、草案计划、安全执行器、格式分析、资产关系、目录合并建议与 L1 规则基础设施。`scan --db` 可直接写入后续 `analyze --db` 所需的文件和目录语境记录。
- 执行前置条件：计划须处于 `APPROVED`；每个写操作必须位于显式配置的 `SourceRoots` 内；根目录内任意符号链接会被拒绝；执行失败不会把路径写入审计结果。
- 预留：L2 本地统计学习、L3 行业资料学习、L4 决策反馈学习；外部 AI 继续关闭。
- 禁止：扫描阶段直接产生破坏性文件操作；AI 独立决定删除；跨备份域自动去重。

详见 [知识地图](knowledge/maps/knowledge-map.md) 与 [开发路线](knowledge/maps/roadmap.md)。
