# NDG 多渠道实时执行台账

## 2026-10-16 公开发行及下一目标（当前有效）

- **macOS**：[v0.5.0-beta.11](https://github.com/FNB2026/nas-data-governance/releases/tag/v0.5.0-beta.11) 已公开；具有经审批的有限 Quarantine/Restore，Purge 当前禁用。新 Governance UX v2 正向源核验仍 [#111 BLOCKED](https://github.com/FNB2026/nas-data-governance/issues/111)，[#112](https://github.com/FNB2026/nas-data-governance/pull/112) 经独立限定复审和八项 CI 后已合并为 `65b49a8`；开发 profile 与研究记录不认证完整 B-2 或 #111。
- **Linux Docker**：[v0.5.0-docker-preview.1](https://github.com/FNB2026/nas-data-governance/releases/tag/v0.5.0-docker-preview.1) 已公开；GHCR 固定 OCI index `sha256:49dada73f1eb9cc74485ed5d0b92023d7cb4c3df462d03e105f00158b91668e1`。仅本地文件只读扫描/断点/有限聚合 Web，不含 Quarantine/Restore/Purge。
- **fnOS**：独立只读 HTTPS 开发候选已在 AMD64 实机完成有限 Chrome 登录／扫描／退出、服务及核心重启、权限与 TLS／Cookie／CSRF 验收，详见 [真实记录](../development/FNOS-CGI-ENTRY-2026-10-15.md)。仍为实验性，完整安装／升级、中途断点恢复、手机及 NAS 整机重启未认证；代理客户端需为局域网域名设置直连。公开 Preview.1 原 SSH 入口的 BLOCKED 证据及发行资产保留，尚无完整 fnOS 发行。
- **当前执行顺序**：fnOS 正常浏览器安装/访问优先，统一治理核心与 Docker 完整版、Mac 后续候选分渠道推进；FPK 不阻挡 Compose。新写入能力的公开发行需另行集中批准。
- **用户当前下一目标**：桌面与 Docker 均实现有完整权限和审计保障的 Quarantine → Restore → 经单独高风险授权的 Purge；同时更新 README、链接、图标和 GitHub 仓库 metadata。两个端的写入验收分别完成；不得将开发中能力宣传为已发布。
- 历史发行/安全 FAIL、INCONCLUSIVE 和旧快照保留在下文；此节代表当前产品入口状态。GitHub 正式 Tag、资产和已发布镜像均不可原地改写。


## 2026-10-15 Fast Release Track（历史执行快照）

按所有者最新指令，采用独立渠道最小发行：Docker Read-only Public Preview →
fnOS Docker → Mac 后续 Beta。下方 2026-10-14 的 B→C→D 串行门禁已被本节取代，
历史 FAIL / BLOCKED / NOT RUN 原样保留。#111、完整 25 项 UX、App Store、原生 FPK
和非开发者全面走查不再阻挡独立 Docker 只读预览。

当前基线 main `e55484ee6c95a73290a2182b66b977ddf4d61dd5`；Mac beta.11 资产不变。
Docker 候选使用独立 `packaging/docker/PREVIEW_VERSION`；原生 CI 仅提供构建、
SBOM 和漏洞证据，独立 Linux 主机 HTTP、断点、重启、持久化、备份恢复及源 SHA-256
实测另记。公开镜像和 GitHub Release 仍须完成候选后集中取得一次对外授权。

安装边界和证据清单见 [Docker 只读预览](DOCKER-READONLY-PREVIEW.md)。

本轮固定候选源码 `1db1e4ab9a1287d3f7f9b801b471b00bfc7c7d1e`：双架构构建、
SBOM/漏洞扫描、独立 ARM64 Linux 4,000 文件生命周期/备份恢复、真实浏览器主流程，
以及 fnOS AMD64 Docker/Compose 7 文件安装/重建/保留/卸载的限定实测已通过。
[真实验收记录](DOCKER-PREVIEW-RESULTS-2026-10-15.md) 保留较早镜像撤换、驱动失败和
favicon 401。fnOS 现有 sshd 禁止 TCP 转发且真实连接被拒，客户端入口 BLOCKED，
平台仍为实验性，不将宿主 PASS 扩写为完整支持。当前只准备 Draft 候选，注册表/Release 未公开；Mac #111、#112 的独立复审
与下一签名 Beta 仍按各自门禁处理，不成为此 Docker 候选的前置项。



## 2026-10-14 Phase B 收口顺序修订

按所有者最新指令，Phase B 分为 B-1（Issue #109 原因分类和安全重新核验）、
B-2（T01–T10 核心 / T11–T18 异常 / T19–T25 体验）、B-3（UX v2 RC-1 条件）。
当前 Mac 拒绝的已证原因是此版本尚未实现所需挂载边界证明，发生在源读取及
授权检查之前，不能解释为 inode、Bookmark、卷 UUID、权限或 NAS 故障。
B-1 按最新“PASS / 明确拒绝”契约，在独立复核、exact CI 和 post-merge
通过后可关闭 #109 的分类闭环；原 Mac 正常核验缺口转由 #111 持续追踪，
保持 BLOCKED，不将分类闭合记作 Mac Source Verification PASS。
RC-1 要求 T01–T25、Mac Source Verification、GUI、非开发者和最终 exact CI
均闭合。Docker/fnOS/MAS 在之后各自验收，当前不跨渠道放行。

## 后续执行优先级：验收收口

用户新顺序：**Phase A 合并与门禁 → Phase B UX v2 完整验收 → Phase C
Docker disposable host 实测 → Phase D fnOS 首次安装实验 → Phase E App Store
重新评估**。不新增无关功能，只处理验收中确证的阻断缺陷。

Phase A 已取得 main `5b802233c2a33a61fb57587d92724e70836400a1` 的本地
required 与 CI/Security/Linux 八项成功回执，#105/#106/#107 均已正常合并。
Phase B 已开始 bounded store 基线与 T01–T25 分层盘点，目前 **BLOCKED**：
Darwin 新核验无等价挂载证明、正式 GUI 和非开发者走查未认证。
详见 [Phase B 验收台账](../ux/UX-V2-ACCEPTANCE.md)。
在 B 关闭前不执行 C/D 新现场试验；可只读登记依赖。不以代码合并、离线模型
或 SQL 基线升级设备/签名/用户验收。MAS NO-GO、历史 CarbonCore FAIL、
原 fnOS LIMITED BLOCKED、beta.11 不可变资产均保留。

更新时间：2026-10-14。当前证据基线：main `20c545b60d88a2ea67beef5be1766cc97ec152d0`；较早快照按下方日期完整保留。这是时间绑定的快照；每轮先重新读取远端 main、exact HEAD、工作树和 lease。产品状态与代码合并、测试层、发行物层、运行层分别记录。

## 2026-10-14 后续执行快照（优先于下方历史记录）

本节证据基线为 main `20c545b60d88a2ea67beef5be1766cc97ec152d0`。
仅记录已经取得的源码、自动化与合并证据，不升级设备或发行验收。

| 工作 | 实际状态 | 保留限制 |
|---|---|---|
| 台账 #104 | 已合并 `af26c584c2b01e2e2261ef6003fe8aa8f837dd83`；独立复核及 exact main 本地/远端门禁 PASS | 文档合并不代表新运行场景通过 |
| NAS namespace #106 | 已正常 squash 合并 `b686cfe4dabf0f8eaa182c598e34f092474f415d`；final `ab1549e609b8955a55f2cfa129f92e0443e1a17f` 五文件独立复核、19 项离线测试及八项 CI SUCCESS | 只有离线约束模型，无真实 kernel、HTTP 或清理后端；设备仍 LIMITED BLOCKED |
| UX 源新鲜度 #105 | final `0e0705b76fe9291459bde9ccc5564326f16d22a5` 独立源码复核无新 P1/P2，八项 exact CI SUCCESS；已正常 squash 合并 `20c545b60d88a2ea67beef5be1766cc97ec152d0`，main 本地/八项远端 post-merge PASS | Darwin 新核验主动 UNVERIFIED，不读源、不发新 token；Linux 普通文件/合成拒绝 CI 通过，真实挂载负例未认证 |
| 正式产品和真实环境 | beta.11 资产不变；MAS 1A/1B INCONCLUSIVE、环境 BLOCKED、CarbonCore 历史 FAIL、MAS NO-GO | 新签名 GUI、SMB/其他 NAS、FPK、完整 UX P0 和非开发者理解测试均未通过 |

网络读取与推送曾实际返回 EOF/TLS 错误；后续成功操作按各自 exact SHA 单独留证。
不能将旧 CI 或本地门禁作为未读到的新远端结果。#106 合并后本地 required 已返回 0，
其八项标准 post-merge CI/Security/Linux 检查已按同一 source 读取为 SUCCESS。

## 2026-10-14 较早执行快照（历史保留）

| 工作 | 真实状态 | 未关闭范围 |
|---|---|---|
| 时间排序 #102 | MERGED `02164ad63ddc0c818e4b764ee1e69ed998c4b00f`；最终 exact `bd3b96be18cd0c7607920dc591e69012f182e30d` 经独立边界复核 | 原偶发排序 FAIL 保留，不能追认其唯一根因 |
| UX2 批次安全 #98 | MERGED `ed7287c1842aec2dd0608895cd027fc502990864`；最终 exact `390f80844b39c46334c86b25c816ac89798a3a07` 独立复核 | 库内 subject/CAS 不证明未重新索引的源文件未改变；仅治理意图记录 |
| UX3 历史查询 #100 | MERGED `82f6fd3dbe2e0f434e6f7d715866676abbc54f40`；最终 exact `ae653be45681dcb425251620a7d560af15adac94` 独立复核 | 只读发现/查询，不自动重放遗失请求、不结案中断批次 |
| main 合并后门禁 | exact `82f6fd3` 与 `95eade6` 各自本地 required、远端 CI/Security/Linux 八项 SUCCESS | 不替代原生 GUI、正式签名或人类走查 |
| fnOS disposable host 协议 #103 | MERGED `95eade639a05141aee169ab22fd19b7bec7b15fa`；最终 exact `335958156ad379159f752b0bddc0a41239828ce3` 经独立复核及八项 CI。真实 `92fd581` 窗口 **LIMITED BLOCKED** | `internal` 网络下未取得要求的 loopback 映射；HTTP/扫描/重启/单 writer/隐私验收 NOT RUN |
| fnOS 本窗口保护证据 | 15 个既有容器（12 running、3 stopped）30 条限定状态记录前后相同；7 合成文件 SHA 不变；本轮 3 自有资源在只读查询时点均不存在 | 不是其他历史试验清理证明，不认证生产 NAS、FPK、SMB 或全系统隐私 |
| UX 下一 P0 | 独立源新鲜度工作树开始实现；真实合成 same-size/mtime-restored 编辑已复现旧 Preview 放行 | 尚无合并/运行认证；有界、授权、根目录保护与 Commit 再校验须独立复核 |
| MAS / 其他 NAS | MAS 1A/1B INCONCLUSIVE、环境 BLOCKED、CarbonCore 历史 FAIL、MAS NO-GO；其他 NAS NOT RUN | 当前 fnOS 账号与源码合并均不解除这些门禁 |

beta.11 Release API 再核对仍为公开 Pre-release，原发布时间及四个资产 digest
一致，未重建或修改。新功能仅未来构建；无新渠道公开授权。
主手册正文与历史段落完整保留，旧“Draft/HOLD/尚未执行”只代表各自采样时点。
详见 [本轮证据索引](EVIDENCE-INDEX.md#2026-10-14-合并与真实设备窗口)。

## 当前事实与历史校准

beta.11 已公开为 **Pre-release**（`2026-10-11T16:01:08Z`），不是当前 Draft。用户此前明确回答“确认豁免并授权公开 beta.11”；全新 Mac 离线首装为 **WAIVED_BY_OWNER**，从未实测、不得记 PASS。首发限定支持范围已批准：已验收功能、限定窗口日志隐私、人工独立源文件/数据库备份。历史 Go/No-Go 的 BLOCKED/Draft 原记录保留，后续授权和实时发行状态见 [证据索引](EVIDENCE-INDEX.md)。当前总委托不授权任何新渠道直接公开、商店最终提审或真实生产写入。

| 工作包 | 流程状态 | 验收状态 / 证据层 | 责任方 | 下一触发条件 |
|---|---|---|---|---|
| G0 总执行工作台 | 已合并 PR #94；持续维护 | 历史主手册保留，独立复审/CI/Security 与 post-merge PASS；本次增量另经门禁 | Codex 总控 | 随真实证据更新，不推断运行状态 |
| G1 Developer ID beta.11 | 公开发行已完成 | 正式限定功能/隐私门禁 PASS；新 Mac 首装 WAIVED_BY_OWNER；公开状态已验证 | 已有发行负责人 / Codex 只读监护 | 新缺陷走新版本；不得覆盖既有资产 |
| G2 MAS Phase 0 | CLOSED | 已有原型证据；非商店发行能力 | Codex | 仅保护回归 |
| G2 Phase 1A | 代码已合并 | 正式完整运行 INCONCLUSIVE | Codex / 独立测试环境负责人 | 可证明隔离的 Mac/SMB 环境 |
| G2 Phase 1B | PR #90 已合并 | 运行 INCONCLUSIVE；独立环境 BLOCKED；CarbonCore 历史 FAIL | Codex / 环境负责人 | 证明测试服务、挂载、App 与日志进程身份，不接管现有 QEMU |
| G2 Phase 1C 预检原型 | CLOSED：仅预检原型 | exact HEAD 独立复审及 CI/Security post-merge PASS；正式导入 NOT RUN / BLOCKED | Codex / 独立 reviewer | 后续正式 exporter/importer 先关闭项目锁、来源信任与 staging 边界 |
| G2 Required Reason API 兼容性 | 已合并研究：[PR #92](https://github.com/FNB2026/nas-data-governance/pull/92) | 研究范围独立复审 PASS；Security / CI PASS；合并后 CI/Security PASS；卷身份替代等价仍 BLOCKED | Codex / 独立 reviewer | 官方 API/实际身份矩阵与 DTS 问题完成，保持 fail-closed |
| G3 MAS 分发与商店审核 | BLOCKED / NO-GO | App ID 未确认注册；本机未发现 Apple Distribution/Mac Installer Distribution identity（仅环境证据）；Profile/运行门禁/合规未闭合，未上传 | 账户持有人 + Codex | G2 闭合后集中办理真实账户步骤与最终提交授权 |
| G4 Linux/Docker 安全只读 MVP | PR #93 已合并 | 专用 Ubuntu amd64/ARM64 容器合成只读闭环 PASS；固定公开镜像、真实网络源、升级/备份及完整 G4 仍未闭合 | Codex / 独立 reviewer | G6 镜像证据、明确服务身份及独立部署窗口 |
| G5 fnOS FPK | PR #95 非可安装研究骨架已合并 | 24 合成/离线检查 PASS；嵌套挂载防线已补；无 FPK，实际安装/升级/卸载 NOT RUN，设备/OCI 依赖 BLOCKED | Codex / 隔离设备负责人 | 已验收 OCI digest、独立 fnOS 设备、平台许可；构建 wrapper 持续默认拒绝 |
| G6 多渠道维护机制 | PR #97 / #101 已合并 | 双架构实验 OCI/runtime、全 frontend lock 双格式库存和对应实际漏洞扫描限定 PASS；签名/重现性/完整 G6 未闭合 | Codex / 独立 reviewer | 443beeb 合并后八项远端检查及本地 required/race PASS；签名和重现性另验，不公开镜像 |

## 连续执行与 writer 边界

- 主控只调度具备依赖和安全隔离条件的工作；G2 环境/外部审批阻断不停止 G4 源码工作。
- 文档分支 `codex/multichannel-ux-status` 从上述 main 建立，仅写本目录。原子独占 lease 存在该工作树私有 Git 元数据中，不提交机器路径或进程信息。八小时有效期；续约/交接先核查所有权，不抢占其他 writer。
- 每份后续回执包含 exact SHA、范围、独立 reviewer、测试返回码、CI URL 与证据层。未取得实际容器、正式 App、设备或平台结果时保持 NOT RUN/INCONCLUSIVE/BLOCKED。
- 日常分支、push、合规 PR 合并已授权；不使用管理员 bypass。独立 AI 报告不是作者自批 GitHub Review；以实时 ruleset 决定是否须外部账号。
- 私有现场、生产 NAS、既有工作树、历史 DB、QEMU、其他会话与 beta.11 资产保护。MAS Quarantine/Restore/Purge 禁写；Docker/fnOS 首发默认只读。

## 集中人工行动队列

当前不重复索要已豁免的 beta.11 首装批准。后续真实需要时一次集中说明：独立 Mac/SMB 测试窗口；MAS App ID/证书/Profile/协议的账户操作；fnOS 隔离设备与开发者资格；各渠道最终对外提交/公开批准。设备与许可缺失不是技术 PASS，也不授权生产写入。

参考：[主手册](NDG-MULTICHANNEL-MASTER.md)、[能力矩阵](CAPABILITY-MATRIX.md)、[风险表](RISK-REGISTER.md)。

G4 / G5 的 2026-10-12 ACTIVE 快照已由下列新回执推进：PR #93 和 #95 已正常合并，三套 main CI/Security/原生容器门禁均成功。源码、受限合成容器和正式部署层仍分别裁定。MAS 研究及 fnOS 外部政策阻断保持不变。

### 本轮环境与政策补录

总控的本机签名 identity 检查：Developer ID Application 数量 1，未发现 Apple Distribution 或 Mac Installer Distribution；这是当前机器/Keychain 可见状态，既不证明 MAS 账户无资格，也不证明可签发商店产物。官方 fnpack 1.2.3 私有下载的 help 与 Docker 模板验证 PASS，只是工具层证据，不是 Developer ID 签名、FPK 安装或平台审核证明。

[fnOS 官方测试应用规范](https://developer.fnnas.com/docs/quick-started/test-application/) 当前明确限定手动安装用于本地测试、不作为公开分发方式；外部 FPK 分发路线需平台确认和对应许可。主手册原快照“自安装 Beta”表述保留历史，但不得据此开展公众分发。

## 追加主线：Governance UX v2

用户 2026-10-13 明确要求在连续三端执行中实施 UX v2。679 行附件完整读取，
SHA-256 `136aeb9e9092781f55ca57b6499ffba3e7634870e063bfa614ff68a1da9cc3fa`。
UX0 源码基线、UX1 合并工作台和 UX2 有界批次分别实施，独立工作树与 writer。
前端 exact commit `214cabfd4d84297eca8f1d04957b27ad8d5635ad` 独立复审中；
后端尚未冻结。初步合成页面/组件证据不替代 Wails/SQLite/文件完整性。

P0 仅记录治理意图：默认最多 100 个显式组，不执行、不批准、不解除 HOLD 或恢复锁。
保持旧枚举/历史 PRIMARY_RETENTION 可读；未认证风险、可回收物理空间、全筛选选择
和各平台写入能力明确不可用。未确认回执必须查询，不得整批换 key 自动重放。
进程重启后的 GUI 批次发现、独立真实开发 App 集成、5–8 人理解测试以及未来签名
候选仍未完成；不能把初始前端切片称为完整 UX v2 PASS。

此前三端工作继续执行；UX 变更只作用于未来构建，beta.11 不变。MAS 1A/1B
INCONCLUSIVE、历史 CarbonCore FAIL、MAS NO-GO 不因新 UI 改写。

## 2026-10-13 较早开发采样（历史保留，以下不是当前 main）

本次较早采样 main 为 `15328e0054c1c3e476dd756436ce52d6f3c50c1a`。上节初始
`214cabfd` 为历史采样，下面是最新开发提交；代码合并和完整 P0 验收均未完成。

| 工作 | 最新 exact HEAD | 实际已取得证据 | 当前阻断 / 下一步 |
|---|---|---|---|
| [UX1 PR #96](https://github.com/FNB2026/nas-data-governance/pull/96) | `8e6d1d1e69ed3b57a5edcd94fc0066221e103dc3` | 本地 Go/race、前端 32 文件/296 测试和构建通过；前一 HEAD 的 GitHub Codex 一项 P1、两项 P2 已分别修正并补回归 | Draft；最新改动需独立复审、CI；依赖 #98。原浏览器 mock 是早期快照，不是最新正式 App |
| [UX2 PR #98](https://github.com/FNB2026/nas-data-governance/pull/98) | `e4dfaaadccb94ebf33e9d1be2fc48ba3e060f5e8` | 代码 `0a60a727` 本地 Go/race、真实 Go adapter/SQLite/合成文件闭环、Wails dev build/SDK 检查和全历史 Gitleaks 通过；后续提交仅文档及 MAS 测试夹具 | Draft；需不同 reviewer 核对 root 增量。不是原生 GUI/签名验收 |
| [G6 PR #97](https://github.com/FNB2026/nas-data-governance/pull/97) | `bbe9d35e3fa034824cf87889262982771c7beb75` | 本地 Go/race、15 offline OCI 测试及全历史 Gitleaks 通过 | Draft；新实验 workflow 未实际 dispatch，四份真实 SBOM/漏洞扫描/可复现性 NOT RUN |

PR #96 的修正包括：按管理项目不可变 ID 保持待确认批次；同路径替换项目
不继承旧 key；页面重挂后终态清除 busy；当前页选择不覆写旧快照；全局能力
与后端渠道政策相交，未知/读取失败拒绝记录和旧审批。PR #98 的 RED 证明了
未知 schema 过晚拒绝及百分号 SQLite URI 与 literal owner 路径不一致；最新代码
已提前只读 schema 检查、拒绝 URI 元字符，并完成 Wails 2.13.0 SDK 再生成。

初始 #98 Gitleaks / Desktop Build FAIL、#96 全历史 synthetic marker FAIL 均保留。
十个历史异常已逐个确认是人工测试 key/标记，只用 exact commit/file/rule/line
指纹排除；未来值改为 runtime random。没有整文件/路径或未来模式放行。

独立本地 agents 已返回 usage limit；GitHub Codex 对 #97 也只返回额度不足。
这不是审查 PASS。#96 旧 HEAD 的实际审查不扩展为最新 root 修正的批准。
不购买额度、不自批、不管理员 bypass；暂不合并，不借 CI 解除审查门禁。

完整 P0 仍缺：进程重启后的 GUI 批次发现、未重新索引的物理源变更 stale 契约、
实际隔离开发 GUI 集成、5–8 非开发者理解测试及未来签名候选。已知 key
持久回执查询仅解决后端查找；索引快照 CAS 仅证明库内证据变化，不证明未索引
源文件没变。可继续准备本地前后端集成树，正式 GUI/签名层状态保持 NOT RUN。

台账本身最初 full race 在 `TestListGroupDecisionsOrderingByUpdatedAt` 出现
一次排序断言 FAIL；随后该用例 20 次 PASS。未确定根因，不删失败记录、不用
重复绿色替代根因证明；新门禁回执须单独记录。beta.11、MAS 和 fnOS 状态不变。

### 当前门禁补录

PR #96 `8e6d1d1` 与 #97 `bbe9d35` 的八项 exact-HEAD 远端检查已全部
SUCCESS。仍保持 Draft：独立审查未覆盖最新增量，#96 还依赖后端。
PR #98 `b1cdae9c` 的 App/SDK 已构建成功，但 MAS 测试因访问容器外夹具
被授权防线正确拒绝；新 `e4dfaaa` 通过真实 sandbox 授权/管理项目创建
布置负例，本地 MAS/race/vet 和全量默认 Go/race 通过，正在等新 CI。

本台账新增快照的 make test/vet/build/public-check 和 fresh full race
均返回 0；这是独立新回执，不撤回之前排序用例的间歇 FAIL/未知根因。
十项 exact 历史 synthetic fingerprint 元数据同步只为严格历史扫描，
不是隐私产品放宽；Gitleaks v8.28.0 扫描 308 commits 无新发现。


## 2026-10-13 历史校准：UX 与 NAS 并行（当前状态见首节）

| 项目 | 当前实际状态 | 下一步骤及限制 |
|---|---|---|
| UX1 #96 | MERGED，merge c95c55d588c7b9ede71e8931e54c492506080f08；exact独立AI复审、8CI和post-merge PASS | 仅前端切片，不是native/signed/P0 PASS |
| UX2 #98 | Draft，25f75ae451ca8f01f13e4844e819df8b6bf74084；完整布局及marker、现存WAL、hot-journal拒绝、旧页明确导航已复审；297前端/fullrace PASS，8CI PASS | 新确定时间排序缺陷涉及候选audit；等待独立#102及候选回归，保持Draft |
| UX3 #100 | 堆叠Draft，fbe32a0d1c8d3a27c3296bedb10e9b8629ebbd52；成员集合与终态/原型码两P2 RED→GREEN；独立复审及309前端/fullrace PASS | manual CI另绑定SHA；依赖正常merge后再retarget；未知请求自动恢复和physical stale仍open |
| 时间排序 #102 | 独立Draft，f7369c4e8c8a3ceaec8f0a04e8576594489783fc；初始排序回归 GREEN，8CI及fullrace PASS | 独立复审发现 UTC 年越界与非法 offset 两项 P2，真实SQLite RED，HOLD；作者不自批。原timestamp偶发FAIL保留，不追认唯一根因 |
| G6 #97 | MERGED，bacc160111eeebf57388a67301ac0f30135698a8 | 初始真实run38387062893 native/security PASS，但source SBOM dev库存缺失，永久保留 |
| G6 #101 | MERGED443beeb0e19fa6a737cd8fef5c20e0197bfe4392；exact063d0ee代码/实际run38391959321独立复核及8CI PASS，合并后门禁PASS | 207lock/202dev/205tuple，双格式全覆盖，npm/go/Grype0；只关闭此次库存缺口，unsigned/repro/fullG6仍open |
| fnOS 当前用户设备 | 已授权测试；只读Linux x86_64/Docker28.5.2/overlay2与12现有容器核验；普通账号Docker拒绝后sudo只读成功 | 尚未建立隔离测试区，不读取真实源、不改现有容器。独立disposable host脚本/复审优先；不是FPK安装或SMB PASS |
| 各类 NAS | 复用Linux amd64/ARM64只读核心与容器协议 | 每类平台需自己的设备/挂载/权限/安装证据，不能从fnOS推定其他NAS兼容PASS |

beta.11、MAS/CarbonCore旧状态及无生产写授权边界保持。设备凭据和真实路径仅私有，绝不入库。独立macOS环境仍BLOCKED，fnOS设备不替代该环境。
