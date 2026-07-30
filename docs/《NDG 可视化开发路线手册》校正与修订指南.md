# 《NDG 可视化开发路线手册》校正与修订指南

版本：2026-07-28
性质：基于全仓库代码核查对手册 v2026-07-27 的正式校正
适用范围：作为手册修订与首个可视化 PR 的依据

---

## 一、校正结论总览

经全仓库只读核查，手册 v2026-07-27 的 27 条可核验断言中：

- **25 条与代码完全一致**（架构判断、能力清单、代码引用、风险登记均准确）。
- **2 处需要正式修正**（`ListFiles` 过滤描述过时、迁移编号与现有 005 冲突）。
- **2 处编号需重新校准**（ADR 编号、里程碑编号 M7 已被占用）。

本指南逐条给出代码证据与修订文本，可直接用于更新手册。

---

## 二、四项正式修正

### 修正 1：`ListFiles` 已按 active 过滤

**问题：** 手册 3.2 节称现有 `ListFiles`「没有按 `active` 过滤」，并据此推断 missing 文件可能混入重复组。

**代码事实：** `internal/store/sqlite.go:197` 已具备过滤：

```sql
SELECT storage_id, path, name, size, mode, mtime, device, inode,
       quick_hash, content_sha256, discovered_at
FROM file_instances
WHERE file_status = 'active'
```

`ListFiles` **已经按 active 过滤**。手册该论据过时。

**但底层担忧仍成立**，因为重复报告的主路径不走 `ListFiles`，而走 JSONL：

- `cmd/nas-governance/main.go:395,411` 中 `runDuplicates` 与 `runPlan` 调用 `idx.Read(*path)`。
- `internal/index/jsonl.go:28-35` 的 `Read` 无任何 active/missing 状态语义，读入全部记录。
- `internal/report/duplicates.go:5-19` 的 `DuplicateGroups` 对传入集合全量内存分组，不过滤状态。

此外 `ListFiles` 仍存在三点不足（手册其余描述成立）：

- SELECT 列表未取 `file_status` 字段本身；
- 一次返回全部 active 文件，无分页；
- 不适合直接支撑桌面端分页浏览。

**修订文本（替换手册 3.2 节相关表述）：**

> SQLite 的 `ListFiles` 已默认过滤 `active`；但当前重复报告的 JSONL 路径缺少 active/missing 状态语义，且 SQLite 和 JSONL 两条查询路径均未形成适合桌面端的分页读模型。可视化查询必须建立统一的 SQLite Query Model，默认 `WHERE file_status = 'active'`，并覆盖 JSONL 与 SQLite 两条路径。

**任务影响：** 不需要再把「过滤 active」列为待开发任务；仍需建立统一 SQLite Query Model 与分页。

---

### 修正 2：数据库迁移编号整体后移

**问题：** 手册 11.1 节建议「Migration 005：任务运行」新增 `job_runs`/`job_events`，6.2 节再次写「迁移 005」。但仓库 `schemas/005_deletion_lifecycle.sql` 已占用 005。

**代码事实：**

- `schemas/005_deletion_lifecycle.sql:1` 注释为「-- 005: M7 分级删除生命周期」，已建 `quarantine_items / restore_plans / restore_journal / purge_plans / purge_journal`。
- `schemas/schemas.go:24-25` 的 `All()` 返回 `[Schema001 … Schema005]`，迁移序列止于 005。
- `003_execution_journal.sql`、`004_scan_checkpoints.sql` 均存在且语义对应手册所述 journal 与 checkpoint。

**修订（编号映射）：**

| 上一版建议 | 修正后编号 | 建议文件名 | 内容 |
|---|---|---|---|
| Migration 005 | **006** | `006_desktop_jobs.sql` | `job_runs` / `job_events` |
| Migration 006 | **007** | `007_directory_context_query_fields.sql` | 查询字段反规范化 |
| Migration 007 | **008** | `008_governance_decisions.sql` | `group_decisions` |
| Migration 008 | **010** | `010_scan_snapshots.sql` | `scan_runs` / `scan_run_membership` |

**配套要求：**

- 每次新增迁移后同步修改 `schemas/schemas.go` 的 `All()` 返回值与对应 `var Schema00X string`。
- 增加「旧数据库连续升级到最新 schema」的集成测试。
- 注意：V1（正确性基座）优先处理 007、008；006 留给 V2 任务系统。

---

### 修正 3：可视化 ADR 使用 0006

**问题：** 手册第二十部分建议新增 `docs/adr/0005-desktop-visualization-architecture.md`，但 0005 已被占用。

**代码事实：** `docs/adr/` 现有：

```
0001-read-only-first.md
0002-plans-remain-advisory.md
0003-execution-root-boundary.md
0004-execution-journal-and-crash-recovery.md
0005-tiered-deletion-and-purge.md
```

**修订：** 可视化架构 ADR 应为：

```
docs/adr/0006-desktop-visualization-architecture.md
```

---

### 修正 4：可视化不再命名为 M7，改用 V 系列

**问题：** 手册称「M1—M6 已完成，M7 可视化尚未开始」。但仓库已实际使用 M7 表示分级删除生命周期。

**代码事实：**

- `schemas/005_deletion_lifecycle.sql:1`：「-- 005: M7 分级删除生命周期」。
- `cmd/nas-governance/m7_e2e_test.go:71` `TestM7RestoreAndPermanentPurgeCLI`、`:178` `TestM7L1CopiedFixtureDryRunQuarantineAndRollback`，覆盖隔离→恢复→永久清除与回滚的端到端验证。

M7 在仓库中已指代「分级删除、恢复与清除生命周期」，且已实现或进入完成状态。

**修订（里程碑重新定义）：**

后端继续使用 M 系列，桌面端独立使用 V 系列，避免再次编号碰撞：

```
M1 安全索引                         已完成
M2 目录语境与治理计划                  已完成
M3 安全执行                         已完成
M4 格式分析                         已完成
M5 资产关系与本地学习                  已完成
M6 生产化收束                        已完成
M7 分级删除、恢复与清除生命周期          已实现 / 完成中

V1 正确性与查询基座                   尚未开始
V2 应用服务与任务系统                  尚未开始
V3 只读桌面 Alpha                    尚未开始
V4 治理复核工作台                     尚未开始
V5 执行与恢复工作台                   尚未开始
```

手册原「M7.0—M10」可视化阶段全部改编号为 V1—V5。

---

## 三、修正后的项目阶段判断

当前 NDG 已完成的后端能力（M1—M7）：

- 安全扫描与增量索引（BFS、不跟随符号链接、默认不跨挂载、断点续扫、哈希复用）。
- 目录职责、业务锚点、权威等级与冲突识别。
- 格式分析（magic bytes、图片尺寸、音视频编码时长、PDF 页数、压缩包条目、OOXML 分类、PSD/工程/侧车保护）。
- 版本、派生与侧车关系。
- 治理草案与人工复核 CLI。
- 安全隔离执行（DRAFT→APPROVED→STALE_CHECKED→EXECUTING→VERIFIED/ROLLED_BACK 状态机、stale 检查含 path/size/mtime/inode/device/hash、隔离而非永久删除、跨卷复制校验删源、失败逆序回滚）。
- 持久化 execution journal 与崩溃恢复。
- 隔离文件恢复与分级永久清除（M7）。
- L1—L4 本地学习（内置规则、统计学习、行业资料学习、决策反馈；结果先进 draft；外部 AI 默认关闭；已审批规则不被直接覆盖）。
- 真实 NAS 场景测试与 M7 E2E。

**当前真正缺失的是四层能力：**

```
应用服务层   internal/app
持久任务层   internal/jobs + job_runs
分页查询层   internal/query + SQLite 读模型
桌面呈现层   Wails + React
```

**目标架构：**

```
CLI ───────┐
           ├→ internal/app → 核心领域与持久化
Desktop ───┘
```

CLI 与桌面共享同一套应用服务，桌面不解析 CLI 输出。

---

## 四、修正后的开发次序与迁移映射

### V1：正确性与查询基座（1—2 周）

优先处理：

1. 硬链接物理身份与可释放容量（`PhysicalIdentity`：StorageID/Device/Inode/LinkCount/Reliable）。
2. SQLite 重复组分页查询（keyset pagination，稳定 `group_id = SHA256(domain + storage + content_sha256)`）。
3. JSONL 与 SQLite 重复结果语义统一（active/missing/unavailable 状态统一）。
4. 稳定排序（治理候选容量降序 → 风险 → SHA-256）。
5. 跨数据源和跨备份域治理边界（跨域只显示「相关副本」，不进自动治理候选）。
6. `ReviewDecision` 与 `PlanState` 分离（复核状态：UNREVIEWED/KEEP_ALL/DRAFT_ACTION/DEFERRED/REJECTED_SUGGESTION；操作计划沿用现有安全状态机）。
7. 修正 CLI 中「approve this plan」的误导文案（当前 `review.go:121` 提示 approve，实际 REVIEW→SKIP）。

对应迁移：

```
007_directory_context_query_fields.sql   （role/protected/business_anchor/authority_level/branch_point/rule_version 反规范化）
008_governance_decisions.sql             （group_decisions 表）
```

注意：006 留给 V2 任务系统，V1 先做 007、008。

### V2：应用服务与任务系统（1—2 周）

新增目录：

```
internal/app/           应用用例（ScanService/DuplicateService/ReviewService/PlanService/ExecutionService/RecoveryService）
internal/query/         UI 只读查询模型
internal/jobs/          后台任务管理（JobManager）
internal/events/        结构化进度事件
internal/presentation/  DTO、脱敏视图模型
```

新增迁移：

```
006_desktop_jobs.sql    （job_runs / job_events）
```

- `job_events` 只保存脱敏、低频、结构化里程碑，不保存每个文件事件。
- 现有 `internal/runner` 继续作为 semaphore 并发执行器，不改造成持久队列；`JobManager` 位于 runner 上层。
- CLI 改为调用应用服务，但用户行为和现有测试保持不变。

### V3：只读桌面 Alpha（2—3 周，`v1.1.0-alpha.1`）

技术栈：

```
Wails v2（锁定 v2.13.0，不采用 v3 Alpha）
React + TypeScript strict
Vite
TanStack Query / Table / Virtual
React Router
```

第一版只暴露（全部只读）：

- 项目管理（创建/打开/关闭）。
- 数据源验证与添加（12 项预检）。
- 扫描与续扫（停止并保留断点，非「暂停」）。
- 任务进度（结构化事件，不含路径）。
- 重复结果（分页、筛选、虚拟滚动）。
- 文件组详情（按需加载）。
- 目录分支对照（共同祖先、分叉点、ParentChain、角色/锚点/权威/保护）。
- 格式和关系信息。
- 隐私显示模式与截图模式。

**不暴露任何真实文件写操作。** 桌面绑定层中根本不暴露执行方法，而非仅在界面隐藏按钮。

### V4：治理复核工作台（2—3 周，`v1.1.0-alpha.2`）

增加：

- 保留全部 / 交叉归档 / 备份关系 / 主保留项。
- 生成隔离草案。
- 批量规则预览（应用前必须显示影响数量，先预览不直接提交）。
- 计划版本（审批后不可原地任意修改，变更产生新版本）。
- 决策历史与规则复核。

**此阶段仍不执行真实文件写入。**

### V5：执行与恢复工作台（2—4 周，`v1.1.0-beta.1`）

最后接入现有安全能力：

- APPROVED gate / stale 检查 / dry-run / SourceRoots / 隔离区验证 / execution journal。
- 隔离执行 / 恢复 / 回滚 / M7 purge 生命周期。
- 应用启动恢复检测（`ListExecutingPlans` / `ListRecoverableRuns`）。
- 恢复锁（未完成恢复前新写操作锁定）。

**桌面端不能新增绕过 CLI 安全约束的快捷路径。**

---

## 五、建议 PR 顺序（修正后）

按当前最新 PR 为 #6 推算，从 #7 开始：

| 建议 PR | 内容 | 关键验收 |
|---|---|---|
| #7 | ADR-0006 + 手册校正 | 明确 Wails/React/层次/安全边界；修正 ListFiles/迁移编号/里程碑 |
| #8 | 重复组正确性 | active、硬链接、稳定 ID、容量模型 |
| #9 | Query read model | SQLite 分页、筛选、详情查询（迁移 007） |
| #10 | ReviewDecision 重构 | 复核与执行批准分离（迁移 008） |
| #11 | `internal/app` 抽取 | CLI 行为不变 |
| #12 | JobManager 与事件 | 可取消、可恢复、无路径事件（迁移 006） |
| #13 | Wails + React 脚手架 | 能启动、版本和项目打开 |
| #14 | 数据源页面 | 验证、添加、策略 |
| #15 | 扫描任务页面 | 启动、进度、停止、续扫 |
| #16 | 重复结果页面 | 分页、筛选、虚拟滚动 |
| #17 | 目录语境检查器 | 分支、角色、锚点、评分 |
| #18 | 治理复核 | 决策、批量预览、草案 |
| #19 | 计划与 dry-run | 无真实写入 |
| #20 | 执行中心 | Journal、隔离、回滚 |
| #21 | 审计与恢复 | 重启恢复、时间线 |
| #22 | Desktop CI/Release | macOS/Linux 构建和许可 |

不要在一个 PR 里同时完成 Wails 脚手架 + 扫描重构 + SQLite 迁移 + 完整 UI + 执行功能。

---

## 六、第一个正式 PR 内容清单

第一个 PR 不应直接创建 Wails，而应是文档校准：

```
PR：docs/architecture
- 新增 docs/adr/0006-desktop-visualization-architecture.md
- 修正手册中 ListFiles 描述（已过滤 active，问题在 JSONL 路径与无分页）
- 修正迁移编号为 006—009
- 将可视化路线改为 V1—V5
- 明确 M7 已用于 deletion lifecycle
- 同步更新 README 中扫描管线表述（实际对所有新文件算 quick hash）
```

完成文档校准后，再开始 `007_directory_context_query_fields.sql` 和 `internal/app`。

---

## 七、附录：已核实一致的关键断言速查

以下断言经代码核查全部成立，无需修改：

| 手册位置 | 断言 | 代码证据 |
|---|---|---|
| 一·仓库核查 | 最新提交 `bf45251…` / 2026-07-16 / #6 | git log 三要素一致 |
| 一·仓库核查 | go.mod 无 Wails/React/TS，纯 Go 核心 | go.mod 仅 dslipak/pdf、x/text、modernc.org/sqlite |
| 4.3 | 无 `internal/app`，`main.go` 直接编排核心包 | import 直接导入 scanner/store/planner/executor/learning |
| 4.4 | 无 frontend/、无 cmd/ndg-desktop、无 adapters/wails | LS 确认均不存在 |
| 3.1 | `byHash[SHA256]=append(...)` 不区分硬链接 | `report/duplicates.go:9,14` |
| 3.3 | `DuplicateGroups` 内存全量分组、无分页无排序 | `duplicates.go` 全函数 20 行 |
| 3.4 | review 提示 approve 但实为 REVIEW→SKIP | `review.go:121,127-131` |
| 3.5 | 扫描对所有新文件算 quick hash | `main.go:258-266` 无大小预筛 |
| 5.2 | 启用 WAL、写连接=1、OpenReadOnly=1 | `001_initial.sql:2`；`sqlite.go:60,98` |
| 6.1 | runner 仅 semaphore，不持久化/不限流/无优先级 | `runner.go:1-13` 注释明示 |
| 11.1 | directory_contexts 主存 context_json 未反规范化 | `001_initial.sql:13`；`sqlite.go:248` |
| M3 | 状态机六状态及转换 | `domain/model.go:148-155` + `statemachine.go:25-44` |
| M3 | stale 检查 path/size/mtime/inode/hash（额外含 device） | `executor/stale.go:90-107` |
| M3 | 隔离非永久删除、MOVE/COPY/RENAME、跨卷校验删源、逆序回滚 | `executor.go:498-510`、`ops.go:128-145`、`executor.go:552-572` |
| M4 | magic bytes + 各类元数据 + PSD/工程/侧车保护 | `format/detect.go` + `metadata.go` + `filepolicy/policy.go` |
| M5 | L1-L4 学习/draft/外部AI关闭/已审批不被覆盖 | `learning/{rule,stats,corpus,feedback}.go` |
| 1.1 | 扫描器 BFS、单目录失败不终止、不跟随符号链接、默认不跨挂载 | `scanner.go:86-134` |
| 12.1 | 扫描在内存维护 `[]domain.FileInstance` 对全量切片分组/哈希 | `main.go:196,217-220,289-303` |
| M7 | M7 已用于分级删除生命周期（隔离/恢复/清除 E2E） | `005_deletion_lifecycle.sql:1` + `m7_e2e_test.go:71,178` |

---

## 最终判断

NDG 当前状态应重新定义为：

> 后端治理能力已完成 M1—M7，包含扫描、语境、计划、安全执行、恢复和分级清除；桌面可视化尚未开始。下一阶段不是补做去重算法，而是建立应用服务、任务状态、分页读模型和 Wails 桌面工作台。

第一版可视化的胜负不取决于是否比 Duplicate Cleaner 更漂亮，而取决于它能否准确回答：

> 这些文件为什么相同、为什么仍可能都要保留、为什么建议处理其中某一个，以及处理失败后如何证明并恢复。
