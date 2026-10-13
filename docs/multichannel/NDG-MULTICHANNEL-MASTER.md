> **2026-10-13 新采样（覆盖上次状态，不覆盖历史证据）**：UX #96 与 G6 #97 已独立复审并正常合并，主干 `c95c55d588c7b9ede71e8931e54c492506080f08` 八项合并后检查及本地 gates/race PASS。#98 完整布局/WAL/hot-journal/兼容页修复已经复审，仍等待独立时间排序修复 #102 后收口；#100 批次历史两项 P2 已修正，保持堆叠 Draft。G6 双架构真实 native/SBOM/漏洞结果与 #101 清单补录见实时台账。用户已授权其现有 fnOS 为设备，仅以独立 disposable 区验证；真实 NAS 数据和现有容器仍保护。UX v2 与 NAS 各端同时推进。

> **较早历史快照（由首行和实时台账取代）：2026-10-13 后续采样**：UX #96/#98 和 G6 #97 的最新代码、测试、历史失败与独立审查容量限制已写入实时台账；三项保持 Draft。原手册以及 MAS/历史隐私状态保留，mock/Go/GUI/正式发行证据不互相替代。

> **2026-10-13 增量执行**：PR #93 Linux/Web 与 #95 fnOS 非可安装研究骨架已合并；仅各自限定证据层通过，完整发行/设备仍未闭合。用户追加 Governance UX v2 开发，保持 P0 治理意图与文件执行分离。当前事实见实时台账，原手册正文不覆盖。

> 入库来源与许可：本手册由账户持有人作为本轮附件提供，并明确授权在本仓库入库、持续执行和维护；本入库版本遵循仓库原始文档贡献的 CC BY 4.0 许可。私人本机附件位置不入库。仅规范化 Markdown 行尾空格，实质正文全部保留。源附件正文 SHA-256：`02ff2b5b24640b52b6ac4a9d58acb1c065102bc118595993e535265d9d2e833a`。
>
> **2026-10-12 实时校准**：下文第 1、5、10、12 节中的 beta.11 Draft / 未授权公开 / 未使用 Mac 待办是附件原始快照，不能代表当前发行事实。beta.11 已于 `2026-10-11T16:01:08Z` 公开为 Pre-release；用户已明确豁免全新 Mac 离线首装并授权该次公开，该项记 `WAIVED_BY_OWNER`，不是 `PASS`。PR #91 已合并为 `584aafeb4cf1bb68cbda3bde2d26698b05555640`。最新门禁与责任人仅以 [RELEASE-STATUS](RELEASE-STATUS.md) 和 [EVIDENCE-INDEX](EVIDENCE-INDEX.md) 为准。历史正文及失败证据保留。后续其他渠道公开与外部提审仍须其专属最终授权。

> **fnOS 政策校准**：2026-10-12 读取[官方测试应用规范](https://developer.fnnas.com/docs/quick-started/test-application/)，手动安装只用于本地测试、不应作为公开分发方式。下文第 8/10 节历史“自安装 Beta”不是现行公开分发许可。官方 fnpack 1.2.3 help/template 的本地验证不证明签名、安装、平台审核或分发权。外部路线须另取平台确认。

# NDG 三端开发、验收与上线总执行手册（Codex 主控版）

> **文档日期**：2026-10-12（项目状态快照；执行时须重新读取实时 GitHub / 本地仓库）
> **执行主体**：Codex（统筹、调度、独立复核、门禁裁定）＋ TRAE / WorkBuddy（授权范围内的主要实现工具）
> **项目**：`FNB2026/nas-data-governance`
> **目标**：从当前真实进度连续推进到：①现有 macOS Developer ID 直装版安全公开；② Mac App Store 商店版提交审核并上线；③ Docker 镜像＋Web GUI 可部署发行；④飞牛 fnOS `.fpk` 应用可安装、升级、运行并申请应用中心上架；⑤形成统一 CI、版本、回滚、维护机制。
> **性质**：**一份完整的长期任务委托，不是一次性 shell 脚本**。Codex 应持续维护进度、自动调度可执行工作，不必向用户逐项征求确认。
> **证据原则**：已完成、源码级 PASS、发行物级 PASS、实际部署 PASS、平台审核通过，彼此不能替代。任何结果须绑定 exact commit / 构建身份 / 测试环境 / 证据。

---

## 0. 给 Codex 的顶层授权与总指令（开始时完整阅读）

你是 NDG 全渠道发行项目的**唯一流程编排者**，不是仅提交研究报告的顾问。请在已授权的工具与操作权限范围内，持续完成本手册的研发、集成、检查、PR、CI、测试发行、审核资料和收尾，直到每条渠道达到可证实的终态，或被一个真实外部条件阻断。

**用户不希望把任务拆成许多次对话。** 允许你在内部把工程拆成里程碑、PR、工作树与证据包，但不要为了重复性日常步骤要求用户决定。遇到普通的实现细节，可自主选择保守、可维护、安全优先的方案；能够修复的问题应自行修复和复验。不能伪称拥有未连接的 Computer Use、IDE、Apple、fnOS 或 NAS 权限；无法访问的操作须明确记为 `BLOCKED/NEEDS_HUMAN_ACTION`，其余可独立推进的工作继续推进。

既有用户授权涵盖日常合规的 push、合并通过门禁的 PR、创建必要分支、执行非破坏性测试。**并不等同于**对任意历史 release 公开、修改真实业务数据、使用付费账户采购、清理磁盘、变更现有 NAS 生产服务、覆盖签名资产的无条件授权。新的正式公开发布须有本手册后文规定的最终发布批准记录；仅满足技术门禁不构成自动公开授权。

### 0.1 绝对边界

1. 遵守仓库 `AGENTS.md`、现行 ADR、发布保护、用户已批准的范围；**冲突时安全限制优先**，不得用本手册覆盖更严格的既有控制。
2. 首次执行先只读侦察：`git status --porcelain=v1`、`git worktree list`、`git branch -vv`、`git remote -v`、main SHA、open PR、CI、release/tag、docs 最新状态；核验工具账户/授权和签名环境。**不要覆盖任何正在使用的工作树**。
3. 不得修改、重打、重签、重传、移动或删除 `v0.5.0-beta.11`、历史 beta 的 tag 和官方发行资产；新版本必须通过新候选完成。
4. 真实 NAS、现有生产 Docker/HA、历史故障 DB、私人路径和共享挂载，默认只读、不停服、不卸载、不扫描、不重建索引、不清理。凡需运行有写入风险的集成测试，使用**隔离的合成数据和独立存储**。没有严格身份和测试范围证明就停止该项。
5. 无权限时不绕过 macOS TCC/Gatekeeper/Sandbox，不禁用安全检查，不修改隐私数据库、不索取或输出私钥。不能为迎合苹果审核添加不真实的 API 使用理由。
6. 禁止默认执行 Purge / 永久删除 / 自动改写源数据；不得把权限错误解释成“文件不存在”，不得以 UI 成功代替数据库/源文件一致性证明。
7. 公共仓库、PR、Actions、附件、镜像层、构建 provenance 和普通日志均不得包含真实路径、文件名、NAS 凭据、Bookmark 数据、token、私有数据库和密钥。
8. 并行工作仅限**独立 worktree / 分支 / 服务 / 测试数据库**。同一工作树任何时刻只能有一个 writer；交接时检查 writer lease 与当前任务所有权，不篡改 lease state 取得权限。
9. 没有必要不要把现有 Wails 桌面整体重写；共享 Go 核心，桌面 GUI 与 Web GUI 各自使用适配器；杜绝三份独立扫描引擎。
10. 始终区别以下状态：`PASS`、`FAIL`、`INCONCLUSIVE`、`BLOCKED`、`NOT RUN`、`WAIVED_BY_OWNER`、`DEFERRED_OUT_OF_SCOPE`。没有新证据不得抬升状态，历史 FAIL 不得被后来的不同场景覆盖。

### 0.2 人工介入最小化规则

只在以下情形向用户发出**合并的一次性行动请求**（列明精确操作、必要性、风险、预期产物），而不是每个 PR 都提问：

- Apple Developer / App Store Connect 需账户持有人接受协议、授权证书或确认 App ID、税务银行及合规信息；正式提交审核/设置对公众上架方式时需要负责人确认外部发布授权。
- fnOS 开发者平台需注册、实名认证、开通应用提交权限或最终对公众发布许可。
- 需要全新 Mac/独立 macOS 测试环境、真实外设、用户可见系统对话框，且自动化无法安全模拟。
- 需要修改既有生产 NAS/HA/SMB、写入真实数据、不可逆操作、付费支出或品牌/许可证决策。
- 法律、商业、隐私或平台合规声明存在不能从仓库事实确认的内容。

缺少某项权限不意味着总项目暂停：切换到其他独立渠道继续推进；在状态台账留下明确 blocker、下一次检查条件和责任人。

---

## 1. 截至 2026-10-12 的真实基线（必须复核而非盲信）

| 事项 | 已知证据及当前状态 | 立即动作 |
|---|---|---|
| 正式直装候选 | `v0.5.0-beta.11`；RC `791f19119542e69b62ee62dd358fed188000837e`；annotated tag 对应同一 RC；Developer ID DMG 签名、公证、校验与正式发行 workflow 已成功，Release **仍 Draft** | 保持资产不可变，沿用已批准的首个 Beta 有限支持合同，完成剩余门禁，另行提交最终公开批准 |
| 直装发行剩余门禁 | `docs/release/beta.11-formal-acceptance-go-no-go-2026-10-11.md` 明示 **NO-GO/BLOCKED**：一台未安装过 beta.11 的 Mac 进行离线首装仍未完成；部分广义隐私/历史恢复不在获批准的首发认证范围 | 单独排程“未用 Mac 离线首装”；不得以本机删除 App/容器冒充新 Mac 证明；可另外做本机隔离复测但必须标注有限证据 |
| MAS Phase 0 | 已合并，状态 CLOSED | 仅回归保护 |
| MAS Phase 1A | 来源授权/Sandbox 能力已合入 main；实际完整运行证据仍 **INCONCLUSIVE** | 补真实可归因的系统权限/Bookmark/GUI 证据 |
| MAS Phase 1B | PR #90 已合并，main 基线 `cf520128e29897707832c936c74bf8ee5ea9081c`；UI 显式重新授权实现已入主干；独立 SMB/macOS 环境缺失，完整运行 **INCONCLUSIVE**；CarbonCore 历史严格日志窗口 **FAIL** | 独立 macOS＋合成 SMB 三窗口定位，补身份变更、恢复锁、取消/重启/Resume 等用例 |
| MAS Phase 1C | [Draft PR #91](https://github.com/FNB2026/nas-data-governance/pull/91)；容器/备份格式设计，`internal/masbackup` 只读闭合备份校验原型，源码自动门禁通过；**未接入 Wails、无正式导入、无 MAS 上传** | exact-head 独立复核、保持 Draft 直到边界明确且 CI 通过；后续开发正式 exporter/importer 前先解决锁与格式来源可信性 |
| MAS 阻断项 | `statfs` 用于挂载身份判断，但 Apple 将其列入 Disk Space Required Reason API；现有用途找不到明显匹配的列举用途 | 优先评估等价合规 API/缩减调用及必要的 Apple 技术咨询；**不可虚填隐私理由** |
| Mac 分发身份 | 现有 Developer ID `com.fnb.ndg`；MAS 设计 ID `com.fnb.ndg.mas` **尚未确认可用/注册** | 保留独立 MAS Bundle ID 和容器；避免两个渠道共写 SQLite，待可签发阶段办理 App ID 与证书/描述文件 |
| Docker | 当前未从 main 发现已合并的正式 Docker/FnOS 发行路径；仓库含 Go 核心、Wails v2、React/TS、SQLite，可复用业务核心 | 新建受控 Linux API 服务和 Web UI 适配层；原有执行安全语义保持不变 |
| 飞牛 FnOS | 可通过官方应用框架、`fnpack` 与 `.fpk`；可评估 Docker 模板包装 Linux 服务＋网页入口 | **先 Docker，后 FnOS FPK**；以 fnOS 真机安装、权限、升级、卸载生命周期验证为准 |

**关键提醒**：旧 Draft PR #56 是 beta.5 的历史文档工作，不能按旧标题或旧执行稿当作当前最终发布事实。工作台的权威时间线以最新 main、beta.11 正式 Go/No-Go、MAS 文档及对应实时 PR 为准。首次执行时检查是否有更新于本快照的证据，然后在台账中写明差异。

### 1.1 当前证据入口

- [项目仓库](https://github.com/FNB2026/nas-data-governance)
- `AGENTS.md`
- `README.md`
- `docs/release/beta.11-formal-acceptance-go-no-go-2026-10-11.md`
- `docs/mas/README.md`
- `docs/mas/phase1a-2026-10-12.md`
- `docs/mas/phase1b-runtime-2026-10-12.md`
- `docs/mas/phase1b-pending-tests-2026-10-12.md`（在 PR #91 分支）
- `docs/mas/phase1c-container-and-compatibility-2026-10-12.md`（在 PR #91 分支）
- `docs/release/NDG-v0.5-Beta-RC-Release-Execution-Manual.md`（包含**历史章节**，不能直接复用旧状态）
- GitHub PR #90 / #91、Tags、Draft Releases、Actions 与 branch ruleset

---

## 2. 技术架构：一套核心，三类发行介质、四个交付入口

```text
                            ┌───────────────────────────────┐
                            │ NDG Go domain core            │
                            │ scan/hash/plan/audit/journal  │
                            │ Quarantine/Restore policies   │
                            └──────────────┬────────────────┘
                                           │
                   ┌───────────────────────┼────────────────────────┐
                   │                       │                        │
           macOS application          Linux daemon               fnOS package
             Wails + React        REST API + Web React         .fpk installer
                   │                       │                        │
        ┌──────────┴─────────┐             │                        │
        │ Developer ID DMG   │        Docker / OCI image  ──────────┘
        │ Mac App Store      │        + compose templates       UI launcher
        │ Sandbox + grants   │        + local volumes           + lifecycle
        └────────────────────┘
```

1. **直装 macOS Developer ID**：现有 Wails app，保留原有签名/公证/手动下载更新路径；首个公开 Beta 仍按现有门禁，而非被 MAS 进度强制阻断。
2. **Mac App Store**：独立 Bundle ID、App Sandbox、最小权限的只读扫描等经批准能力，由 Apple 管理分发/更新。**不要把直装版 DMG 或原 Developer ID 证书当作 MAS 提交产物**。MAS 的原生用户权限与数据容器逻辑与直装版隔离。
3. **通用 Docker 发行**：Go Linux 后台进程 + 同源 React Web UI；局域网浏览器访问，独立用户/会话认证，按明确选择的卷进行治理。**不能**把 Wails macOS `.app` 当作 Linux 容器服务。
4. **FnOS App Center**：`.fpk` 是安装/系统集成层，首选官方支持的 Docker 应用模板包装**同一个经校验 OCI digest**，在 fnOS 应用桌面提供入口和权限/资源声明；如果官方当前政策不允许该类包装，才在同一 Go 核心上建立原生 Linux `.fpk` 运行器，提交 ADR 说明原因。

### 2.1 架构不变量

- 核心业务 API 与平台 I/O 使用接口隔离：`filesystem`、`source_authorizer`、`project_store`、`quarantine_store`、`execution_policy`、`audit`。
- 所有渠道执行统一：只读扫描 → 分析 → 草案 → 审批 → stale 检查 → Dry Run → 可回滚隔离/恢复 → 验证/审计。平台特定权限只会缩小能力，不能绕过核心门禁。
- macOS MAS 首发建议保持**只读治理**；Docker/FnOS 也默认只读，写入功能仅在独立安全门禁、显式挂载和业务 GUI 二次确认后开放。Purge 首发维持禁用。
- 所有容器/主机的 SQLite DB、WAL/SHM 必须位于**本地受控持久化卷**，不要放进可扫描共享根目录，也不要由两个活跃进程共同打开同一 DB。
- 同一版本的算法语义尽量相同；不同平台的文件系统/权限差异应记录在 capability matrix 中，而非静默改变结果解释。
- 前端复用**业务组件、样式和状态模型**，桌面 Wails binding 与 Web HTTP adapter 分离；不得将仅前端隐藏视为安全限制。

### 2.2 建议仓库目录（Codex 可按现状微调）

```text
cmd/
  ndg-desktop/                 # 现有 Wails，不迁走历史入口
  ndg-server/                  # 新增 Linux 服务入口
internal/
  apihttp/                     # Authn/Authz + HTTP DTO + event streaming
  platformaccess/              # 各平台 Source/FS 适配层
  masbackup/                   # 现有试验模块，正式化前不得承诺可导入
cmd/ndg-desktop/frontend/      # 现有前端，提炼共享组件
web/                           # 可选独立构建入口，尽可能复用 frontend
packaging/
  docker/Dockerfile
  docker/compose.yaml
  fnos/<appname>/               # 官方 fnpack 模板 + manifest/cmd/ui/wizard
  mas/                          # MAS-only 构建配置/entitlements/manifest
scripts/
  release/
  packaging/
docs/
  multichannel/                # 本手册与持续证据台账
.github/workflows/             # source gates / Docker / fnOS / MAS
```

**禁止**为遵守该目录样式而执行高风险大规模重构；优先增量适配，代码复用须有测试证明。

---

## 3. 唯一主流程：Codex 自动连续执行的阶段与优先级

这里是**执行顺序**，不是要求用户手动依次派发的碎任务。根据资源可行性并行独立阶段，遵守 writer lease。

| 流水线 | 前置条件 | 主要工作 | 完成标志 |
|---|---|---|---|
| G0 事实冻结与自动工作台 | 当前即可 | 扫描仓库、保护旧资产、建立证据台账、CI 状态、隔离 worktree、风险矩阵 | 可追溯 exact-head 基线；不会碰生产数据 |
| G1 Developer ID Beta 公开门禁 | beta.11 发行物已固定 | 独立 Mac 离线首装；审核有限范围说明；最终发布决策 | 技术门禁 PASS + 明确最终发行授权后 GitHub 发布，或 BLOCKED 有证据 |
| G2 MAS 源码/环境 | #91 审核中 | Phase1A/B 真实场景、CarbonCore 归因、`statfs` 合规修正、容器边界 | 来源授权/SQLite/GUI 测试可归因、隐私理由合法，不存在开放安全 blocker |
| G3 MAS 构建与审核运营 | G2 PASS | 分发签名、打包、上传、TestFlight/独立测试、App Store Connect 信息/审核 | 正式 Apple 审核结果及上架状态可核验；若被拒，反馈进入修复循环 |
| G4 Linux/Docker MVP | G0 + Go 现状调查 | HTTP 服务、Web GUI、Linux FS、Docker/Compose、自动安全测试 | Linux amd64/arm64（通过实测的架构）持久化、重启、扫描、恢复、认证均 PASS |
| G5 FnOS `.fpk` | G4 的 digest 固定 | fnpack 官方模板、应用入口、权限/卷、生命周期、升级卸载 | FnOS 真机安装、升级回归 PASS，FPK 可分发并进入商店审核队列 |
| G6 多渠道维护 | 至少一条可用 | 发布矩阵、变更日志、版本规则、回滚、漏洞与升级维护 | 一份 release manifest 解释每个渠道“何时/以何源码/什么能力”发布 |

**建议调度策略**：先启动 G0；独立环境/Apple 授权尚未具备时，推进 G4；G2 中与 Apple 无关的源码/测试可和 G4 在不同 worktree 并行；G1 的真实首装可作为单独外部待办，不要无谓阻塞 G4/G5。

---

## 4. G0：启动、现状核验、最小管理文件

第一次运行只读取，记录原样返回码和异常。**命令仅作为示例，需先确认所在目录就是本仓库、未被其他 IDE 占用**。

```bash
git rev-parse --show-toplevel
git status --porcelain=v1
git worktree list --porcelain
git branch -vv
git remote -v
git fetch --prune origin
git rev-parse origin/main
git tag -l 'v0.5.0-beta.*'
gh pr list --state open --json number,title,isDraft,headRefName,baseRefName,updatedAt
gh release list --limit 30
gh run list --limit 20
```

检查本地 Apple 工具与 CI 能力可用性：`xcodebuild -version`、`codesign`、`xcrun notarytool`、`xcrun altool`、`xcrun stapler`、`docker`、`docker buildx`、`fnpack`、`gh`。绝不能把 `--help` 成功等价为证书和上架权限有效。

创建或更新（以 PR 提交，避免覆盖旧实时文档）以下文件：

```text
docs/multichannel/NDG-MULTICHANNEL-MASTER.md       # 本手册入库版
docs/multichannel/RELEASE-STATUS.md                 # 各阶段表、blocked、owner、next trigger
docs/multichannel/CAPABILITY-MATRIX.md              # 直装 / MAS / Docker / FnOS 的能力/限制
docs/multichannel/EVIDENCE-INDEX.md                 # exact-head/CI/build/test/archive 索引
docs/multichannel/RISK-REGISTER.md                  # 变更及失败风险
scripts/packaging/check-multichannel-boundaries.sh  # 可选，发现凭据/构建越界
```

每个阶段用 `READY → ACTIVE → REVIEW → PASS / BLOCKED / FAIL` 状态机，记录开始/结束时间、提交 SHA、负责 IDE、证据 hash、CI URLs、残留问题。**不要另建 10 份互相矛盾的“唯一真相”文档**。重复材料仅链接，不复制大段旧结论。

### 4.1 IDE 工具、模型与交接

- Codex：总控、任务分发、Git / PR / CI 收尾、独立核查，优先利用低成本自动化处理机械操作，复杂安全裁定使用高推理档。
- TRAE / WorkBuddy：以拥有实际写入 lease 的一方执行实现；推荐同一长任务内由单 IDE 连续完成，另一方只做只读独立复核或在正式交接后接管。不得双 IDE 同目录同时修改。
- 当 IDE 新对话/窗口打开或策略切换时检查模型/档位/上下文上限；模型类别应注明 `纯文本`/`多模态`，按任务类型选用而非绝对绑定未经证实的模型名称。
- 可用 Computer Use 时，Web GPT 用于架构争议与独立审查；控制 IDE 的自动化首先确认对方已停止写入，上传预览后关闭预览。没有 Computer Use 许可就改用 git/CLI/API 工作流，不假装已点击。
- 状态探测参考：GPT 短轮 1–3 分钟；IDE 执行 5、10、15 分钟阶梯并设置合理上限；避免在高负载运行时高频干扰。仅在已有实际调度器支持时实施，别靠纯文本承诺“后台执行”。

---

## 5. G1：现有 Developer ID macOS Beta 的安全公开

**这个渠道与 MAS 不共享发行产物和验收结论**。

1. 从 beta.11 正式 Go/No-Go 读取已批准的**有限首发支持合同**，保持：受验收功能、有限捕获窗口日志隐私、用户管理的独立源文件＋数据库备份；不承诺全系统长负载隐私与未知历史 Purge 恢复；永久 Purge 禁用，原有 Recovery Lock 行为仍保守。
2. 在**从未运行过 beta.11 的独立 Mac**（或按既有正式验收合同明确认可的等效全新环境）下载官方未修改 DMG；记录来源、SHA、quarantine 属性、DMG staple、首次离线安装和 Gatekeeper 行为、App About 版本/RC、首次启动、安装路径、无联网依赖。**删本机 App 再装不等价于未使用 Mac**。
3. 若无独立设备，明确 `BLOCKED`; 不得自造 PASS。此前同机 sandbox/临时账户测试可辅助，但不替换明确要求的验收。
4. 形成最终候选验收摘要：已通过、批准的排除范围、限制、已知失败、受影响平台与用户安全说明。
5. 如发现涉及源文件误改、数据库损坏、权限绕过、恢复锁绕过或隐私泄漏，立即 `NO-GO`，通过**新 RC/tag** 修复验证，绝不能替换 beta.11 不可变资产。
6. 完成独立验收后提供供负责人一次性批准的 `GO / NO-GO` 材料。**GitHub Release 从 Draft 变公开、对外公告，仅在明确公开授权后执行。** 同时保留 prerelease 标记直到具备正式稳定版证据。
7. 首个公开版下载页说明：Apple Silicon / macOS 13+；未经证实的平台不声明支持；发行说明提供完整 SHA-256、能力边界、备份要求、已知限制、反馈与安全漏洞入口。GitHub 直装的更新路径由开发者管理，而非 Apple Store 自动更新。

**不提前收取费用**，除非有另行明确的商业模式与订单履约、退款、许可证、隐私和平台规则决策；不能为赶上架私自集成支付/联网许可证。

---

## 6. G2：Mac App Store 工程完整路线

### 6.1 从 Draft PR #91 开始

1. 获取 PR #91 exact head/base 和完整 diff；审查 `internal/masbackup` 的 TOCTOU、symlink、跨设备、manifest、schema/hash、SQLite 完整性、cancel、staging 删除失败、原包不变及私人数据边界。独立复现目标测试和 CI；如果符合该 PR 的**原型限定职责**才合并，不能因为 CI 绿宣布导入功能完成。
2. Phase 1A、1B 的真实运行 `INCONCLUSIVE` 和 CarbonCore 历史 `FAIL` 在台账原样保留。不能依赖早期 ad-hoc `.app` 的 About commit 推断后来正式 build 通过。
3. 先在隔离的 Mac/VM＋纯合成 SMB 服务完成可归因验证：进程/签名 hash、bundle ID、端口、挂载端点/真实服务对应、源文件 hash、采集窗口；不触碰宿主机真实 NAS。

### 6.2 必须实现/收口的 MAS 平台约束

**App Sandbox / 来源授权**

- 原生 `NSOpenPanel` 让用户主动选择读目录；需要跨启动持久授权时使用 security-scoped bookmark，后台调用与 StopAccess 必须配对；拒绝 root/folder 外任意扫描和“拿到字符串路径就访问”。
- 重新授权必须是用户主动操作，不能自动继承已改变的 NAS 端点；权限丢失不应记为 missing；恢复源授权不解锁历史 Recovery Lock。
- `network.client`、受限位置等 entitlement 仅在真实功能需要时保留；不能为排障授予 Full Disk Access。
- MAS 禁止执行有风险的文件写入；所有产品 API、CLI 入口、内部 service、恢复/诊断分支均编译/运行时 fail-closed。不能依靠隐藏按钮。

**Project container / 数据迁移**

- 使用独立 `com.fnb.ndg.mas`（仅为**候选设计 ID**），验证在 Apple 账户中合法可用后再注册；MAS 与直装版不能共用 live DB、WAL/SHM、settings 和 bookmarks。
- 跟随 Phase1C 设计：源版在持有项目所有权锁时创建 closed backup（推荐 SQLite 官方 backup/snapshot 机制）；带固定版本/schema/哈希清单，**清单 hash 不是身份签名**；导入只复制到 MAS 私有 staging，校验完整性/ACL/哈希/模式，再迁移 staging DB 并原子入账，原件不变。原产品未有受信 exporter 之前不允许宣传“一键迁移”。
- 导入后产生新 MAS 项目身份，源目录必须重新经选择器授权。未知 schema、缺失清单、旧记录 Recovery Lock 不明、复制中断或 cleanup 失败均拒绝；不得自动解锁或删除不明恢复数据。

**隐私 API 与 App Store 审查**

- 优先消除/替代将 `statfs` 用于挂载身份识别的 Required Reason 风险；Apple Disk Space API 理由须对应真实磁盘空间使用场景。不能虚称展示容量来覆盖身份调用。替代方案须验证 SMB 身份判定、挂载变化、路径越界与同设备特殊情况；没有合规方案则寻求 Apple 指导并标记 blocker。
- 检查 `stat/lstat`、`NSUserDefaults`、Wails/Go/SQLite 及 transitive framework 的 required reason，输出真实 `PrivacyInfo.xcprivacy`，对照 Apple 当前政策审查，绝不能复制与事实不一致的模板。
- 形成“本地不上传数据”主张的具体核查：实际数据流、抓包/目的地址、事件与日志、更新/崩溃机制、第三方 SDK；官网 App Privacy 表、隐私政策、权限说明与二进制行为必须一致。
- 验证 MAS app 为自包含 app、没有另装代码/后台自启动/root 需求，不依赖非商店自更新器；代码及构建工具符合当前审核规则。

### 6.3 真机矩阵（逐项可归因）

| 类别 | 最低实测场景 | 通过判据 |
|---|---|---|
| 授权 | 未授权、授权、撤销、重授、stale bookmark、源移动/替换 | 拒绝越权；状态明确；旧索引不被假 missing；无隐藏写入 |
| SMB 网络 | 合成源扫描中断、同身份重连、端点变化、重授权、断点续扫 | checkpoint 一致；完整 hash 候选覆盖正确；权限与网络暂停区分 |
| Recovery Lock | 有未解决 Journal 的项目重新授权、重启 | 锁保持；无自动清理；只在正常恢复机制下解锁 |
| 生命周期 | 选目录弹窗中关窗口、取消、关闭 App、慢速/阻塞 I/O、再次打开 | scope lease 不提前释放；记录和停止行为真实；不可声称硬中断内核 IO |
| 项目存储 | container ACL、SQLite WAL/SHM、安全升级、备份/导入（如该功能已正式支持） | 独立容器私有、DB 完整、hash 不变、跨版不共享 DB |
| 安全隐私 | NDG/App/WebKit/Apple 进程归属三窗口日志和 stdout/stderr | 明确观测范围；重现历史 CarbonCore FAIL 并定位归属；不误报全系统隐私 PASS |
| GUI | 多窗口、关闭/重启、首次授权、禁用操作提示、错误消息 | 不制造虚假“已扫描”/“可写入”信号；UI 和后台能力一致 |
| 打包 | 真正 MAS 签名/profile/Installer 包/审核提交 | codesign + entitlements + archive identity + Apple 处理结果绑定 exact build |

历史 CarbonCore 严格零标记窗口若仍 FAIL，必须归因并决定是否为无法消除的平台行为；**保留严格 FAIL**，如提出范围豁免只能经明确负责人接受、附实验和审查合规说明。不能把 Apple 系统日志自动记到本 App 头上，也不能据此宣称全系统无痕。

### 6.4 Mac App Store 正式提交链

1. 确认 Apple 组织账号、Team ID、法律主体、App Store Connect 协议/税务/银行（如果涉及收费）、角色、App 名称、Bundle ID 可用性。此前 Developer ID 团队签名证据**不证明** Apple Distribution / Mac Installer Distribution 私钥与 MAS profile 已就绪。
2. 独立 MAS 构建配置、Info.plist、logo、版本号/构建号、sandbox entitlements、Privacy Manifest、签名证书、必要的 provisioning profile。签名及 profile 从账户真实配置派生，**不得在 Git 提交密钥**。
3. 使用当时 Apple 支持的 Xcode/命令行方法对 app 和分发上传包签名；确认 `codesign --verify --deep --strict --verbose=2` 和 `codesign -d --entitlements :-`，核验产物有效 App Sandbox 与 Bundle ID。不要把 DMG 公证流程当作 MAS 上传流程。对非 Xcode 工程按苹果“Creating distribution-signed code”执行手动签名/包化，验证接收格式。
4. 上传前创建 App Store Connect app record；准备简中与必要英文信息：产品描述、关键词、类目、年龄分级、客服网站、隐私政策 URL、截图、权限说明、数据声明、合规/加密申报、审核备注和一个**合成 NAS 演示步骤**。不可声称尚未上线功能。
5. 可先走 TestFlight for macOS（如适用）/受控外部测试。`Xcode/Transporter/altool` 的选用以官方当时可用上传接口为准；上传后必须确认 **Processing→可用构建**，不是 CLI 上传成功就算 Apple 已接受。
6. 验证 app 内网络/沙箱/读数据能力与审核账号环境一致，准备“无真实 NAS 也可审核”的合成演示数据和说明；正式审核前向负责人提交一次性 `REVIEW-SUBMISSION-GO` 摘要。收到明确授权才执行 `Submit for Review`；平台通过后按批准的手动发布/自动发布策略上线。
7. 审核拒绝时保存具体条款、截图、复现与提交编号，进入修复 PR → 全门禁 → 新构建号 → 重传/重审循环；不通过删除安全限制或虚填描述“规避”。
8. **完成定义**：可验证 Apple App Store Connect 的正式上线状态、用户所在商店地区可获取（按获准地区）、安装启动/授权/只读扫描可用；否则记为 `SUBMITTED/IN_REVIEW/REJECTED/BLOCKED`，不能称为“已上线”。

官方依据：[Mac Sandbox](https://developer.apple.com/documentation/xcode/configuring-the-macos-app-sandbox)、[macOS 签名](https://developer.apple.com/documentation/xcode/creating-distribution-signed-code-for-mac)、[Mac Store Profile](https://developer.apple.com/help/account/provisioning-profiles/create-an-app-store-provisioning-profile/)、[App 审核规则](https://developer.apple.com/cn/app-store/review/guidelines/)、[上传构建](https://developer.apple.com/help/app-store-connect/manage-builds/upload-builds/)、[提交审核](https://developer.apple.com/help/app-store-connect/manage-submissions-to-app-review/submit-an-app)、[Required Reason](https://developer.apple.com/documentation/bundleresources/app-privacy-configuration/nsprivacyaccessedapitypes/nsprivacyaccessedapitype)。

---

## 7. G4：Docker / Web GUI 发行（应优先兑现的可独立路线）

### 7.1 最小可用产品范围

首个 Docker Beta 只包含经实测可移植的安全功能：项目管理、源目录显式映射、只读扫描与暂停/恢复、分层哈希、重复结果与目录语境、审计查询、诊断级健康检查、Web GUI。Quarantine/Restore 一旦纳入必须另开**写权限卷和显式的审批/隔离/回滚安全门禁**，没有这套真机证据就保持禁用并在 GUI 说明。永久 Purge 一律不进入首发。

不做：未经许可远程开放公网登录、绕过身份认证、装入 NAS root 文件系统、自动扫描整个 `/volume*`、全盘增量删除、绑定 Docker socket、默认挂载 `/`、保留可通过 API 强制跳过审批的 debug 路由。

### 7.2 Linux API 与 Web 前端

- 新增 `cmd/ndg-server`；通过 Go 现有领域 service 执行扫描与计划，不复制算法。Wails 特定绑定抽象成 service interface。HTTP API 必须进行业务权限校验和字段脱敏，不将 Go struct 原样 JSON 序列化给 Web。
- 提供 `/healthz`（仅进程状态）、`/readyz`（数据库与迁移状态）、版本/构建身份接口，以及 projects、sources、scans、results、plans、audit 的受控路由；`DELETE`/执行类 API 默认不可用。
- 首次访问使用本地安全初始化流程。默认强随机凭据或一次性本地初始化令牌，并要求用户改密；不要公开内置密码。认证采用安全会话 Cookie（HttpOnly、SameSite、HTTPS 场景 Secure）或同等级机制；所有状态改变接口考虑 CSRF；请求限速、idle timeout、origin/host 校验、防止 SSRF 与 DNS rebinding；WebSocket/SSE 也要鉴权和并发控制。
- 默认仅回环监听（或 Docker 网络内网），通过明确配置才绑定宿主网段；推荐由用户自有反向代理提供 TLS；若需手机访问，也不能默认公网暴露。
- Web UI 从现有 React 组件中抽可复用部分；新增 typed HTTP adapter，统一错误处理、网络暂停、扫描长轮询/SSE 和无权限提示。不能向浏览器发送主机绝对路径、凭据或 Bookmark；向使用者呈现的路径信息必须遵循产品已定义的必要性和脱敏边界。
- API 请求发起者不能自由填写未授权宿主机路径；只能使用管理员在**服务器端**预定义的安全扫描根别名（例如 `source_a`），并验证 `openat`/文件句柄层根约束、符号链接、挂载越界、TOCTOU、设备/ inode 身份。真实 Linux mount 行为与 macOS 不应直接共享未经验证的假设。
- 数据库单 writer；进程重启/升级前正确 drain jobs、checkpoint/恢复锁，schema 迁移必须可回滚或提供验证备份；DB 存于单独的持久卷，配置和权限 `0700/0600`，拒绝网络盘上的活 WAL。

### 7.3 Dockerfile / Compose / 安全默认值

- Multi-stage：固定 Go/Node 版本及 base image digest，CI 锁依赖；编译静态/匹配 libc 的 Linux 二进制与前端静态资源；运行镜像不带构建工具、源码、密钥；OCI labels 写入版本/SHA/许可证与主页。
- 默认非 root UID/GID，必要时提供 NAS 主机 UID/GID 映射；`read_only: true` rootfs（验证 Go/SQLite 临时需求后挂明确 tmpfs/状态卷），`cap_drop: [ALL]`、`no-new-privileges`、合理 CPU/内存/PID 限制；不开放 `--privileged`、host network、`/var/run/docker.sock`。
- Compose 默认 `/data` 数据卷 R/W（仅 DB/缓存/应用内部状态）、`/sources/...` 源卷 `:ro`、`/quarantine` **不映射**。若将来写模式经安全门禁通过，再通过新的 profile/独立卷明确开启。
- 初始镜像必须附 `.env.example` 与随机凭据初始化流程；配置不应进入镜像层或构建参数。镜像发布到 GHCR（前提是 registry 权限已具备）；必要时追加 Docker Hub，但不要强制另建账号。
- `linux/amd64` 和 `linux/arm64` 分别运行真实架构或可信仿真集成测试。FnOS 设备实际 CPU 架构单独核对，不能只看打包标签。
- 输出 SHA/digest/SBOM/provenance/漏洞扫描；任何含 secrets 的 build args 会污染 provenance。用 BuildKit secret mounts / Actions secrets。

**安全 Compose 结构示意（非可直接运行的最终文件；Codex 应根据实现生成）：**

```yaml
services:
  ndg:
    image: ghcr.io/fnb2026/ndg:<VERIFIED_DIGEST_OR_VERSION>
    restart: unless-stopped
    user: "1000:1000"                 # 按目标 fnOS/NAS 权限实际调整
    read_only: true
    cap_drop: ["ALL"]
    security_opt: ["no-new-privileges:true"]
    ports:
      - "127.0.0.1:8080:8080"         # LAN 访问需显式调配代理和认证
    volumes:
      - ./ndg-data:/data:rw
      - /path/to/approved-source:/sources/source_a:ro
    tmpfs:
      - /tmp:rw,nosuid,nodev,size=128m
```

### 7.4 真实验收矩阵

- 两架构容器启动、`healthz/readyz`、Web 浏览器首次初始化/退出登录、会话过期、错误凭据节流、CSRF、鉴权未通过无法启动扫描。
- 不存在的数据源、非授权路径、`../`、symlink、mountpoint、换 inode/设备、容器内映射变化；确认不能逃逸批准根。
- 只读源真实 `:ro`，即使 API 被直接调用也不能写；宿主文件 before/after hashes 全等。
- 合成重复文件组覆盖 quick hash / full SHA，硬链接/目录语境，人工草案与审计；“扫描完成”与 full hash coverage 区分显示。
- 断开只用于测试的 mount/service，再恢复；断点续扫不中途变 missing；网络/权限区别，完整候选集合修复后不丢失；测试中断进程、重启、迁移失败回滚。
- 数据卷备份/恢复、数据升级与降级策略、SQLite integrity_check；同一状态目录多容器同时启动必须拒绝第二 writer。
- 登录日志、反向代理日志、普通 stdout/stderr、请求/响应日志无真实路径/令牌、可疑上传默认关闭；发布镜像秘密扫描 PASS。
- 首发 write-disabled 策略通过 API 直调用负面测试证明，不能只是 UI 禁用。

完成后提供最少：`Dockerfile`、`compose.yaml`、`.env.example`、用户安装/卸载/升级/备份文档、SBOM、安全报告、CI 工作流、至少一次本地/真 NAS **独立测试目录**的运行证据。测试宿主中其他容器（尤其现有 Home Assistant、巴法云等）绝不可中断。

---

## 8. G5：飞牛 FnOS `.fpk` 应用包装、测试、上架

### 8.1 分发路线的优先级

**优选**：使用当前官方 `fnpack create <appname> --template docker` 模板，将 G4 经验证的 Docker 镜像以固定 digest 作为运行负载，补 fnOS 应用清单、权限、向导、启动/停止/健康检查、桌面 GUI URL 入口、升级卸载生命周期。用户在 FnOS 应用中心安装的是 `.fpk`，实际计算工作由受控 Docker 服务完成。**FnOS 原生体验 ≠ 必须重写后端**。

**备选**：若 fnOS 正式审核/权限模型不接受 Docker 模板或用户必须离线安装镜像，则做原生 Linux 二进制 `.fpk`，共享 Go `ndg-server` 和 Web 静态资产。选择需提交 ADR，写清升级、供应链、安全与运维代价；不能两条路线同时大规模投入。

### 8.2 FPK 工程与权限

1. 运行前核对 fnOS 当前正式开发规范、官网与 `fnpack --help` 的命令、字段/模板及应用中心审核条款，不照抄过期社区示例。官方入口：[fnOS Developer](https://developer.fnnas.com/docs/guide)；镜像文档可参考 [fnOS 文档同步仓库](https://github.com/ckcoding/fnnas-docs) 但不是官方实时审核答复。
2. 以官方模板为基线生成 `manifest`（名称、版本、描述、维护者、架构、兼容最低系统版本、升级信息）、`cmd/` 生命周期脚本、`app/ui/config` GUI 启动入口、`config/` 权限与资源文件、`wizard/` 安装向导（路径/端口/持久化设置）。不得猜字段名或 API 名；实际按本机 fnpack/官方文档校准。
3. 应用运行用户采用最小权限；安装时让用户明确指定**只读扫描路径**和私有数据目录。不能默认访问全盘/所有共享目录；向导必须说明权限及数据风险；端口冲突、镜像拉取失败、权限不足应能诊断和安全回滚。
4. `start`/`stop`/`status`/`upgrade`/`uninstall`：正常停止应 drain scan、持久化 checkpoint、正确关闭 DB；升级前做安全数据库备份并校验兼容性；卸载明确区分卸载应用与删除项目数据库、缓存/隔离文件，绝不能静默清除业务数据。
5. Web GUI 入口可由 fnOS 应用桌面图标打开，须在 fnOS 的授权网关与端口模型里真实测试；不能由于“fnOS 登录过”就假设后端 API 已鉴权。检查 CSRF、会话、反代 Host、端口跨网段暴露以及 NAS 共享目录 ACL。
6. 构建：`fnpack build` 形成 `.fpk`；签名/提交要求遵照官方平台实时规则；产物 hash 与内嵌镜像 digest、源 commit 绑定。若 `.fpk` 安装后会从外部拉镜像，提供明确离线限制及失败恢复；不得声称离线可用。

### 8.3 FnOS 设备验收必须覆盖

- 在**隔离的测试 App 名称/端口/目录/卷**安装 `.fpk`，确认 GUI 可达、Docker 状态正确、容器不是 root/privileged，合成样本 100% 保持只读。
- FnOS 重启、App Center stop/start、镜像不可用/下载失败、数据卷迁移、NAS 空间不足、用户名权限变更、应用升级同一 DB、卸载重装保留数据、明确选择删除数据的行为。
- x86/ARM 只声明已实测的架构；在用户当前 FnOS 真实 NAS 上的验证**不得**停止现有 HA、其他容器、现有挂载或服务，只操作独立测试命名空间。
- 若应用中心开发者权限可用，按平台流程提交图标、截图、说明、隐私信息、演示、支持和安全报告；跟踪“已提交 / 审核中 / 退回 / 已上架”真实状态。获得最终发布授权前不将试验品宣传为正式商店版。

**完成判据**：用户通过 `.fpk` 安装，能从 fnOS 应用入口打开 Web GUI，在授权的测试路径只读扫描、停止/恢复、升级和卸载不丢数据，且能核验应用中心的最终上架状态。若平台未通过审核，只能写“FPK 测试版可安装/已提交”，不能写“应用中心已上线”。

---

## 9. G6：安全 CI、分发流水线与版本/升级策略

### 9.1 分渠道 CI

建议将以下门禁分成复用 workflow，不因项目大就每次重新全量构建全部渠道，但**发布候选**必须重新通过其完整所属门禁。

| 门禁 | 触发 | 最低要求 |
|---|---|---|
| Core Gate | 所有业务代码 PR | `go test ./...`、关键 `-race`、`go vet`、lint、数据库迁移、公共边界与 secret 检查 |
| Frontend Gate | GUI/DTO 变化 | 类型检查、单元测试、生产构建、Wails/Web adapter 契约、安全/可达性 |
| MAS Gate | macOS 沙箱/打包变更 | Darwin `-tags mas` 目标测试、Xcode/entitlements/PrivacyInfo 审核、真实 Sandbox 证据（发行前） |
| Docker Gate | Linux/API/镜像变更 | 两架构镜像、容器/Compose 集成、安全默认值、SBOM/provenance/漏洞扫描 |
| FnOS Gate | FPK 变更 | fnpack 静态校验、manifest/脚本检查、真机升级/生命周期签收 |
| Release Gate | 发布 tag / 明确批准的候选 | exact-head、所有必要 gate、构建/签名/digest/审计记录、人工终局审批、回滚预案 |

CI 权限最小化：只在需要推镜像时给 `packages:write`，需要 provenance 签名时再考虑 `id-token:write`；固定 Actions 到可信版本或 SHA，避免从不可信 PR 运行含密钥的发布工作流。Registry 权限应分离 PR 验证与生产 push。

### 9.2 发布身份与兼容性

- 采用**核心代码版本统一、各渠道发行版本独立**的策略；Mac beta.11 已冻结，Docker 首版和 FnOS 首版不强制冒充 beta.11 同一构建。所有正式产物记录 `source_sha`、`core_version`、`channel`、`channel_version`、`build_number`、`artifact_sha256/digest`、`database_schema`、`capabilities`。
- MAS 与 Developer ID 可各自有 Bundle ID、签名、发行序列；MAS 构建号单调递增且与 App Store Connect 记录一致；同一版可有不同平台功能开关，但用户说明应明确。
- Docker 镜像按不可变 digest 发布，tag `:edge` / `:beta` / `:stable` 只能指向经批准产物；默认文档推荐 digest pin，避免 `latest` 漂移。
- FnOS 包必须明确绑定 OCI image digest 或内嵌二进制 SHA；升级时同时验证 schema 可用、配置兼容、数据卷不被重建。
- 正式发布之后修复安全/兼容问题应创建新版本，不复写既有 Git tag、DMG、`.fpk` 或现有镜像 digest。

### 9.3 更新、回滚与备份

- **GitHub DMG**：由开发者维护下载/更新提示和安装校验；自动静默替换 App、拉取额外代码不得被错用在 MAS 渠道。
- **Mac App Store**：由 App Store 分发与更新；本地 DB 升级在 App 第一次打开时安全迁移，保留必要备份/回退信息。
- **Docker**：固定旧镜像 digest，升级先备份本地 DB、记录 schema 与卷目录；失败时恢复匹配 DB 快照再切回旧 digest。不得假定新 schema 能直接被旧进程打开。
- **FnOS**：通过包版本升级脚本管控镜像与 DB，停止服务、备份、验证、升级、健康检查；失败自动停止并提示恢复，绝不随意 rollback DB 而覆盖新增用户操作。
- **任一渠道**：原始 NAS 文件与数据库需要独立备份；“数据库备份”不等于“源文件已备份”，Quarantine 与 Restore 的安全约束不能被自动更新破坏。

---

## 10. 明确的终局发布条件（DoD / Go-No-Go）

| 渠道 | 技术上可发布 | 可宣称正式上线 |
|---|---|---|
| Developer ID DMG | frozen build PASS、受批准功能/隐私范围、**新 Mac 离线首装 PASS**、安全问题清零或正式批准例外 | 负责人独立批准公开 + GitHub Draft 转公开 + 链接/校验/安装实测 |
| Mac App Store | MAS Sandbox 和真实环境 PASS、Required Reason 合规、App record/签名/profile/包和文案合规、必要权限批准 | Apple 实际审核通过并在批准地区公开可获取；仅“上传成功”不算 |
| Docker | 合成/独立 NAS 容器验收 PASS、鉴权/只读默认/持久卷/升级/隐私/漏洞 Gate PASS | 负责人批准公开 + 已发布不可变镜像 digest + 安装文档 + 公开可拉取验证 |
| FnOS | 上游 Docker digest 固定、FPK 真机 install/upgrade/uninstall/GUI/权限验收 PASS | 负责人批准 + 飞牛应用中心审核通过/正式展示（若仅自安装则明确称自安装 Beta） |

**可并行的剩余工作不会因为一条渠道 BLOCKED 而全部停止**。如果项目被外部依赖卡住，Codex 应交付尽可能多的已验证可用成果，准确报告 blocker，而不能用虚构 PASS 或无条件无限循环掩盖失败。

### 10.1 回退触发器

- 任何源文件内容被非预期修改、恢复锁丢失、越权写入、目录根逃逸、DB 不一致 → 立即停止有风险测试与发布，把状态标 `FAIL / SECURITY_BLOCKER`，保留现场/证据，禁止自动清理。
- 签名、App Sandbox entitlement 与预期不符，Apple 隐私声明不能自洽 → MAS NO-GO；不迁就审核要求偷换技术事实。
- 镜像/FPK 发现默认开放公网或默认 root/privileged、不安全凭据、升级丢数据 → Docker/FnOS NO-GO。
- 发现证据只对应不同 commit、不同 app/镜像、不同数据库 → 标 `INCONCLUSIVE` 并在同一正式产物重测。
- 发布失败时只回退**测试环境/已批准的新发行通道**；不得触碰 beta.11 历史资产或用户生产实例。

---

## 11. Codex 的连续执行协议（避免频繁询问）

每次启动/唤醒时循环运行：

```text
1. READ  -> 读 RELEASE-STATUS / EVIDENCE-INDEX / AGENTS / 实际 GitHub、worktrees、lease
2. SELECT -> 从未阻断路径中选择优先级最高、依赖完整的工作包
3. CLAIM -> 原子取得独占 writer lease，写明仓库/工作树/任务/到期与归属
4. EXECUTE -> 委托 IDE 或亲自实现；持续保存 patch、测试回执与身份
5. VERIFY -> 独立复核 diff、负面测试、确切 CI、仓库保密边界和运行证据
6. LAND -> 满足现行保护规则时允许 push / merge；失败先修复或拒绝合并
7. RECORD -> 更新一份统一台账、风险表、支持矩阵和下一步
8. RELEASE? -> 对正式渠道运行独立 Go/No-Go；需要外部批准时请求一次性授权
9. CONTINUE -> 切换到其他未阻断工作包，直至达成终态或只能等待外部条件
```

**GitHub main 保护优先**：不使用管理员 bypass 来规避不应绕过的审查、CI 或 branch ruleset；历史 beta.11 标签创建曾用管理员 override 是历史记录，不是新工作的通用特权。独立 AI 复核不能冒充 GitHub 人类 Approve；如 ruleset 要求人类 Review，必须等待真实授权者。

### 11.1 失败重试

- CI lint/type/test：先定位具体测试失败，修复后以新的 exact SHA 重跑；不得“刷绿”地跳过/删除失败用例。
- macOS 签名/上传：核验证书、Profile、entitlements、有效账号角色、构建版本；密钥问题交外部审批；不在公共 Actions 中打印 credential。
- Docker 失败：重建固定 base/image digest 的可复现最小例子；必要时缩小到先只读 Linux/amd64，但 **不能**把未测 arm64 标 PASS。
- FnOS 工具不匹配：优先官方实际 SDK 文档/模板，不为了“让命令成功”猜 Manifest 字段。
- 真实运行环境不可隔离：保留 BLOCKED，并推进能在安全沙箱独立完成的其它渠道。

### 11.2 汇报频率与格式

每完成一个**有用户意义的里程碑**或遇到需要用户动作的硬阻断再汇报，不按每次测试/PR 报流水账。消息包含：

```text
NDG Multi-Channel Milestone Report
报告时间：
当前 main / 关键 PR / Exact SHA：
Developer ID：PASS | FAIL | BLOCKED（证据链接）
Mac App Store：PASS | INCONCLUSIVE | BLOCKED（证据链接）
Docker：PASS | ACTIVE | BLOCKED（镜像 digest / 证据）
FnOS：PASS | ACTIVE | BLOCKED（FPK SHA / 证据）
本轮实际合入：
独立复核：
未解决安全问题：
需要负责人唯一行动（若有）：
下一批无需用户操作的工作：
```

不使用“已全部完成”这种总括，除非四条路线各自有真实满足 DoD 的终态。平台审核不在我们控制内，应把其实际状态如实保留。

---

## 12. 一份面向用户的集中授权清单（只在实际需要时发起）

Codex 将外部依赖汇总为一张清单，优先只让用户处理**不能自动化**的事情：

1. **一台未运行过 beta.11 的 Mac**，用于既有直装候选首装验收；可由用户亲自操作，Codex 给最少的图形步骤及证据模板。
2. Apple 组织账号中是否可创建 **`com.fnb.ndg.mas`** App ID、Apple Distribution/Mac Installer Distribution 签名能力、Mac App Store Connect 记录与必需协议；按真实账户状态逐条办理。任何税务、结算、收费决策由用户确认。
3. 飞牛开发者平台资格及正式应用提审权限（如必须实名认证或同意协议，由用户完成）。
4. 真 FnOS 的**隔离测试安装区**，明确可操作的合成数据根、卷、测试端口、容器命名空间；未经明确划定绝不修改现有 HA 等系统。
5. **最终外部公开发行批准**：按四渠道分别提交简短风险/证据清单，在用户批准相应渠道后才执行公众可见发布或平台正式送审。

这些不是启动前必须回答的五道题。Codex 应先完成不依赖这些条件的源码、CI、打包和可隔离测试，接近真正门槛时再合并请求。

---

## 13. 官方参考与持续复核

- Apple：[App Review Guidelines](https://developer.apple.com/cn/app-store/review/guidelines/)
- Apple：[Prepare your Mac app](https://developer.apple.com/documentation/xcode/preparing-your-app-for-distribution)
- Apple：[Sandbox](https://developer.apple.com/documentation/xcode/configuring-the-macos-app-sandbox)
- Apple：[Required Reason APIs](https://developer.apple.com/documentation/bundleresources/app-privacy-configuration/nsprivacyaccessedapitypes/nsprivacyaccessedapitype)
- Apple：[Mac App Store profile](https://developer.apple.com/help/account/provisioning-profiles/create-an-app-store-provisioning-profile/)
- Apple：[Upload builds](https://developer.apple.com/help/app-store-connect/manage-builds/upload-builds/)
- Apple：[Submit to review](https://developer.apple.com/help/app-store-connect/manage-submissions-to-app-review/submit-an-app)
- Docker：[Build via GitHub Actions](https://docs.docker.com/build/ci/github-actions/)
- Docker：[Multi-platform](https://docs.docker.com/build/ci/github-actions/multi-platform/)
- Docker：[SBOM and provenance](https://docs.docker.com/build/ci/github-actions/attestations/)
- 飞牛：[官方开发者文档入口](https://developer.fnnas.com/docs/guide)
- 飞牛：[同步文档，辅助检索但须与官方最新规范核对](https://github.com/ckcoding/fnnas-docs)

开发/审核过程如有新政策、新 API、新审查反馈，更新 `RISK-REGISTER` 和渠道实施细则，不偷偷修改本手册的安全底线。

---

## 14. 最终交付物清单（给 Codex 的结束条件）

**代码与分发**：经合并的业务实现 PR、现有签名 DMG 正式发布记录（达到 G1 时）、MAS 商店上传并审核/上架记录（达到 G3 时）、可拉取的 Docker OCI image digest、可安装的 FnOS `.fpk` SHA-256、各渠道源码/构建 hash 对照表。

**质量与安全**：Go / frontend / macOS sandbox / Linux / Docker / FnOS 测试结果，恢复锁、权限负面测试、合成 NAS 断网/恢复证据，Gitleaks/Govulncheck/漏洞检测、SBOM/provenance、签名及合规记录。明确保存 INCONCLUSIVE/FAIL/批准排除范围，不覆盖失败历史。

**产品文档**：用户安装/升级/备份/恢复/卸载文档、MAS 截图/隐私政策/审核资料、Docker Compose 模板、FnOS GUI/向导/应用中心资料、功能差异矩阵、变更日志、发布决策/回滚手册、统一状态台账。

**收尾检查**：原有项目、历史 DB、beta.11 tag 和四类正式资产未被篡改；当前 main 工作树和各临时 worktree 归属清晰；无未经授权的生产写入；公开发布状态与 GitHub/Apple/fnOS/Registry 实际一致。

> **最终指令**：从 G0 开始，不等待用户逐项分配工作。对权限范围内能安全推进的事项持续推进；每一次提审/对外发布尊重渠道级最终授权；每一个无法完成的环节提供精确 blocker 和可执行的唯一人工步骤。目标是把 NDG 真正做成可安装、可升级、可恢复且不夸大安全能力的跨平台产品，而不是只输出计划文件。
