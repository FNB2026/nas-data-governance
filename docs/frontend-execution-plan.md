# NDG 桌面端前端重构执行方案

> 状态：历史方案，已于 2026-07-29 被[《NDG 桌面端前端架构与后端协同推进方案》](desktop-frontend-architecture.md)取代。
> 保留目的：记录早期五页面拆分、现有 Binding 清单和迁移背景；不得再作为导航、页面边界或实施顺序的现行依据。
> 冲突处理：凡与七域导航、诊断能力归位、治理复核、执行中心或审计恢复闭环冲突之处，以现行主文档为准。

## 1. 目标与现状

### 1.1 现状

当前前端为单页面卡片布局（`App.tsx` 约 517 行），所有功能区域平铺在同一个滚动页面中，通过 `ProjectPanel` → `ScanPanel` → `StorageList` → `DuplicateGroups` → `DiagnosticPanel` 的顺序自上而下排列。项目打开后，用户需要在同一个长页面中上下滚动寻找不同功能区，缺乏页面层级感和导航指引。

### 1.2 目标架构

改为左侧导航栏 + 右侧内容区的多页面布局，共五个页面：

| 页面 | 标识 | 说明 |
| --- | --- | --- |
| 概览 | `overview` | 项目入口、存储摘要、最近任务、诊断报告快捷入口 |
| 扫描位置 | `scan-location` | 资源库列表 + 扫描目标管理，支持拖放文件夹 |
| 扫描进度 | `progress` | 实时进度条、阶段统计、任务历史 |
| 重复文件组 | `duplicates` | 左侧筛选面板 + 右侧交替底色文件组列表 |
| 诊断报告 | `diagnostics` | 三类只读诊断报告（格式/治理/合并） |

导航栏在项目未打开时仅显示「概览」入口；项目打开后根据读写模式动态启用其他页面。

### 1.3 设计原则

- **后端优先**：每个页面的数据来源严格绑定后端 API，前端只做展示和状态管理，不引入本地 mock 数据
- **读写模式感知**：只读模式（`OpenProject`）下禁用所有写操作入口；读写模式（`OpenProjectReadWrite`）下解锁扫描功能
- **状态提升**：跨页面共享的状态（项目信息、存储列表、任务列表）保持在 `App.tsx` 顶层，通过 props 下发
- **渐进加载**：大列表使用 keyset 分页，进度使用轮询，避免一次性拉取全部数据

---

## 2. 后端 API 全景

后端绑定层位于 `internal/adapters/wails/bindings.go`，通过 Wails 框架将 Go 方法暴露为前端可调用的 JavaScript 函数。前端通过 `wailsjs/go/wails/API` 模块调用。

### 2.1 API 清单与页面映射

| API 方法 | 签名 | 所属页面 | 读写要求 |
| --- | --- | --- | --- |
| `GetVersion` | `() → VersionInfo` | 概览 | 无 |
| `OpenProject` | `(path) → ProjectInfo` | 概览 | 无（只读打开） |
| `OpenProjectReadWrite` | `(path) → ProjectInfo` | 概览 | 无（读写打开） |
| `CloseProject` | `() → void` | 概览 | 无 |
| `GetProjectInfo` | `() → ProjectInfo` | 概览 | 需项目已打开 |
| `ValidateProjectPath` | `(path) → void` | 概览 | 无 |
| `ListStorages` | `() → StorageInfo[]` | 概览、扫描位置 | 需项目已打开 |
| `ListDuplicateGroups` | `(req) → ListGroupsResponse` | 重复文件组 | 需项目已打开 |
| `GetGroupDetail` | `(storageID, sha256) → GroupDetailResponse` | 重复文件组 | 需项目已打开 |
| `StartScan` | `(req) → StartScanResponse` | 扫描位置 | 需读写模式 |
| `GetScanProgress` | `(jobID) → ScanJobProgress` | 扫描进度 | 需读写模式 |
| `CancelScan` | `(jobID) → void` | 扫描进度 | 需读写模式 |
| `ListRecentJobs` | `(limit) → JobSummary[]` | 概览、扫描进度 | 需读写模式 |
| `GetJobDetail` | `(jobID) → JobDetailResponse` | 扫描进度 | 需读写模式 |
| `DiagnoseFormats` | `(req) → formatdiag.Report` | 诊断报告 | 需项目已打开 |
| `DiagnoseGovernance` | `(req) → governancediag.Report` | 诊断报告 | 需项目已打开 |
| `DiagnoseMerges` | `(req) → merge.DiagnosticReport` | 诊断报告 | 需项目已打开 |

### 2.2 核心数据类型

```typescript
// 项目信息
interface ProjectInfo {
  path: string;
  is_open: boolean;
  storage_count: number;
}

// 存储条目
interface StorageInfo {
  id: string;
  root_path: string;
  kind: string;
  created_at: string;
}

// 扫描请求
interface StartScanRequest {
  root: string;
  storage_id: string;
  full_scan?: boolean;
  workers?: number;
}

// 扫描进度
interface ScanJobProgress {
  job_id: string;
  state: string;        // QUEUED | RUNNING | CANCEL_REQUESTED | COMPLETED | FAILED | CANCELLED
  stage: string;        // DISCOVERING | METADATA_INDEXING | QUICK_HASHING | ...
  discovered: number;
  processed: number;
  failed: number;
  warning_count: number;
  error_code?: string;
  created_at: string;
  started_at?: string;
  completed_at?: string;
}

// 重复组摘要（列表项）
interface GroupSummary {
  group_id: string;
  sha256: string;
  size: number;
  storage_id: string;
  path_count: number;
  physical_copy_count: number;
  hardlink_alias_count: number;
  physical_reclaimable_bytes: number;
  sample_path: string;
  decision_type?: string;
}

// 重复组详情
interface GroupDetailResponse {
  group_id: string;
  sha256: string;
  size: number;
  storage_id: string;
  path_count: number;
  physical_copy_count: number;
  hardlink_alias_count: number;
  physical_reclaimable_bytes: number;
  sample_path: string;
  decision_type?: string;
  files: FileItem[];
}

// 任务摘要
interface JobSummary {
  job_id: string;
  job_type: string;
  state: string;
  stage: string;
  discovered?: number;
  processed?: number;
  failed?: number;
  error_code?: string;
  created_at: string;
  completed_at?: string;
}

// 任务详情（含事件流）
interface JobDetailResponse {
  job_id: string;
  state: string;
  stage: string;
  discovered: number;
  processed: number;
  failed: number;
  warning_count: number;
  error_code?: string;
  created_at: string;
  started_at?: string;
  completed_at?: string;
  events: JobEvent[];
}
```

### 2.3 后端行为约束

以下约束来自 `bindings.go` 源码和 `AGENTS.md` 规则，前端必须遵守：

1. **项目互斥**：同一时间只能打开一个项目。调用 `OpenProject` 或 `OpenProjectReadWrite` 时如果已有项目打开，后端返回 `ErrProjectAlreadyOpen`，前端必须先调用 `CloseProject`
2. **路径校验**：`ValidateProjectPath` 拒绝符号链接、目录、非 `.db/.sqlite/.sqlite3` 扩展名；读写模式允许文件不存在（创建新库）
3. **扫描前置条件**：`StartScan` 要求项目以读写模式打开（`scanRunner != nil`），否则返回 `ErrProjectNotReadWrite`
4. **进度轮询**：后端不推送事件，前端需通过 `GetScanProgress` 轮询（当前间隔 1 秒）
5. **崩溃恢复**：`OpenProjectReadWrite` 在打开时自动将上次未完成的任务标记为 `FAILED`（`error_code: "crash_recovery"`），前端应在任务列表中正确展示这些历史记录
6. **重复组分页**：`ListDuplicateGroups` 使用 keyset 分页，`next_cursor` 为空表示已到末尾；`page_size` 默认 20
7. **物理副本计数**：`physical_copy_count` 和 `hardlink_alias_count` 区分真实冗余和硬链接别名，`physical_reclaimable_bytes` 仅计算物理副本可回收空间，纯硬链接组不产生可回收容量
8. **诊断只读**：三类诊断 API 均为只读，生成的操作计划状态为 `DRAFT`，后端拒绝返回非草稿结果

---

## 3. 页面实现方案

### 3.1 概览页

**职责**：项目入口、全局摘要、快捷导航

**布局结构**：

```
┌───────────────────────────────────────┐
│  项目状态卡片                            │
│  ┌─ 项目路径 ── 存储数量 ── 模式标记 ─┐  │
│  │  [打开项目] / [关闭项目] [刷新]   │  │
│  └──────────────────────────────────┘  │
├───────────────────────────────────────┤
│  存储摘要          │  最近任务           │
│  ┌ 存储ID  路径 ─┐ │  ┌ 任务 类型 状态 ┐│
│  │ default  /... │ │  │ abc  扫描 完成 ││
│  │ backup   /... │ │  │ def  扫描 失败 ││
│  └──────────────┘ │  └──────────────┘│
├───────────────────────────────────────┤
│  诊断快捷入口                          │
│  [格式诊断]  [治理诊断]  [合并诊断]      │
└───────────────────────────────────────┘
```

**后端 API 调用链**：

```
页面挂载
  ├─ GetVersion()                          → 版本号显示在顶栏
  ├─ [用户输入路径 + 选择模式]
  ├─ OpenProject(path) 或 OpenProjectReadWrite(path)
  │   └─ 成功后并行调用：
  │       ├─ GetProjectInfo()              → 项目元信息
  │       ├─ ListStorages()                → 存储列表
  │       └─ ListRecentJobs(20)            → 最近任务（仅读写模式）
  └─ [用户点击关闭]
      └─ CloseProject()                    → 清空所有状态
```

**组件拆分**：

| 组件 | 职责 | 调用的 API |
| --- | --- | --- |
| `ProjectPanel`（复用现有） | 项目打开/关闭表单 | `OpenProject`, `OpenProjectReadWrite`, `CloseProject`, `GetProjectInfo` |
| `StorageSummary` | 存储列表紧凑展示 | `ListStorages`（数据从 App.tsx 传入） |
| `RecentJobs` | 最近 5 条任务摘要 | `ListRecentJobs`（数据从 App.tsx 传入） |
| `DiagQuickEntry` | 三个诊断入口按钮 | 无直接调用，点击后跳转到诊断报告页 |

**关键实现细节**：

- `RecentJobs` 显示最近 5 条而非全部，点击「查看全部」跳转到扫描进度页
- 存储摘要中每个存储条目显示 `id`、`root_path`（截断显示）、`kind`、`created_at`
- 诊断快捷入口在项目未打开时禁用，点击后切换到诊断报告页并预选对应 tab
- 项目打开成功后，自动加载存储列表和任务列表，数据存入 App.tsx 顶层 state 供其他页面使用

### 3.2 扫描位置页

**职责**：管理扫描目标目录，配置扫描参数，启动扫描

**布局结构**：

```
┌──────────────┬──────────────────────────┐
│  资源库列表    │  扫描目标配置              │
│              │                          │
│ ┌ default ─┐ │  根目录: [____________]   │
│ │ /Volumes  │ │  [拖放文件夹到此区域]      │
│ │ /NAS      │ │                          │
│ └──────────┘ │  存储 ID: [_________]     │
│              │  并发数:  [____]          │
│ ┌ backup ──┐ │  ☐ 全量扫描               │
│ │ /Backup  │ │                          │
│ └──────────┘ │  [立即扫描]               │
│              │                          │
└──────────────┴──────────────────────────┘
```

**后端 API 调用链**：

```
页面挂载
  └─ ListStorages()                        → 左侧资源库列表

[用户选择存储或手动输入 storage_id]
  └─ 将 storage_id 填入扫描表单

[用户拖放文件夹或手动输入路径]
  └─ Wails OnFileDrop 事件 → 填入 root 字段

[用户点击「立即扫描」]
  ├─ 前端校验 root 非空
  ├─ StartScan({
  │     root: scanRoot,
  │     storage_id: scanStorageId || "default",
  │     full_scan: scanFullScan,
  │     workers: scanWorkers || 4
  │   })
  └─ 返回 job_id → 切换到扫描进度页，开始轮询
```

**Wails 文件拖放集成**：

`main.go` 已配置 `DragAndDrop` 选项。前端通过 Wails 运行时事件监听文件拖放：

```typescript
// 在 ScanLocationPage 组件中注册事件监听
useEffect(() => {
  if (!hasWailsRuntime()) return;

  // Wails v2 文件拖放事件
  const offDrop = EventsOn("filedrop", (files: string[]) => {
    if (files && files.length > 0) {
      setScanRoot(files[0]);  // 取第一个文件/目录路径
    }
  });

  return () => {
    offDrop();
  };
}, []);
```

`EventsOn` 需要从 `wailsjs/runtime/runtime` 导入。`main.go` 中的 `DragAndDrop.EnableFileDrop: true` 和 `DisableWebViewDrop: false` 确保拖放事件同时传递到 Go 层和 WebView 层。

**组件拆分**：

| 组件 | 职责 | 调用的 API |
| --- | --- | --- |
| `StorageLibrary` | 左侧存储列表，点击选中 | `ListStorages`（数据从 props 传入） |
| `ScanConfigForm` | 扫描参数表单 + 拖放区域 | `StartScan` |
| `DropZone` | 文件拖放区域，视觉反馈 | Wails `filedrop` 事件 |

**关键实现细节**：

- 左侧资源库列表从 `ListStorages` 获取，点击某条存储后自动将其 `id` 填入右侧的 `storage_id` 字段
- 拖放区域支持高亮反馈：拖入时边框变色，松开后路径填入 `root` 输入框
- 如果项目以只读模式打开，整个页面禁用并提示「请以读写模式重新打开项目以使用扫描功能」
- 扫描启动成功后自动切换到「扫描进度」页面，并将 `job_id` 传入开始轮询

### 3.3 扫描进度页

**职责**：实时展示扫描进度、阶段信息、任务历史

**布局结构**：

```
┌───────────────────────────────────────┐
│  当前扫描状态                           │
│  [████████████░░░░░░] 65%              │
│  阶段: 完整哈希  发现: 1600  处理: 1040  │
│  失败: 3  警告: 5                       │
│  [取消扫描]                             │
├───────────────────────────────────────┤
│  任务历史                               │
│  ┌ ID    类型  状态   阶段  发现 处理 ─┐ │
│  │ abc   扫描  完成   收尾  2470 2470  │ │
│  │ def   扫描  失败   哈希  800  600   │ │
│  └────────────────────────────────────┘ │
│  [加载更多]  筛选: [状态▼] [类型▼]      │
├───────────────────────────────────────┤
│  任务详情（选中后展开）                   │
│  事件流:                                │
│  #1 创建    2024-01-01 10:00:00        │
│  #2 阶段切换 DISCOVERING → INDEXING     │
│  #3 进度更新  processed=500             │
│  ...                                   │
└───────────────────────────────────────┘
```

**后端 API 调用链**：

```
[扫描启动 → job_id 传入]
  └─ setInterval(1000ms)
      └─ GetScanProgress(job_id)
          ├─ 更新进度条和统计数字
          └─ 如果 state ∈ {COMPLETED, FAILED, CANCELLED}:
              ├─ clearInterval 停止轮询
              ├─ ListRecentJobs(20)           → 刷新任务历史
              ├─ ListStorages()               → 刷新存储（可能有新存储）
              ├─ ListDuplicateGroups(...)     → 刷新重复组
              └─ Toast 通知（成功/失败/取消）

[用户点击取消]
  └─ CancelScan(job_id)                    → 后端将状态改为 CANCEL_REQUESTED

[用户点击任务详情]
  └─ GetJobDetail(job_id)                  → 含事件流

[用户筛选任务历史]
  └─ 前端客户端筛选（state / job_type）
```

**进度轮询的状态机**：

```
              StartScan 返回 job_id
                      │
                      ▼
              ┌── QUEUED ──┐
              │            │
              ▼            │
           RUNNING         │
              │            │
    ┌─────────┼─────────┐  │
    ▼         ▼         ▼  │
 COMPLETED  FAILED  CANCELLED
    │         │         │
    └─────────┴─────────┘
              │
        停止轮询 + 通知
```

`CANCEL_REQUESTED` 是中间态：用户点击取消后，后端立即将状态设为 `CANCEL_REQUESTED`，扫描协程退出后变为 `CANCELLED`。前端在轮询到 `CANCEL_REQUESTED` 时继续轮询，直到变为 `CANCELLED`。

**组件拆分**：

| 组件 | 职责 | 调用的 API |
| --- | --- | --- |
| `ProgressDisplay` | 进度条 + 阶段统计 + 取消按钮 | `GetScanProgress`（数据从 props 传入），`CancelScan` |
| `JobHistory` | 任务列表 + 筛选 + 分页 | `ListRecentJobs`（数据从 props 传入） |
| `JobDetail`（复用现有） | 任务详情 + 事件流 | `GetJobDetail` |

**关键实现细节**：

- 进度百分比计算：`processed / discovered * 100`，当 `discovered = 0` 时显示 0%
- 阶段标签使用 `STAGE_LABELS` 映射（发现文件 → 索引元数据 → 快速哈希 → 完整哈希 → 上下文分类 → 格式分析 → 分组 → 规划 → 收尾）
- 任务历史筛选在现有 `ScanPanel` 中已实现（`stateFilter` + `typeFilter`），迁移时保持逻辑不变
- 扫描完成后的数据刷新策略：并行调用 `ListStorages` + `ListRecentJobs` + `ListDuplicateGroups`，避免串行等待
- 无活跃扫描时，进度区域显示空状态提示「暂无进行中的扫描任务」

### 3.4 重复文件组页

**职责**：展示和管理重复文件组，支持筛选和详情查看

**布局结构**：

```
┌──────────────┬──────────────────────────────────┐
│  筛选面板     │  文件组列表                        │
│              │                                  │
│  存储         │  ┌── A组（白色底色）──────────────┐ │
│  [全部 ▼]    │  │ SHA-256  abcd…1234           │ │
│              │  │ 存储: default  大小: 2.5 MB   │ │
│  最小可回收   │  │ 路径数: 3  物理副本: 2        │ │
│  [___] MiB   │  │ 可回收: 2.5 MB  [详情]       │ │
│              │  └──────────────────────────────┘ │
│  [应用筛选]   │  ┌── B组（浅灰底色 #f5f5f7）─────┐ │
│              │  │ SHA-256  efgh…5678           │ │
│              │  │ 存储: default  大小: 10 MB    │ │
│              │  │ 路径数: 5  物理副本: 3        │ │
│              │  │ 可回收: 20 MB  [详情]        │ │
│              │  └──────────────────────────────┘ │
│              │  ┌── C组（白色底色）──────────────┐ │
│              │  │ ...                          │ │
│              │  └──────────────────────────────┘ │
│              │  [加载更多]                       │
└──────────────┴──────────────────────────────────┘
```

**后端 API 调用链**：

```
页面挂载 / 项目打开
  └─ ListDuplicateGroups({
        storage_id: "",
        page_size: 20,
        cursor: "",
        min_reclaimable_bytes: 0
      })
      └─ 返回 { groups, next_cursor, total_count }

[用户设置筛选条件并点击「应用筛选」]
  └─ ListDuplicateGroups({
        storage_id: selectedStorageId,
        page_size: 20,
        cursor: "",
        min_reclaimable_bytes: minMiB * 1024 * 1024
      })

[用户点击「加载更多」]
  └─ ListDuplicateGroups({
        storage_id: appliedStorageId,
        page_size: 20,
        cursor: nextCursor,            ← 上一次返回的 next_cursor
        min_reclaimable_bytes: appliedMinimumBytes
      })
      └─ 追加到已有 groups 列表尾部

[用户点击某组的「详情」]
  └─ GetGroupDetail(storage_id, sha256)
      └─ 返回 { files: FileItem[], ... }
```

**交替底色实现**：

当前代码已使用 `:nth-child` 伪类实现交替底色，但基于 `<table>` 的 `<tr>`。重构为卡片布局后，使用 `.group-card:nth-child(odd)` 和 `.group-card:nth-child(even)` 实现相同效果：

```css
.group-card:nth-child(odd)  { background: #ffffff; }
.group-card:nth-child(even) { background: #f5f5f7; }
```

**组件拆分**：

| 组件 | 职责 | 调用的 API |
| --- | --- | --- |
| `DuplicateFilterPanel` | 左侧筛选：存储下拉 + 最小可回收输入 + 应用按钮 | 无直接调用，通过回调将筛选条件传给父组件 |
| `GroupCard` | 单个重复组卡片展示 | 无直接调用，数据从 props 传入 |
| `GroupList` | 卡片列表 + 加载更多 + 交替底色 | `ListDuplicateGroups`（通过父组件） |
| `GroupDetail`（复用现有） | 组详情弹窗：文件成员列表 | `GetGroupDetail` |

**关键实现细节**：

- `min_reclaimable_bytes` 的单位转换：前端输入 MiB，乘以 `1024 * 1024` 后传给后端
- 分页防抖：使用 `groupsRequestInFlight` ref 防止「加载更多」按钮被快速连点导致重复请求
- 筛选条件分两层：`storageFilter`/`minReclaimableMiB` 是用户输入的待应用值，`appliedStorageFilter`/`appliedMinimumBytes` 是已生效的值。只有点击「应用筛选」后才将前者同步到后者并发起请求
- `physical_copy_count` 和 `hardlink_alias_count` 在卡片中分别展示，帮助用户区分真实冗余和硬链接
- `decision_type` 字段如果存在，在卡片中以标签形式展示（如 `PRIMARY_RETENTION`、`QUARANTINE_CANDIDATE` 等）

### 3.5 诊断报告页

**职责**：运行和展示三类只读诊断报告

此页面直接复用现有 `DiagnosticPanel` 组件，无需大幅改动。唯一调整是将存储选择下拉的数据源从 props 传入（App.tsx 的 `storages` state），确保与概览页数据一致。

**后端 API 调用链**：

```
[用户选择存储 + 点击「运行诊断」]
  ├─ 格式诊断 tab:
  │   └─ DiagnoseFormats({ storage_id: selectedId })
  │       → formatdiag.Report { summary, large_unknown, extension_mismatches, metadata_gaps, safety_notes }
  │
  ├─ 治理诊断 tab:
  │   └─ DiagnoseGovernance({ storage_id: selectedId })
  │       → governancediag.Report { summary, duplicate_reviews, zero_byte_reviews, media_aggregates, large_media_reviews, media_relations, safety_notes }
  │
  └─ 合并诊断 tab:
      └─ DiagnoseMerges({ storage_id: selectedId })
          → merge.DiagnosticReport { summary, name_similar_reviews, safety_notes }
```

**治理诊断报告的关键字段利用**：

治理诊断返回的 `governancediag.Report` 包含丰富的治理信息，前端应充分展示：

| 字段 | 含义 | 展示建议 |
| --- | --- | --- |
| `summary.duplicate_groups` | 重复组总数 | 统计卡片 |
| `summary.theoretical_redundant_bytes` | 理论冗余字节数 | 格式化为 GB/TB |
| `summary.draft_plans` | 草稿操作计划数 | 统计卡片 |
| `summary.critical_plans` | 关键风险计划数 | 红色高亮 |
| `summary.quarantine_candidate_actions` | 隔离候选动作数 | 统计卡片 |
| `summary.zero_byte_files` | 零字节文件数 | 统计卡片 |
| `summary.large_media_files` | 大媒体文件数 | 统计卡片 |
| `summary.large_media_bytes` | 大媒体总字节数 | 格式化 |
| `duplicate_reviews[].draft_plan` | 每个重复组的草稿计划 | JSON 展开 |
| `large_media_reviews[]` | 大媒体文件审查详情 | 表格或卡片 |
| `media_relations[]` | 媒体文件关系 | 关系图或列表 |
| `safety_notes[]` | 安全提示 | 警告框 |

---

## 4. 状态管理策略

### 4.1 顶层状态分层

`App.tsx` 作为状态容器，将状态分为四层：

```
App.tsx
├── 全局状态（跨页面共享）
│   ├── version                    // 版本信息
│   ├── project                    // 项目信息 { path, is_open, storage_count }
│   ├── isReadWrite                // 读写模式标记
│   ├── storages                   // 存储列表
│   └── currentPage                // 当前页面标识
│
├── 扫描状态（扫描位置 + 扫描进度共享）
│   ├── scanRoot, scanStorageId, scanFullScan, scanWorkers  // 表单
│   ├── activeJobId                // 活跃任务 ID
│   ├── scanProgress               // 轮询获得的进度
│   └── cancelling                 // 取消中标记
│
├── 重复组状态（重复文件组页专用）
│   ├── groups, nextCursor, totalCount
│   ├── storageFilter, minReclaimableMiB           // 待应用筛选
│   └── appliedStorageFilter, appliedMinimumBytes  // 已应用筛选
│
└── 详情状态（各页面弹窗/展开）
    ├── selectedGroup              // 选中的重复组详情
    └── selectedJob                // 选中的任务详情
```

### 4.2 页面间数据流

```
概览页                           扫描位置页
  │                                │
  │ OpenProject/RW 成功             │
  │   ├─ storages ──────────────→  │ 左侧资源库列表
  │   └─ jobs ──────────────────→  │ （只读模式禁用）
  │                                │
  │                         StartScan 成功
  │                                │
  │                                ▼
  │                           扫描进度页
  │                                │
  │                    activeJobId 传入
  │                    GetScanProgress 轮询
  │                    扫描完成后:
  │                      ├─ storages 刷新 ──→ 概览页存储摘要更新
  │                      ├─ groups 刷新 ────→ 重复文件组页更新
  │                      └─ jobs 刷新 ──────→ 概览页任务列表更新
  │                                │
  ▼                                ▼
重复文件组页                    诊断报告页
  │                                │
  │ ListDuplicateGroups      DiagnoseFormats/Governance/Merges
  │ GetGroupDetail            （只读，不修改全局状态）
```

### 4.3 轮询生命周期管理

进度轮询的 `useEffect` 依赖 `activeJobId`：

```typescript
useEffect(() => {
  if (!activeJobId) return;

  const intervalId = setInterval(async () => {
    const p = await GetScanProgress(activeJobId);
    setScanProgress(p);

    if (TERMINAL_STATES.has(p.state)) {
      clearInterval(intervalId);
      setActiveJobId(null);
      // 终态后的数据刷新...
    }
  }, 1000);

  return () => clearInterval(intervalId);  // 组件卸载或 activeJobId 变化时清理
}, [activeJobId]);
```

- 轮询在页面切换时不会中断：因为 `activeJobId` 存储在 App.tsx 顶层，`useEffect` 挂载在 App.tsx 而非子组件
- 用户从扫描进度页切走再切回时，进度数据仍在更新，UI 自动反映最新状态
- 终态触发后的刷新操作使用 `Promise.all` 并行执行，避免串行等待

---

## 5. 导航栏实现

### 5.1 导航项定义

```typescript
type Page = "overview" | "scan-location" | "progress" | "duplicates" | "diagnostics";

interface NavItem {
  page: Page;
  label: string;
  icon: string;       // SVG 路径或图标标识
  requiresProject: boolean;   // 是否需要项目已打开
  requiresReadWrite: boolean;  // 是否需要读写模式
}

const NAV_ITEMS: NavItem[] = [
  { page: "overview",      label: "概览",     requiresProject: false, requiresReadWrite: false },
  { page: "scan-location", label: "扫描位置",  requiresProject: true,  requiresReadWrite: true  },
  { page: "progress",      label: "扫描进度",  requiresProject: true,  requiresReadWrite: true  },
  { page: "duplicates",    label: "重复文件组", requiresProject: true,  requiresReadWrite: false },
  { page: "diagnostics",   label: "诊断报告",  requiresProject: true,  requiresReadWrite: false },
];
```

### 5.2 导航项启用/禁用逻辑

```typescript
function isNavEnabled(item: NavItem, projectOpen: boolean, isReadWrite: boolean): boolean {
  if (!item.requiresProject) return true;
  if (!projectOpen) return false;
  if (item.requiresReadWrite && !isReadWrite) return false;
  return true;
}
```

- 项目未打开时：仅「概览」可点击，其他项灰色禁用
- 只读模式打开时：「概览」「重复文件组」「诊断报告」可点击，「扫描位置」「扫描进度」禁用
- 读写模式打开时：全部可点击

### 5.3 页面切换的副作用

切换页面时不销毁全局状态，但某些页面进入时需要触发数据加载：

```typescript
const handlePageChange = (page: Page) => {
  setCurrentPage(page);

  // 进入扫描进度页时，如果没有活跃扫描但有历史任务，刷新任务列表
  if (page === "progress" && !activeJobId && isReadWrite) {
    void loadJobs();
  }

  // 进入重复文件组页时，如果列表为空，自动加载
  if (page === "duplicates" && groups.length === 0 && project) {
    void loadGroups("", appliedStorageFilter, appliedMinimumBytes);
  }
};
```

---

## 6. CSS 样式方案

### 6.1 布局骨架

```css
.app {
  display: flex;
  flex-direction: column;
  min-height: 100vh;
}

.app-body {
  display: flex;
  flex: 1;
  overflow: hidden;
}

.app-sidebar {
  width: 200px;
  flex-shrink: 0;
  background: var(--card-bg);
  border-right: 1px solid var(--border);
  padding: 1rem 0;
}

.app-content {
  flex: 1;
  overflow-y: auto;
  padding: 2rem;
  max-width: 1000px;
  margin: 0 auto;
  width: 100%;
}
```

### 6.2 重复组卡片交替底色

```css
.group-card {
  border-radius: var(--radius);
  padding: 1rem 1.5rem;
  margin-bottom: 0.5rem;
  border: 1px solid var(--border);
  transition: box-shadow 0.15s;
}

.group-card:hover {
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.08);
}

/* 奇数组白色底色，偶数组浅灰底色 */
.group-card:nth-child(odd) {
  background: #ffffff;
}

.group-card:nth-child(even) {
  background: #f5f5f7;
}
```

### 6.3 拖放区域样式

```css
.dropzone {
  border: 2px dashed var(--border);
  border-radius: var(--radius);
  padding: 2rem;
  text-align: center;
  color: var(--text-muted);
  transition: border-color 0.2s, background 0.2s;
}

.dropzone--active {
  border-color: var(--accent);
  background: rgba(0, 113, 227, 0.05);
  color: var(--accent);
}
```

---

## 7. 文件拖放集成

### 7.1 main.go 配置（已完成）

`main.go` 中已配置 Wails 的 `DragAndDrop` 选项：

```go
DragAndDrop: &options.DragAndDrop{
    EnableFileDrop:     true,
    DisableWebViewDrop: false,
},
```

`EnableFileDrop: true` 使 Wails 在文件拖入窗口时触发 `filedrop` 事件，携带文件路径数组。`DisableWebViewDrop: false` 允许 WebView 层同时接收原生拖放事件，以便前端做出视觉反馈。

### 7.2 前端事件监听

Wails v2 的运行时事件通过 `wailsjs/runtime/runtime` 模块暴露：

```typescript
import { EventsOn } from "../wailsjs/runtime/runtime";
```

在扫描位置页组件中注册监听：

```typescript
useEffect(() => {
  if (!hasWailsRuntime()) return;

  const cancel = EventsOn("filedrop", (files: string[]) => {
    if (files && files.length > 0) {
      // 取第一个路径，如果是目录则作为扫描根目录
      const droppedPath = files[0];
      setScanRoot(droppedPath);
      pushToast("info", "已添加路径", droppedPath);
    }
  });

  return () => {
    if (typeof cancel === "function") cancel();
  };
}, []);
```

### 7.3 拖放视觉反馈

使用 HTML5 Drag & Drop API 的 `dragenter`/`dragleave` 事件实现视觉反馈（这些事件在 WebView 层触发，与 Wails 的 `filedrop` 事件互补）：

```typescript
const [dropActive, setDropActive] = useState(false);

<div
  className={`dropzone ${dropActive ? "dropzone--active" : ""}`}
  onDragEnter={(e) => { e.preventDefault(); setDropActive(true); }}
  onDragOver={(e) => { e.preventDefault(); }}
  onDragLeave={(e) => { e.preventDefault(); setDropActive(false); }}
  onDrop={(e) => { e.preventDefault(); setDropActive(false); }}
>
  拖放文件夹到此区域，或手动输入路径
</div>
```

`onDrop` 的 `preventDefault` 阻止 WebView 默认的文件打开行为，实际的路径数据由 Wails 的 `filedrop` 事件提供。

---

## 8. 实施步骤

### 阶段一：导航骨架与页面路由（预计 2 小时）

1. 在 `App.tsx` 中添加 `Page` 类型和 `currentPage` state
2. 创建 `Sidebar` 组件，渲染导航项列表
3. 将 `<main>` 改为 `.app-body > .app-sidebar + .app-content` 布局
4. 根据 `currentPage` 条件渲染对应页面组件
5. 实现导航项启用/禁用逻辑
6. 验证：项目未打开时仅「概览」可点击；只读模式打开时禁用扫描相关页

### 阶段二：概览页重组（预计 1.5 小时）

1. 将 `ProjectPanel` 移入概览页
2. 创建 `StorageSummary` 组件，紧凑展示存储列表
3. 创建 `RecentJobs` 组件，展示最近 5 条任务
4. 创建 `DiagQuickEntry` 组件，三个诊断入口按钮
5. 实现诊断入口跳转：点击后切换到诊断报告页并预选 tab
6. 验证：项目打开后概览页正确展示存储和任务摘要

### 阶段三：扫描位置页（预计 2 小时）

1. 创建 `ScanLocationPage` 组件
2. 创建 `StorageLibrary` 组件（左侧存储列表）
3. 创建 `ScanConfigForm` 组件（右侧参数表单）
4. 创建 `DropZone` 组件，集成 Wails `filedrop` 事件
5. 将 `StartScan` 调用逻辑从 `ScanPanel` 迁移到 `ScanConfigForm`
6. 扫描启动后自动切换到扫描进度页
7. 验证：拖放文件夹路径正确填入；只读模式下页面禁用

### 阶段四：扫描进度页（预计 1.5 小时）

1. 创建 `ProgressPage` 组件
2. 将 `ProgressDisplay` 从 `ScanPanel` 拆出
3. 将 `JobHistory` 从 `ScanPanel` 拆出
4. 复用现有 `JobDetail` 组件
5. 验证：进度轮询正常；取消扫描正常；终态后数据刷新正常

### 阶段五：重复文件组页重构（预计 2 小时）

1. 创建 `DuplicatesPage` 组件，左右分栏布局
2. 创建 `DuplicateFilterPanel` 组件（左侧筛选）
3. 将 `DuplicateGroups` 组件从表格改为卡片列表
4. 实现 `.group-card:nth-child` 交替底色
5. 复用现有 `GroupDetail` 组件
6. 验证：筛选正常；分页加载正常；交替底色正确

### 阶段六：诊断报告页调整（预计 0.5 小时）

1. 将 `DiagnosticPanel` 移入诊断报告页
2. 确保存储选择下拉数据从 App.tsx 传入
3. 实现从概览页诊断快捷入口跳转时预选 tab
4. 验证：三类诊断报告运行正常

### 阶段七：集成验证（预计 1 小时）

1. `tsc --noEmit` 通过
2. `vite build` 通过
3. `wails build` 通过
4. 手动冒烟测试：完整走查项目打开 → 扫描 → 查看重复组 → 运行诊断的全流程
5. 验证窗口拖动、文件拖放、Toast 通知等交互功能

---

## 9. 后端 API 利用率分析

### 9.1 已利用的 API（当前版本）

| API | 利用方式 | 利用率 |
| --- | --- | --- |
| `GetVersion` | 顶栏版本号 | 完全利用 |
| `OpenProject` | 只读打开项目 | 完全利用 |
| `OpenProjectReadWrite` | 读写打开项目 | 完全利用 |
| `CloseProject` | 关闭项目 | 完全利用 |
| `GetProjectInfo` | 刷新项目信息 | 完全利用 |
| `ValidateProjectPath` | 未使用 | **待利用** |
| `ListStorages` | 存储列表展示 | 完全利用 |
| `ListDuplicateGroups` | 重复组列表 + 分页 + 筛选 | 完全利用 |
| `GetGroupDetail` | 重复组详情 | 完全利用 |
| `StartScan` | 启动扫描 | 完全利用 |
| `GetScanProgress` | 进度轮询 | 完全利用 |
| `CancelScan` | 取消扫描 | 完全利用 |
| `ListRecentJobs` | 任务历史 | 完全利用 |
| `GetJobDetail` | 任务详情 + 事件流 | 完全利用 |
| `DiagnoseFormats` | 格式诊断 | 完全利用 |
| `DiagnoseGovernance` | 治理诊断 | 完全利用 |
| `DiagnoseMerges` | 合并诊断 | 完全利用 |

### 9.2 待增强的利用点

重构后可在以下方面进一步提升后端功能利用率：

1. **`ValidateProjectPath` 预校验**：在概览页项目打开表单中，用户输入路径后实时调用此方法进行预校验，在打开前给出路径合法性反馈，避免 `OpenProject` 失败后再提示

2. **`ListStorages` 在扫描位置页联动**：当前存储列表仅在概览页展示，重构后在扫描位置页左侧也展示存储列表，点击存储条目自动填充 `storage_id`，让用户明确扫描目标所属的存储区域

3. **`DiagnoseGovernance` 的 `large_media_minimum` 参数**：当前诊断面板未暴露此参数，重构后可在治理诊断 tab 中增加大媒体阈值输入框，让用户自定义大媒体文件的判定标准

4. **`DiagnoseFormats` 的 `large_unknown_minimum` 参数**：同上，格式诊断 tab 中增加大未知文件阈值输入框

5. **`ListRecentJobs` 的 `limit` 参数**：概览页调用时传入 `limit=5` 仅展示最近 5 条，扫描进度页调用时传入 `limit=20` 展示更多历史

6. **`GetJobDetail` 的事件流展示**：当前仅以 JSON 形式展示事件列表，可进一步解析 `JobEvent` 的 `event_type` 和 `payload` 字段，渲染为时间线视图，利用 `EVENT_LABELS` 映射提供中文标签

7. **`GroupDetailResponse.decision_type` 字段**：当前在列表中展示，重构后可在卡片上以彩色标签形式突出显示决策类型，帮助用户快速识别需要关注的组

---

## 10. 风险与注意事项

1. **状态迁移风险**：从单页面布局迁移到多页面布局时，需确保所有 `useState` 和 `useEffect` 的依赖项正确迁移。特别注意进度轮询的 `useEffect` 必须留在 App.tsx 顶层，不能跟随页面组件卸载而清理

2. **Wails 运行时事件清理**：`EventsOn` 返回的取消函数必须在组件卸载时调用，否则会导致事件监听器泄漏。每次项目关闭/重新打开时，需确保旧的 `filedrop` 监听器被正确清理

3. **只读模式降级**：大量功能依赖读写模式（扫描、任务历史），需在 UI 层面清晰传达模式限制。导航栏的禁用状态需要配合 tooltip 或说明文字，告知用户为何某页不可用

4. **大列表性能**：重复文件组在大规模扫描后可能有数千组，卡片渲染比表格更耗性能。如果列表超过 200 项，考虑引入虚拟滚动（如 `react-window`）

5. **CSS 变量一致性**：新增的导航栏、拖放区域等样式必须复用 `:root` 中定义的 CSS 变量（`--bg`、`--card-bg`、`--accent`、`--border` 等），避免硬编码颜色值
