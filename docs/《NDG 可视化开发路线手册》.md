# 《NDG 可视化开发路线手册》

版本：2026-07-27（原始版）
适用仓库：`FNB2026/nas-data-governance`
目标平台：macOS、Linux
推荐桌面技术：Go + Wails v2 + React + TypeScript + SQLite

> **修订说明（2026-07-28）**
>
> 本手册已于 2026-07-28 经全仓库代码核查并发布正式校正。请配合阅读：
>
> - [《NDG 可视化开发路线手册》校正与修订指南](./《NDG 可视化开发路线手册》校正与修订指南.md) — 四项正式修正与代码证据
> - [ADR-0006 桌面可视化架构](./adr/0006-desktop-visualization-architecture.md) — 已采纳的架构决策
> - [桌面信息架构](./ui/information-architecture.md) — 一级导航与页面职责
>
> **已确认的修正项（详见校正指南）：**
>
> 1. `ListFiles` 已按 `file_status='active'` 过滤（`sqlite.go:197`），原手册 3.2 节「没有过滤」描述过时；真正问题在 JSONL 路径（`idx.Read`）无状态语义且无分页。
> 2. 迁移编号整体后移：005 已被 `005_deletion_lifecycle.sql` 占用，可视化迁移改为 006—009。
> 3. 可视化 ADR 编号为 0006（0005 已被 tiered-deletion-and-purge 占用）。
> 4. M7 已用于「分级删除生命周期」（`005_deletion_lifecycle.sql:1`、`m7_e2e_test.go`），可视化改用 V1—V5 编号。
>
> 本文件保留原始内容作为历史参考；正式执行以校正指南和 ADR-0006 为准。

---

## 一、最新仓库核查结论

截至本次检查，GitHub 连接器返回的仓库状态为：

| 项目                  | 当前状态                                          |
| ------------------- | --------------------------------------------- |
| 仓库                  | `FNB2026/nas-data-governance`                 |
| 可见性                 | Public                                        |
| 默认分支                | `main`                                        |
| 最新提交                | `bf4525135bb9a5daed5a305348fe5ddf6e3e03f1`    |
| 最新提交时间              | 2026-07-16                                    |
| 最新提交内容              | `Harden public release safety gates (#6)`     |
| 当前开放 PR             | 无                                             |
| 当前可视化前端             | 尚未建立                                          |
| Wails 依赖            | 尚未引入                                          |
| React/TypeScript 前端 | 尚未引入                                          |
| 当前发布物               | CLI，macOS/Linux 四种架构                          |
| 当前主线状态检查            | 工作流存在，但连接器未返回 HEAD 的实际状态结果，不能据此声称最新 main 已经全绿 |

最新提交仍是开源前安全收口，内容集中在执行日志 fail-closed、私有产物权限、源目录与隔离目录验证、版本信息和发布流程，并没有进入桌面可视化开发。

仓库当前已经明确覆盖只读扫描、增量扫描、断点续扫、失败补扫、SQLite 索引、目录语境、格式分析、资产关系、合并建议、计划、执行、崩溃恢复、L1—L4 学习和人工复核 CLI。

从仓库结构和 `go.mod` 看，目前仍然是纯 Go 核心加 CLI：没有 Wails、没有前端目录、没有桌面绑定层。

因此，NDG 当前准确的发展阶段是：

> **M1—M6 数据治理核心和生产安全基座已经完成；M7 可视化应用尚未正式开始。**

这和“核心功能还没做完，只是先画一个界面”完全不同。NDG 已经具备相当完整的领域内核，现在要解决的是应用服务化、查询模型、桌面适配和交互设计。

---

# 第一部分：对当前实际开发进度的重新判断

## 1.1 已经完成的能力

### M1：安全索引

已经具备：

* 只读文件系统扫描。
* 不跟随符号链接。
* 默认不跨挂载点。
* 文件属性采集。
* 快速指纹。
* SHA-256 完整哈希。
* 完全重复组。
* JSONL 和 SQLite 索引。
* SMB 高位 device/inode 无损存储。
* 扫描错误脱敏。
* 哈希失败有限重试。
* 哈希失败私有清单。
* 局部补扫。
* 验证后导入索引。

扫描器采用自定义 BFS，单个目录读取失败不会终止全部扫描；不同路径的硬链接被有意保留为不同文件实例。

### M2：目录语境与治理计划

已经具备：

* 上级目录链。
* 目录角色。
* 目录权威等级。
* 业务锚点。
* 原始、正式归档、缓存、临时、备份、敏感等职责判断。
* 目录职责冲突识别。
* 业务锚点分歧识别。
* 可解释保留评分。
* DRAFT 计划。
* 人工复核路由。

当前 Planner 已经可以明确处理你最重视的规则：

> 内容完全相同，但目录职责不同，不自动去重。

目录角色不同或业务锚点不同的文件会进入 REVIEW，而不会自动生成隔离动作。

### M3：安全执行

已经具备：

* DRAFT、APPROVED、STALE_CHECKED、EXECUTING、VERIFIED、ROLLED_BACK 状态机。
* 执行前 path、size、mtime、inode、hash stale 检查。
* 隔离而非永久删除。
* MOVE、COPY、RENAME。
* 跨卷复制、校验、删除源。
* 失败逆序回滚。
* 持久化 execution journal。
* 崩溃后恢复。
* SourceRoots 边界。
* 源根目录和隔离区重叠拒绝。
* 符号链接根目录拒绝。
* 普通日志路径脱敏。

非 dry-run 执行已经强制要求 SQLite Journal；Journal 初始化或完成记录失败时，执行器会停止并回滚。

### M4：格式分析

已经具备：

* 基于 magic bytes 的格式识别。
* 图片基础尺寸。
* 音频、视频基础编码及时长。
* PDF 页数。
* 压缩包条目。
* OOXML 分类。
* PSD、工程文件、侧车文件保护。
* 大型 unknown 和扩展名冲突诊断。
* 元数据缺口刷新。

### M5：资产关系与学习

已经具备：

* 资产组。
* 版本关系。
* 原始与派生关系。
* 目录合并建议。
* L1 内置规则。
* L2 统计学习。
* L3 行业资料学习。
* L4 决策反馈学习。
* 所有学习结果首先进入 draft。
* 外部 AI 默认关闭。
* 已审批规则不会被学习结果直接覆盖。

### M6：生产化收束

已经具备：

* 增量扫描。
* 哈希复用。
* 断点续扫。
* worker 并发控制。
* 故障演练。
* 崩溃恢复。
* 规则、计划、合并、冲突的交互式 CLI 复核。
* Race test。
* CI、Dependabot、CODEOWNERS、公开边界检查。
* CLI 多平台发布。

这些能力在路线文档中均被标记为已完成。

---

## 1.2 “生产就绪”应如何理解

仓库文档中的“生产化收束”是指：

> 核心扫描、分析、计划和执行引擎已经具备在真实 NAS 数据上运行所需的安全约束。

它并不意味着：

* 已经有普通用户可用的桌面软件。
* 已经有稳定的可视化任务管理器。
* 已经支持百万条结果的分页浏览。
* 已经有桌面安装包、签名和自动更新。
* 已经完成可视化错误恢复。
* 已经完成批量治理交互。
* 已经达到 Duplicate Cleaner 或 Czkawka 的 GUI 完整度。

当前所谓“人工复核管理界面”，实际是基于 `bufio.Reader` 的交互式 CLI，支持 plans、rules、merges、conflicts 四类对象。

因此，项目现阶段不能被描述为“桌面应用已经做了一半”，更准确的描述是：

> **治理引擎成熟度较高，桌面应用成熟度接近零。**

---

## 1.3 与之前分析相比，最新仓库带来的修正

之前的基本判断仍然成立：

* 不应重写扫描引擎。
* 不应直接 Fork Czkawka。
* 不应通过子进程解析 CLI 输出做正式 GUI。
* 应先抽取应用服务层，再接 Wails。
* 应保留 CLI 作为一等入口。

但结合最新代码，开发顺序需要进一步收紧。可视化之前至少要先解决以下五项问题：

1. 硬链接和真实可回收容量。
2. active/missing 文件过滤。
3. SQLite 分页查询和稳定重复组标识。
4. REVIEW 与 APPROVED 语义分离。
5. 持久化任务与结构化进度事件。

这些不是前端美化问题，而是 UI 一旦展示给用户，就必须准确的问题。

---

# 第二部分：NDG 可视化产品的正确定位

## 2.1 不要把 NDG 做成 Duplicate Cleaner 的复刻

Duplicate Cleaner 的产品逻辑主要是：

```text
设置匹配规则
→ 扫描
→ 找到重复文件
→ 批量选择
→ 删除或移动
```

NDG 的正确逻辑应该是：

```text
建立本地治理项目
→ 注册数据源
→ 只读扫描
→ 建立文件与目录语境索引
→ 发现重复、版本、派生和目录关系
→ 解释为什么相同
→ 判断是否应该处理
→ 生成治理草案
→ 人工复核
→ 执行前预检
→ 隔离执行
→ 校验、审计与恢复
```

两者的核心区别是：

| Duplicate Cleaner | NDG                   |
| ----------------- | --------------------- |
| 重复文件工具            | 数据治理工作台               |
| 找相同文件             | 解释文件关系                |
| 内容相同是主要依据         | 内容、目录职责、业务锚点和保护策略共同判断 |
| 主要目标是节省空间         | 主要目标是降低数字资产混乱和错误处理风险  |
| 批量标记删除            | 批量形成治理草案              |
| 删除结果              | 可审批、可审计、可恢复的执行计划      |

NDG 的桌面产品定义应固定为：

> **一个面向个人、家庭和组织数字资产的本地化、语境感知、可解释、可审批和可恢复的数据治理工作台。**

---

## 2.2 可视化不等于多做图表

第一版最重要的不是饼图、折线图和动画，而是把复杂的治理证据呈现清楚。

用户需要看懂的是：

* 哪些文件内容相同。
* 它们分别在哪些目录中。
* 两条目录分支从哪里开始分叉。
* 每个目录承担什么职责。
* 是否存在受保护项。
* 是否属于同一个业务锚点。
* 哪一个副本更适合作为保留项。
* 系统为什么建议保留、隔离或复核。
* 这个操作是否已经通过 dry-run。
* 执行失败后能否恢复。

因此，NDG 首版应以：

* 表格。
* 目录分支对照。
* 证据面板。
* 状态机。
* 操作计划。
* 审计时间线。

作为主要视觉形式，而不是以统计大屏为中心。

---

## 2.3 第一阶段明确不做什么

首个桌面版本不应纳入：

* Windows 支持。
* NAS 常驻 Agent。
* 浏览器远程管理。
* 用户账号系统。
* 云同步。
* 外部 AI 判断。
* 图片像素相似识别。
* 音频指纹。
* 视频抽帧相似识别。
* 自动永久删除。
* 自动跨备份域去重。
* 插件市场。
* 多用户协同审批。
* 自动更新。
* 花哨的关系图谱。

这些都可以后续增加，但不能阻塞第一版工作台。

---

# 第三部分：可视化之前必须修正的核心问题

## 3.1 硬链接不能按普通副本计算

当前扫描器有意把不同路径的硬链接保留为不同文件实例。

而当前完全重复报告只是把所有相同 SHA-256 的文件放进同一个 Map：

```go
byHash[f.ContentSHA256] = append(byHash[f.ContentSHA256], f)
```

随后只判断成员数是否大于 1。

这在“文件路径关系”层面是合理的，但在“可回收容量”层面会产生误导。

例如：

```text
/A/photo.jpg
/B/photo.jpg
```

如果二者是同一 inode 的硬链接：

* 路径数量是 2。
* 内容副本路径数量是 2。
* 物理数据对象数量是 1。
* 删除其中一条路径不会释放文件内容占用。
* 只有最后一个硬链接被删除时才会真正释放数据块。

因此，UI 必须区分：

```text
路径实例 Path Instance
物理文件对象 Physical Object
内容身份 Content Identity
```

建议增加：

```go
type PhysicalIdentity struct {
    StorageID string
    Device    uint64
    Inode     uint64
    LinkCount uint64
    Reliable  bool
}
```

当 device 和 inode 均有效时：

```text
physical_key = storage_id + device + inode
```

当文件系统不能提供稳定 inode 时：

```text
physical_key = unknown
```

不能强行把路径判定成独立物理副本。

重复组至少要显示：

| 指标      | 含义                   |
| ------- | -------------------- |
| 路径数     | 有多少个文件路径             |
| 物理副本数   | 有多少个不同物理对象           |
| 硬链接别名数  | 多少路径共享同一物理对象         |
| 物理可回收上限 | 在至少保留一个物理对象时最多释放多少   |
| 治理候选容量  | 通过目录职责和保护规则后可进入草案的容量 |
| 已批准容量   | 已经通过计划审批的容量          |

不能只显示一个“可释放 120GB”。

---

## 3.2 必须过滤 missing 文件

当前迁移已经给 `file_instances` 增加 `file_status`，用于标记 active 和 missing。

但是现有 `ListFiles`：

* 没有读取 `file_status`。
* 没有按 `active` 过滤。
* 一次返回全部文件。
* 没有分页。

如果桌面端直接基于 `ListFiles` 构建结果，可能会把已经不存在的旧文件重新显示在重复组里。

可视化查询必须默认：

```sql
WHERE file_status = 'active'
```

missing 文件应进入独立的“历史状态”或“索引差异”视图，而不参与当前重复容量、当前治理建议和当前执行计划。

---

## 3.3 重复组必须改为数据库查询

当前 `DuplicateGroups` 在内存中对全部文件分组：

```text
读取全部文件
→ 建立 map[sha256][]file
→ 返回全部重复组
```

问题包括：

* 文件越多，内存占用越大。
* Map 迭代顺序不稳定。
* 没有分页。
* 没有排序。
* 无法快速筛选风险等级。
* 无法只加载单组详情。
* 无法边扫描边查看。
* 难以计算物理副本。
* 难以过滤 missing。
* 无法在 UI 重启后恢复用户位置。

建议把重复组升级为 SQLite 只读查询模型。

第一版不必立刻永久保存所有重复组，可以按需查询：

```sql
SELECT
    storage_id,
    content_sha256,
    MAX(size) AS file_size,
    COUNT(*) AS path_count
FROM file_instances
WHERE file_status = 'active'
  AND content_sha256 IS NOT NULL
  AND content_sha256 <> ''
GROUP BY storage_id, content_sha256
HAVING COUNT(*) > 1
ORDER BY file_size DESC, content_sha256
LIMIT ?
```

建议增加部分索引：

```sql
CREATE INDEX IF NOT EXISTS idx_files_active_content
ON file_instances(storage_id, content_sha256, size)
WHERE file_status = 'active'
  AND content_sha256 IS NOT NULL
  AND content_sha256 <> '';
```

组标识必须稳定：

```text
group_id = SHA256(
    governance_domain_id
    + storage_id
    + content_sha256
)
```

不能使用数组顺序或自增页码作为组 ID。

### 为什么默认按 storage 分组

同一个 SHA-256 可能同时存在于：

* 工作盘。
* 归档盘。
* 冷备盘。
* 异地备份。
* NAS 快照。
* 外置备份盘。

这些不应被合并成一个普通清理组。

默认规则应该是：

```text
同一治理域、同一数据源内部
→ 可以形成普通重复组

跨数据源或跨备份域
→ 只显示为“相关副本”
→ 不进入自动治理候选
```

---

## 3.4 REVIEW 和 APPROVED 必须彻底分开

当前 `review plans` 的交互提示是：

```text
approve this plan?
```

但用户选择 yes 后，程序做的是：

```text
REVIEW → SKIP
```

即保留所有相关文件，并写入一个“approved”输出文件。

这容易在 GUI 中造成严重语义混乱：

* 用户以为自己批准了文件操作。
* 实际上只是把 REVIEW 改成 SKIP。
* “复核完成”和“批准执行”被混在一起。

建议拆成两套状态。

### 治理复核状态

```text
UNREVIEWED
KEEP_ALL
DRAFT_ACTION
DEFERRED
REJECTED_SUGGESTION
```

### 操作计划状态

沿用现有安全状态机：

```text
DRAFT
→ APPROVED
→ STALE_CHECKED
→ EXECUTING
→ VERIFIED
或 ROLLED_BACK
```

界面文案应改为：

| 当前含义            | 建议文案        |
| --------------- | ----------- |
| REVIEW→SKIP     | 保留全部并关闭本组复核 |
| 形成操作            | 生成治理草案      |
| Plan Approved   | 批准该操作计划     |
| execute dry-run | 执行只读预检      |
| execute         | 执行已批准计划     |

“完成复核”绝不能叫“批准执行”。

---

## 3.5 文档中的扫描管线与实际代码需要统一

README 表述为：

> 文件先按大小分组，只有潜在重复项才计算完整哈希。

当前实现实际是：

```text
所有新文件或变更文件
→ 先计算 quick hash

size + quick hash 相同
→ 再计算完整 SHA-256
```

即当前会对所有新文件读取快速指纹，而不是先仅通过大小筛选 quick hash 候选。

这不属于逻辑错误，但会影响：

* 前端显示的阶段名称。
* NAS 网络读取量。
* 任务进度估算。
* Czkawka 性能对比。

建议后续优化为：

```text
阶段 A：仅采集元数据
阶段 B：SQL 按 size 筛选候选
阶段 C：仅对候选计算 quick hash
阶段 D：size + quick hash 再筛选
阶段 E：计算完整 SHA-256
```

这更适合大型 NAS，也更接近 Czkawka 的分层候选策略。

该优化可以放在只读 Alpha 之后、公开 Beta 之前，不必阻塞第一张界面，但前端阶段名称必须忠实反映当前实现。

---

# 第四部分：目标技术架构

## 4.1 总体架构

```text
┌───────────────────────────────────────┐
│ React + TypeScript + Vite             │
│ 页面、表格、目录对照、复核、审计       │
└─────────────────┬─────────────────────┘
                  │ BackendPort
┌─────────────────▼─────────────────────┐
│ Wails 桌面适配层                       │
│ DTO 转换、事件转发、文件选择对话框       │
└─────────────────┬─────────────────────┘
                  │
┌─────────────────▼─────────────────────┐
│ internal/app 应用服务层                │
│ 扫描用例、查询用例、复核用例、执行用例   │
└───────┬─────────┬─────────┬───────────┘
        │         │         │
┌───────▼───┐ ┌───▼────┐ ┌──▼──────────┐
│ query     │ │ jobs   │ │ events       │
│ 只读模型   │ │任务状态 │ │结构化事件     │
└───────┬───┘ └───┬────┘ └──┬──────────┘
        │          │          │
┌───────▼──────────▼──────────▼──────────┐
│ 现有核心                               │
│ scanner / store / planner / executor  │
│ dircontext / format / relations       │
└─────────────────┬─────────────────────┘
                  │
┌─────────────────▼─────────────────────┐
│ 本机 SQLite 项目数据库                 │
│ 本地磁盘 / 已挂载 SMB / NAS 数据源      │
└───────────────────────────────────────┘
```

CLI 也调用同一个 `internal/app`：

```text
CLI ───────┐
           ├── internal/app ── 核心引擎
Desktop ───┘
```

---

## 4.2 为什么不能让 GUI 调 CLI 子进程

不推荐正式使用：

```text
Wails
→ exec.Command("nas-governance", "scan", ...)
→ 解析 stdout
```

这种方案的问题是：

* 进度只能解析文本。
* 文案变化会破坏前端。
* 难以安全取消。
* 难以传递结构化错误。
* 难以恢复任务。
* 无法共享内存状态。
* 测试复杂。
* CLI 和 GUI 容易产生不同语义。
* 可能重新暴露路径日志。

它只适合一两天的临时验证，不应进入主分支产品架构。

---

## 4.3 应用服务层

当前 `cmd/nas-governance/main.go` 直接导入 scanner、store、planner、executor、learning 等大量核心包，并负责业务编排。

建议增加：

```text
internal/app/
├── container.go
├── project_service.go
├── source_service.go
├── scan_service.go
├── analysis_service.go
├── duplicate_service.go
├── review_service.go
├── plan_service.go
├── execution_service.go
├── recovery_service.go
└── rule_service.go
```

应用服务职责：

* 组合核心包。
* 管理数据库生命周期。
* 管理任务。
* 执行业务状态转换。
* 返回稳定 DTO。
* 执行权限检查。
* 不包含 Wails。
* 不包含 React。
* 不直接输出终端文字。

例如：

```go
type ScanService interface {
    StartScan(
        ctx context.Context,
        req StartScanRequest,
        sink ProgressSink,
    ) (JobSnapshot, error)

    CancelScan(
        ctx context.Context,
        jobID string,
    ) error
}

type DuplicateService interface {
    ListGroups(
        ctx context.Context,
        query DuplicateGroupQuery,
    ) (DuplicateGroupPage, error)

    GetGroup(
        ctx context.Context,
        groupID string,
    ) (DuplicateGroupDetail, error)
}
```

CLI 中的 `runScan()` 只负责：

```text
解析 flag
→ 构造 StartScanRequest
→ 调用 ScanService
→ 将结果格式化到终端
```

---

## 4.4 Wails 版本选择

建议固定使用 Wails v2 的当前稳定版本，不采用 v3 Alpha。Wails v2.13.0 已于 2026 年 7 月发布，而 Wails v3 官方仍标记为 Alpha。([Wails][1])

建议：

```text
Wails: v2.13.0 精确锁定
React: 锁定当前项目版本
TypeScript: strict
Node: 锁定一个明确主版本
npm/pnpm: 只选一个
lockfile: 必须提交
```

不要在 CI 中使用：

```text
wails@latest
node@latest
```

Wails 能够为绑定的 Go 结构生成 TypeScript 声明，这适合 NDG 的强类型 DTO。([Wails][2])

### Wails 隔离原则

Wails 相关代码只能出现在：

```text
cmd/ndg-desktop/
internal/adapters/wails/
```

禁止 Wails 依赖进入：

```text
internal/scanner
internal/planner
internal/executor
internal/domain
```

这样未来即使换成：

* Tauri。
* Qt。
* Web UI。
* NAS 服务端。

核心引擎也不需要重写。

---

## 4.5 推荐仓库结构

```text
cmd/
├── nas-governance/             现有 CLI
└── ndg-desktop/                Wails 桌面入口

internal/
├── app/                        应用用例
├── query/                      UI 只读查询模型
├── jobs/                       后台任务管理
├── events/                     结构化进度事件
├── presentation/               DTO、脱敏视图模型
├── adapters/
│   └── wails/                  Wails 绑定适配
├── scanner/
├── fingerprint/
├── store/
├── planner/
├── executor/
├── dircontext/
├── format/
├── relations/
└── ...

frontend/
├── src/
│   ├── app/
│   ├── routes/
│   ├── features/
│   │   ├── projects/
│   │   ├── sources/
│   │   ├── scans/
│   │   ├── duplicates/
│   │   ├── governance/
│   │   ├── execution/
│   │   ├── audit/
│   │   └── settings/
│   ├── components/
│   ├── platform/
│   ├── stores/
│   ├── styles/
│   └── generated/
├── package.json
└── vite.config.ts

docs/
├── ui/
│   ├── information-architecture.md
│   ├── visual-language.md
│   ├── privacy-model.md
│   └── interaction-states.md
└── adr/
    └── 0005-desktop-visualization-architecture.md
```

---

# 第五部分：项目与数据源模型

## 5.1 一个 NDG 项目是什么

建议把一个项目定义为：

```text
一个本地项目数据库
+ 一组数据源
+ 一组扫描配置
+ 一组保护与排除规则
+ 治理复核历史
+ 操作计划
+ 执行日志
+ 恢复状态
```

例如：

```text
~/Library/Application Support/NDG/projects/home-nas/
├── governance.db
├── project.json
├── private/
│   ├── hash-failures/
│   ├── diagnostics/
│   └── exports/
└── cache/
```

Linux 对应使用用户应用数据目录。

项目目录保持：

```text
目录权限 0700
文件权限 0600
```

这与现有私有产物安全策略一致。

---

## 5.2 SQLite 必须放在本机

NAS 文件可以通过 SMB 挂载后扫描，但项目 SQLite 不应放在 SMB 共享目录。

当前数据库启用了 WAL，并将写连接数限制为 1。

SQLite 官方明确说明，WAL 依赖共享内存，不适合放在网络文件系统上。([SQLite主页][3])

正确结构是：

```text
Mac 本地 SSD
└── NDG SQLite 数据库

NAS SMB 挂载
└── 仅作为被扫描数据源
```

### 连接设计

建议：

* 一个写连接。
* 独立只读 QueryDB。
* 只读池初始设置 2—4 个连接。
* 所有写操作串行。
* UI 查询不直接占用写事务。
* 长查询必须支持 context 取消。

当前 `OpenReadOnly()` 也把连接数限制为 1。

可视化阶段可以新增专用查询连接，而不改变执行器的单写安全原则。

---

## 5.3 数据源页面需要的预检

用户添加数据源时，不能直接开始扫描，应先做：

1. 路径是否存在。
2. 是否为真实目录。
3. 是否为符号链接。
4. 是否可读取。
5. 是否与已有数据源重叠。
6. 是否与隔离区重叠。
7. 当前挂载设备标识。
8. 是否可能跨挂载点。
9. 是否属于参考数据源或备份域。
10. 是否启用默认排除目录。
11. 是否包含项目数据库自身。
12. 是否检测到高风险系统目录。

返回结构不应只是 true/false：

```go
type SourceValidationResult struct {
    Valid          bool
    Readable       bool
    IsSymlink      bool
    MountID        string
    Overlaps       []SourceOverlap
    Warnings       []ValidationWarning
    BlockingErrors []ValidationError
}
```

---

# 第六部分：任务系统与进度事件

## 6.1 当前 runner 不是持久化任务队列

路线文档称其为“任务队列与资源控制”，但 `internal/runner` 自己明确说明：

* 不持久化任务队列。
* 不做 I/O 速率限制。
* 不做任务优先级。
* 只是 semaphore 控制的进程内并发器。

因此 GUI 不能直接把 Runner 当作任务中心。

需要在 Runner 上方增加：

```text
JobManager
```

---

## 6.2 建议增加 job_runs

迁移 005：

```sql
CREATE TABLE job_runs (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL,
    job_type TEXT NOT NULL,
    state TEXT NOT NULL,
    stage TEXT NOT NULL,
    created_at TEXT NOT NULL,
    started_at TEXT,
    completed_at TEXT,
    cancel_requested INTEGER NOT NULL DEFAULT 0,
    progress_json TEXT NOT NULL DEFAULT '{}',
    warning_count INTEGER NOT NULL DEFAULT 0,
    error_code TEXT
);
```

状态：

```text
QUEUED
RUNNING
CANCEL_REQUESTED
CANCELLED
COMPLETED
FAILED
```

阶段：

```text
DISCOVERING
METADATA_INDEXING
QUICK_HASHING
FULL_HASHING
CONTEXT_CLASSIFYING
FORMAT_ANALYZING
GROUPING
PLANNING
FINALIZING
```

扫描 checkpoint 继续负责文件遍历位置。

`job_runs` 负责：

* UI 任务历史。
* 任务状态。
* 开始和结束时间。
* 当前阶段。
* 警告数量。
* 重启后恢复显示。

二者不要混为一张表。

---

## 6.3 不要提供假的“暂停”

当前能力是：

```text
context cancel
+ checkpoint resume
```

不是：

```text
冻结所有 goroutine
+ 稍后从完全相同的运行现场继续
```

所以按钮不应写：

```text
暂停
```

建议写：

```text
停止并保留断点
```

任务终止后显示：

```text
已停止，可从检查点继续
```

这是更准确的产品语义。

---

## 6.4 进度事件结构

```go
type ProgressEvent struct {
    Sequence        int64     `json:"sequence"`
    JobID           string    `json:"jobId"`
    JobType         string    `json:"jobType"`
    Stage           string    `json:"stage"`
    State           string    `json:"state"`
    FilesDiscovered int64     `json:"filesDiscovered"`
    FilesProcessed  int64     `json:"filesProcessed"`
    BytesRead       int64     `json:"bytesRead"`
    CandidateFiles  int64     `json:"candidateFiles"`
    DuplicateGroups int64     `json:"duplicateGroups"`
    WarningCount    int64     `json:"warningCount"`
    UpdatedAt       time.Time `json:"updatedAt"`
}
```

禁止在普通进度事件中包含：

* 当前完整路径。
* 文件名。
* 错误中的原始路径。
* 数据库路径。
* 隔离区路径。

具体失败路径只能通过用户主动打开私有详情页后查询。

### 事件名称

```text
job:created
job:stage
job:progress
job:warning
job:completed
job:failed
job:cancelled
plan:changed
execution:progress
recovery:required
```

Wails 提供 Go 与 JavaScript 间的事件机制，可以用于这些结构化通知。([Wails][4])

但事件只能是刷新提示，SQLite 才是最终状态来源：

```text
收到事件
→ 触发 React Query 重新查询
→ 以数据库状态为准
```

不要把前端内存中的最后一条事件当作事实来源。

### 事件频率

建议限制为每秒 5—10 次。

否则扫描几十万文件时会造成：

* Wails IPC 压力。
* React 重渲染。
* 日志和事件队列膨胀。
* UI 卡顿。

---

# 第七部分：Wails API 设计

## 7.1 只绑定用例，不绑定底层能力

禁止直接绑定：

```go
ReadFile(path string)
DeleteFile(path string)
MoveFile(source, target string)
RunCommand(command string)
RawSQL(query string)
```

这些接口会绕过 NDG 的安全边界。

Wails 只暴露高层用例：

```go
type DesktopAPI struct {
    projects  *app.ProjectService
    sources   *app.SourceService
    scans     *app.ScanService
    duplicates *app.DuplicateService
    reviews   *app.ReviewService
    plans     *app.PlanService
    execution *app.ExecutionService
    recovery  *app.RecoveryService
}
```

---

## 7.2 第一版接口建议

### 项目

```go
CreateProject(req CreateProjectRequest)
OpenProject(projectPath string)
CloseProject()
GetProjectSummary()
ListRecentProjects()
```

### 数据源

```go
ValidateSource(path string)
AddSource(req AddSourceRequest)
RemoveSource(sourceID string)
ListSources()
UpdateSourcePolicy(req UpdateSourcePolicyRequest)
```

### 扫描

```go
StartScan(req StartScanRequest)
CancelJob(jobID string)
ResumeScan(jobID string)
GetJob(jobID string)
ListJobs(query JobQuery)
```

### 重复结果

```go
ListDuplicateGroups(query DuplicateGroupQuery)
GetDuplicateGroup(groupID string)
ListGroupMembers(groupID string, query MemberQuery)
```

### 治理复核

```go
SaveGroupDecision(req SaveDecisionRequest)
PreviewBulkDecision(req BulkDecisionRequest)
ApplyBulkDecision(req BulkDecisionRequest)
ListPendingReviews(query ReviewQuery)
```

### 计划

```go
CreateDraftPlans(req CreatePlansRequest)
ListPlans(query PlanQuery)
GetPlan(planID string)
ApprovePlan(planID string)
RejectPlan(planID string)
```

### 执行

```go
ValidateExecutionEnvironment(req ExecutionPreflightRequest)
ExecuteDryRun(req ExecuteRequest)
ExecuteApproved(req ExecuteRequest)
CancelExecution(runID string)
```

执行本身仍建议串行，不允许因为 UI 存在就把文件写操作并发化。

### 恢复

```go
ListRecoverableRuns()
InspectRecovery(runID string)
RecoverRun(runID string)
RollbackRun(runID string)
```

---

## 7.3 DTO 与领域对象分离

不要直接把整个 `domain.OperationPlan` 暴露给 React。

原因：

* 领域对象可能包含完整路径。
* 领域结构变化会破坏前端。
* 前端不需要内部所有字段。
* 部分字段需要脱敏。
* UI 需要聚合字段和显示状态。

例如：

```go
type DuplicateGroupRow struct {
    ID                          string   `json:"id"`
    SHA256Prefix                string   `json:"sha256Prefix"`
    FileSize                    int64    `json:"fileSize"`
    PathCount                   int      `json:"pathCount"`
    PhysicalCopyCount           int      `json:"physicalCopyCount"`
    HardlinkAliasCount          int      `json:"hardlinkAliasCount"`
    PhysicalReclaimableBytes    int64    `json:"physicalReclaimableBytes"`
    GovernanceCandidateBytes    int64    `json:"governanceCandidateBytes"`
    ApprovedReclaimableBytes    int64    `json:"approvedReclaimableBytes"`
    Roles                       []string `json:"roles"`
    AnchorCount                 int      `json:"anchorCount"`
    ContainsProtected           bool     `json:"containsProtected"`
    Risk                        string   `json:"risk"`
    ReviewState                 string   `json:"reviewState"`
}
```

---

# 第八部分：前端技术方案

## 8.1 推荐技术栈

```text
React
TypeScript strict
Vite
TanStack Query
TanStack Table
TanStack Virtual
React Router
轻量可访问组件原语
CSS Variables
Lucide 图标
```

TanStack Table 本身不直接提供虚拟化，但可以和 TanStack Virtual 组合，适合 NDG 的大量文件表格。([TanStack][5])

### 状态分工

| 状态          | 建议归属             |
| ----------- | ---------------- |
| SQLite 后端数据 | TanStack Query   |
| 表格排序、筛选、选中  | TanStack Table   |
| 虚拟列表窗口      | TanStack Virtual |
| 当前弹窗、抽屉     | React 本地状态       |
| 跨页面 UI 偏好   | 轻量 UI Store      |
| 扫描事实状态      | SQLite/后端        |
| 计划状态        | SQLite/后端        |
| 敏感路径        | 不写 localStorage  |

不要把后端业务状态复制到大型前端全局 Store。

---

## 8.2 BackendPort 隔离

前端不要在所有组件里直接导入 Wails 生成文件。

应建立：

```ts
export interface BackendPort {
  listSources(): Promise<SourceView[]>
  startScan(request: StartScanRequest): Promise<JobView>
  listDuplicateGroups(
    query: DuplicateGroupQuery
  ): Promise<DuplicateGroupPage>
}
```

然后提供：

```text
WailsBackendPort
MockBackendPort
```

用途：

* 普通浏览器中开发 UI。
* Vitest 测试。
* E2E 使用合成数据。
* 避免所有组件依赖 Wails 运行时。
* 将来支持 Web 管理端时可以替换适配器。

---

## 8.3 Wails 开发环境安全

`wails dev` 会启动前端开发服务器并生成 Go/JS 绑定，开发时还可以通过浏览器控制台调用绑定方法。([Wails][6])

因此：

* 开发模式只能使用合成测试目录。
* 不在真实 NAS 项目中开启前端 DevTools。
* 不在控制台打印 DTO。
* 不打印完整路径。
* 生产构建关闭调试日志。
* 生产构建不开放任意底层文件 API。

---

# 第九部分：信息架构与一级导航

沿用最适合 NDG 的七个一级入口：

```text
数据源
扫描任务
重复结果
治理复核
执行中心
审计与恢复
设置
```

不建议再额外增加一个独立“Dashboard”作为一级入口。项目摘要可以放在“数据源”顶部或应用首页。

---

## 9.1 数据源

### 页面目标

回答：

* 我现在打开的是哪个治理项目？
* 数据来自哪里？
* 哪些目录被保护？
* 数据源是否在线？
* 项目数据库在哪里？
* 当前是否具备扫描条件？
* 是否配置了隔离区？

### 页面布局

```text
项目摘要
────────────────────────────
数据源卡片
────────────────────────────
扫描与保护策略
────────────────────────────
隔离区与执行准备状态
```

### 数据源卡片字段

* 名称。
* Storage ID。
* 根目录。
* 数据源类型。
* 挂载状态。
* 可读状态。
* 最近扫描时间。
* 最近扫描文件数。
* missing 文件数。
* 保护目录数量。
* 排除目录数量。
* 是否属于备份域。
* 是否允许形成普通治理候选。

### 数据源类型

```text
工作数据源
正式归档
备份数据源
冷备数据源
参考数据源
只读审计数据源
```

参考和备份类型默认：

```text
允许参与关系比对
禁止自动形成清理计划
```

---

## 9.2 扫描任务

### 页面结构

左侧：

* 任务列表。
* 运行中。
* 已完成。
* 已停止。
* 失败。

中间：

* 当前阶段。
* 进度。
* 文件数。
* 已读取容量。
* 哈希候选。
* 警告。
* 开始和持续时间。

右侧：

* 扫描参数。
* 数据源。
* 排除规则。
* worker 数。
* 增量或完整扫描。
* 检查点。
* 失败摘要。

### 不应显示的内容

全局进度区不显示：

* 当前完整路径。
* 最近处理的文件名。
* 路径滚动日志。

用户主动打开“私有详细诊断”后，才显示具体对象。

### 进度呈现

目录发现阶段无法预先知道总文件数，不能显示虚假的 78%。

应该显示：

```text
正在发现文件
已发现 132,485 个文件
已访问 18,204 个目录
```

只有具有明确分母的阶段才显示百分比：

```text
完整哈希 14,025 / 26,400
```

---

## 9.3 重复结果

这是 NDG 的核心工作区。

推荐三栏结构：

```text
左：筛选与批量规则
中：重复组列表
右：证据与目录语境检查器
```

### 重复组列表列项

* 风险。
* 文件代表名称。
* 文件大小。
* 路径数量。
* 物理副本数量。
* 硬链接标志。
* 物理可回收上限。
* 治理候选容量。
* 目录角色数量。
* 业务锚点数量。
* 是否包含保护项。
* 复核状态。
* 计划状态。

### 默认排序

```text
治理候选容量降序
→ 风险等级
→ SHA-256
```

必须使用稳定排序。

### 筛选器

* 未复核。
* 可形成草案。
* 目录角色不同。
* 业务锚点不同。
* 含保护项。
* 硬链接。
* 同目录。
* 跨目录。
* 跨数据源。
* 文件类型。
* 文件大小。
* 成员数量。
* 风险等级。
* 计划状态。

### 组详情

点击某组后，右侧显示：

```text
内容身份
物理身份
目录分支对照
目录职责
业务锚点
保护规则
保留评分
版本/派生/侧车关系
系统建议
建议证据
用户决策
```

---

## 9.4 目录分支可视化

这是 NDG 区别于传统去重工具的关键视觉组件。

例如：

```text
共同根目录
└── 客户数字资产库
    ├── 财务部
    │   └── 销售发票
    │       └── 2019年存档
    │           └── 广州广粤文化.pdf
    │
    └── 甲方已完结合同文件
        └── 其他单次合作甲方
            └── 温泉鹏
                └── 广州广粤文化.pdf
```

组件需要明确显示：

* 共同祖先。
* 分叉点。
* 两边各自的 ParentChain。
* 每一级目录角色信号。
* 业务锚点。
* 权威等级。
* 保护标志。

视觉上不要只把两条完整路径放在两个文本框里。

---

## 9.5 治理复核

### 复核页面的核心问题

用户不是在回答：

> 删除哪个？

而是在回答：

> 这组相同内容在不同存储语境中分别承担什么职责？

### 可用决策

```text
保留全部
选择主要保留项
生成隔离草案
标记为交叉归档
标记为备份关系
暂缓处理
纠正目录角色
纠正业务锚点
```

第一版不提供：

```text
永久删除全部副本
忽略所有保护规则
自动选择全库
```

### 批量决策助手

安全预设可以包括：

* 所有受保护项保留。
* 目录角色不同则全部保留。
* 业务锚点不同则全部保留。
* 正式归档优先于临时目录。
* 原始资料优先于缓存。
* 同一缓存目录中的完全重复项生成隔离草案。
* 同一临时目录中的完全重复项生成隔离草案。
* 参考数据源中的文件始终保留。
* 跨备份域只建立关系，不生成处理计划。

每次批量应用前必须显示：

```text
将影响 1,243 个重复组
将生成 684 个草案计划
将保留 559 个组
不会执行任何文件写操作
```

然后先预览，不直接提交。

---

## 9.6 执行中心

### 入口条件

只有以下计划可以进入：

```text
状态为 APPROVED
没有未解决 REVIEW
隔离区通过验证
SourceRoots 通过验证
数据库 Journal 可用
独立备份确认
```

### 执行流程

```text
计划审批
→ 运行 dry-run
→ 查看 dry-run 结果
→ 再次 stale 检查
→ 输入确认语句
→ 串行执行
→ 每步 Journal 落盘
→ 校验
→ VERIFIED 或 ROLLED_BACK
```

### 强确认

高风险执行不要只用一个红色按钮。

建议要求输入：

```text
执行 18 个已批准隔离计划
```

确认内容必须包含实际数量。

### 容量显示

同时显示：

* 原始候选容量。
* dry-run 可执行容量。
* 因 stale 被阻止的容量。
* 因保护规则被阻止的容量。
* 预计隔离后释放容量。
* 实际完成容量。

---

## 9.7 审计与恢复

页面分为：

```text
操作审计
执行 Journal
未完成执行
回滚记录
私有诊断
导出
```

### Journal 时间线

```text
计划创建
计划批准
dry-run 通过
stale 检查通过
BeginJournal
动作 1 完成
动作 2 完成
动作 3 失败
开始回滚
动作 2 已回滚
动作 1 已回滚
计划 ROLLED_BACK
```

### 重启检测

桌面应用启动时应先调用：

```text
ListExecutingPlans
ListRecoverableRuns
```

若存在未完成执行，应直接展示：

```text
检测到上次未完成的文件操作
在完成恢复前，新的写操作已被锁定
```

不能把恢复入口藏在设置中。

---

## 9.8 设置

建议包含：

### 扫描

* 默认 worker。
* 哈希重试次数。
* 重试间隔。
* 默认排除目录。
* 是否跨挂载点。
* 增量扫描默认值。

### 隐私

* 路径显示方式。
* 截图隐私模式。
* 默认导出脱敏。
* 是否保存最近项目。
* 错误详情显示策略。

### 性能

* 查询页大小。
* 事件刷新频率。
* 只读连接数。
* 本地缓存上限。

### 开发者

* 版本。
* commit。
* build time。
* 数据库 schema。
* 规则版本。
* 诊断导出。
* 日志目录。

### AI 与联网

明确显示：

```text
外部 AI：关闭
遥测：关闭
云上传：关闭
```

而不是完全不告诉用户。

---

# 第十部分：视觉语言

## 10.1 总体风格

NDG 不适合：

* 大量渐变。
* 游戏化动画。
* Emoji 图标。
* 过度圆角。
* 卡片堆叠。
* 移动端式大空白。
* 只靠颜色区分状态。

更适合：

```text
高信息密度
稳定
工具化
精确
可审计
轻量层级
清楚的路径结构
```

### 桌面布局建议

* 左侧一级导航：220—240px。
* 顶部项目工具栏：48—52px。
* 主工作区：自适应。
* 右侧检查器：360—440px。
* 表格行高：34—40px。
* 详情区允许可调整宽度。
* 支持多列排序与固定列。

---

## 10.2 状态颜色

建议语义：

| 状态     | 视觉语义    |
| ------ | ------- |
| 安全、已验证 | 绿色      |
| 信息、运行中 | 蓝色      |
| 需要复核   | 黄色      |
| 高风险    | 橙色      |
| 阻止、失败  | 红色      |
| 缺失、失效  | 灰色      |
| 受保护    | 紫色或盾牌标志 |

每个状态必须同时有：

* 图标。
* 文字。
* 颜色。

不能只靠红绿。

---

## 10.3 路径隐私模式

增加全局：

```text
隐私显示
```

开启后：

```text
/Volumes/NAS/客户资料/甲方/深圳某公司/合同.pdf
```

显示为：

```text
…/甲方/深***司/合同.pdf
```

或：

```text
数据源 A / … / 合同.pdf
```

截图模式还应隐藏：

* 用户名。
* 挂载点。
* IP。
* NAS 名称。
* 数据库路径。
* 项目代号。
* 业务锚点。

---

# 第十一部分：数据库和查询模型

## 11.1 建议迁移顺序

### Migration 005：任务运行

新增：

```text
job_runs
job_events
```

`job_events` 只保存脱敏、低频、结构化里程碑，不保存每个文件事件。

### Migration 006：查询字段反规范化

当前 `directory_contexts` 主要存储 `context_json`。

为了快速筛选，建议增加：

```text
role
protected
business_anchor
authority_level
branch_point
rule_version
```

保留 JSON 作为完整证据，但常用字段反规范化。

### Migration 007：复核决策

```sql
CREATE TABLE group_decisions (
    id TEXT PRIMARY KEY,
    group_id TEXT NOT NULL,
    decision_type TEXT NOT NULL,
    retained_file_id INTEGER,
    reason TEXT,
    rule_id TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
```

### Migration 008：扫描快照

后续可增加：

```text
scan_runs
scan_run_membership
```

用于明确：

* 某组来自哪一次扫描。
* 扫描后文件变化。
* 历史结果对比。
* 增量扫描统计。

第一版可以先依赖 active 状态，不必立即建立完整快照系统。

---

## 11.2 分页方式

不要使用大 Offset：

```sql
LIMIT 100 OFFSET 200000
```

大量结果时应使用 keyset pagination：

```sql
WHERE
    reclaimable_bytes < :last_bytes
    OR (
        reclaimable_bytes = :last_bytes
        AND content_sha256 > :last_hash
    )
ORDER BY reclaimable_bytes DESC, content_sha256 ASC
LIMIT 100
```

前端返回：

```ts
type Page<T> = {
  items: T[]
  nextCursor?: string
  totalEstimate?: number
}
```

---

## 11.3 详情按需加载

重复组列表不返回所有成员完整数据。

列表只返回聚合字段。

点击后再调用：

```text
GetDuplicateGroup
ListGroupMembers
```

格式、上下文、版本关系也应按需要加载。

这能避免：

* 一次传输几十万路径。
* React 维护巨型数组。
* Wails IPC 阻塞。
* 敏感路径无必要地进入前端内存。

---

# 第十二部分：扫描引擎的可视化适配

## 12.1 当前内存模型需要逐步改造

当前扫描会在内存中维护：

```go
var files []domain.FileInstance
```

随后对整个切片执行候选分组和完整哈希。

这在当前真实测试规模上能够运行，但长期面对：

* 50 万文件。
* 100 万文件。
* 超长路径。
* 多个数据源。
* GUI 长时间常驻。

更适合改为批处理：

```text
BFS 发现
→ 批量写入 metadata
→ SQL 找相同 size
→ 快速哈希候选
→ 批量更新 quick_hash
→ SQL 找 size+quick_hash 候选
→ 完整哈希
→ 批量更新 SHA-256
```

### 建议批量大小

初始可以：

```text
元数据写入：1,000—5,000 条/事务
哈希结果更新：500—2,000 条/事务
```

最终以基准测试调整。

---

## 12.2 扫描和分析仍需保持独立

工程护栏明确要求扫描、分析、计划、审批、执行、校验和审计不得合并成一步。

GUI 可以提供“完整工作流”向导，但底层仍然要表现为多个可见阶段：

```text
任务 1：扫描
任务 2：格式分析
任务 3：关系分析
任务 4：生成治理候选
```

不能出现一个：

```text
一键智能整理
```

按钮直接从扫描运行到文件写入。

---

# 第十三部分：测试策略

## 13.1 后端测试

保持现有 Go 测试，并新增：

### 应用服务测试

* StartScan 参数验证。
* 任务创建。
* 取消。
* 重启恢复。
* 错误码映射。
* 路径脱敏。
* dry-run gate。
* APPROVED gate。
* recovery gate。

### Query 测试

* active/missing。
* 硬链接。
* 物理副本容量。
* 跨 storage。
* 稳定排序。
* keyset pagination。
* 目录角色筛选。
* 业务锚点筛选。
* 保护项筛选。
* 10,000 成员组。

### Contract 测试

验证：

```text
Go DTO
→ Wails 生成 TypeScript
→ JSON 字段
```

不发生无意变化。

---

## 13.2 合成测试语料

```text
testdata/visual-corpus/
├── exact-same-directory/
├── exact-cross-directory/
├── same-size-different-content/
├── same-name-different-content/
├── hardlinks/
├── symlinks/
├── missing-after-scan/
├── changed-after-approval/
├── protected-paths/
├── role-divergence/
├── anchor-divergence/
├── cross-storage/
├── backup-domain/
├── zero-byte/
├── unicode-nfc-nfd/
├── permission-denied/
├── long-path/
├── large-group/
└── crash-recovery/
```

所有前端截图、测试数据库和演示数据都必须来自这套合成语料。

---

## 13.3 Czkawka 对照

Czkawka 的核心与前端分离，多个界面共享同一个 core，适合作为 NDG 的工程参考和重复检测对照。([GitHub][7])

但对照时不要比较哈希字符串，而要比较：

```text
规范化相对路径集合
```

输出：

```text
BOTH_FOUND
ONLY_NDG
ONLY_CZKAWKA
```

需要注意：

* Czkawka 是参考基线，不是真理。
* NDG 的目录语境决策不会和 Czkawka一致。
* 两边只比较“发现了哪些字节级重复”。
* 不比较“是否应该删除”。

---

## 13.4 前端测试

建议：

```text
Vitest
React Testing Library
浏览器 E2E
Wails 最小烟雾测试
```

浏览器 E2E 使用 `MockBackendPort`：

* 打开项目。
* 添加数据源。
* 启动模拟扫描。
* 查看进度。
* 打开重复组。
* 查看目录分支。
* 应用治理决策。
* 生成计划。
* dry-run。
* 恢复页面。

Wails 烟雾测试只验证：

* 应用能启动。
* Go 绑定可调用。
* SQLite 能打开。
* 页面可加载。
* 事件能接收。

真实文件执行仍在 Go 的临时目录集成测试中完成。

---

# 第十四部分：CI 与发布

## 14.1 当前 CI 状态

当前 CI 已经包含：

* Go 格式。
* `go mod tidy` 检查。
* `public-check`。
* `go vet`。
* race test。
* CLI build。

可视化后需要拆成多个 Job。

---

## 14.2 推荐 CI

### Job 1：Go Core

```text
gofmt
go mod tidy check
go vet
go test -race
make public-check
CLI build
```

### Job 2：Frontend

```text
lockfile install
TypeScript typecheck
lint
unit test
frontend production build
```

### Job 3：Binding Contract

```text
生成 Wails bindings
git diff --exit-code
```

防止 Go DTO 已修改但前端生成类型未更新。

### Job 4：Desktop macOS

```text
Wails build
应用启动烟雾测试
资源检查
版本信息检查
```

### Job 5：Desktop Linux

Linux Wails 构建需要安装 GTK 和 WebKit 相关系统依赖，构建工作流应在原生 Linux Runner 上运行。([Wails][8])

### Job 6：公开边界

扩展现有 `public-check`，检查：

* `frontend/` 是否含真实路径。
* fixture 是否脱敏。
* 截图是否泄露。
* source map 是否被错误发布。
* `.env`。
* 私有数据库。
* JSONL。
* Wails 构建缓存。
* 前端日志样本。
* npm 调试日志。

---

## 14.3 发布物

第一阶段：

| 平台          | 推荐形式                |
| ----------- | ------------------- |
| macOS arm64 | `.app` + zip，后续 DMG |
| macOS amd64 | `.app` + zip        |
| Linux amd64 | tar 或 AppImage      |
| Linux arm64 | tar，后续 AppImage     |

公开 Beta 前增加：

* macOS 签名。
* macOS 公证。
* SHA-256 校验。
* 第三方许可。
* 前端依赖许可清单。
* SBOM。
* 版本、commit、build time。
* Release notes。
* 升级兼容性说明。

当前 CLI Release 已经有版本注入和 SHA-256SUMS，可以沿用其原则。

首版不建议增加自动更新。先保持用户主动下载和校验，降低供应链与回滚复杂度。

---

# 第十五部分：分阶段开发路线

## M7.0：可视化前置正确性

预计：1—2 周。

### 目标

在绘制正式 UI 前，建立准确、稳定、可分页的结果模型。

### 工作内容

1. 增加硬链接物理身份。
2. 增加 LinkCount 或可靠性字段。
3. 重复查询默认过滤 active。
4. 增加物理副本和容量计算。
5. 跨数据源默认隔离治理域。
6. 稳定 group ID。
7. 稳定排序。
8. 增加 SQLite Query API。
9. 拆分 ReviewDecision 与 PlanState。
10. 修正 CLI 中“approve this plan”的误导文案。
11. 增加对应测试。

### 验收

* 硬链接不会被计为可释放物理副本。
* missing 文件不会出现在当前重复组。
* 同一查询多次结果顺序一致。
* 25 万文件不需要载入全部记录才能显示第一页。
* 复核完成不再等同于批准执行。
* 原 CLI 行为兼容或提供迁移说明。

---

## M7.1：应用服务层

预计：1—2 周。

### 目标

CLI 和未来桌面端共用同一套用例。

### 工作内容

```text
internal/app
internal/query
internal/presentation
```

逐步抽取：

* ScanService。
* DuplicateService。
* ReviewService。
* PlanService。
* ExecutionService。
* RecoveryService。

### 验收

* CLI 不再直接编排 scanner/store/planner/executor 的大部分流程。
* 现有 CLI E2E 测试保持通过。
* 应用服务不引用 Wails。
* 服务返回结构化错误。
* 普通错误不含路径。

---

## M7.2：任务和事件

预计：1 周。

### 目标

为桌面端建立可恢复的任务模型。

### 工作内容

* `job_runs`。
* JobManager。
* ProgressSink。
* 事件节流。
* 取消。
* 从 checkpoint 继续。
* 应用重启后任务状态重建。

### 验收

* 扫描进度不依赖 stdout。
* 任务取消后状态准确。
* 重启后可看到上次中止任务。
* 不存在伪“暂停”。
* 所有普通事件不含路径。

---

## M7.3：只读桌面 Alpha

推荐版本：`v1.1.0-alpha.1`
预计：2—3 周。

### 包含

* 创建和打开项目。
* 添加数据源。
* 数据源预检。
* 启动只读扫描。
* 查看任务进度。
* 停止并保留断点。
* 查看重复组。
* 查看成员。
* 目录分支对照。
* 风险、保护、角色、锚点。
* 私有路径显示。
* 隐私截图模式。

### 不包含

* 计划审批。
* 文件写操作。
* 隔离。
* MOVE。
* 恢复写操作。
* 学习规则审批。

### Alpha 1 安全原则

桌面绑定层中根本不暴露执行方法，而不是只在界面上隐藏执行按钮。

---

## M8：治理复核工作台

推荐版本：`v1.1.0-alpha.2`
预计：2—3 周。

### 包含

* 组复核状态。
* 保留全部。
* 交叉归档标记。
* 备份关系标记。
* 选择主要保留项。
* 生成治理草案。
* 批量决策助手。
* 规则预览。
* 规则复核。
* 合并建议复核。
* 复核历史。
* 撤销尚未批准的决策。

### 验收

* 所有批量规则必须先预览。
* 任何批量规则都不能直接写文件。
* 受保护、跨职责和跨锚点默认保留。
* 决策和操作计划完全分离。
* 审批后计划不可原地任意修改；变更要产生新版本。

---

## M9：执行与恢复 Beta

推荐版本：`v1.1.0-beta.1`
预计：2—4 周。

### 包含

* 计划审批。
* 执行环境预检。
* dry-run。
* stale 结果。
* 隔离执行。
* Journal 时间线。
* 执行进度。
* 验证结果。
* 失败回滚。
* 应用启动恢复。
* 恢复锁。

### 验收

* 没有 dry-run 结果不能真实执行。
* 非 APPROVED 不能执行。
* Journal 不可用不能执行。
* 隔离区和 SourceRoots 重叠不能执行。
* 执行仍串行。
* 崩溃后能够恢复或回滚。
* 不提供永久删除。

---

## M10：性能、发布与公开 Beta

推荐版本：`v1.1.0-beta.2` 或 `v1.1.0-rc.1`
预计：2—4 周。

### 包含

* 流式/批量扫描持久化。
* size-first 快速哈希优化。
* QueryDB 读连接。
* 虚拟表格。
* keyset pagination。
* 25 万文件基准。
* 超大组基准。
* macOS 签名公证。
* Linux 安装包。
* SBOM。
* 前端许可清单。
* 完整公开边界检查。
* 文档和引导。

### 建议性能预算

这些是工程目标，不是当前已达到的数据：

| 场景          |         目标 |
| ----------- | ---------: |
| 打开 25 万文件项目 | 2 秒内出现项目摘要 |
| 重复结果首屏      |    500ms 内 |
| 翻到下一页       |    300ms 内 |
| 表格滚动        |      无明显卡顿 |
| 前端一次加载组数    |    100—200 |
| 事件刷新频率      | 不超过 10 次/秒 |
| 后端完整路径传输    |      仅详情按需 |
| 页面打开        |  不加载全部文件实例 |

---

# 第十六部分：建议 PR 顺序

按当前仓库最新为 PR #6 推算，可以从 #7 开始。

| 建议 PR | 内容                 | 关键验收                   |
| ----- | ------------------ | ---------------------- |
| #7    | ADR：NDG 可视化架构      | 明确 Wails、React、层次和安全边界 |
| #8    | 重复组正确性             | active、硬链接、稳定 ID、容量模型  |
| #9    | Query read model   | SQLite 分页、筛选、详情查询      |
| #10   | ReviewDecision 重构  | 复核与执行批准分离              |
| #11   | `internal/app` 抽取  | CLI 行为不变               |
| #12   | JobManager 与事件     | 可取消、可恢复、无路径事件          |
| #13   | Wails + React 脚手架  | 能启动、版本和项目打开            |
| #14   | 数据源页面              | 验证、添加、策略               |
| #15   | 扫描任务页面             | 启动、进度、停止、续扫            |
| #16   | 重复结果页面             | 分页、筛选、虚拟滚动             |
| #17   | 目录语境检查器            | 分支、角色、锚点、评分            |
| #18   | 治理复核               | 决策、批量预览、草案             |
| #19   | 计划与 dry-run        | 无真实写入                  |
| #20   | 执行中心               | Journal、隔离、回滚          |
| #21   | 审计与恢复              | 重启恢复、时间线               |
| #22   | Desktop CI/Release | macOS/Linux 构建和许可      |

不要在一个 PR 里同时完成：

```text
Wails 脚手架
+ 扫描重构
+ SQLite 迁移
+ 完整 UI
+ 执行功能
```

这会让审查、回滚和定位问题变得困难。

---

# 第十七部分：风险登记表

| 风险           | 当前表现                    | 处理方式                  |
| ------------ | ----------------------- | --------------------- |
| CLI 与核心耦合    | `main.go` 直接编排大量包       | 抽取 `internal/app`     |
| 大量数据查询       | `ListFiles` 返回全部        | QueryDB、分页、按需详情       |
| missing 混入   | `ListFiles` 未过滤状态       | 所有当前视图默认 active       |
| 硬链接容量误判      | 不同路径进入同一 SHA 组          | 路径/物理对象双层模型           |
| 复核语义混乱       | REVIEW→SKIP 被称为 approve | ReviewDecision 独立     |
| 任务不持久        | Runner 只是 semaphore     | JobManager + job_runs |
| 进度泄露路径       | 扫描对象包含路径                | 普通事件只发聚合值             |
| SQLite 放 NAS | WAL 不适合网络文件系统           | DB 固定在本机              |
| GUI 造成过度信任   | 用户看到建议就直接操作             | Alpha 只读、dry-run 强制   |
| 批量误处理        | 跨页全选和自动操作               | 预览、数量、原因和撤销           |
| 前端卡顿         | 全量路径进入 React            | 虚拟表格、分页、懒加载           |
| Wails 供应链变化  | v3 仍为 Alpha             | 锁定 Wails v2.13        |
| 开发模式泄密       | DevTools 可调用绑定          | 只用合成数据开发              |
| 范围膨胀         | 过早做媒体相似、远程 Agent        | 推迟到 M11 以后            |
| 跨平台拖延        | 同时支持 Windows            | 首阶段仅 macOS/Linux      |

---

# 第十八部分：明确禁止的实现方式

以下实现不应进入正式架构：

1. React 直接执行任意系统命令。
2. 前端直接调用删除、移动文件。
3. GUI 通过解析 CLI 文本运行核心流程。
4. 扫描完成后把全部文件一次性传给 React。
5. 把 SQLite 放在 NAS SMB 目录。
6. 依靠数组位置表示重复组 ID。
7. 把硬链接路径数当作物理副本数。
8. missing 文件参与当前治理容量。
9. 用“批准”表示 REVIEW→SKIP。
10. 扫描后自动生成并执行删除。
11. AI 直接修改计划状态。
12. 跨备份域自动去重。
13. 在普通日志中打印完整路径。
14. 在浏览器 localStorage 保存敏感路径。
15. 在公开仓库放真实 NAS 截图。
16. 在第一版加入永久删除。
17. 前端隐藏按钮但后端仍暴露危险 API。
18. 为追求进度显示而伪造扫描百分比。
19. 把所有诊断、复核和执行做成“一键整理”。
20. 在 Wails v3 Alpha 上开始正式产品基线。

---

# 第十九部分：现实工期

以一名主要开发者配合 AI 编程、现有 Go 核心继续复用为前提：

| 阶段               |    预计 |
| ---------------- | ----: |
| 正确性和查询基座         | 1—2 周 |
| 应用服务层与任务事件       | 1—2 周 |
| 只读 Desktop Alpha | 2—3 周 |
| 治理复核             | 2—3 周 |
| 执行与恢复            | 2—4 周 |
| 性能、打包和公开 Beta    | 2—4 周 |

综合：

```text
可演示原型：2—3 周
只读可用 Alpha：4—6 周
治理闭环 Alpha：6—9 周
可执行 Beta：8—12 周
较稳定公开版：10—16 周
```

前提是严格控制范围：

* 不做 Windows。
* 不做远程 NAS Agent。
* 不做相似媒体。
* 不做云端。
* 不做多人协作。

---

# 第二十部分：最佳下一步

现在不应先做高保真界面，也不应先把 Duplicate Cleaner 的截图复刻出来。

最合理的连续三步是：

### 第一步：建立 ADR

新增：

```text
docs/adr/0005-desktop-visualization-architecture.md
```

明确：

* Wails v2。
* React + TypeScript。
* 本机 SQLite。
* CLI 与桌面共享应用服务。
* GUI 不解析 CLI。
* 第一版只读。
* 不支持 Windows。
* 不暴露原始文件操作 API。

### 第二步：修正重复组查询模型

完成：

* active 过滤。
* 硬链接物理身份。
* 可回收容量分层。
* 稳定 group ID。
* SQLite 分页。
* 跨数据源治理域。
* 确定性排序。
* Query 测试。

### 第三步：抽取 `internal/app`

先从：

```text
scan
duplicates
plan
```

三个流程开始。

CLI 改为调用应用服务，但用户行为和现有测试保持不变。

完成这三步后，再创建 Wails 壳。此时桌面前端面对的是：

```text
稳定用例
稳定 DTO
稳定查询
稳定状态
```

而不是面对一组 CLI 函数和不断变化的内部结构。

---

## 最终判断

NDG 当前并不缺扫描能力，也不缺治理概念。它真正缺的是一个把现有能力安全组织起来的应用层和工作台。

正确路线不是：

```text
先做一个漂亮的 Duplicate Cleaner 界面
再想办法连接现有代码
```

而是：

```text
先修正重复结果的物理与治理语义
→ 建立 SQLite 读模型
→ 抽取应用服务
→ 建立持久任务与结构化事件
→ 上线只读桌面 Alpha
→ 增加治理复核
→ 增加 dry-run
→ 最后开放受控隔离执行
```

经过这一轮核查，NDG 的最佳产品方向可以进一步概括为：

> **Czkawka 提供扫描架构参考，Duplicate Cleaner 提供桌面工作流参考，NDG 自身提供目录语境、治理决策、安全审批和崩溃恢复能力。**

第一版可视化的胜负不取决于是否比 Duplicate Cleaner 更漂亮，而取决于它能否准确回答：

> 这些文件为什么相同、为什么仍可能都要保留、为什么建议处理其中某一个，以及处理失败后如何证明并恢复。

[1]: https://wails.io/changelog/ "https://wails.io/changelog/"
[2]: https://wails.io/zh-Hans/docs/howdoesitwork/ "https://wails.io/zh-Hans/docs/howdoesitwork/"
[3]: https://www2.sqlite.org/wal.html "https://www2.sqlite.org/wal.html"
[4]: https://wails.io/docs/reference/runtime/events/ "https://wails.io/docs/reference/runtime/events/"
[5]: https://tanstack.com/table/latest/docs/guide/virtualization "https://tanstack.com/table/latest/docs/guide/virtualization"
[6]: https://wails.io/docs/gettingstarted/development/ "https://wails.io/docs/gettingstarted/development/"
[7]: https://github.com/qarmin/czkawka/blob/master/instructions/Instruction.md "https://github.com/qarmin/czkawka/blob/master/instructions/Instruction.md"
[8]: https://wails.io/docs/gettingstarted/building/ "https://wails.io/docs/gettingstarted/building/"
