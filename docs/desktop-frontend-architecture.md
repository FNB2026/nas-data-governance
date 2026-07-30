# NDG 桌面端前端架构与后端协同推进方案

> 文档性质：可直接交给 TRAE 执行的工程实施基线
> 项目：NAS Data Governance（NDG）
> 日期：2026-07-29
> 适用范围：Wails + React 桌面端、Wails Binding 层、Go 应用服务层
> 状态：实施版（取代此前“五页面拆分方案”作为当前前端推进主文档）
> 关联：[桌面治理闭环知识卡](../knowledge/cards/11-desktop-governance-workflow.md) · [桌面信息架构摘要](ui/information-architecture.md) · [ADR-0006](adr/0006-desktop-visualization-architecture.md)

---

## 0. 文档目标

本文件用于指导 NDG 从“后端全链路已打通、桌面端基础功能可用”的工程状态，推进到一套：

- 简单易用；
- 信息结构稳定；
- 扫描、复核、执行、恢复链路完整；
- 不把 NDG 做成普通 Duplicate Cleaner；
- 能承载 NAS、家庭资料、办公资料、私密资料等多种场景；
- 可持续扩展而不反复推翻前端；
- 前后端职责清晰；
- 可由 TRAE 分阶段实施、验证和提交；

的正式桌面应用。

本文件同时回答四个问题：

1. 桌面端最终应采用什么信息架构；
2. 当前已有后端能力如何映射到前端；
3. 后端还需要补充哪些 Wails API 与数据结构；
4. TRAE 应按照什么顺序实施，如何验收，哪些行为禁止。

---

# 1. 当前项目事实基线

## 1.1 已完成能力

截至 2026-07-29，项目已经完成：

- 文件扫描与 SHA-256 指纹；
- SQLite 持久化；
- 目录上下文感知：目录角色、权威性、业务锚点；
- 重复文件规划、保留评分、计划/审批/执行三步分离；
- JobManager：持久化任务、取消、崩溃恢复；
- 格式识别与分析；
- SMB/NFS/FUSE 环境下的物理身份可靠性处理；
- Quarantine 隔离与 Purge 清理；
- 格式、治理、合并三类诊断；
- CLI 到应用服务层迁移；
- Wails + React 桌面端基本链路；
- 项目只读/读写打开；
- 存储列表、重复组分页、组详情；
- 扫描启动、轮询、取消、任务历史；
- Toast、历史筛选、窗口拖拽与显示问题修复；
- Go、Race、Vet、TypeScript、Vite、Wails 构建验证。

因此，当前阶段不是从零设计一个 UI，也不是重写后端。核心工作是：

> 将已经存在于后端和 CLI 的治理能力，按照稳定的信息架构暴露给桌面端，并建立统一、安全、可理解的前端交互。

## 1.2 当前主要缺口

当前桌面端仍然存在以下结构性缺口：

1. 功能集中在单页或少量面板中，缺乏稳定一级导航；
2. 扫描、重复结果、诊断之间可以使用，但没有形成治理闭环；
3. 后端已有隔离、清理和规划能力，桌面端尚未充分接线；
4. 目录角色、业务锚点、权威性等 NDG 核心差异化信息未成为主要界面内容；
5. 执行前检查、审批、dry-run、stale、保护项、Journal、恢复等安全能力尚未形成完整桌面流程；
6. App.tsx 虽已从 973 行降至约 485 行，但仍不应继续承担所有跨页面状态；
7. 三类诊断目前按后端模块集中展示，尚未放回对应业务场景；
8. 桌面端仍处于手动冒烟验收阶段，重构必须避免破坏当前已工作的链路。

---

# 2. 产品与工程总原则

## 2.1 产品定位

NDG 不是“找出相同文件并删除”的工具，而是：

> 面向大量异构文件的本地数据治理、去重决策、隔离执行、审计与恢复系统。

重复内容只是输入证据，目录语境、业务职责、保护规则和执行安全共同决定最终动作。

## 2.2 信息架构优先于当前 API

页面按用户任务组织，不按 Go 包、诊断函数或现有 API 数量组织。

错误方式：

```text
一个 API 模块 = 一个一级页面
扫描参数 = 一个页面
扫描进度 = 另一个页面
```

正确方式：

```text
用户任务领域 = 一个稳定一级入口
同一领域内包含创建、运行、历史、详情、错误和恢复状态
```

当现有 API 不足时，应建立 API Gap，不应为了迁就接口而扭曲产品结构。

## 2.3 渐进披露

默认界面只显示普通用户完成任务所需的信息；高级参数、技术诊断、完整 JSON、错误堆栈放入可展开区域或开发者模式。

默认模式应让用户能完成：

```text
打开项目 → 添加/确认数据源 → 扫描 → 查看结果 → 复核 → 隔离 → 审计
```

高级模式再显示：

- worker 数；
- 快速哈希与完整哈希细节；
- SQLite schema；
- 完整事件 payload；
- 原始 JSON；
- 内部错误码；
- 规则版本；
- 物理身份诊断。

## 2.4 安全优先

所有写操作必须符合：

```text
发现事实 ≠ 治理决定
治理决定 ≠ 执行批准
执行批准 ≠ 已经执行
隔离完成 ≠ 可以永久清理
```

禁止将“删除重复项”作为结果页的直接主按钮。

## 2.5 能力感知

桌面端不得仅通过页面名称猜测后端是否支持功能。后端应返回项目模式、能力开关和执行准备状态，前端据此启用、禁用或隐藏功能。

## 2.6 不进行大爆炸式重写

实施过程中必须保留当前可用链路。每个阶段都应满足：

- `go test -race ./...` 通过；
- `go vet ./...` 通过；
- `tsc --noEmit` 通过；
- `vite build` 通过；
- `wails build` 通过；
- 当前 smoke test 数据库仍可打开；
- 已有扫描、分页、诊断功能不回退。

---

# 3. 最终一级信息架构

不增加独立 Dashboard，固定采用七个一级入口：

```text
数据源
扫描任务
重复结果
治理复核
执行中心
审计与恢复
设置
```

## 3.1 一级导航标识

```typescript
export type AppRoute =
  | "sources"
  | "scan-jobs"
  | "duplicate-results"
  | "governance-review"
  | "execution-center"
  | "audit-recovery"
  | "settings";
```

## 3.2 导航启用规则

| 一级入口 | 未打开项目 | 只读项目 | 读写项目 | 存在恢复锁 |
|---|---:|---:|---:|---:|
| 数据源 | 可用 | 可用 | 可用 | 可用 |
| 扫描任务 | 禁用 | 只读历史可用，新建禁用 | 可用 | 新建扫描禁用 |
| 重复结果 | 禁用 | 可用 | 可用 | 可用 |
| 治理复核 | 禁用 | 可查看 | 可编辑草案 | 只读 |
| 执行中心 | 禁用 | 禁用 | 条件满足后可用 | 禁用新执行 |
| 审计与恢复 | 禁用 | 可用 | 可用 | 强制优先进入 |
| 设置 | 可用 | 可用 | 可用 | 可用，但写入项目设置受限 |

## 3.3 首次进入行为

- 应用启动后进入“数据源”；
- 若发现未完成执行或可恢复运行，顶部显示全局恢复横幅；
- 若存在恢复锁，默认跳转“审计与恢复”；
- 若无项目打开，其他业务入口保持可见但禁用，并显示原因；
- 不隐藏整个导航，避免用户无法理解软件能力范围。

---

# 4. 应用整体布局

## 4.1 桌面框架

```text
┌──────────────────────────────────────────────────────────────┐
│ Titlebar：NDG / 项目名 / 只读或读写 / 全局状态 / 窗口控制      │
├──────────────┬───────────────────────────────────────────────┤
│ Sidebar      │ Page Header                                   │
│              ├───────────────────────────────────────────────┤
│ 七个一级入口  │ Main Content                                  │
│              │                                               │
│              │                                               │
│              │                                               │
├──────────────┴───────────────────────────────────────────────┤
│ 可选 Statusbar：版本 / 任务状态 / 后端连接 / 隐私模式          │
└──────────────────────────────────────────────────────────────┘
```

## 4.2 尺寸建议

- Sidebar：208–224 px；
- 页面内容最大宽度不统一锁死为 1000 px；
- 数据密集页面应使用可用宽度；
- 表单和说明页面可限制为 1100–1280 px；
- 最小窗口建议：1180 × 720；
- 低于 1100 px 时，三栏页面折叠为“两栏 + 详情抽屉”；
- 不要求移动端适配，但要求小窗口不溢出、不遮挡主要按钮。

## 4.3 全局顶栏信息

至少显示：

- 应用名称；
- 当前项目文件名；
- 项目模式：只读 / 读写；
- 活跃扫描状态；
- 是否存在恢复锁；
- 隐私模式状态；
- 版本号放在设置或状态栏，不占据主要视觉区域。

---

# 5. 页面一：数据源

## 5.1 用户目标

回答以下问题：

- 当前打开的是哪个项目；
- 项目是否只读；
- 有哪些数据源；
- 数据源是否在线；
- 哪些目录被保护；
- 是否具备扫描条件；
- 隔离区是否已配置并可写；
- 当前是否具备执行准备条件。

## 5.2 页面结构

```text
项目摘要
├── 项目数据库路径
├── 只读/读写模式
├── schema/规则版本
├── 打开、关闭、刷新
└── 路径预校验状态

数据源列表
├── ID、根路径、类型
├── 在线状态
├── 最近扫描时间
├── 文件数量与容量摘要
├── 保护规则数量
└── 扫描按钮

扫描与保护策略
├── SourceRoots
├── 排除规则
├── 保护目录
├── 跨挂载策略
└── 默认扫描参数

隔离与执行准备
├── 隔离区路径
├── 可写性
├── 是否位于受允许范围
├── Journal 可用性
└── 独立备份确认状态

格式与数据质量摘要
└── 格式诊断入口及摘要
```

## 5.3 默认交互

### 未打开项目

展示一个清晰的项目入口，而不是空白页面：

- 最近项目；
- 打开已有数据库；
- 新建项目数据库；
- 只读打开；
- 读写打开；
- 路径合法性提示。

普通用户默认选择“读写打开”，只读模式放在次要选项或下拉菜单中，但必须明确说明区别。

### 已打开项目

- 项目摘要收起为紧凑顶栏卡片；
- 数据源成为主要内容；
- “开始扫描”按钮位于每个可扫描数据源卡片和页面顶部；
- 点击开始扫描跳转“扫描任务”，并预填数据源。

## 5.4 前端组件

```text
pages/sources/
├── SourcesPage.tsx
├── ProjectOpenCard.tsx
├── ProjectSummary.tsx
├── SourceList.tsx
├── SourceCard.tsx
├── ScanReadinessPanel.tsx
├── ProtectionSummary.tsx
├── QuarantineReadiness.tsx
└── FormatQualitySummary.tsx
```

## 5.5 当前可复用能力

- `OpenProject`；
- `OpenProjectReadWrite`；
- `CloseProject`；
- `GetProjectInfo`；
- `ValidateProjectPath`；
- `ListStorages`；
- `DiagnoseFormats`；
- `GetVersion`。

## 5.6 后端需补充

建议增加或在 Wails Binding 层暴露：

```go
GetAppCapabilities() AppCapabilities
GetProjectReadiness() ProjectReadiness
GetStorageDetail(storageID string) StorageDetail
ListProtectionRules(storageID string) []ProtectionRule
GetQuarantineConfig() QuarantineConfig
ValidateQuarantine() QuarantineValidation
ListRecentProjects(limit int) []RecentProject
```

接口名称可按现有代码规范调整，但返回能力必须完整。

## 5.7 验收标准

- 未打开项目时，用户能在 1 个页面内完成打开或新建；
- 路径错误在点击打开前即可发现；
- 只读与读写模式视觉上清楚；
- 数据源是否可扫描有明确原因；
- 不需要进入“设置”才能知道隔离区是否准备好；
- 格式诊断不再单独占一级页面。

---

# 6. 页面二：扫描任务

## 6.1 用户目标

- 新建扫描；
- 查看运行中的扫描；
- 查看任务历史；
- 查看任务参数、阶段、告警和失败原因；
- 取消任务；
- 识别崩溃恢复产生的失败任务；
- 重新发起相同参数任务。

## 6.2 页面结构

宽屏采用三栏：

```text
左：任务列表
├── 运行中
├── 已完成
├── 已停止
├── 失败
└── 新建扫描

中：任务主区域
├── 阶段
├── 进度
├── 文件数/容量
├── 警告/失败
├── 持续时间
└── 事件时间线

右：参数与诊断
├── 数据源
├── 根目录
├── 完整/增量
├── worker
├── 排除规则
├── 检查点
└── 失败摘要
```

无选中任务时，中间区域显示“新建扫描”。

## 6.3 进度规则

### 禁止虚假百分比

`DISCOVERING` 阶段不使用 `processed / discovered` 计算总百分比，因为分母仍在变化。

显示方式：

```text
正在发现文件
已发现 182,430 项
已读取 1.26 TB
持续 04:13
```

仅在后端能够提供稳定工作总量时显示百分比，例如：

```text
完整哈希 68%
124,300 / 182,900
```

后端返回建议：

```typescript
interface ProgressMetric {
  mode: "indeterminate" | "determinate";
  current?: number;
  total?: number;
  unit: "files" | "bytes" | "groups" | "steps";
}
```

### 终态

终态包括：

- `COMPLETED`；
- `FAILED`；
- `CANCELLED`。

`CANCEL_REQUESTED` 仍然继续轮询，按钮状态改为“正在取消”。

## 6.4 路径隐私

全局进度区不显示：

- 当前完整路径；
- 最近文件名滚动列表；
- 实时路径日志。

这些内容只允许在：

- 高级详情；
- 开发者模式；
- 用户主动展开；

的情况下显示，并服从隐私模式脱敏设置。

## 6.5 前端组件

```text
pages/scan-jobs/
├── ScanJobsPage.tsx
├── JobList.tsx
├── JobListItem.tsx
├── NewScanForm.tsx
├── ScanDropZone.tsx
├── JobProgress.tsx
├── StageProgress.tsx
├── JobStats.tsx
├── JobTimeline.tsx
├── JobParameters.tsx
└── JobFailureSummary.tsx
```

## 6.6 当前可复用能力

- `StartScan`；
- `GetScanProgress`；
- `CancelScan`；
- `ListRecentJobs`；
- `GetJobDetail`；
- Wails `filedrop`；
- 当前 Toast 通知；
- 当前任务筛选。

## 6.7 后端需优化

### 任务列表分页与筛选

当前 `ListRecentJobs(limit)` 只能满足早期需求。建议升级为：

```go
ListJobs(req ListJobsRequest) ListJobsResponse
```

建议字段：

```typescript
interface ListJobsRequest {
  state?: string[];
  job_type?: string[];
  storage_id?: string;
  page_size?: number;
  cursor?: string;
}

interface ListJobsResponse {
  jobs: JobSummary[];
  next_cursor?: string;
}
```

### 进度快照

建议统一：

```go
GetJobProgress(jobID string) JobProgressSnapshot
```

增加：

- `bytes_discovered`；
- `bytes_processed`；
- `candidate_files`；
- `candidate_bytes`；
- `elapsed_ms`；
- `estimated_remaining_ms`，仅有可靠估算时返回；
- `progress.mode/current/total/unit`；
- `can_cancel`；
- `can_retry`。

### 重试

建议增加：

```go
RetryJob(jobID string) StartJobResponse
```

重试应复制原任务参数，前端允许用户修改后再启动。

## 6.8 轮询策略

短期继续轮询，不强制改为事件推送：

- 活跃任务：800–1200 ms；
- `CANCEL_REQUESTED`：500–800 ms；
- 页面不活跃但应用仍前台：1500–2500 ms；
- 应用后台或窗口最小化：3000–5000 ms；
- 终态后停止轮询；
- 每次请求必须防止前一个请求未完成时重复进入；
- 项目切换时必须取消旧项目轮询并丢弃迟到结果。

## 6.9 验收标准

- 新建扫描与查看进度位于同一一级页面；
- 发现阶段不显示虚假百分比；
- 取消任务状态正确；
- 崩溃恢复任务有明确说明；
- 任务历史不再仅依赖客户端筛选；
- 拖入多个目录时给出选择或明确只取第一项，不静默忽略；
- 扫描完成后刷新重复结果、数据源摘要和任务列表。

---

# 7. 页面三：重复结果

## 7.1 用户目标

- 查看内容完全相同的文件组；
- 区分物理副本与硬链接别名；
- 判断理论可回收上限；
- 查看目录语境、目录角色、业务锚点和保护状态；
- 识别交叉归档、备份关系和高风险组；
- 将组送入治理复核，而不是直接删除。

## 7.2 页面结构

采用三栏，而不是大卡片瀑布流：

```text
左：筛选与批量规则
├── 数据源
├── 风险
├── 最小容量
├── 物理副本数量
├── 硬链接
├── 保护项
├── 目录角色
├── 业务锚点
├── 复核状态
└── 计划状态

中：紧凑重复组列表
├── 风险
├── 代表文件名
├── 文件大小
├── 路径数
├── 物理副本数
├── 硬链接标志
├── 物理可回收上限
├── 治理候选容量
├── 目录角色数
├── 业务锚点数
├── 保护项
├── 复核状态
└── 计划状态

右：证据与目录语境检查器
├── 文件成员
├── 内容哈希与大小
├── 物理身份
├── 上级 1–4 级目录
├── 目录角色
├── 权威性
├── 业务锚点
├── 保护规则
├── 备份关系
└── 进入治理复核
```

## 7.3 列表形态

默认使用紧凑表格或虚拟化行列表，不使用大型卡片作为主视图。

允许：

- 交替浅色底纹；
- 固定表头；
- 列宽调整；
- 列显示配置；
- 键盘上下选择；
- 选中行高亮；
- 右侧详情随选中项刷新。

默认排序：

```text
治理候选容量降序
→ 风险等级降序
→ SHA-256 升序
```

排序必须稳定，分页游标必须与排序规则一致。

## 7.4 容量概念必须区分

界面不得只显示一个“可释放空间”。至少区分：

1. 文件组总逻辑容量；
2. 理论物理冗余上限；
3. 治理候选容量；
4. 因保护规则不可执行容量；
5. dry-run 后预计可执行容量；
6. 实际释放容量。

当前重复结果页至少显示前 3 项；后 3 项在执行中心显示。

## 7.5 前端组件

```text
pages/duplicate-results/
├── DuplicateResultsPage.tsx
├── DuplicateFilterPanel.tsx
├── DuplicateGroupTable.tsx
├── DuplicateGroupRow.tsx
├── ResultSummaryBar.tsx
├── EvidenceInspector.tsx
├── FileMemberList.tsx
├── DirectoryContextTree.tsx
├── PhysicalIdentityBadge.tsx
├── ProtectionBadge.tsx
└── GovernanceEntryActions.tsx
```

## 7.6 当前可复用能力

- `ListDuplicateGroups`；
- `GetGroupDetail`；
- keyset 分页；
- `physical_copy_count`；
- `hardlink_alias_count`；
- `physical_reclaimable_bytes`；
- `decision_type`；
- 当前组详情组件。

## 7.7 后端需补充或扩展

建议升级列表请求：

```typescript
interface ListDuplicateGroupsRequest {
  storage_id?: string;
  min_reclaimable_bytes?: number;
  risk_levels?: string[];
  has_protected_items?: boolean;
  has_hardlinks?: boolean;
  review_states?: string[];
  plan_states?: string[];
  directory_roles?: string[];
  business_anchor_count_min?: number;
  page_size?: number;
  cursor?: string;
  sort?: "governance_candidate_desc" | "risk_desc" | "size_desc";
}
```

建议扩展 `GroupSummary`：

```typescript
interface GroupSummary {
  group_id: string;
  sha256: string;
  representative_name: string;
  size: number;
  storage_id: string;
  path_count: number;
  physical_copy_count: number;
  hardlink_alias_count: number;
  physical_reclaimable_bytes: number;
  governance_candidate_bytes: number;
  risk_level: "LOW" | "MEDIUM" | "HIGH" | "CRITICAL";
  directory_role_count: number;
  business_anchor_count: number;
  has_protected_items: boolean;
  review_state: string;
  plan_state?: string;
  decision_type?: string;
}
```

建议扩展详情：

```typescript
interface FileContextEvidence {
  file_id: string;
  path: string;
  path_segments: string[];
  physical_identity?: string;
  is_hardlink_alias: boolean;
  directory_roles: string[];
  authority_score?: number;
  business_anchors: string[];
  protection_matches: ProtectionMatch[];
  backup_relation?: BackupRelation;
  stale_state?: string;
}
```

## 7.8 验收标准

- 用户无需打开 JSON 即可理解组内证据；
- 硬链接组不得显示虚假可回收容量；
- 目录上级语境至少显示 1–4 级；
- 有保护项的组在列表和详情中均有明显标志；
- 列表能承载数千组，不因卡片布局严重降低性能；
- 结果页没有直接“永久删除”主按钮；
- 点击治理操作后进入“治理复核”，并保留当前筛选和选中状态。

---

# 8. 页面四：治理复核

## 8.1 用户目标

对重复组作出可解释的治理决定，而不是只选择删除项。

可用决定：

- 保留全部；
- 选择主要保留项；
- 生成隔离草案；
- 标记交叉归档；
- 标记备份关系；
- 暂缓处理；
- 纠正目录角色；
- 纠正业务锚点；
- 标记需要进一步复核。

## 8.2 页面结构

```text
左：待复核队列
├── 风险
├── 容量
├── 决策状态
└── 批量选择

中：证据对比
├── 文件成员并排比较
├── 路径语境
├── 目录角色
├── 权威性
├── 业务锚点
├── 保护项
└── 推荐理由

右：治理决策
├── 决策类型
├── 主要保留项
├── 隔离候选项
├── 说明/备注
├── 影响数量
├── 预览
└── 保存草案
```

## 8.3 批量操作规则

任何批量应用必须先显示：

- 影响重复组数量；
- 影响文件数量；
- 原始逻辑容量；
- 理论物理冗余；
- 保护项数量；
- 高风险项数量；
- 将生成的草案数量；
- 无法应用的原因。

批量规则必须先预览，再确认保存为草案，不允许直接进入执行。

## 8.4 推荐结果的表达

推荐不得只显示一个分数。至少显示：

- 推荐动作；
- 推荐置信度；
- 主要证据；
- 反对证据；
- 风险提示；
- 用户可纠正的目录角色或业务锚点。

示例：

```text
建议：标记为交叉归档，保留全部
理由：两个副本分别位于“财务归档”和“客户合同归档”，目录角色不同；
其中一个路径具有业务锚点“2019 销售发票”，另一个具有锚点“已完结合同”。
虽然内容哈希相同，但职责不同，不进入隔离候选。
```

## 8.5 前端组件

```text
pages/governance-review/
├── GovernanceReviewPage.tsx
├── ReviewQueue.tsx
├── EvidenceComparison.tsx
├── ContextComparison.tsx
├── RecommendationPanel.tsx
├── DecisionEditor.tsx
├── BatchRuleBuilder.tsx
├── BatchPreviewDialog.tsx
└── DraftPlanSummary.tsx
```

## 8.6 当前可复用能力

后端已存在：

- 目录上下文；
- 角色、权威、业务锚点；
- 重复文件规划；
- 保留评分；
- 计划/审批/执行分离；
- 治理诊断；
- 合并诊断。

当前桌面端可以先以 `DiagnoseGovernance` 和 `DiagnoseMerges` 的结果构建只读复核界面，再接入正式编辑 API。

## 8.7 后端需暴露的核心接口

建议围绕应用服务层增加 Wails Binding：

```go
ListReviewItems(req ListReviewItemsRequest) ListReviewItemsResponse
GetReviewEvidence(groupID string) ReviewEvidence
SaveReviewDecision(req SaveReviewDecisionRequest) ReviewDecision
PreviewBatchDecision(req PreviewBatchDecisionRequest) BatchDecisionPreview
ApplyBatchDecisionDraft(req ApplyBatchDecisionDraftRequest) BatchDraftResult
ListDraftPlans(req ListPlansRequest) ListPlansResponse
GetPlanDetail(planID string) PlanDetail
ApprovePlan(req ApprovePlanRequest) PlanDetail
RejectPlan(req RejectPlanRequest) PlanDetail
```

注意：

- `SaveReviewDecision` 只保存复核决定或 DRAFT；
- 不得在保存决定时执行文件操作；
- `ApprovePlan` 不等于执行；
- API 必须返回版本号或 revision，避免旧页面覆盖新决定；
- 批量应用必须由后端再次验证保护规则，不信任前端统计。

## 8.8 验收标准

- 用户可以明确解释每个决定；
- 目录角色和业务锚点可以修正；
- 推荐与最终决定分开保存；
- 批量操作先预览；
- 保存草案不会移动、删除或隔离文件；
- 只读项目可查看但不能保存决策；
- 冲突或过期 revision 有明确错误提示。

---

# 9. 页面五：执行中心

## 9.1 用户目标

- 查看已批准计划；
- 进行 dry-run；
- 理解被 stale、保护规则或环境问题阻止的项目；
- 执行隔离；
- 在安全条件满足后执行 Purge；
- 查看预计释放与实际释放差异。

## 9.2 入口条件

只有满足以下条件时，执行按钮才可用：

```text
计划状态 = APPROVED
无未解决 REVIEW
隔离区验证通过
SourceRoots 验证通过
Journal 可用
项目为读写模式
无恢复锁
独立备份确认完成
```

前端必须逐项展示检查结果，不允许只显示“条件不满足”。

## 9.3 页面结构

```text
计划列表
├── DRAFT
├── REVIEW
├── APPROVED
├── EXECUTING
├── COMPLETED
├── FAILED
└── ROLLED_BACK

计划详情
├── 动作数量
├── 文件数量
├── 容量口径
├── 风险级别
├── 审批信息
├── dry-run 结果
├── stale 阻止
├── 保护阻止
└── 执行准备检查

执行控制
├── 重新 dry-run
├── 执行隔离
├── 清理隔离区
├── 高风险确认语句
└── 执行进度
```

## 9.4 容量展示

必须同时显示：

- 原始候选容量；
- dry-run 可执行容量；
- stale 阻止容量；
- 保护规则阻止容量；
- 预计释放容量；
- 实际完成容量。

禁止只显示一个容易误导的“大号释放空间”。

## 9.5 高风险确认

不可只使用红色按钮或普通确认框。

确认语句必须包含实际数量，例如：

```text
确认隔离 126 个文件，共 48.2 GB
```

Purge 应更严格，例如：

```text
确认永久清理隔离区中的 126 个文件，共 48.2 GB
```

前端输入只作为交互门槛，后端仍必须重新校验计划、revision、stale、路径和隔离范围。

## 9.6 前端组件

```text
pages/execution-center/
├── ExecutionCenterPage.tsx
├── PlanList.tsx
├── PlanSummary.tsx
├── ReadinessChecklist.tsx
├── DryRunSummary.tsx
├── BlockedActionsTable.tsx
├── ExecutionConfirmation.tsx
├── ExecutionProgress.tsx
└── PurgePanel.tsx
```

## 9.7 当前后端能力

后端已完成：

- 重复文件规划；
- 三步分离；
- Quarantine；
- Purge；
- stale 检查；
- 校验；
- 审计；
- 回滚相关约束；
- 26 个隔离集成测试；
- Purge 限制在隔离区内。

因此本页面属于“已有服务能力的桌面接线”，不应长期占位。

## 9.8 后端需暴露的接口

建议：

```go
ListPlans(req ListPlansRequest) ListPlansResponse
GetPlanDetail(planID string) PlanDetail
ValidatePlanExecution(planID string) ExecutionReadiness
DryRunPlan(planID string) DryRunReport
StartQuarantine(req StartQuarantineRequest) StartJobResponse
StartPurge(req StartPurgeRequest) StartJobResponse
GetExecutionProgress(jobID string) JobProgressSnapshot
CancelExecution(jobID string) error
```

所有执行动作应进入统一 JobManager，避免扫描任务与执行任务采用两套状态机制。

建议 `job_type` 至少包括：

```text
SCAN
ANALYZE
QUARANTINE
PURGE
RECOVERY
EXPORT
```

## 9.9 验收标准

- 桌面端能完成 CLI 已能完成的隔离流程；
- 执行前检查逐项可见；
- dry-run 与真实执行分离；
- stale 和保护阻止原因可查看；
- Purge 只能对隔离区对象执行；
- 执行任务可审计；
- 执行中关闭应用后，下次启动可以进入恢复流程；
- 后端不信任前端传入的路径、容量和动作数量。

---

# 10. 页面六：审计与恢复

## 10.1 用户目标

- 查看操作审计；
- 查看执行 Journal；
- 处理未完成执行；
- 查看可恢复运行；
- 查看回滚记录；
- 导出脱敏报告；
- 查看技术诊断和日志。

## 10.2 启动优先检查

应用打开项目后，应优先调用：

```go
ListExecutingPlans()
ListRecoverableRuns()
```

若存在未完成执行：

- 顶部显示全局恢复横幅；
- 禁止新扫描以外的冲突写操作，具体锁定范围由后端能力返回；
- 禁止启动新的隔离或 Purge；
- 导航优先进入“审计与恢复”；
- 用户必须明确选择恢复、回滚、标记失败或查看详情。

## 10.3 页面结构

```text
恢复提示区
├── 未完成计划
├── 可恢复运行
├── 风险说明
└── 恢复操作

审计时间线
├── 用户操作
├── 计划变化
├── 审批
├── dry-run
├── 隔离
├── Purge
└── 回滚

Journal 与执行详情
├── 动作序列
├── 路径脱敏
├── 前后状态
├── 错误
└── 校验结果

导出与诊断
├── 脱敏报告
├── 私有诊断
├── 日志目录
└── 复制技术信息
```

## 10.4 前端组件

```text
pages/audit-recovery/
├── AuditRecoveryPage.tsx
├── RecoveryBanner.tsx
├── RecoverableRunList.tsx
├── RecoveryActionPanel.tsx
├── AuditTimeline.tsx
├── JournalViewer.tsx
├── RollbackHistory.tsx
├── DiagnosticExport.tsx
└── TechnicalInfoPanel.tsx
```

## 10.5 后端需暴露的接口

```go
ListExecutingPlans() []ExecutingPlanSummary
ListRecoverableRuns() []RecoverableRun
GetRecoverableRun(runID string) RecoverableRunDetail
ResumeRun(runID string) StartJobResponse
RollbackRun(runID string) StartJobResponse
MarkRunFailed(req MarkRunFailedRequest) RunState
ListAuditEvents(req ListAuditEventsRequest) ListAuditEventsResponse
GetJournal(runID string) JournalDetail
ListRollbackRecords(req ListRollbackRequest) ListRollbackResponse
ExportAuditReport(req ExportAuditRequest) ExportResult
GetPrivateDiagnostics() PrivateDiagnosticReport
OpenLogDirectory() error
```

## 10.6 验收标准

- 恢复锁不会只存在于前端内存；
- 刷新页面或重启应用后仍能恢复锁状态；
- 用户能明确知道恢复、回滚和标记失败的差异；
- Journal 默认脱敏；
- 导出文件不包含未经用户同意的完整私密路径；
- 所有恢复动作自身也进入审计。

---

# 11. 页面七：设置

## 11.1 设置分组

### 扫描

- 默认 worker 数；
- 默认完整/增量；
- 重试策略；
- 排除规则；
- 跨挂载策略；
- 检查点策略。

### 隐私

- 路径显示：完整 / 部分 / 文件名；
- 截图模式；
- 导出脱敏；
- 是否记录最近项目；
- 错误详情级别；
- 是否在任务详情显示最近路径。

### 性能

- 重复组页大小；
- 事件刷新频率；
- 只读连接数量；
- 缓存上限；
- 虚拟列表 overscan。

### 开发者

- 版本；
- commit；
- build time；
- schema 版本；
- 规则版本；
- 运行诊断；
- 打开日志目录；
- 复制系统信息。

### AI 与联网

必须明确显示当前状态：

- 外部 AI：关闭；
- 遥测：关闭；
- 云上传：关闭；
- 联网更新检查：开启或关闭；

不得用模糊措辞。

## 11.2 设置作用域

区分：

- 应用级设置；
- 项目级设置；
- 数据源级设置。

前端应在字段旁标明作用域，避免用户误以为只影响当前项目。

## 11.3 后端接口

```go
GetSettings() SettingsSnapshot
UpdateAppSettings(req UpdateAppSettingsRequest) SettingsSnapshot
UpdateProjectSettings(req UpdateProjectSettingsRequest) SettingsSnapshot
UpdateStorageSettings(req UpdateStorageSettingsRequest) StorageSettings
ResetSettings(scope string) SettingsSnapshot
```

设置应由后端持久化和校验，前端不以 localStorage 作为唯一事实来源。

---

# 12. 诊断能力重新归位

不再保留独立“诊断报告”一级页面。

| 诊断能力 | 新位置 | 用途 |
|---|---|---|
| 格式诊断 | 数据源 | 数据质量、格式错配、大型未知文件 |
| 治理诊断 | 治理复核 | 重复治理候选、零字节、大媒体、草稿计划 |
| 合并诊断 | 重复结果/治理复核 | 名称相似、目录合并候选 |
| 私有诊断 | 审计与恢复/设置 | 技术排障、日志、环境状态 |

短期迁移期间可以保留 `DiagnosticPanel` 组件，但应拆成三个可复用视图，不再以一级导航存在。

---

# 13. 前端代码架构

## 13.1 推荐目录

```text
frontend/src/
├── app/
│   ├── App.tsx
│   ├── AppShell.tsx
│   ├── routes.ts
│   ├── navigation.ts
│   ├── capability.ts
│   └── startup.ts
│
├── api/
│   ├── wailsClient.ts
│   ├── projectApi.ts
│   ├── storageApi.ts
│   ├── jobsApi.ts
│   ├── duplicatesApi.ts
│   ├── governanceApi.ts
│   ├── executionApi.ts
│   ├── auditApi.ts
│   ├── settingsApi.ts
│   ├── errorCodes.ts
│   └── types.ts
│
├── state/
│   ├── ProjectProvider.tsx
│   ├── AppStatusProvider.tsx
│   ├── NotificationProvider.tsx
│   └── selectors.ts
│
├── hooks/
│   ├── useProject.ts
│   ├── useCapabilities.ts
│   ├── useJobPolling.ts
│   ├── usePaginatedJobs.ts
│   ├── useDuplicateGroups.ts
│   ├── useAsyncAction.ts
│   ├── useDiscardStaleResult.ts
│   └── usePrivacyPath.ts
│
├── pages/
│   ├── sources/
│   ├── scan-jobs/
│   ├── duplicate-results/
│   ├── governance-review/
│   ├── execution-center/
│   ├── audit-recovery/
│   └── settings/
│
├── components/
│   ├── layout/
│   ├── data-display/
│   ├── feedback/
│   ├── forms/
│   ├── overlays/
│   └── privacy/
│
├── design-system/
│   ├── tokens.css
│   ├── base.css
│   ├── components.css
│   ├── typography.css
│   └── states.css
│
├── utils/
│   ├── formatBytes.ts
│   ├── formatDuration.ts
│   ├── formatDateTime.ts
│   ├── stableSort.ts
│   ├── guards.ts
│   └── testIds.ts
│
└── tests/
    ├── fixtures/
    ├── unit/
    └── integration/
```

目录可根据现有仓库结构调整，但职责边界应保留。

## 13.2 App.tsx 职责

`App.tsx` 只负责：

- 挂载 Provider；
- 启动初始化；
- 渲染 AppShell；
- 全局错误边界；
- 不持有重复组分页、扫描表单、诊断结果等页面业务状态。

目标：`App.tsx` 控制在约 100–180 行以内。

## 13.3 状态分层

### 全局状态

- 应用版本与能力；
- 当前项目；
- 项目模式；
- 全局恢复锁；
- 隐私模式；
- 活跃任务摘要；
- Toast。

### 领域查询状态

放在 hooks 或页面 Provider：

- 任务列表与分页；
- 重复组筛选与分页；
- 治理队列；
- 计划列表；
- 审计时间线。

### 页面本地状态

- 表单输入；
- 当前展开项；
- 抽屉开关；
- 未应用筛选条件；
- 当前 tab。

## 13.4 不建议立即引入大型状态库

当前规模可使用：

- React Context；
- `useReducer`；
- 自定义 hooks；
- 明确的 API service；

先解决边界问题。只有在跨页面缓存、乐观更新、复杂失效明显增加后，再考虑 TanStack Query 或 Zustand。

禁止把所有状态重新集中到一个超大 Context。

## 13.5 请求生命周期

所有异步请求应统一处理：

```typescript
interface AsyncState<T> {
  status: "idle" | "loading" | "success" | "error";
  data?: T;
  error?: AppError;
}
```

要求：

- 防止重复请求；
- 项目切换时丢弃旧请求结果；
- 页面卸载后不更新状态；
- 对可重试错误提供重试；
- 对权限或模式错误给出明确下一步；
- 不向普通用户直接显示 Go 堆栈或原始 panic。

## 13.6 API 错误标准化

后端错误应转换为稳定错误码：

```typescript
interface AppError {
  code: string;
  message: string;
  details?: Record<string, unknown>;
  retryable: boolean;
  user_action?: string;
}
```

建议至少包含：

```text
PROJECT_ALREADY_OPEN
PROJECT_NOT_OPEN
PROJECT_NOT_READ_WRITE
INVALID_PROJECT_PATH
STORAGE_NOT_FOUND
SOURCE_OFFLINE
SCAN_ALREADY_RUNNING
JOB_NOT_FOUND
JOB_NOT_CANCELLABLE
PLAN_STALE
PLAN_NOT_APPROVED
UNRESOLVED_REVIEW
PROTECTED_ITEM_BLOCKED
QUARANTINE_NOT_READY
JOURNAL_NOT_READY
RECOVERY_LOCK_ACTIVE
REVISION_CONFLICT
PERMISSION_DENIED
INTERNAL_ERROR
```

前端只针对错误码决定行为，不解析英文错误字符串。

---

# 14. 设计系统与视觉规则

## 14.1 风格目标

- 专业、可靠、克制；
- 数据治理工具而非消费级清理动画；
- 不使用大量渐变、玻璃拟态和夸张阴影；
- 不使用 Emoji 代替功能图标；
- 强调信息密度、状态清晰和风险层级。

## 14.2 颜色语义

颜色只表达稳定语义：

- 蓝色：主要操作、当前选择；
- 绿色：验证通过、完成；
- 黄色：待复核、警告；
- 红色：阻止、高风险、失败；
- 灰色：禁用、次要、未配置；
- 紫色或青色：可用于治理建议，但不可与风险色混淆。

不能只靠颜色表达状态，必须同时有文本或图标。

## 14.3 风险按钮

按钮层级：

- Primary：继续当前安全流程；
- Secondary：查看、刷新、编辑；
- Tertiary：次要辅助；
- Destructive：清理、回滚等高风险动作。

“隔离”不等同于“删除”，但仍属于高影响操作，应使用警示样式而非普通 Primary。

## 14.4 空状态

每个页面都必须实现：

- 未打开项目；
- 无数据源；
- 无任务；
- 无重复结果；
- 无待复核项；
- 无可执行计划；
- 无审计记录；
- 无恢复任务。

空状态必须告诉用户下一步，不只显示“暂无数据”。

## 14.5 加载状态

- 页面骨架屏用于首屏；
- 按钮级 spinner 用于短操作；
- 轮询进度不反复闪烁；
- 大表格加载更多保留当前内容；
- 筛选切换时可显示顶部轻量加载条；
- 不要清空旧数据后再显示 loading，避免界面跳动。

## 14.6 路径显示

统一使用 `PathText` 组件：

- 支持完整、部分、文件名模式；
- 中间截断，不只截尾；
- hover 或点击可查看完整路径；
- 复制时根据隐私设置决定复制完整或脱敏值；
- 截图模式下自动脱敏用户目录、NAS 名称和共享名。

---

# 15. 后端协同优化总表

## 15.1 当前桌面已暴露能力

| 能力 | 状态 | 前端位置 |
|---|---|---|
| 版本信息 | 已有 | 设置/状态栏 |
| 打开项目 | 已有 | 数据源 |
| 只读/读写 | 已有 | 数据源/全局能力 |
| 项目路径校验 | 已有但应充分利用 | 数据源 |
| 存储列表 | 已有 | 数据源/扫描任务 |
| 启动扫描 | 已有 | 扫描任务 |
| 扫描进度 | 已有 | 扫描任务 |
| 取消扫描 | 已有 | 扫描任务 |
| 任务历史与详情 | 已有 | 扫描任务/审计 |
| 重复组分页 | 已有 | 重复结果 |
| 组详情 | 已有 | 重复结果 |
| 格式诊断 | 已有 | 数据源 |
| 治理诊断 | 已有 | 治理复核 |
| 合并诊断 | 已有 | 重复结果/治理复核 |

## 15.2 后端已有但桌面需接线

| 能力 | 后端状态 | 桌面目标 |
|---|---|---|
| 目录角色/权威/业务锚点 | 已完成 | 重复结果、治理复核 |
| 重复规划与保留评分 | 已完成 | 治理复核 |
| 计划/审批/执行分离 | 已完成原则与服务 | 治理复核、执行中心 |
| Quarantine | 已完成 | 执行中心 |
| Purge | 已完成 | 执行中心 |
| stale/校验/审计 | 已完成 | 执行中心、审计恢复 |
| 崩溃恢复 | JobManager 已完成 | 扫描任务、审计恢复 |
| 物理身份安全 | 已完成 | 重复结果证据 |

## 15.3 优先新增 Binding 组

### P0：能力与准备状态

```go
GetAppCapabilities
GetProjectReadiness
GetQuarantineConfig
ValidateQuarantine
ListRecoverableRuns
ListExecutingPlans
```

### P1：治理复核

```go
ListReviewItems
GetReviewEvidence
SaveReviewDecision
PreviewBatchDecision
ApplyBatchDecisionDraft
ListPlans
GetPlanDetail
ApprovePlan
RejectPlan
```

### P2：执行

```go
ValidatePlanExecution
DryRunPlan
StartQuarantine
StartPurge
GetExecutionProgress
CancelExecution
```

### P3：审计恢复

```go
ListAuditEvents
GetJournal
GetRecoverableRun
ResumeRun
RollbackRun
MarkRunFailed
ExportAuditReport
```

### P4：设置与体验

```go
GetSettings
UpdateAppSettings
UpdateProjectSettings
UpdateStorageSettings
ListRecentProjects
OpenLogDirectory
GetPrivateDiagnostics
```

## 15.4 能力描述接口

建议新增统一能力返回：

```typescript
interface AppCapabilities {
  project_open: boolean;
  project_mode: "closed" | "read_only" | "read_write";
  can_scan: boolean;
  can_edit_reviews: boolean;
  can_approve_plans: boolean;
  can_execute_quarantine: boolean;
  can_execute_purge: boolean;
  recovery_lock_active: boolean;
  telemetry_enabled: boolean;
  cloud_upload_enabled: boolean;
  external_ai_enabled: boolean;
  reasons: CapabilityReason[];
}
```

前端导航和按钮统一读取该对象，避免多个组件重复推断权限。

## 15.5 API 版本与生成类型

- Wails 生成的 TypeScript 类型作为传输层类型；
- 前端可以建立 ViewModel，但不得手写一份与后端重复且长期漂移的 DTO；
- API 响应增加 `api_version` 或应用能力版本；
- 项目详情返回 `schema_version`、`rules_version`；
- Breaking change 必须同步更新生成代码和前端适配层；
- 禁止在组件中直接散落调用 `wailsjs/go/...`，统一通过 `src/api` 包装。

## 15.6 后端列表接口统一规范

所有大列表统一：

```typescript
interface PageRequest {
  page_size?: number;
  cursor?: string;
}

interface PageResponse<T> {
  items: T[];
  next_cursor?: string;
  total_count?: number;
  snapshot_revision?: string;
}
```

要求：

- 默认稳定排序；
- cursor 与筛选条件绑定；
- 筛选改变必须重置 cursor；
- `snapshot_revision` 可用于识别扫描后数据变化；
- 不要求每个大列表都返回昂贵的精确 `total_count`。

## 15.7 后端执行安全

所有执行 API 必须：

- 只接受计划 ID、revision 和用户确认信息，不接受前端提交任意源路径列表；
- 后端重新读取计划；
- 后端重新检查 stale；
- 后端重新检查保护规则；
- 后端重新验证隔离区；
- 后端创建 Journal；
- 后端返回持久化 job ID；
- 所有状态变化可审计；
- 应用崩溃后能够恢复或明确失败；
- Purge 只能作用于隔离区记录，不能接受普通文件系统路径。

---

# 16. 分阶段实施路线

## Phase 0：冻结基线与手动验收

目标：在重构前确认当前功能真实可用。

任务：

1. 使用 `var/smoke-test.db` 完成只读打开；
2. 验证存储列表；
3. 验证重复组分页与详情；
4. 验证三类诊断渲染；
5. 使用读写模式启动一次扫描；
6. 验证进度、取消、Toast、任务历史；
7. 记录所有现存 UI 缺陷；
8. 为当前关键流程补最小回归测试或测试清单。

完成门槛：

- 当前链路有明确验收记录；
- 不带未知严重交互问题进入重构；
- 建立重构前截图或录屏参考。

## Phase 1：AppShell 与七域导航

目标：建立稳定框架，不改变业务行为。

任务：

1. 创建 `AppShell`；
2. 创建七个路由标识；
3. 创建 Sidebar；
4. 将当前功能按领域临时迁移；
5. 取消独立概览；
6. 取消独立诊断一级入口；
7. 建立全局能力和项目状态；
8. 保持扫描轮询位于稳定生命周期中；
9. 未接线页面显示“能力建设中”及已有后端状态，不伪造按钮。

完成门槛：

- 所有现有功能都能从新导航访问；
- 没有功能丢失；
- App.tsx 明显缩小；
- 导航禁用原因明确。

## Phase 2：数据源与扫描任务

目标：让用户能够清晰完成项目打开和扫描。

任务：

1. 数据源页重构；
2. 路径预校验；
3. 项目模式与能力展示；
4. 合并扫描表单、进度和历史；
5. 改正发现阶段百分比；
6. 任务列表后端分页；
7. 事件时间线可读化；
8. 文件拖放稳定化；
9. 扫描完成后的全局数据失效刷新。

完成门槛：

- 新用户不阅读说明也能完成首次扫描；
- 只读限制清楚；
- 任务错误有可行动提示；
- 不显示虚假进度。

## Phase 3：重复结果与语境证据

目标：呈现 NDG 与普通去重工具的差异。

任务：

1. 三栏重复结果页；
2. 紧凑虚拟化列表；
3. 稳定排序与 keyset 分页；
4. 目录 1–4 级语境；
5. 角色、权威、业务锚点；
6. 保护项和备份关系；
7. 物理身份与硬链接证据；
8. 格式/合并诊断归位；
9. 进入治理复核的上下文传递。

完成门槛：

- 用户可以理解为什么“相同内容”不一定应去重；
- 大列表性能可接受；
- 容量口径清晰；
- 保护项不可被忽略。

## Phase 4：治理复核与计划

目标：完成可解释决策与草案计划。

任务：

1. 复核队列；
2. 证据对比；
3. 决策编辑；
4. 目录角色/业务锚点纠正；
5. 批量规则预览；
6. 草案计划；
7. 审批与驳回；
8. revision 冲突处理；
9. 治理诊断归位。

完成门槛：

- 决策可解释、可追踪；
- 草案不会触发文件操作；
- 批量操作有预览；
- 只读项目不会写入。

## Phase 5：执行中心

目标：把后端现有 Quarantine/Purge 安全地接入桌面端。

任务：

1. 执行准备状态；
2. dry-run；
3. stale 与保护阻止；
4. 隔离执行任务；
5. Purge；
6. 高风险确认语句；
7. 容量对账；
8. 统一 JobManager 展示；
9. 关闭应用后的恢复验证。

完成门槛：

- 桌面端可替代 CLI 完成安全隔离；
- 后端安全约束不因 UI 接入降低；
- 执行结果可审计和恢复。

## Phase 6：审计恢复与设置

目标：形成正式可发布桌面应用的信任闭环。

任务：

1. 启动恢复检查；
2. 恢复锁；
3. Journal；
4. 审计时间线；
5. 回滚记录；
6. 脱敏导出；
7. 设置持久化；
8. 隐私模式；
9. 开发者信息；
10. AI、遥测、云上传状态透明化。

完成门槛：

- 崩溃后用户知道发生了什么；
- 能继续、回滚或明确失败；
- 隐私设置真实生效；
- 用户能确认软件没有进行隐蔽联网。

## Phase 7：体验与发布打磨

任务：

- 键盘导航；
- 列表虚拟化压测；
- 4K、高 DPI、小窗口；
- 中文长路径；
- 网络盘掉线；
- 错误文案；
- 首次使用引导；
- 截图隐私模式；
- 性能与内存检查；
- 发布构建与升级策略。

---

# 17. 迁移现有组件的建议

| 当前组件/能力 | 新位置 | 处理方式 |
|---|---|---|
| `ProjectPanel` | 数据源 | 保留逻辑，重构布局与校验 |
| `StorageList` | 数据源 | 升级为 SourceList |
| `ScanPanel` | 扫描任务 | 拆为表单、进度、历史、详情 |
| `DuplicateGroups` | 重复结果 | 从普通表格/卡片升级为三栏列表 |
| `GroupDetail` | 重复结果右侧 | 改为 EvidenceInspector |
| `DiagnosticPanel` | 多页面 | 拆分三个诊断视图 |
| `JobDetail` | 扫描任务/审计 | 时间线化，保留原始 JSON 高级展开 |
| Toast | 全局反馈 | 迁移到 NotificationProvider |
| Wails filedrop | 扫描任务 | 封装 hook，保证清理 |
| 当前轮询 effect | `useJobPolling` | 保证页面切换不断开 |

禁止一次性删除旧组件后再重写。应先建立新壳层，再逐个迁移并保持可运行。

---

# 18. 测试与验收矩阵

## 18.1 项目生命周期

- 打开不存在但允许创建的数据库；
- 打开已有数据库；
- 只读打开；
- 读写打开；
- 打开错误扩展名；
- 打开符号链接；
- 已打开项目时再次打开；
- 关闭项目时存在活跃任务；
- 项目切换后旧请求迟到；
- schema 不兼容。

## 18.2 扫描

- 空目录；
- 大量小文件；
- 大文件；
- 中文路径；
- 超长路径；
- 权限不足；
- SMB/NFS/FUSE；
- 网络盘中途掉线；
- 取消；
- 取消请求中关闭应用；
- 崩溃恢复；
- `discovered = 0`；
- 发现阶段无百分比；
- 完整扫描与增量扫描；
- 多次快速点击启动。

## 18.3 重复结果

- 无重复组；
- 纯硬链接组；
- 多物理副本；
- 有保护项；
- 跨目录角色；
- 多业务锚点；
- 交叉归档；
- 备份关系；
- 数千组分页；
- 快速点击加载更多；
- 筛选后 cursor 重置；
- 扫描完成后快照变化。

## 18.4 治理复核

- 保留全部；
- 主要保留项；
- 隔离草案；
- 暂缓；
- 修正角色；
- 修正业务锚点；
- 批量预览；
- 保护项阻止；
- revision 冲突；
- 只读模式写入尝试。

## 18.5 执行与恢复

- 未批准计划；
- 未解决 REVIEW；
- stale；
- 隔离区不可写；
- SourceRoots 不匹配；
- Journal 不可用；
- 隔离成功；
- 隔离部分失败；
- Purge 非隔离路径；
- 执行中崩溃；
- 恢复；
- 回滚；
- 重复恢复请求；
- 恢复锁期间启动新执行。

## 18.6 前端质量

- TypeScript 严格模式；
- 组件错误边界；
- 无未清理事件监听；
- 无重复轮询；
- 无卸载后 setState；
- 键盘可操作主要控件；
- 状态不只靠颜色表达；
- 4K 和高 DPI；
- 小窗口；
- 深色模式可暂缓，但 CSS token 不阻碍未来支持。

---

# 19. TRAE 执行要求

以下内容可视为对 TRAE 的直接工程指令。

## 19.1 开始前

1. 阅读：
   - `AGENTS.md`；
   - 当前 `bindings.go`；
   - 应用服务层；
   - 当前 `App.tsx` 和组件目录；
   - 本文档；
   - `docs/ui/information-architecture.md`。
2. 运行并记录基线：

```bash
go test -race ./...
go vet ./...
gofmt -l .
cd frontend && npm run typecheck
cd frontend && npm run build
wails build
```

如仓库脚本名称不同，使用仓库已有等价命令。

3. 不要在基线未通过时开始大规模重构；
4. 不要根据本文虚构当前不存在的 Go 类型，先定位已有服务能力；
5. API 命名可适配现有代码规范，但业务语义不可丢失。

## 19.2 实施方式

- 严格按 Phase 0 → Phase 7 推进；
- 每个 Phase 拆成独立小 PR 或提交组；
- 每次提交只解决一个明确领域；
- 前后端接口变更同步提交生成绑定；
- 每完成一个页面，补充空态、加载态、错误态、只读态；
- 不使用 mock 数据冒充已完成能力；
- 尚未接线的页面可以展示真实后端进度说明，但不创建无效按钮；
- 保留 CLI，桌面端接线不应破坏 CLI；
- 复用应用服务，不在 Wails Binding 中复制业务逻辑；
- Wails Binding 只做参数转换、错误标准化和服务调用。

## 19.3 禁止事项

- 禁止把七域导航改回五个临时页面；
- 禁止新增独立 Dashboard；
- 禁止保留独立“诊断报告”一级入口；
- 禁止在重复结果页直接永久删除；
- 禁止前端提交任意路径列表要求后端执行；
- 禁止使用 `processed / discovered` 作为发现阶段总进度；
- 禁止用大量卡片替代数据密集列表；
- 禁止把完整私密路径默认显示在进度区；
- 禁止把所有状态继续堆入 App.tsx；
- 禁止通过匹配错误字符串控制业务；
- 禁止用 localStorage 作为项目治理状态的唯一事实来源；
- 禁止因前端接入而绕过审批、stale、保护、Journal 或隔离范围检查；
- 禁止大爆炸式一次性重写全部前端。

## 19.4 每阶段输出

每个阶段完成后，TRAE 应输出：

1. 修改摘要；
2. 新增/修改文件列表；
3. API 变化；
4. 数据库或 schema 变化；
5. 自动化测试结果；
6. 手动验收步骤；
7. 已知限制；
8. 下一阶段建议；
9. 对本文档中未能实现部分的原因说明。

---

# 20. 完成定义（Definition of Done）

NDG 桌面端达到本轮架构推进完成，需要同时满足：

## 易用性

- 新用户能在不阅读长说明的情况下完成项目打开和首次扫描；
- 主要流程不超过七个稳定一级入口；
- 页面标题、操作和状态清晰；
- 高级参数默认收起；
- 错误提示包含下一步动作。

## 产品完整性

- 数据源、扫描、重复结果、治理复核、执行、审计恢复形成闭环；
- 目录角色与业务锚点进入主要界面；
- 相同内容不被默认等同于可删除；
- 诊断能力回到业务上下文。

## 安全性

- 计划、审批、执行分离；
- dry-run 可见；
- stale、保护、隔离区、Journal 检查不可绕过；
- Purge 仅作用于隔离区；
- 崩溃后可恢复或回滚；
- 所有写操作可审计。

## 工程质量

- App.tsx 不再是业务状态中心；
- API 调用集中封装；
- 大列表支持分页与稳定排序；
- 轮询无泄漏、无重复请求；
- 传输类型与后端同步；
- 错误码稳定；
- 现有 Go/前端/Wails 验证持续通过。

## 隐私与信任

- 路径显示可配置；
- 截图和导出支持脱敏；
- 外部 AI、遥测、云上传状态透明；
- 进度区不默认暴露文件名和完整路径；
- 私有诊断与普通用户界面分离。

---

# 21. 最终推进优先级

按对产品价值和返工风险排序：

```text
P0  当前桌面链路手动验收
P1  七域 AppShell 与能力模型
P2  数据源 + 扫描任务整合
P3  重复结果三栏化 + 目录语境证据
P4  治理复核 + 草案计划 + 审批
P5  Quarantine/Purge 桌面接线
P6  审计恢复 + 设置 + 隐私
P7  性能、可访问性与发布打磨
```

核心判断：

> 当前后端并非“只完成扫描和重复检测”。目录语境、规划、隔离和清理已经具备。下一步应优先将这些能力组织成完整桌面治理流程，而不是继续扩展一个以扫描、重复列表和诊断标签为中心的临时前端。
