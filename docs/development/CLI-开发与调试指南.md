# NDG CLI 开发与调试指南

本文档面向开发者，提供 NAS Data Governance CLI 工具的命令行操作参考。普通用户请参阅 [用户指南](../user-guide/NDG-用户指南.md)。

## 构建

```bash
make test
make build
```

构建产物位于 `./bin/nas-governance`。

## 安全体验：只读扫描

以下命令只读取样本数据并生成项目私有产物，不执行移动、隔离、重命名或删除动作。首次使用请只针对复制出来的合成测试目录。

```bash
./bin/nas-governance scan --root /path/to/read-only-sample --out ./var/index.jsonl --db ./var/governance.db --workers 4
./bin/nas-governance retry-hashes --root /path/to/read-only-sample --failures ./var/index.jsonl.hash-failures.jsonl --index ./var/index.jsonl --out ./var/index-recovered.jsonl
./bin/nas-governance import-index --index ./var/index.jsonl --db ./var/governance.db --batch-size 5000
./bin/nas-governance analyze --index ./var/index.jsonl --out ./var/formats.json --db ./var/governance.db --workers 4 --batch-size 500
# 中断后复用已落库记录；规则升级后可只重新分析旧 unknown
./bin/nas-governance analyze --index ./var/index.jsonl --out ./var/formats.json --db ./var/governance.db --workers 4 --resume --refresh-unknown
./bin/nas-governance analyze --index ./var/index.jsonl --out ./var/formats.json --db ./var/governance.db --workers 8 --resume --refresh-metadata
./bin/nas-governance diagnose-formats --db ./var/governance.db --out ./var/format-review-private.json
# 长任务可选写入 owner-only 聚合进度，不含路径/文件名/哈希
./bin/nas-governance scan --root /path/to/read-only-mount --out ./var/index.jsonl --db ./var/governance.db --progress-out ./var/scan-progress.json
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

文件写操作不属于快速开始。执行前必须同时满足：已有独立备份；先在复制的测试目录验证；隔离区位于所有源根目录之外；人工逐项批准计划；持久化 SQLite Journal 可用。

先批准明确选中的计划，并执行完全只读的预检：

```bash
./bin/nas-governance approve --plan ./var/plan.json --out ./var/approved.json --plan-id PLAN_ID
./bin/nas-governance execute --dry-run --plan ./var/approved.json --quarantine /path/outside/source/quarantine --source-root /path/to/copied-test-data --out ./var/audit-dry-run.json
```

人工复核 dry-run 审计结果后，真实执行必须提供 `--db`。Journal 初始化或动作完成记录失败时，执行器会在下一次文件写入前停止并回滚已完成动作：

```bash
./bin/nas-governance execute --plan ./var/approved.json --quarantine /path/outside/source/quarantine --source-root /path/to/copied-test-data --retention 720h --db ./var/governance.db --out ./var/audit.json
./bin/nas-governance recover --db ./var/governance.db
```

### 分级删除生命周期

`execute` 成功隔离的文件会进入 `quarantine_items` 私有台账，默认保留 30 天。受保护目录角色自动进入 `HOLD`。隔离项可通过独立的草案、审批、dry-run、执行链路恢复：

```bash
./bin/nas-governance quarantine list --db ./var/governance.db --out ./var/quarantine-private.json
./bin/nas-governance quarantine restore-plan --db ./var/governance.db --item-id ITEM_ID --out ./var/restore-plan.json
./bin/nas-governance quarantine restore-approve --db ./var/governance.db --plan-id RESTORE_PLAN_ID --digest APPROVAL_DIGEST
./bin/nas-governance quarantine restore-execute --dry-run --db ./var/governance.db --plan-id RESTORE_PLAN_ID --digest APPROVAL_DIGEST --quarantine /path/outside/source/quarantine --source-root /path/to/copied-test-data
./bin/nas-governance quarantine restore-execute --db ./var/governance.db --plan-id RESTORE_PLAN_ID --digest APPROVAL_DIGEST --quarantine /path/outside/source/quarantine --source-root /path/to/copied-test-data
```

保留期届满只产生 `DRAFT` 永久清除候选，不会自动删除。永久清除必须逐项二次审批，并在执行时再次提交相同摘要：

```bash
./bin/nas-governance purge plan --db ./var/governance.db --out ./var/purge-plan.json
./bin/nas-governance purge approve --db ./var/governance.db --plan-id PURGE_PLAN_ID --digest APPROVAL_DIGEST
./bin/nas-governance purge execute --dry-run --db ./var/governance.db --plan-id PURGE_PLAN_ID --digest APPROVAL_DIGEST --quarantine /path/outside/source/quarantine
./bin/nas-governance purge execute --db ./var/governance.db --plan-id PURGE_PLAN_ID --digest APPROVAL_DIGEST --quarantine /path/outside/source/quarantine
./bin/nas-governance purge recover --db ./var/governance.db --quarantine /path/outside/source/quarantine
```

永久清除仅允许处理受管隔离项，不允许直接删除源目录文件。

## 治理诊断

```bash
# 登记 critical HOLD；普通 approve（包括 --all）会拒绝 critical
./bin/nas-governance next-steps hold-critical --plan ./var/plan.json --out ./var/critical-hold-register.json
# 将大媒体分为在线保护/项目归档复核/冷存储复核，仅产生 DRAFT
./bin/nas-governance next-steps media-tiering --review ./var/governance-review-private.json --out ./var/media-tiering-draft.json
```

`diagnose-governance` 只读项目 SQLite：对完全重复组生成仅 DRAFT 的复核计划，对零字节文件区分占位标记/失败产物/潜在临时产物/无法解释空文件，并聚合大容量媒体的格式、编码、时长、尺寸、目录职责及版本/派生/侧车关系。

`diagnose-paths` 可读取私有历史扫描日志或扫描产生的 `0600` 哈希失败清单，在显式任务根目录内检查原路径、NFC/NFD 文件名变体以及"目录项可见但无法只读打开"的状态。

`diagnose-merges` 只读取索引并解释合并门控：兄弟目录总对数、有限后缀归一后的名称相似对，以及文件名 Jaccard 0.10/0.25/0.50 分档。

## 仓库结构

```text
cmd/nas-governance/       CLI 入口（含 quarantine/restore/purge 生命周期）
cmd/ndg-desktop/          Wails 桌面应用入口
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
internal/purge/           恢复与永久清除草案计划（只建议，不写文件）
internal/runner/          worker pool（semaphore 并发控制）
internal/drill/           NAS 故障演练（隔离只读场景）
internal/report/          完全重复报告
internal/app/             应用服务层（扫描编排、检查点）
internal/adapters/wails/  Wails 绑定层与 DTO
internal/version/         版本信息
internal/privatefs/       私有文件系统安全
internal/events/          事件层
internal/project/         项目生命周期管理
internal/jobs/            作业与崩溃恢复
knowledge/cards/          白皮书知识卡
knowledge/maps/           概念关系与开发路线
schemas/                  SQLite 目标模式与规则示例
docs/adr/                 架构决策记录
```

## 桌面应用构建

```bash
# 安装 Wails CLI
go install github.com/wailsapp/wails/v2/cmd/wails@v2.13.0

# 构建桌面应用
make desktop-build

# 前端开发
cd cmd/ndg-desktop/frontend
npm ci
npm run dev
```

## 当前边界

- 已有：只读扫描（含增量、断点续扫、哈希有限重试与失败补扫）、JSONL/SQLite 索引、目录语境、格式分析、资产关系、目录合并建议、草案计划、安全执行器（含执行日志与崩溃恢复）、受管隔离/恢复/延时清除/永久清除、L1-L4 规则学习、人工复核 CLI。
- 执行前置条件：计划须处于 `APPROVED`；非 dry-run 必须提供 SQLite `--db`；每个写操作必须位于显式配置的 `SourceRoots` 内；源根目录与隔离根目录必须是互不重叠的真实目录且不能是符号链接。
- 并发与可观测性：`scan`/`analyze` 支持 `--workers N`和可选 `--progress-out`；进度快照仅有阶段、计数、失败数和复用数，权限为 `0600`。扫描每 1,000 个文件更新聚合检查点。
- 永久清除边界：仅从受管隔离区逐项执行；默认 30 天冷静期；保护项进入 HOLD；无 `--all`；审批摘要须在执行时再次提供。
- 禁止：扫描阶段直接产生破坏性文件操作；AI 独立决定删除；直接对源目录执行 PURGE；跨备份域自动去重。
