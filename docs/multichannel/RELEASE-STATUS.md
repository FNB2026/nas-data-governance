# NDG 多渠道实时执行台账

更新时间：2026-10-12。证据基线：main `584aafeb4cf1bb68cbda3bde2d26698b05555640`。这是时间绑定的快照；每轮先重新读取远端 main、exact HEAD、工作树和 lease。产品状态与代码合并、测试层、发行物层、运行层分别记录。

## 当前事实与历史校准

beta.11 已公开为 **Pre-release**（`2026-10-11T16:01:08Z`），不是当前 Draft。用户此前明确回答“确认豁免并授权公开 beta.11”；全新 Mac 离线首装为 **WAIVED_BY_OWNER**，从未实测、不得记 PASS。首发限定支持范围已批准：已验收功能、限定窗口日志隐私、人工独立源文件/数据库备份。历史 Go/No-Go 的 BLOCKED/Draft 原记录保留，后续授权和实时发行状态见 [证据索引](EVIDENCE-INDEX.md)。当前总委托不授权任何新渠道直接公开、商店最终提审或真实生产写入。

| 工作包 | 流程状态 | 验收状态 / 证据层 | 责任方 | 下一触发条件 |
|---|---|---|---|---|
| G0 总执行工作台 | ACTIVE | 原五文档独立复审无阻断；后续事实补录、本 PR CI/Security 待完成 | Codex 总控及文档独占 writer | 台账 PR 审查、CI、Security 通过后合并 |
| G1 Developer ID beta.11 | 公开发行已完成 | 正式限定功能/隐私门禁 PASS；新 Mac 首装 WAIVED_BY_OWNER；公开状态已验证 | 已有发行负责人 / Codex 只读监护 | 新缺陷走新版本；不得覆盖既有资产 |
| G2 MAS Phase 0 | CLOSED | 已有原型证据；非商店发行能力 | Codex | 仅保护回归 |
| G2 Phase 1A | 代码已合并 | 正式完整运行 INCONCLUSIVE | Codex / 独立测试环境负责人 | 可证明隔离的 Mac/SMB 环境 |
| G2 Phase 1B | PR #90 已合并 | 运行 INCONCLUSIVE；独立环境 BLOCKED；CarbonCore 历史 FAIL | Codex / 环境负责人 | 证明测试服务、挂载、App 与日志进程身份，不接管现有 QEMU |
| G2 Phase 1C 预检原型 | CLOSED：仅预检原型 | exact HEAD 独立复审及 CI/Security post-merge PASS；正式导入 NOT RUN / BLOCKED | Codex / 独立 reviewer | 后续正式 exporter/importer 先关闭项目锁、来源信任与 staging 边界 |
| G2 Required Reason API 兼容性 | REVIEW：[Draft PR #92](https://github.com/FNB2026/nas-data-governance/pull/92) | 研究范围独立复审 PASS；Security PASS / CI 正在运行；卷身份替代等价仍 BLOCKED | Codex / 独立 reviewer | 官方 API/实际身份矩阵与 DTS 问题完成，保持 fail-closed |
| G3 MAS 分发与商店审核 | BLOCKED / NO-GO | App ID 未确认注册；本机未发现 Apple Distribution/Mac Installer Distribution identity（仅环境证据）；Profile/运行门禁/合规未闭合，未上传 | 账户持有人 + Codex | G2 闭合后集中办理真实账户步骤与最终提交授权 |
| G4 Linux/Docker 安全只读 MVP | ACTIVE：`codex/linux-web-mvp` | 尚无可发布镜像 digest 或容器运行 PASS；本机 Docker 未发现 | Codex 实现 writer / 独立 reviewer | 源码、认证、路径边界测试后实际两架构容器验收 |
| G5 fnOS FPK | ACTIVE：`codex/fnos-readonly-package` | 官方 fnpack 1.2.3 私有下载及 help/template PASS；非可安装研究骨架与 fail-closed validator 实现中；OCI digest / FPK 真机仍 BLOCKED | Codex / 隔离设备负责人 | 固定已验收 OCI digest；研究骨架不代表 FPK 安装；核对官方提交政策及独立设备 |
| G6 多渠道维护机制 | ACTIVE：证据与风险台账 | 单一状态/能力/证据/风险入口已建立；未来流水线 NOT RUN | Codex 总控 | 随各独立渠道门禁增量维护 |

## 连续执行与 writer 边界

- 主控只调度具备依赖和安全隔离条件的工作；G2 环境/外部审批阻断不停止 G4 源码工作。
- 文档分支 `codex/multichannel-ledger` 从上述 main 建立，仅写本目录。原子独占 lease 存在该工作树私有 Git 元数据中，不提交机器路径或进程信息。八小时有效期；续约/交接先核查所有权，不抢占其他 writer。
- 每份后续回执包含 exact SHA、范围、独立 reviewer、测试返回码、CI URL 与证据层。未取得实际容器、正式 App、设备或平台结果时保持 NOT RUN/INCONCLUSIVE/BLOCKED。
- 日常分支、push、合规 PR 合并已授权；不使用管理员 bypass。独立 AI 报告不是作者自批 GitHub Review；以实时 ruleset 决定是否须外部账号。
- 私有现场、生产 NAS、既有工作树、历史 DB、QEMU、其他会话与 beta.11 资产保护。MAS Quarantine/Restore/Purge 禁写；Docker/fnOS 首发默认只读。

## 集中人工行动队列

当前不重复索要已豁免的 beta.11 首装批准。后续真实需要时一次集中说明：独立 Mac/SMB 测试窗口；MAS App ID/证书/Profile/协议的账户操作；fnOS 隔离设备与开发者资格；各渠道最终对外提交/公开批准。设备与许可缺失不是技术 PASS，也不授权生产写入。

参考：[主手册](NDG-MULTICHANNEL-MASTER.md)、[能力矩阵](CAPABILITY-MATRIX.md)、[风险表](RISK-REGISTER.md)。

G4 由独立 writer 实现窄完整 HTTP/Web 只读 MVP、会话/CSRF/Host-Origin/服务根别名/单 writer 及 Docker 模板；拟用 PR 专用 Ubuntu runner 验证合成容器运行，不发布。MAS 专项仅独立审计、Foundation disposable 探针与 exporter 设计，身份等价未证实仍 BLOCKED。fnOS 已从官方规范只读审计进入独立非可安装研究骨架实现，不接生产；官方手动安装仅本地测试，不是公众分发许可。

### 本轮环境与政策补录

总控的本机签名 identity 检查：Developer ID Application 数量 1，未发现 Apple Distribution 或 Mac Installer Distribution；这是当前机器/Keychain 可见状态，既不证明 MAS 账户无资格，也不证明可签发商店产物。官方 fnpack 1.2.3 私有下载的 help 与 Docker 模板验证 PASS，只是工具层证据，不是 Developer ID 签名、FPK 安装或平台审核证明。

[fnOS 官方测试应用规范](https://developer.fnnas.com/docs/quick-started/test-application/) 当前明确限定手动安装用于本地测试、不作为公开分发方式；外部 FPK 分发路线需平台确认和对应许可。主手册原快照“自安装 Beta”表述保留历史，但不得据此开展公众分发。
