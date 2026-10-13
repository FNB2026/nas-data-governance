# NDG 多渠道实时执行台账

更新时间：2026-10-13。证据基线：main `15328e0054c1c3e476dd756436ce52d6f3c50c1a`。这是时间绑定的快照；每轮先重新读取远端 main、exact HEAD、工作树和 lease。产品状态与代码合并、测试层、发行物层、运行层分别记录。

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
| G6 多渠道维护机制 | ACTIVE：独立容器证据门禁实现 | 拟以 dispatch-only CI 生成实验 OCI/SBOM/漏洞回执；尚未实际运行，不是签名 attestation 或公开发行 | Codex / 独立 reviewer | 正常 PR 门禁/合并后，执行真实两架构证据窗口 |

## 连续执行与 writer 边界

- 主控只调度具备依赖和安全隔离条件的工作；G2 环境/外部审批阻断不停止 G4 源码工作。
- 文档分支 `codex/multichannel-ledger` 从上述 main 建立，仅写本目录。原子独占 lease 存在该工作树私有 Git 元数据中，不提交机器路径或进程信息。八小时有效期；续约/交接先核查所有权，不抢占其他 writer。
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
