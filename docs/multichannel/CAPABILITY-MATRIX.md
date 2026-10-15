# NDG 多渠道能力矩阵

## 2026-10-15 当前公开能力摘要（以下为历史快照）

| 能力 | Mac beta.11 已公开 | Linux Docker preview.1 已公开 | fnOS | MAS / FPK |
| --- | --- | --- | --- | --- |
| 只读扫描和重复分析 | 有限定验收 | AMD64/ARM64 本地人工源已实测；Web 仅前 200 组聚合显示 | Docker 宿主合成源实测，客户端访问 BLOCKED | 未正式发行 |
| Quarantine 隔离 | 经审批有条件支持 | 禁用 | 禁用 | 未发行 |
| Restore 恢复 | 经审批有条件支持 | 禁用 | 禁用 | 未发行 |
| Purge 永久清理 | 后端禁用 | 后端禁用 | 禁用 | 未发行 |
| 新 UX v2 治理意图记录 | #111 Darwin 源证明 BLOCKED，尚未进入公开 beta.11 | 禁用 | 禁用 | 未发行 |
| 发行身份 | [Mac beta.11](https://github.com/FNB2026/nas-data-governance/releases/tag/v0.5.0-beta.11) | [Docker preview.1](https://github.com/FNB2026/nas-data-governance/releases/tag/v0.5.0-docker-preview.1) | 实验性宿主证据，无可用发布 | NO-GO/研究中 |

**下一目标**：两个正式渠道均完成安全隔离、恢复和人工确认的永久清理。实施仍需独立平台根边界、最小权限、执行锁、恢复与审计证据；旧版本和旧快照不被追认。


快照：2026-10-13，main `15328e0054c1c3e476dd756436ce52d6f3c50c1a`。状态必须绑定列对应发行身份，不将 main 新能力归入 beta.11。详见 [实时台账](RELEASE-STATUS.md)。

| 能力 / 边界 | Developer ID beta.11 | MAS 当前开发层 | Docker / Web MVP | fnOS FPK |
|---|---|---|---|---|
| 发行身份 | `com.fnb.ndg`，正式 RC `791f19119542e69b62ee62dd358fed188000837e`，公开 Pre-release | 设计 `com.fnb.ndg.mas` 未注册确认；dev/ad hoc 证据不是 MAS 正式包 | PR #93 已合并；原生两架构合成容器已测，未固定公开候选 digest | 等待同一已验收 Linux/OCI 核心，不另造扫描引擎 |
| 只读扫描 / 分层哈希 / 目录语境 | 受已有正式限定验收；断点及候选完整性证据按版本来源保留 | 源码授权层已合并，完整真实运行 INCONCLUSIVE | 合成本地源容器 PASS：7 quick / 6 full / 3 groups；真实网络源/完整发布仍待测 | NOT RUN |
| 来源权限 | 用户明确源根，保护与恢复锁约束 | 原生用户选择 + 私有 Bookmark + 租约；权限失效不记 missing | 服务端预设源别名，浏览器不得提交任意主机路径；首发源卷只读 | 向导须明确只读路径，不能默认全盘 |
| Quarantine / Restore | 正式限定正常闭环与批准组合 Crash 证据；历史限制保留 | 后端禁止写入 | 默认禁止，待独立写入门禁，不凭 UI 隐藏 | 默认禁止，不从 fnOS 登录推定执行授权 |
| Purge | 后端 Create/Approve/Dry Run/Execute 禁用；未知历史恢复不认证 | 禁止 | 首发禁止 | 首发禁止 |
| 数据库与备份 | 人工独立源/DB 备份支持；无受支持跨版 exporter | 独立容器；预检原型未接产品；0700 不证明所有权/容器；不迁旧 Bookmark | 私有本地持久卷、进程单 writer、升级备份待验证 | 同一持久 DB 生命周期待真机验证；不得卸载静默删数据 |
| 隐私认证 | 限定窗口 PASS；全系统、全进程持续负载不认证 | CarbonCore 历史严格 FAIL，归因/环境 BLOCKED；普通日志禁私人信息 | 合成容器限定 stdout/stderr 窗口 PASS；反代、镜像/provenance 与更广隐私待测 | 含宿主/应用网关日志边界待测试 |
| 重新安装 / 平台覆盖 | Apple Silicon/macOS 13+ 有限支持；新 Mac 首装 WAIVED_BY_OWNER，不是 PASS | 未实际发布；正式 Sandbox/平台审核未闭合 | 专用 Ubuntu amd64/ARM64 容器合成验收 PASS；正式宿主 NOT RUN | 官方 fnpack 1.2.3 help/template PASS；PR #95 研究骨架已合并；设备 CPU/安装升级卸载均 NOT RUN |
| 公开渠道 | GitHub Pre-release 已有专属明确授权并公开 | NO-GO / 未上传 | NOT RELEASED，公开需渠道最终批准 | NOT RELEASED；官方手动安装仅本地测试，不能据此公开分发；提交/上线需资格、门禁和批准 |

正式 Crash Recovery B/C 原 App 记录永久 INCONCLUSIVE，限定 Gate 依据仅获准的组合证据；A 的审计限制永久保留。Phase 1C 合并只关闭预检原型，不开放正式导入。所有新渠道复用 Go 核心及相同恢复安全语义，不重写三套算法。

MAS 兼容性研究 [PR #92](https://github.com/FNB2026/nas-data-governance/pull/92) 已取得范围限定的独立复审，但不改变授权运行、身份等价或商店发行状态。本机签名 identity 数量不等于账户资格；fnpack 工具运行不等于 FPK 安装/开发者签名/上架。

Governance UX v2 属未来 main 开发线：只读统一证据页和有界意图记录正在实施，
不能归入已冻结 beta.11。MAS/Web/fnOS 仍无批次决定写入许可；后台数据持久化
不是 Quarantine、Restore 或 Purge 的执行授权。

较早历史快照（由下方最新证据增量取代）：UX 开发限定证据：#96 前端组件及早期合成浏览器、#98 真实 Go/SQLite 临时
文件后端测试已取得各自证据；尚未合并、尚无最新独立复审与正式原生 GUI。
G6 #97 为离线 OCI 验证实现，正式 native collector 尚未执行。旧 beta.11
不参与新项目 owner lease，不得宣称跨旧二进制的全版本互斥。


### 2026-10-13 最新证据增量（历史段落保留）

- UX1 #96 已合并，仅前端切片。UX2 #98 / UX3 #100 当前独立审查的数据库
  布局/WAL/hot-journal/回执成员/安全标签与旧页导航已补；完整P0仍未认证。
- G6 #97 / #101 已正常合并。实际run38391959321两架构native、内容身份、
  207frontend锁项双格式完整库存及对应真实漏洞扫描限定PASS；unsigned、repro、
  所有构建层、长期/所有stdout隐私仍未认证。旧383870库存缺失永久保留。
- 用户已提供现有fnOS测试设备，SSH及只读sudo确认x86_64/Docker28.5.2/overlay2、
  12个现有容器。仅独立新disposable区域/资源验证已授权；现有数据/容器不变。
  NAS Docker host smoke、FPK、内置SMB/其他NAS平台都尚未运行，不能扩大支持。
- fnOS可用不解决独立macOS/Bookmark/CarbonCore环境，MAS状态不变。


## 2026-10-14 较早能力增量（历史保留）

- #98/#102/#100 已按独立 exact review 正常合并 main，真实 SQLite、Wails
  调用链、组件及 CI 证据闭合。批次历史发现仅只读，不能重放/修复状态。
- 完整 UX v2 仍未 PASS：外部源变更但未重新索引的 P0 检查正在独立实现；
  原生 GUI、未来正式候选、5–8 名非开发者理解走查仍 NOT RUN。
- #103 限定协议代码已合并；fnOS 主机试验已真实启动专用限制容器，但缺少要求的 loopback 映射，
  在 HTTP 之前安全停止：LIMITED BLOCKED。已取得本窗口源字节/既有容器
  状态与精确自有资源清理证据，不等于扫描、单 writer 或重启持久化 PASS。
- 容器镜像仍绑定历史 unsigned `063d0ee` 实验源码，不含最新 main UX 代码；
  不把本轮主机协议代码、Ubuntu CI 或原有 Mac DMG 混为同一发行身份。
- FPK 安装/升级/卸载、真实 SMB、其他 NAS 平台、MAS 写入能力未开放。
  beta.11 资产不变；MAS 1A/1B INCONCLUSIVE、CarbonCore FAIL 与 NO-GO 保留。

## 2026-10-14 后续能力校准

- #106 已合并，只验证自有 namespace 探针设计的离线约束和安全拒绝。
  默认运行仍 BLOCKED，模型接受不是实际 namespace/权限下降/HTTP/清理证明。
  不连接 NAS，不激活现场后端，不升级此前 LIMITED BLOCKED。
- #105 新核验只作用于选定重复候选，最多 100 组、256 成员和 128 MiB
  索引字节，Preview/Commit 分别核验；不会全量读取普通单例。
  final `0e0705b` 已独立复核，八项 exact CI 全成功并正常合并 #105；
  同 source main 合并后本地/八项远端门禁也已闭合，不替代真实 GUI 或挂载场景。
- Linux 源码要求 rooted `openat2` 的 BENEATH/NO_SYMLINKS/NO_XDEV，
  不支持或拒绝时无路径降级。ENOSYS/EXDEV 的实际分类为
  SOURCE_UNAVAILABLE/BLOCKED，不是 UNVERIFIED 枚举，不修改索引 file_status。
  本机只完成 Linux 测试二进制交叉编译，不能记真实 Linux 挂载验收 PASS。
  新 exact Verify 已实际运行 Linux 全量 Go race 与 Wails/fingerprint 测试，
  关闭普通合成文件的运行缺口；真实 bind mount/SMB 矩阵仍 NOT RUN。
- Darwin 没有等价嵌套挂载边界证明，因此新增核验前置 UNVERIFIED，
  不打开源文件、不获取源租约、不发行新 preview token。
  旧扫描/Resume/执行路径不变；Mac 的该新增能力仍 BLOCKED，完整跨平台 P0 未通过。
- 已发起且响应不确定的请求保留原 token/key 进行回执重试；过期的新提交仍拒绝。
  组件/调用链测试不是原生 GUI 或正式签名 App 证据。
