# NDG v0.5 Beta — RC / Release Execution & UI Final Polish 执行操作手册

> 项目：NAS Data Governance（NDG）
> 首个公开 Beta 候选版本：`v0.5.0-beta.2`（UI 收口及 RC 冻结后创建）
> 阶段：Beta RC / Release Execution
> 文档性质：首个公开 Beta 发布前唯一执行基线；本文中的清单负责实时进度管理
> 仓库核对：2026-09-21，`main` = `4bade7715b47d4a71e4a114f835388602f2e865a`
> 当前代码版本：`0.5.0-beta.1`；同名标签已存在于远端，指向上述 SHA；`VERSION` 尚未提升到 `0.5.0-beta.2`
> 已锁定路线：**UI Final Polish → `VERSION` 提升至 beta.2 → RC 冻结 → 新标签发行 → 实机验收 → 首个 Public Beta**

## 执行入口与优先级

本手册是本次发布的**唯一实时进度表和执行决策记录**。`readiness-audit-v0.5.md` 和 `release-gates-v0.5.md` 是 2026-07-30 的历史审计快照，保留原文作为证据，不更新其中的状态，也不得把其旧状态当作当前事实。本文与 `AGENTS.md`、已接受 ADR、实际代码或发布平台状态冲突时，先遵守安全边界并重新取证，在下方“执行状态与证据”记录修正。

每个关口依次记录：负责人、目标提交/产物、检查日期、证据链接或本地记录、PASS/FAIL/BLOCKED、待办。没有证据的项目保持 `[ ]`。`[x]` 只表示已在所列证据层验证；自动化测试、GitHub CI、签名产物、实机体验彼此不能替代。发布、NAS 写操作、真实数据隔离/恢复应分别按相应权限和安全流程执行。

### 版本决策（2026-09-21 锁定）

首个公开 Beta **选择 UI Final Polish 后发行 `v0.5.0-beta.2`**。`v0.5.0-beta.1` 保留为不可变的历史 RC/发布尝试；不补凭据后重跑它来作为首个公开 Beta，不移动、不删除该标签。UI 和必要的 Release Blocker 修复进入新提交，`VERSION`、应用版本元数据和发行说明在新候选构建前同步为 `0.5.0-beta.2`。只有 UI 验收及全部发布门槛通过后，才冻结 RC SHA、创建并推送 `v0.5.0-beta.2`。若代码变更迫使 RC 重新冻结，须重新跑相关关口；已推送的新标签同样不得移动。

六项 Apple/GitHub 发布 Secret 仍是签名发行硬门槛。凭据只由有权限的持有人配置在 `release-macos` Environment；文档仅记录配置状态和验证结果，不记录值。`VERSION` 目前保留 `0.5.0-beta.1`，在 UI/Release 修复进入目标提交时再按仓库版本同步脚本提升，避免当前主干与既有标签产生没有对应代码的虚假 beta.2 状态。

### 执行状态与证据（2026-09-21 快照；后续只在此处更新实时状态）

| 关口 | 状态 | 当前证据 / 下一动作 |
|---|---|---|
| 首发版本决策 | `[x]` | 2026-09-21：选择 UI Final Polish 后以 `v0.5.0-beta.2` 首发；`beta.1` 仅保留历史 RC/失败发布尝试。 |
| 历史版本与来源 | `[x]` | `VERSION=0.5.0-beta.1`；远端标签 `v0.5.0-beta.1` 指向 `4bade771`，与核对时 `main` 一致。标签不可覆盖，也不作为首个公开 Beta。 |
| beta.2 版本与 RC | `[ ]` | UI/Release 修复后同步版本、完成验收、固定新 RC SHA；当前尚无 beta.2 提交或标签。 |
| 仓库发布能力 | `[x]` | `scripts/release/`、`.github/workflows/release.yml`、`make version-check`、`make desktop-build` 已存在；须以标签提交中的实现为准。 |
| 标签工作流 Verify | `[x]` | [GitHub Actions run 31780920930](https://github.com/FNB2026/nas-data-governance/actions/runs/31780920930)：Verify 成功。 |
| 标签工作流 Build Unsigned | `[x]` | 同一 run 的 `Build Unsigned .app` 成功；只证明未签名构建。 |
| 签名与公证 | `[ ] BLOCKED` | beta.1 run 在 `Verify required secrets` 失败；六项 `release-macos` Environment Secrets 均报告缺失。配置后在 **beta.2 新标签工作流**中验证实际签名、公证与 staple；不要记录凭据值。 |
| Release / DMG / SHA-256 / SBOM | `[ ]` | `gh release view v0.5.0-beta.1` 返回 `release not found`；目标为 beta.2 标签生成的 DMG、校验和、SBOM 与 Draft Release。 |
| GitHub 防护 | `[ ]` | `release-macos` 环境及 required reviewer 存在；REST branch protection 查询返回 `Branch not protected`。还需核查仓库 ruleset、Secret Scanning、Push Protection 等的实际生效情况。 |
| UI Final Polish 与 RC 冻结 | `[ ]` | 七域代码存在；本文的 UI 验收、最小窗口、错误与危险操作流程尚须逐项验证。通过后才能冻结 beta.2 RC。 |
| Clean Install / 真实 NAS / 治理恢复 | `[ ]` | 尚无签名发行物及实机验收证据；只在隔离测试数据与可恢复路径上验证写操作。 |
| Public Beta | `[ ]` | 以上所有阻断关口通过、记录发布决策后才能公开。 |

### 已存在标签的处理规则

`v0.5.0-beta.1` 已在远端，不再执行本文任何“创建/推送同名标签”的旧命令，也不将其失败工作流重跑结果作为本轮首发候选。UI、安全或发布代码修复进入新提交并使用 `v0.5.0-beta.2`；同步 `VERSION` 和发行说明。不得删除、移动或强推 `beta.1` 标签，也不得把新提交的手工构建冒充 `beta.1` 标签产物。本手册在 `main` 上管理执行，不改变已固定的标签内容。

> 核验标签指向时，`v0.5.0-beta.*` 是 **annotated tag**，必须用 `^{commit}` 或 `git rev-list -n1` 取 commit，
> 不要比较 tag object —— 详见 §27.1。

---

# 0. 本轮工作的最终目标

本轮不是继续开发 NDG 的新能力，而是完成 NDG 从：

```text
“功能完整的开发项目”
```

到：

```text
“普通用户可以下载安装、理解、使用、出错后能够恢复的 Beta 产品”
```

的最后一次跨越。

最终必须得到：

```text
NDG-0.5.0-beta.2-macos.dmg
```

并满足：

```text
Developer ID 签名
        ↓
Hardened Runtime
        ↓
Apple Notarization
        ↓
Staple
        ↓
SHA-256
        ↓
SBOM
        ↓
GitHub Release
        ↓
全新 Mac 安装路径验证
        ↓
真实 NAS 扫描
        ↓
治理 / 隔离 / 恢复验证
        ↓
Public Beta
```

本轮完成后，`v0.5.0-beta.2` 才视为首个公开 Beta 发布。

---

# 1. 当前工程事实基线

以下能力清单来自原执行稿，用于确定验收范围；“已完成”不等于已在签名发行物及真实 NAS 上通过验收。以本文状态表的当前证据和各关口结果为准。

截至原稿所依据的主干，NDG 已完成的主要能力包括：

* Go + Wails 桌面架构；
* React + TypeScript 前端；
* SQLite 项目数据库；
* 本地文件 / NAS 扫描；
* SMB / NFS / WebDAV / FUSE 环境处理；
* 增量扫描；
* 快速哈希与完整 SHA-256；
* 物理副本与硬链接识别；
* 断点续扫；
* 网络中断安全暂停与恢复；
* 目录角色与目录语境；
* 权威性、业务锚点及保护规则；
* 重复文件分析；
* 治理草案；
* 人工复核；
* 审批；
* stale check；
* dry-run；
* Quarantine；
* Restore；
* Purge；
* Journal；
* Crash Recovery；
* Operation Audit；
* 路径脱敏；
* GitHub CI；
* Gitleaks；
* Govulncheck；
* Release Workflow；
* macOS 签名 / 公证脚本；
* SBOM；
* SHA-256；
* 用户指南；
* README 产品化；
* 公共仓库治理。

当前前端已经存在七个正式业务域：

```text
数据源
扫描任务
重复结果
治理复核
执行中心
审计与恢复
设置
```

现阶段禁止把 NDG 再退化成：

```text
扫描 → 找重复文件 → 删除
```

NDG 的正式业务闭环必须保持为：

```text
发现
 ↓
理解
 ↓
判断
 ↓
审批
 ↓
执行
 ↓
验证
 ↓
审计
 ↓
恢复
```

---

# 2. 本轮总原则

## 2.1 Feature Freeze

从本手册执行开始进入：

```text
Feature Freeze
```

以下内容原则上不得进入 `v0.5.0-beta.2`：

* 新文件分析算法；
* 新 AI 功能；
* 新 NAS 协议；
* 新平台；
* Windows；
* Linux GUI；
* Intel Mac；
* 云同步；
* 远程账号；
* 多用户系统；
* 新规则引擎；
* 大规模数据库 Schema 重构；
* Wails 大版本升级；
* React 技术栈迁移；
* UI 框架迁移；
* 新 Dashboard；
* 不影响 Beta 发布的“顺手优化”。

允许进入本轮的修改必须属于：

```text
Bug Fix
UI/UX Final Polish
Accessibility
Release Blocker
Security
Crash / Data Safety
Release Engineering
Documentation
Beta Onboarding
```

---

# 3. 本轮实施顺序

以下是关口顺序；已由上方证据确认的步骤无需重复制造新提交或新标签，失败关口从失败点继续：

```text
RC-0  基线审计
 ↓
UI-1～UI-12  七域、状态、安全操作及首次使用验收（第 5～19 节）
 ↓
RC-1  RC Freeze
 ↓
REL-1～REL-5 发布环境、凭据、身份与 Release Drill（第 22～26 节）
 ↓
REL-6 核验 beta.1 不可变、从新 RC 创建 beta.2 标签（第 27 节）
 ↓
REL-7 beta.2 新标签工作流、签名 / 公证 / DMG / Draft Release（第 28 节）
 ↓
QA-1～QA-10 Clean Install、真实 NAS、恢复与异常测试（第 29～38 节）
 ↓
REL-8 Public Beta
 ↓
POST-1 发布后观察
```

---

# 4. RC-0：发布前基线审计

先在现有工作区做只读基线记录，核查分支、HEAD、未提交文件、远端与标签。不得为了运行本手册直接切换分支、拉取或覆盖用户当前工作。确需代码修复时再按仓库协作流程创建独立分支或工作树。

只读命令：

```bash
git status --short --branch
git rev-parse HEAD
git ls-remote --tags origin 'v0.5.0-beta.1' 'v0.5.0-beta.1^{}'
cat VERSION
```

当前基线应确认；UI 收口后再执行 beta.2 版本同步与一致性检查：

```text
VERSION = 0.5.0-beta.1
```

运行测试或构建前记录已有的未提交内容。正式发行产物必须来自确定的干净标签提交，不得混入工作树内容。

---

## 4.1 基线测试

执行：

```bash
go test -race -count=1 ./...
go vet ./...
make public-check
make version-check
make frontend-check
make desktop-build
```

如果仓库已有对应目标，再执行：

```bash
make wails-check
```

以及：

```bash
./scripts/release/test-version-mapping.sh
./scripts/release/test-macos-scripts.sh
./scripts/release/test-release-workflow.sh
```

要求：

```text
所有测试 PASS
所有构建 PASS
无新增 warning
无 dependency resolution 异常
无未提交生成文件
```

---

# 5. UI Final Polish 总目标

本轮 UI 不再重构信息架构。

正式信息架构锁定：

```text
数据源
扫描任务
重复结果
治理复核
执行中心
审计与恢复
设置
```

禁止：

* 恢复旧五页面方案；
* 新增 Dashboard；
* 合并治理复核与重复结果；
* 合并执行中心与审计恢复；
* 用技术模块名称替代用户任务；
* 大规模重写 `AppShell`；
* 引入重量级 UI Framework。

本轮 UI 的目标不是“更漂亮”，而是：

> 降低认知成本、强化 NDG 的差异化能力、让危险操作具有明确心理边界，让第一次使用 NDG 的普通用户知道“现在发生了什么、为什么这样建议、下一步该做什么”。

---

# 6. UI-1：应用 Shell 收口

当前：

```text
AppShell
├── Header
├── Sidebar
├── Main Content
└── Footer
```

结构保留。

需要完成以下优化。

## 6.1 Header

Header 仅保留高价值信息：

```text
NDG
项目名称
项目模式
当前任务状态
连接状态
```

版本号不得成为主视觉。

版本详细信息移动到：

```text
设置 → 关于
```

扫描进行时显示：

```text
扫描中 · 已处理 42,813 / 103,211
```

网络异常时：

```text
NAS 连接中断
扫描已安全暂停
[重新检查]
```

禁止只显示技术错误：

```text
PAUSED_NETWORK
ERR_IO
NETWORK_UNAVAILABLE
```

内部错误码只能作为高级信息。

---

# 7. UI-2：数据源页面

页面必须回答四个问题：

```text
我现在打开的是什么？
数据在哪里？
这些数据能不能扫描？
现在安全吗？
```

推荐结构：

```text
项目
├── 项目名称
├── 数据库位置
├── 读写 / 只读
└── 项目健康状态

数据源
├── NAS / 本地
├── 在线状态
├── 最近扫描
├── 文件数
└── 容量

扫描准备
├── 权限
├── 网络
├── 挂载点
├── Source Root
└── 排除规则

安全配置
├── 隔离目录
├── Journal
├── 路径保护
└── 恢复状态
```

普通用户不应看到：

```text
inode
device id
SQLite journal internals
hash worker internals
```

除非进入：

```text
高级信息
```

---

# 8. UI-3：扫描任务页面

必须把扫描理解成一个“任务”，而不是一个进度条。

页面结构：

```text
当前任务

扫描阶段
发现
 ↓
元数据
 ↓
快速哈希
 ↓
候选筛选
 ↓
完整校验
 ↓
索引完成

扫描统计

异常 / Warning

最近任务
```

状态必须统一：

```text
未开始
等待中
扫描中
安全暂停
用户取消
失败
完成
```

网络异常必须使用：

```text
扫描已安全暂停

NAS 当前无法访问。
已保存扫描进度，不会把未读取文件误判为删除。

重新挂载 NAS 后可以继续。

[重新检查连接]
[继续扫描]
```

绝对禁止：

```text
Network Error
Scan Failed
Retry
```

作为唯一解释。

---

# 9. UI-4：重复结果页面

这是 NDG 最重要的产品差异化页面之一。

不得把它设计成：

```text
文件 A
文件 B
文件 C
[删除重复项]
```

必须突出：

```text
内容相同 ≠ 可以删除
```

每个重复组至少需要表达：

```text
内容一致性
物理副本数
硬链接数量
理论可回收空间
目录语境
目录角色
保护状态
治理建议
建议依据
```

推荐：

```text
发现 3 个内容相同文件

★ 建议保留
/财务部/销售发票/2019/xxx.pdf

财务归档
高保护
业务记录
目录稳定

────────────────────

⚠ 不建议自动处理
/客户项目/已完结/xxx.pdf

项目归档
独立业务语境
与上方目录职责不同

────────────────────

○ 可考虑治理
/Downloads/xxx.pdf

临时下载
低业务锚点

[查看治理依据]
[进入治理复核]
```

高级字段通过折叠区域提供：

```text
SHA-256
device
inode
mtime
physical identity
rule version
```

---

# 10. UI-5：治理复核页面

治理复核必须明确区分：

```text
系统建议
```

和：

```text
用户决定
```

建议表达：

```text
NDG 建议：
保留 A
治理 B
保留 C

依据：
✓ A 位于权威业务目录
✓ B 位于临时目录
✓ C 与 A 属于不同业务语境
```

用户操作：

```text
接受建议
调整决定
暂不处理
```

不得出现：

```text
一键删除全部重复文件
智能自动清理
```

Beta 阶段默认必须保持：

```text
Human in the loop
```

---

# 11. UI-6：执行中心

执行中心必须成为明确的安全边界。

建议视觉步骤：

```text
1. 治理计划
      ↓
2. 人工审批
      ↓
3. Dry Run
      ↓
4. 新鲜度校验
      ↓
5. 隔离
      ↓
6. 验证
```

普通治理操作不得直接永久删除。

标准行为：

```text
治理 → Quarantine
```

永久删除：

```text
Purge
```

必须是独立生命周期。

Purge 页面需要明显风险提示。

建议：

```text
永久清理

此操作将真正删除已经进入隔离区且满足清理条件的文件。

不可通过 NDG Restore 恢复。

建议确认：
✓ 隔离时间满足要求
✓ 文件已验证
✓ 已存在独立备份
✓ 当前没有恢复锁

输入：

永久清理

才能继续。
```

禁止仅使用：

```text
[确认]
```

按钮完成不可逆操作。

---

# 12. UI-7：审计与恢复页面

这是 NDG 与普通清理软件的重要区别。

普通用户默认看到：

```text
系统健康
无待恢复任务
最近操作
```

异常时切换为：

```text
检测到未完成操作

NDG 已阻止新的写操作。

上一次操作可能在执行过程中被中断。

[查看影响]
[开始恢复]
```

高级用户再展开：

```text
Operation Log
Journal
Plan ID
Action
Stage
Timestamp
Error
```

不要让 Journal 成为默认主界面。

---

# 13. UI-8：设置页面

建议重新整理为：

```text
常规
扫描
隐私
安全
关于
```

其中：

## 常规

* 默认行为；
* UI 参数；
* 最近项目。

## 扫描

* 完整扫描默认值；
* worker；
* 高级扫描参数。

## 隐私

* 路径脱敏；
* 无遥测；
* 无第三方云上传；
* 外部 AI 状态。

## 安全

* 项目读写状态；
* Recovery Lock；
* Quarantine；
* Purge 状态；
* 本地数据位置。

## 关于

```text
NDG
Version
Commit
Build Time
Channel
License
GitHub
User Guide
Privacy
Support
```

---

# 14. UI-9：设计系统收口

继续使用现有 Design Token。

禁止组件内部随意新增：

```css
#xxxxxx
margin: 17px
border-radius: 13px
```

应全部优先引用：

```text
tokens.css
```

统一：

* spacing；
* radius；
* typography；
* border；
* shadow；
* semantic color；
* status badge；
* button；
* form；
* table；
* card；
* alert；
* dialog；
* drawer；
* empty state。

必须建立统一按钮层级：

```text
Primary
Secondary
Tertiary
Danger
```

Danger 不得与 Primary 使用相同视觉重量。

---

# 15. UI-10：空状态

七个页面都必须有明确 Empty State。

禁止：

```text
空表格
0 records
No Data
```

例如重复结果：

```text
暂未发现重复文件

完成一次扫描后，NDG 会在这里展示经过内容校验的重复文件组。

[开始扫描]
```

治理复核：

```text
当前没有待复核项目

先从“重复结果”中选择需要治理的文件组。
```

执行中心：

```text
暂无已批准治理计划

治理计划必须先经过人工复核与批准，才能进入执行阶段。
```

---

# 16. UI-11：Loading / Error / Disabled 统一

所有页面状态采用统一模型：

```text
idle
loading
ready
empty
warning
error
disabled
```

禁止：

* 有的页面使用 spinner；
* 有的使用文字；
* 有的直接空白；
* 有的 Toast 后什么都不显示。

Disabled 必须解释原因。

例如：

```text
执行中心当前不可用

原因：
项目以只读模式打开。

重新以读写模式打开项目后可使用执行功能。
```

---

# 17. UI-12：首次使用体验

首次启动 NDG 时不要做复杂 Wizard。

推荐：

```text
欢迎使用 NDG

NDG 用于本地 NAS 与大量文件的数据治理。

扫描阶段不会修改源文件。

建议第一次使用：
选择一个较小目录进行体验。

[选择扫描目录]

了解 NDG 的安全机制
```

创建项目后：

```text
第一步
选择数据源

第二步
扫描

第三步
查看重复结果

第四步
治理复核

第五步
安全隔离
```

不要一次展示所有专业能力。

---

# 18. UI Beta 验收标准

UI 收口完成后至少测试：

### 窗口

```text
1180 × 720
1440 × 900
1728 × 1117
全屏
```

检查：

* 无内容截断；
* 无横向异常滚动；
* 主按钮不消失；
* 表格列不崩；
* Drawer / Modal 不超出窗口；
* 长路径不会撑爆布局。

### 状态

必须覆盖：

```text
无项目
只读项目
读写项目
无 NAS
NAS 在线
NAS 断网
扫描中
安全暂停
扫描完成
无重复
大量重复
无治理计划
有草案
已批准
执行中
隔离完成
恢复锁
Purge
错误
```

### P8-D 当前证据（2026-09-22）

K2、K3、K4、K5、K6 已通过真实 NDG.app 验收：路径脱敏开启时，Duplicate Results 的代表文件与路径、Audit 详情、Journal 源/目标路径均不泄露；未作用户决定不能批准；隔离后可按原路径恢复并复核 SHA-256；Purge 经草案、批准、试运行、错误确认拦截和逐字确认后进入 `PURGED`；Recovery Lock 阻止扫描与执行入口，同时保留审计与恢复入口，恢复后将无已完成写入的 `EXECUTING` 计划重置为 `APPROVED`。

K5/K6 fixture 位于 `scripts/release/p8d-fixture/`，只使用 `MkdirTemp` 创建临时项目，不接受外部项目数据库、NAS 或 source root 路径。它用于构造已完整经过最短 24 小时保留期的历史记录，不改变产品默认 30 天保留期。

K1、1180×720 与其余 P8-D 场景仍须按本手册完成；因此本阶段**不得**提升 `VERSION` 或创建 `v0.5.0-beta.2`。

---

# 19. UI Final Polish 完成定义

只有同时满足以下条件，才允许进入 RC Freeze：

```text
[ ] 七个一级入口结构不再变更
[ ] 页面主要流程完整
[ ] 无明显工程调试 UI
[ ] 无裸错误码作为主要提示
[ ] 所有 Empty State 完成
[ ] Loading / Error / Disabled 统一
[ ] Quarantine 与 Purge 明确区分
[ ] 危险操作有强确认
[ ] 路径脱敏覆盖主要界面
[ ] 1180×720 可用
[ ] UI 测试通过
[ ] TypeScript 通过
[ ] Production Build 通过
[ ] Wails Build 通过
```

---

# 20. RC-1：Release Candidate Freeze

UI Final Polish 在目标提交上验收后，重新完整执行：

```bash
go test -race -count=1 ./...
go vet ./...
make frontend-check
make public-check
make version-check
make desktop-build
```

确认无问题后，记录：

```bash
git rev-parse HEAD
```

该 SHA 与 `0.5.0-beta.2` 一并记录为新 RC 候选。确认 `VERSION`、桌面应用元数据、运行时版本和发行说明一致；该 SHA 必须是后续 `v0.5.0-beta.2` 标签的唯一构建来源。

合格的 RC 状态为：

```text
NDG v0.5.0-beta.2 RC
```

从这一刻开始：

```text
禁止 Feature
只允许 Release Blocker Fix
```

---

# 21. Release 文档补齐

本文是唯一实时发布执行基线；如需拆分用户指南或发行说明，可新增静态内容文档，但不得再建立第二份实时状态表。历史材料：

```text
docs/release/
├── readiness-audit-v0.5.md
├── release-gates-v0.5.md
└── NDG-v0.5-Beta-RC-Release-Execution-Manual.md
```

前两份保留为历史审计依据，不更新旧状态；所有实时进度、阻断原因和决策只更新本文“执行状态与证据”。

---

# 22. REL-1：GitHub 发布环境检查

发布前必须人工确认：

## Repository

```text
[ ] main branch protection
[ ] require PR
[ ] required status checks
[ ] force push blocked
[ ] delete protection
[ ] secret scanning
[ ] push protection
[ ] Dependabot alerts
[ ] Private Vulnerability Reporting
```

### 依赖告警审计结论（2026-09-22 冻结）

已对当时 3 条 open alert 逐条做**可达性审计**（判据：漏洞的可触发前提在发布产物中是否成立），
结论冻结如下：

| # | 严重度 | 依赖 | CVE / GHSA | 是否在 Release 产物中可达 |
| --- | --- | --- | --- | --- |
| 17 | high | `labstack/echo/v4` v4.13.3 | CVE-2026-55677 | **否** |
| 18 | medium | `@vitest/mocker` 4.1.10 | CVE-2026-84373 | 否（开发期工具链） |
| 19 | medium | `vitest` 4.1.10 | CVE-2026-84373 | 否（开发期工具链） |

**新增 RC Blocker：0。** echo 之所以被判为不可达，证据是它只经
`wails/v2/internal/app/app_dev.go`（`//go:build dev`）引入 `internal/frontend/devserver`：

```bash
go list -deps ./cmd/ndg-desktop | grep -c 'labstack/echo'          # → 0
go list -deps -tags dev ./cmd/ndg-desktop | grep -c 'devserver'     # → 1
```

且仓库 Go 代码**零 `net/http` 引用**，没有任何 HTTP 服务或静态文件服务。
完整审计见 `var/reports/dependabot-audit-2026-09-22.md`。

**处置决定（2026-09-22 锁定）：RC 前只合单包 PR `#31`（echo）与 `#33`（vitest + @vitest/mocker）。**

- **不合分组升级 PR `#34` / `#35`** —— 它们是 4 个 Go / 5 个 npm 依赖的批量升级，
  会把与告警无关的版本变化带进最终候选，冻版前夕无谓扩大回归面；留到 beta.2 之后的维护窗口。
- **不要在旧 main 上合这些 PR** —— 已核实 `main` 的 HEAD 即 `v0.5.0-beta.1` 指向的提交
  `4bade7715b…`，四个 PR 的 base 全是它，而 UI Final Polish 已远超之。
  正确顺序是：先合 Final Polish → 让 Dependabot 基于新 main rebase / recreate → 再合 `#31` / `#33`。
  PR 显示 `MERGEABLE / CLEAN` 只代表"相对旧 base 干净"。
- 合并后专项验证：`#31` → `go build -tags dev ./cmd/ndg-desktop`、`go test ./...`、`go vet ./...`；
  `#33` → `npm test`、`npm run build`。随后关闭被取代的 `#34` / `#35`。

## Environment

确认存在：

```text
release-macos
```

确认：

```text
Required reviewers
```

已启用。

如果只有一个维护者，应确认当前审批策略不会形成永久死锁。

---

# 23. REL-2：Apple 发布凭据

Release Environment 中需要：

```text
DEVELOPER_ID_CERTIFICATE_P12
DEVELOPER_ID_CERTIFICATE_PASSWORD
APPLE_TEAM_ID
APP_STORE_CONNECT_API_KEY_ID
APP_STORE_CONNECT_API_KEY
APP_STORE_CONNECT_ISSUER_ID
```

禁止：

* 写入仓库；
* 写入 `.env.example` 真值；
* 写入 Issue；
* 写入 PR；
* 写入日志；
* 放入普通 Repository Variable。

必须使用 GitHub Environment Secrets。

---

# 24. REL-3：证书预检

Apple Developer 必须具备：

```text
Developer ID Application
```

而不是：

```text
Apple Development
```

签名目标：

```text
Developer ID Application: ...
```

本地检查：

```bash
security find-identity -v -p codesigning
```

必须看到有效 Developer ID Application identity。

---

# 25. REL-4：Bundle Identity 检查

确认：

```text
Bundle ID
```

由项目正式控制。

不要在 RC 阶段修改 Bundle ID。

检查：

```bash
plutil -p cmd/ndg-desktop/build/darwin/Info.plist
```

重点确认：

```text
CFBundleIdentifier
CFBundleShortVersionString
CFBundleVersion
LSMinimumSystemVersion
LSApplicationCategoryType
```

其中：

```text
CFBundleShortVersionString = 0.5.0
CFBundleVersion = 当前合法构建号
```

不得出现：

```text
0.5.0-beta.1
```

作为 Apple Bundle Version 字段。

---

# 26. REL-5：Release Drill

`v0.5.0-beta.2` 正式打标签前，应从新 RC 候选执行完整模拟。现有 beta.1 run 仅作流水线已跑至未签名产物的历史证据；不能替代 beta.2 的验证。

验证：

```text
Build
Sign
DMG
Notarization 参数
Verification
Manifest
SHA256
SBOM
```

不允许通过：

```text
关闭验证
ad-hoc 签名
跳过公证
Gatekeeper 绕过
```

解决问题。

---

# 27. REL-6：beta.2 标签核验与创建

`v0.5.0-beta.1` 已存在于本地和远端，均指向 `4bade7715b47d4a71e4a114f835388602f2e865a`，保持不动。`beta.2` 创建前须确认：UI Final Polish 已验收，全部 Release Blocker 已修复，`VERSION=0.5.0-beta.2`，版本一致性和 Release Drill 通过，目标提交已合并到 `main`，工作树干净，且本地和远端都不存在 `v0.5.0-beta.2` 标签。

上述门槛通过后，记录 RC SHA，按仓库发布权限创建、核验并推送 `v0.5.0-beta.2`。推送后标签不可移动；如出现新阻断项，在新提交修复并使用下一版本，而非覆盖该标签。发布凭据可提前配置，但只允许 beta.2 的标签产物进入本轮 Draft Release 与实机验收。

## 27.1 标签核验的两个取值陷阱（2026-09-22 锁定）

`v0.5.0-beta.*` 是 **annotated tag**（`git cat-file -t` 返回 `tag`，非 `commit`）。
**所有 tag → commit 的比较统一使用 `^{commit}` 或 `git rev-list -n1`，禁止比较 tag object。**

```bash
$ git rev-parse v0.5.0-beta.1
8aaf0e28d1d68d1c7001c57e9420ea259b999aa8         # tag 对象 —— 不能用于比较
$ git rev-parse 'v0.5.0-beta.1^{commit}'
4bade7715b47d4a71e4a114f835388602f2e865a         # 真正的 commit
$ git rev-list -n1 v0.5.0-beta.1
4bade7715b47d4a71e4a114f835388602f2e865a         # 等价写法
```

注意 `git tag -l --format='%(objectname)'` 取到的**同样是 tag 对象**。
直接用 `git rev-parse <tag>` 去比 commit，判定将**永远不相等**——这是假失败，不是真问题。

## 27.2 About Commit 的位数：本地构建与 Release 构建不同

| 构建路径 | 取值 | 结果 |
| --- | --- | --- |
| 本地 `make desktop-build` | `Makefile` 的 `git rev-parse --short=12 HEAD` | **12 位**缩写 |
| Release workflow | `COMMIT="${{ github.sha }}"` | **40 位**完整 SHA |

本地构建的 `.app` 若直接与 `git rev-parse HEAD` 做字符串相等，必然失败。

**RC 本地验收采用完整 SHA 路线（已锁定）**，不使用模糊前缀判断：

```bash
COMMIT="$(git rev-parse HEAD)" make desktop-build
```

`Makefile` 中 `COMMIT ?= …` 不会覆盖已定义变量，因此环境变量优先，About 将显示完整 40 位 SHA。

## 27.3 三方严格相等（禁止前缀 / 包含判断）

```bash
RC_SHA="$(git rev-parse main)"
TAG_SHA="$(git rev-parse 'v0.5.0-beta.2^{commit}')"   # 必须带 ^{commit}
test "$RC_SHA" = "$TAG_SHA" || { echo "FAIL: tag 未指向 RC SHA" >&2; exit 1; }
# 另需人工确认：About 中显示的 Commit 与 $RC_SHA 逐字相等（40 位）
```

`scripts/release/check-version-consistency.sh` **只校验 `VERSION` 与 tag 名一致，不比对 commit**，
因此本项是人工 Gate，写法必须一次写对；不要把判据放松成"包含即可"或"忽略 commit"。

> `${{ github.sha }}` 在 push 事件下按 GitHub 文档定义为该 ref 的 tip commit，
> 支持 Release 构建注入 commit SHA；首次 beta.2 真正触发后仍应以 About 实测确认一次。

**以上仅为判据用法，不改动 `Makefile`、`release.yml` 或任何 Release 脚本。**
相关只读核查记录见 `var/reports/rc-freeze-preflight-2026-09-22.md`。

---

# 28. REL-7：GitHub Actions Release

`v0.5.0-beta.2` Tag 应触发：

```text
Verify
  ↓
Build Unsigned .app
  ↓
Sign & Notarize
  ↓
Draft Release
```

逐阶段检查。

---

## Verify

必须确认：

```text
Tag = v + VERSION
frontend-check PASS
public-check PASS
version-check PASS
release scripts tests PASS
go vet PASS
race tests PASS
CLI build PASS
```

---

## Build Unsigned

确认：

```text
darwin/arm64
NDG.app
```

存在。

检查 executable bit。

---

## Sign & Notarize

检查：

```text
Developer ID identity
Team ID
Hardened Runtime
Timestamp
Notarization
Staple
```

任何一个失败：

```text
不得进入公开 Release
```

---

## Draft Release

产物至少应包含：

```text
NDG-0.5.0-beta.2-macos.dmg
SHA256
SBOM
Release Notes
```

---

# 29. QA-1：Clean Install 验证

绝对不能只验证：

```text
本机 build/bin/NDG.app
```

必须验证真正 Release Artifact。清理本机测试版本前，先记录安装位置、配置和数据库路径，做好可恢复备份；不得删除用户现有项目数据。

执行：

```text
GitHub Draft Release
 ↓
下载 DMG
 ↓
删除本地测试版本
 ↓
重新安装
 ↓
首次启动
```

重点验证 Gatekeeper。

正常体验应该是：

```text
双击
安装
正常启动
```

不应该要求用户：

```bash
xattr -cr
```

或者：

```text
仍要打开
```

才能运行。

---

# 30. QA-2：代码签名验证

执行：

```bash
codesign --verify --deep --strict --verbose=2 /Applications/NDG.app
```

检查：

```bash
codesign -dv --verbose=4 /Applications/NDG.app
```

确认：

```text
Authority
TeamIdentifier
Identifier
Runtime Version
```

---

# 31. QA-3：Gatekeeper

执行：

```bash
spctl --assess --type execute --verbose=4 /Applications/NDG.app
```

预期：

```text
accepted
```

---

# 32. QA-4：Notarization / Staple

执行：

```bash
xcrun stapler validate /Applications/NDG.app
```

DMG 同样验证：

```bash
xcrun stapler validate NDG-0.5.0-beta.2-macos.dmg
```

---

# 33. QA-5：真实用户工作流

使用真实挂载 NAS。

必须完整走：

```text
启动 NDG
 ↓
新建项目
 ↓
选择 NAS
 ↓
开始扫描
 ↓
观察进度
 ↓
扫描完成
 ↓
查看重复结果
 ↓
查看目录语境
 ↓
生成治理草案
 ↓
人工复核
 ↓
审批
 ↓
Dry Run
 ↓
Quarantine
 ↓
验证
 ↓
Restore
```

Beta 首发必须至少成功完成一次完整闭环。真实 NAS 验收先以只读扫描和合成/可恢复测试文件进行；隔离、恢复及 Purge 不得作用于唯一真实资料。

---

# 34. QA-6：网络恢复

真实测试：

```text
扫描进行
 ↓
断开 NAS
 ↓
NDG 检测不可用
 ↓
PAUSED_NETWORK
 ↓
重新挂载
 ↓
Resume
 ↓
COMPLETED
```

验证：

```text
不得把断网期间未看到的文件判定为删除。
```

---

# 35. QA-7：Crash Recovery

测试：

```text
执行中
 ↓
强制退出
 ↓
重启 NDG
 ↓
Recovery Lock
 ↓
阻止新的写操作
 ↓
审计与恢复
 ↓
恢复
 ↓
解除锁
```

任何存在歧义状态的情况：

```text
Fail Closed
```

而不是：

```text
自动猜测
```

---

# 36. QA-8：Quarantine / Restore

必须测试：

```text
文件进入隔离区
 ↓
SHA-256 校验
 ↓
Journal 完成
 ↓
Quarantine Item 可见
 ↓
Restore
 ↓
原路径恢复
```

保护文件必须确认：

```text
HOLD
```

状态行为正确。

---

# 37. QA-9：Purge

Beta 首发只使用测试文件验证。

必须测试：

```text
Quarantine
 ↓
Purge Eligibility
 ↓
Purge Plan
 ↓
Approval
 ↓
Typed Confirmation
 ↓
Purge
 ↓
Audit
```

不要使用唯一真实文件验证。

---

# 38. QA-10：隐私测试

检查：

```text
普通日志
错误日志
UI
截图
Crash 数据
Audit
Support 数据
```

确认没有：

* 用户名；
* NAS IP；
* SMB 密码；
* NAS 凭据；
* API Key；
* Apple Credential；
* 真实私人路径；
* 私人文件内容。

---

# 39. Release Notes

建议结构：

```markdown
# NDG v0.5.0-beta.2

这是 NDG 首个公开 Beta。

## 核心能力

- 本地 NAS 与文件系统扫描
- 分层哈希重复检测
- 目录语境判断
- 治理复核
- 安全隔离
- 恢复
- 审计
- 网络断线续扫

## 安全原则

NDG 不会仅因为两个文件内容相同就自动删除其中一个。

扫描、分析和规划阶段保持只读。

普通治理操作优先使用可恢复的隔离机制。

## 当前限制

- 仅支持 Apple Silicon Mac
- 当前仍为 Beta
- 建议第一次使用较小目录
- 重要数据必须保留独立备份

## Privacy

NDG 不包含遥测、第三方云上传或外部 AI 调用。
```

---

# 40. Public Beta 发布条件

只有全部满足才允许从 Draft 变成公开 Release：

本节是 `v0.5.0-beta.2` 发布核对项目清单；实时勾选、证据与阻断原因统一记录在文首“执行状态与证据”，不得在此另建一套进度状态。若 RC 冻结后需要修复并改用后续版本，应同步更新本节及发行说明。

```text
[ ] RC SHA 固定
[ ] VERSION 正确
[ ] Tag 正确
[ ] CI 全绿
[ ] Security 全绿
[ ] DMG 构建成功
[ ] Developer ID 签名通过
[ ] Hardened Runtime
[ ] Apple Notarization
[ ] Staple
[ ] Gatekeeper accepted
[ ] SHA-256
[ ] SBOM
[ ] Release Notes
[ ] Clean Install
[ ] 首次启动
[ ] 新建项目
[ ] 本地扫描
[ ] NAS 扫描
[ ] 断网恢复
[ ] 重复结果
[ ] 治理复核
[ ] Dry Run
[ ] Quarantine
[ ] Restore
[ ] Crash Recovery
[ ] Purge 测试
[ ] UI 最低窗口验证
[ ] 路径脱敏
[ ] README 与实际一致
```

---

# 41. 发布后的第一阶段策略

`v0.5.0-beta.2` 首次公开发布后进入：

```text
Beta Stabilization
```

而不是马上：

```text
v0.6 Feature Development
```

首轮重点观察：

```text
安装失败
Gatekeeper
NAS 挂载
权限
扫描性能
数据库损坏
网络暂停
恢复失败
错误信息难懂
治理误判
UI 认知问题
```

Issue 建议使用：

```text
P0 Data Safety
P1 Release Blocker
P2 Functional Bug
P3 UX
P4 Enhancement
```

---

# 42. Beta 修复策略

如果发现普通 Bug：

```text
v0.5.0-beta.3
```

如果只是文档：

```text
可以单独更新 main 文档
```

如果发现数据安全风险：

```text
立即撤下相关 Release
修复
重新完整走 Release Gate
发布 beta.3 或后续新版本
```

禁止覆盖已经推送的：

```text
v0.5.0-beta.1
v0.5.0-beta.2
```

Tag。

---

# 43. 分支策略

本轮可按实际修复内容使用短期分支；创建前核对现有分支、工作树及远端状态。下列名称仅是示例，不能在已存在分支上覆盖工作：

```text
main
 │
 ├── ui/v0.5-final-polish
 │
 ├── release/v0.5-beta-final
 │
 └── fix/v0.5-*
```

UI Final Polish：

```text
ui/v0.5-final-polish
```

完成后 PR：

```text
UI Final Polish for v0.5 Beta
```

如仍需发行代码修复，可创建：

```text
release/v0.5-beta-final
```

Release 分支不得成为长期分支。

最终所有真实代码必须回到：

```text
main
```

新版本的 Tag 从经验证的：

```text
main
```

创建。现有 `v0.5.0-beta.1` 标签保持原状。

---

# 44. Commit 规范

建议：

```text
feat(ui):
fix(ui):
fix(scan):
fix(recovery):
fix(release):
docs(release):
chore(release):
test(release):
```

禁止：

```text
final fix
update
misc
aaa
test123
```

---

# 45. PR 原则

每个 PR 必须描述：

```text
What
Why
User Impact
Safety Boundary
Validation
Rollback
```

涉及：

```text
Scanner
Executor
Quarantine
Purge
Recovery
Release
```

必须明确说明：

```text
是否改变文件系统写行为
```

---

# 46. 本轮禁止事项

在首个公开 Beta `v0.5.0-beta.2` 发布前禁止：

```text
Wails 2.14+ 迁移
React 大版本迁移
UI Framework 更换
数据库重大迁移
新 AI 模块
新 Agent
新平台
Windows
Linux GUI
自动云同步
账号体系
远程服务
NAS 服务端 Agent
自动永久删除
```

这些全部放到：

```text
Post Beta Roadmap
```

---

# 47. 推荐工程节奏

## Phase A — UI Final Polish

目标：

```text
从工程界面
→
变成 Beta 产品界面
```

完成：

```text
Shell
七页
Empty
Loading
Error
Danger
Onboarding
Design Tokens
```

---

## Phase B — RC Freeze

目标：

```text
不再变化
```

输出：

```text
RC SHA
```

---

## Phase C — Release Infrastructure

目标：

```text
保证 Release Workflow 真能跑
```

检查：

```text
GitHub
Apple
Secrets
Environment
Certificate
API Key
```

---

## Phase D — Release

目标：

```text
产生真正可下载 DMG
```

输出：

```text
DMG
SHA256
SBOM
Draft Release
```

---

## Phase E — Real-world Acceptance

目标：

```text
验证用户拿到的那个 DMG
```

而不是：

```text
开发机里的 build
```

---

## Phase F — Public Beta

正式公开：

```text
v0.5.0-beta.2
```

---

# 48. 最终 Definition of Done

NDG `v0.5.0-beta.2` 首次公开发布的完成标准不是：

```text
代码写完
```

也不是：

```text
CI 绿
```

而是：

> 一个不参与项目开发的人，可以从 GitHub Releases 下载 NDG，在 Apple Silicon Mac 上正常安装，在不理解内部 Go / SQLite / Hash / Journal 机制的情况下完成 NAS 扫描、理解重复结果、完成一次治理复核，并且在执行文件操作之前能够明确理解风险；发生网络中断或应用异常时，系统能够安全暂停或恢复，不产生未经用户批准的数据破坏。

达到这个标准，NDG 才真正完成：

```text
v0.5.0-beta.2
```

---

# 49. TRAE / Agent 执行指令

执行本手册时遵守：

```text
1. 不进行超出本手册范围的功能扩展。

2. 任何涉及文件删除、移动、覆盖、恢复、Purge 的修改，
   必须优先保证 fail-closed。

3. 不因为“代码更漂亮”进行无必要重构。

4. 不修改七域信息架构。

5. 不重新引入 Dashboard。

6. 不升级 Wails，除非当前版本出现明确 Release Blocker。

7. 每个阶段结束必须：
   - test
   - vet
   - frontend check
   - build
   - public boundary check

8. Release Artifact 必须来源于 Git Tag。

9. 不允许手工制作一个与 Tag 不对应的 DMG 作为官方 Release。

10. 不允许用绕过 Gatekeeper 的方式解决签名问题。

11. 不允许把 Apple Secret、NAS Credential 或用户路径写入仓库。

12. 如果发现 Release Blocker：
    先停止发布，
    创建最小修复，
    完整重新跑 Release Gate。

13. 如果只是 Enhancement：
    记录到 Post Beta，
    不进入 v0.5.0-beta.2。
```

---

# 50. 最终路线

整个阶段最终只保留一条主线：

```text
现有 main
   ↓
UI Final Polish
   ↓
全测试
   ↓
合并 main
   ↓
VERSION → 0.5.0-beta.2，确认版本元数据一致
   ↓
RC Freeze
   ↓
Release Preconditions
   ↓
新建 v0.5.0-beta.2 Tag（v0.5.0-beta.1 保持不动）
   ↓
GitHub Actions
   ↓
Signed + Notarized DMG
   ↓
Draft Release
   ↓
Clean Install
   ↓
NAS / Governance / Recovery Acceptance
   ↓
Public Beta
   ↓
Beta Stabilization
```

本阶段完成前，不启动下一轮大型功能开发。

---

# 结论

NDG 当前已经不缺核心功能框架，也不缺桌面 UI 基础。

`v0.5.0-beta.2` 首次公开发布前最重要的事情是：

> **停止继续“造东西”，开始证明现有东西可以作为一个产品安全地交付给真实用户。**

本轮的两个核心工作包分别是：

```text
UI Final Polish
```

和：

```text
Release Execution
```

两者完成以后，NDG 才正式从：

```text
NAS Data Governance 工程
```

进入：

```text
NDG 可公开安装的桌面产品
```
