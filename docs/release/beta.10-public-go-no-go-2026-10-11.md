# beta.10 Public Beta 最终 Go / No-Go — 2026-10-11

## 决策

**NO-GO。Release 保持 Draft，Public Beta BLOCKED；不得公开。** 本轮仅审计与文档校准，无产品代码、版本、Tag、DMG 或 SBOM 改动，不生成候选，不执行 Purge。新 Mac 暂无设备，用户明确暂时略过；这不是首装 PASS 或发布豁免。

main 基线 `70de4667a3f73719f7ac885da41f6c0cdba22c35`；正式 RC `c1941f84e2d1c836b57cd0750db1c2554ba6c4c9`。后者至 main 的差异仅证据/测试，不将 main SHA 当作 App Commit。

## 实时门禁表

| 门禁 | 状态 | 证据与下一动作 |
| --- | --- | --- |
| 正式发行身份、签名、公证、Gatekeeper、校验、SBOM | PASS | Release 38090124629；官方 DMG SHA-256 `27f4651d192c856a6297a8ca8761a0ec9ae230d1124c7cafe52d97086d7ad60d`；候选报告及本轮 About 版本/full RC/beta 一致 |
| PR #79 exact HEAD、独立复核、CI/Security、main 门禁 | PASS | 已合并 `70de4667`；HEAD `b564fdd4`；CI 38117154348 / Security 38117154253；post-merge 38117411497 / 38117412953 SUCCESS，Issue #72/#75 CLOSED |
| 限定 Crash Recovery | PASS | [PR #79 报告](beta.10-formal-artifact-crash-acceptance-2026-10-11.md)：A 保锁，D 正式新审批恢复闭环；B/C 仅经明确批准的冻结源码组合证据。正式 B/C 永久 INCONCLUSIVE；A operation_logs=0 不含新拒绝审计保证 |
| Quarantine / Restore | PASS（限定） | beta.7 正式/分类组合证据及 beta.10 D 正式恢复；不将 beta.6/beta.8/beta.9 历史失败转 PASS，不扩展为所有负例在 beta.10 重跑 |
| SMB Resume | PASS（历史正式范围） | beta.6 专项记录；beta.6 RC→beta.10 RC 的 internal/scanner、internal/app/scan.go 无差异。本轮不声称 beta.10 新做网络中断 |
| 全新 Mac 离线首次安装 | BLOCKED | 无从未运行 beta.10 的测试 Mac；用户暂略过，准备协议见下文。旧 beta.5 首装不可替代 |
| npm 高危风险处置 | BLOCKED | source-map-js 1.2.1，1 high；构建/测试链路，未获正式例外批准且未完成正式产物级排除证据。PR #58 的 1.2.2 修复可另审，本轮不合并或重建 |
| Purge 未验收不得开放（本轮要求 5） | BLOCKED | 本轮要求 5 明确未验收不得开放、不能仅前端隐藏。正式 App 显示可执行永久清理=可用、清理页/生成草案启用；冻结后端仅 RW/锁/审批/Dry Run/确认门控，无 beta 身份禁用。NOT RUN；[Issue #80](https://github.com/FNB2026/nas-data-governance/issues/80) 跟踪；不能用文档警告代替后端禁止 |
| 限定日志/事件隐私 | PASS（限定） | PR #79 连续 7205.667 秒，124835 eventMessage +1结束对象，169变体零命中；44 job_events/13 audit 条目。仅选定 App/WebKit predicate，不含 A 缺失审计 |
| 首发隐私标准及更广覆盖 | BLOCKED | 本轮安全裁定建议，精确证据契约尚待批准：实际扫描/审批/隔离/恢复/拒绝/重启窗口，App stdout/stderr+可识别 WebKit/子进程连续捕获，标记零泄漏与无非用户发起第三方数据外送。已有证据支持动作窗口与选定进程；未证明全部相关进程/外送边界。先明确首发最低契约：若要求这些覆盖，仅补对应缺口；若接受已有限定证据，必须明确批准范围，不改历史限制。本项为安全裁定待定，不把建议当作既有不可豁免 Gate |
| 安装/备份/恢复/限制文档 | PASS（文档校准范围） | 用户指南安全首装和完整 DB/WAL/SHM 备份说明；本轮纠正旧审批与歧义恢复说明、加入已知限制。不是备份恢复实测 PASS |
| 冷备份恢复正式实测/首发支持范围确认 | BLOCKED | 本轮文档/支持范围审查建议，非已冻结强制实测项：未见 beta.10 独立冷备份→只读验证→恢复演练证据。先确认首发是否承诺该闭环；需要实测时仅用可丢弃项目，原库不写；否则限定文档支持范围，不承诺自动备份或任意改恢复路径 |
| Draft Notes 与实时入口 | PASS（文档准备） | 本轮已同步 Draft Notes、README/候选/旧门槛表指向此表；Draft/prerelease 仍 true，资产不变；发布前由最终授权确认 |
| Intel/Windows/Linux、自动更新、全系统/长期隐私、Purge 功能验收 | DEFERRED | 首发不支持/不宣称。但 Purge 延后成立的前提是后端禁用；当前 BLOCKED 不被 DEFERRED 覆盖 |
| 最终发布授权 | BLOCKED | 用户未授权公开；所有必须项闭合后再作最终决策 |

## 全新 Mac 离线首装准备

使用现有 beta.10 官方 DMG（不覆盖旧资产）。仅获得 Draft 权限的验收人员从登录的 GitHub Releases 页下载；Draft 链接可能对无权限用户 404，公开发布前不能给普通用户当下载入口。

1. 记录 Apple Silicon 机型/macOS >=13、此前从未运行 beta.10；全新用户账户不等于全新机器 Gate。
2. 下载 DMG/sha256 文件，校验上述 SHA；记录下载方式及 quarantine 属性。下载后不在线启动 App。
3. 断网后首次挂载、拖入 Applications、打开；保持正常 Gatekeeper。不得“仍要打开”、清除隔离属性或关闭保护。
4. 记录首次引导、正常启动；About 截图版本/full RC/构建 `2026-10-10T22:09:04Z`/beta。
5. 记录网络确实关闭与测试时间，保存原始证据于本机私有归档，公开仅脱敏摘要。
6. 若系统阻止启动，保留提示并 FAIL/BLOCKED；不能绕过后算通过。
7. 有设备后才执行；当前 NOT RUN/BLOCKED。

## npm 独立风险判定

[当前 audit](evidence/beta10-public/npm-audit.json)为真实在线查询，1 high / 0 critical；冻结 lock 1.2.1/dev=true。依赖链 Vite→PostCSS→source-map-js，以及 jsdom→css-tree→source-map-js；React/ReactDOM 是直接运行依赖，前端 src 无 source-map-js/SourceMapConsumer 导入，Vite 默认构建未启用 sourcemap。Go 正式 App 嵌入 dist，不分发 Node dev server。

[GHSA-68fv-2mgg-jv7q](https://github.com/advisories/GHSA-68fv-2mgg-jv7q)影响 >=1.0.0 <1.2.2，1.2.2 修复；恶意 indexed source-map offset 可同步阻塞事件循环。当前能论证主要攻击面在构建/测试处理不可信 source map，不能仅凭 dev 标签证明正式二进制不含该实现。本轮未接受任意恶意 source map，也未做 PoC DoS。

CI npm audit 的 continue-on-error=true 是绿 CI 与高危并存的原因。结论：日常 NAS 扫描不是已证明的利用入口，但漏洞仍在冻结构建图；不要宣称风险零。首发前建议选择审 PR #58 修复并按不可变候选规则另行处理，或用产物/输入边界证据形成限期安全例外并获明确批准。两条都未完成，因此 BLOCKED；本轮不擅自换候选。

## Purge 确证

本轮正式 beta.10，打开全新人工目录/独立 DB，无扫描/清理计划；GUI 清理页“生成清理草案”启用，设置安全表“可执行永久清理=可用”，Recovery Lock=无；About full RC 正确。仅浏览，不点击生成、审批或执行，未写真实源/历史现场。

冻结 `bindings.go` 的 CreatePurgePlans/ApprovePurgePlan/ExecutePurge 对 RW 服务提供绑定；GetAppCapabilities 的 CanExecutePurge=isRW&&!recoveryLock。PurgeService 有执行锁、requireNoRecovery、digest、DryRunVerifiedAt 与逐字确认；executor 仍包含不可逆删除。主程序 Bind API，未发现 release channel 禁止路径。因此这是未验收功能仍可达的发布策略阻断，不宣称已经造成数据损坏。需要独立最小后端禁用及对应前端独立能力处理（当前共享 isLocked 包含 CanExecutePurge，简单设 false 会一并禁隔离），或明确批准新正式 Purge 验收；当前两者均未满足。此阻断依据本轮要求 5；历史 R11 仅“默认隐藏或禁用”，不能把新要求归给旧记录。禁止仅隐藏菜单或改说明放行。

## 可执行收口次序

1. 先裁定并处置 Purge 首发后端禁止；若要产品修复，独立 PR/回归/新不可变候选，不能修改 beta.10。
2. npm 独立修复或有证据的限期例外批准。
3. 有新 Mac 后首装；先批准首发备份/隐私证据契约，再仅补明确缺口或限定支持范围。
4. 文档/Notes 跟随最终候选校验后，全部 REQUIRED 变 PASS，再请求最终公开授权。

上述阻断未关闭前，不执行发布命令。
