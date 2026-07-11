# NAS Data Governance

面向个人、家庭和组织数字资产的本地化数据治理与归档工程基座。

本仓库依据《NAS 数据整理与归档概念手册》建立。当前只实现第一阶段“安全索引”的最小闭环：只读扫描、元数据采集、分层哈希、完全重复报告。任何移动、删除、覆盖、硬链接和隔离操作均不在当前运行路径中。

## 快速开始

```bash
make test
make build
./bin/nas-governance scan --root /path/to/read-only-sample --out ./var/index.jsonl
./bin/nas-governance duplicates --index ./var/index.jsonl
```

扫描默认不跟随符号链接，不跨挂载点，不读取文件内容之外的外部服务，不上传任何数据。完整哈希采用 SHA-256；文件先按大小分组，只有潜在重复项才计算完整哈希。

## 仓库结构

```text
cmd/nas-governance/       CLI 入口
internal/domain/          核心对象与安全约束
internal/scanner/         只读文件系统扫描
internal/fingerprint/     快速指纹与完整哈希
internal/index/           JSONL 索引适配器
internal/report/          完全重复报告
knowledge/cards/          白皮书知识卡
knowledge/maps/           概念关系与开发路线
schemas/                  SQLite 目标模式与规则示例
docs/adr/                 架构决策记录
```

## 当前边界

- 已有：只读扫描、排除规则、符号链接跳过、跨挂载点保护、增量可复用的 JSONL 索引格式、完全重复报告。
- 预留：SQLite 仓储、真实格式识别、目录语境、保留评分、操作计划、安全执行器、隔离与回滚。
- 禁止：扫描阶段直接产生破坏性文件操作；AI 独立决定删除；跨备份域自动去重。

详见 [知识地图](knowledge/maps/knowledge-map.md) 与 [开发路线](knowledge/maps/roadmap.md)。
