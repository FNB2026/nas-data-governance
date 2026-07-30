# ADR-0006：桌面可视化架构

状态：已采纳
日期：2026-07-28

## 背景

NDG 后端治理能力已完成 M1—M7（安全索引、目录语境、安全执行、格式分析、资产关系与学习、生产化收束、分级删除生命周期）。当前交付物为纯 Go 核心加 CLI，没有 Wails、没有前端目录、没有桌面绑定层。

经全仓库代码核查（见《NDG 可视化开发路线手册》校正与修订指南），现有 `cmd/nas-governance/main.go` 直接编排 scanner、store、planner、executor、learning 等核心包，未经过独立应用服务层。`internal/runner` 仅是 semaphore 控制的进程内并发器，不持久化任务队列。重复报告主路径走 JSONL（`idx.Read`），缺少 active/missing 状态语义与分页能力。

桌面可视化需要解决的不是"画一个界面"，而是把现有能力安全组织成应用服务、持久任务、分页读模型和桌面工作台。

## 决策

### 1. 技术栈

- 桌面框架：Wails v2，锁定 v2.13.0 稳定版，不采用 v3 Alpha。
- 前端：React + TypeScript strict + Vite。
- 表格/数据：TanStack Query、TanStack Table、TanStack Virtual。
- 路由：React Router。
- 数据库：本机 SQLite（沿用现有），不放在 NAS SMB 目录。

### 2. 分层架构

```
React + TypeScript + Vite
        │ BackendPort
Wails 桌面适配层（DTO 转换、事件转发、文件选择对话框）
        │
internal/app 应用服务层（扫描/查询/复核/执行用例）
        │
internal/query（只读查询模型）  internal/jobs（任务状态）  internal/events（结构化事件）
        │
现有核心：scanner / store / planner / executor / dircontext / format / relations
        │
本机 SQLite 项目数据库 + 本地磁盘 / 已挂载 SMB / NAS 数据源
```

CLI 与桌面共享同一个 `internal/app`：

```
CLI ───────┐
           ├→ internal/app → 核心引擎
Desktop ───┘
```

### 3. Wails 隔离原则

Wails 相关代码只能出现在：

```
cmd/ndg-desktop/
internal/adapters/wails/
```

禁止 Wails 依赖进入：

```
internal/scanner
internal/planner
internal/executor
internal/domain
internal/app
```

未来即使换成 Tauri、Qt、Web UI 或 NAS 服务端，核心引擎也不需要重写。

### 4. 不让 GUI 调 CLI 子进程

禁止正式使用 `exec.Command("nas-governance", ...) → 解析 stdout` 方案。原因：进度只能解析文本、文案变化会破坏前端、难以安全取消、难以传递结构化错误、难以恢复任务、无法共享内存状态、可能重新暴露路径日志。

### 5. 只绑定用例，不绑定底层能力

Wails 禁止直接绑定 `ReadFile`、`DeleteFile`、`MoveFile`、`RunCommand`、`RawSQL` 等接口。只暴露高层用例（ProjectService、SourceService、ScanService、DuplicateService、ReviewService、PlanService、ExecutionService、RecoveryService）。

### 6. 数据库迁移编号

现有迁移止于 `005_deletion_lifecycle.sql`。可视化阶段新增迁移从 006 起：

| 编号 | 文件名 | 内容 | 阶段 |
|---|---|---|---|
| 006 | `006_desktop_jobs.sql` | `job_runs` / `job_events` | V2 |
| 007 | `007_directory_context_query_fields.sql` | 查询字段反规范化 | V1 |
| 008 | `008_governance_decisions.sql` | `group_decisions` | V1 |
| 009 | `009_physical_identity_reliability.sql` | `file_instances.physical_reliable` | V1 修复 |
| 010 | `010_scan_snapshots.sql` | `scan_runs` / `scan_run_membership` | 后续 |

每次新增迁移后同步修改 `schemas/schemas.go` 的 `All()` 与对应 `var Schema00X string`，并增加连续升级集成测试。

### 7. 里程碑编号

后端继续使用 M 系列，桌面端独立使用 V 系列，避免编号碰撞：

```
M1—M7  后端治理能力（已完成）
V1     正确性与查询基座
V2     应用服务与任务系统
V3     只读桌面 Alpha
V4     治理复核工作台
V5     执行与恢复工作台
```

### 8. 第一版只读

桌面 Alpha（V3）不暴露任何真实文件写操作。桌面绑定层中根本不暴露执行方法，而非仅在界面隐藏执行按钮。

### 9. SQLite 必须放在本机

NAS 文件可通过 SMB 挂载后扫描，但项目 SQLite 不放在 SMB 共享目录。WAL 依赖共享内存，不适合网络文件系统。可视化阶段新增专用只读查询连接（初始 2—4 个），不改变执行器单写安全原则。

### 10. 隐私与安全

- 普通进度事件不包含路径、文件名、错误中的原始路径、数据库路径、隔离区路径。
- 开发模式只用合成测试目录，不在真实 NAS 项目中开启前端 DevTools。
- 生产构建关闭调试日志，不开放任意底层文件 API。
- 敏感路径不写 localStorage。

## 理由

NDG 的核心价值不是"找相同文件"，而是"解释文件关系、判断是否应该处理、提供可审批可审计可恢复的执行计划"。这要求桌面端面对的是稳定用例、稳定 DTO、稳定查询、稳定状态，而不是一组 CLI 函数和不断变化的内部结构。因此必须先抽取应用服务层（V2），再创建 Wails 壳（V3）。

Wails v2 是当前稳定版；v3 仍为 Alpha，不应作为正式产品基线。锁定版本可避免供应链变化带来的风险。

## 后果

- 新增 `internal/app`、`internal/query`、`internal/jobs`、`internal/events`、`internal/presentation` 五个包。
- 新增 `cmd/ndg-desktop` 和 `internal/adapters/wails` 两个 Wails 隔离区。
- 新增 `frontend/` 目录。
- CLI 改为调用 `internal/app`，用户行为和现有测试保持不变。
- 新增迁移 006—009，需同步更新 `schemas.go`。
- CI 拆分为 Go Core、Frontend、Binding Contract、Desktop macOS、Desktop Linux、公开边界六个 Job。
- 首版不支持 Windows、不支持远程 NAS Agent、不支持相似媒体识别、不支持云端、不支持多人协作。

## 参考

- 《NDG 可视化开发路线手册》校正与修订指南（docs/《NDG 可视化开发路线手册》校正与修订指南.md）
- ADR-0001 只读优先
- ADR-0003 执行根边界
- ADR-0004 执行 Journal 与崩溃恢复
- ADR-0005 分级删除与隔离区永久清除
