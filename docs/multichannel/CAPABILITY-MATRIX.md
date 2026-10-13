# NDG 多渠道能力矩阵

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

UX 开发限定证据：#96 前端组件及早期合成浏览器、#98 真实 Go/SQLite 临时
文件后端测试已取得各自证据；尚未合并、尚无最新独立复审与正式原生 GUI。
G6 #97 为离线 OCI 验证实现，正式 native collector 尚未执行。旧 beta.11
不参与新项目 owner lease，不得宣称跨旧二进制的全版本互斥。
