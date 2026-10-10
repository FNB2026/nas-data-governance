# beta.7 正式发行物与 Disposable Quarantine / Restore 验收

## 冻结身份与边界

- 修复 PR #64 合并 `ed691a15a4d3c2aa951d6df875c9134bd05dfded`，版本 PR #65 合并后最终 RC `b09166f4ef5f7a4c9fa899a3c1370163fa373e62`。
- 新 annotated Tag `v0.5.0-beta.7` 对象 `fd1faf513bbe6bfbb43f8d3d443e67e18ce1ae09`，peel 为最终 RC。没有 force 或移动 beta.6 Tag。
- main CI `37964191884`、显式 main Security `37964236991` 均 SUCCESS，且对应最终 RC。
- beta.6 正式 FAIL 与失败数据库、日志、夹具保留。源码回归通过不替代正式发行物实测。
- Release 仅 Draft，Public Beta BLOCKED。本轮不进入 Purge，不主动触发 Crash Recovery。

## 正式流水线记录

[Release run 37964574677](https://github.com/FNB2026/nas-data-governance/actions/runs/37964574677) 绑定冻结 RC。

- attempt 1：Verify SUCCESS；macOS Build 在 Wails 依赖下载阶段因 `proxy.golang.org` TCP 443 `i/o timeout` 失败，签名及 Draft 作业跳过。不是 GUI 验收结果。
- attempt 2：仅重跑失败作业，冻结 SHA 不变；Build Unsigned SUCCESS。
- 签名部署 `6966845819`：通过既有 `release-macos` required-review 机制批准，本轮用户授权的维护者代理操作；不是独立人工发行批准，没有修改或移除环境保护规则。
- attempt 2 最终 Verify / Build / Sign & Notarize / Draft 全部 SUCCESS。正式资产保持 Draft，未发布。
- 官方 DMG SHA-256 `074b3ef3bd6dbfffd1bc7d59966fcf6ad3b324ee0ddf0a73f389dd7983cae68f` 与校验文件和 GitHub digest 一致，四份资产大小与 digest 均匹配。
- DMG Staple、App codesign deep/strict、Gatekeeper execute PASS；Developer ID Team `A2DYS82NLA`，Hardened Runtime。票据附于 DMG；App 未单独 Staple（返回65），不能声称全新 Mac 离线首装通过。
- GUI About 实测 `0.5.0-beta.7` / 冻结完整 RC / `2026-10-09T17:23:09Z` / beta。macOS ShortVersion `0.5.0`、build `7` 符合版本映射。旧 beta.6 App 已私有备份，未打开历史失败数据库。

## 夹具准备

五组全新人工可丢弃本地夹具：正常闭环、源变化 stale、恢复目标冲突、隔离内容变化、受保护目录。每组四个文件，独立源根、隔离区与根外对照。全部源文件 SHA-256/大小与独立备份一致；隔离区为空；独立项目数据库尚待正式 GUI 创建。没有真实 NAS 写入。

所有原始路径、文件名、业务测试锚点、数据库快照与日志只保留于本机私有归档。公开记录仅含脱敏汇总。

## 正式运行状态

| 门禁 | 当前状态 |
| --- | --- |
| 正式下载 SHA / 签名 / 公证 / Gatekeeper / About | PASS（本机在线基础检查；离线首装未验收） |
| GUI 扫描、目录语境、计划复核审批与 Dry Run | 五组已执行；protected 在 HOLD 门禁止步 |
| 真实 Quarantine / 原路径 Restore | happy 闭环 PASS（恢复执行由用户点击正式 GUI） |
| durable VERIFIED、重开及重复执行安全 | 正常重启后状态一致、执行入口消失 PASS；未绕过 GUI 发起后端重复执行 |
| stale executed=0 / failed=1 / DRAFT / 旧审批失效 | PASS |
| 恢复目标冲突 | PASS：用户点击正式 GUI 执行，destination_exists，未覆盖 |
| 隔离变化 | 字节保护、实际 stale 中文通知已观察；原始 error_type/status 仍缺证据 |
| 受保护目录 | GUI HOLD / 审批入口阻断 / 源字节保护 PASS |
| 源根边界 | 不匹配源根的 Dry Run 拒绝 PASS；不冒称真实越界写入尝试 |
| 限定运行窗口事件 / stdout / stderr / 统一日志隐私 | 新补证窗口连续采集、零命中；旧缺口不追溯消除，全进程/长时不作 PASS |
| Crash Recovery / Recovery Lock 正式故障测试 | NOT RUN |

独立准备复核指出：`operation_plans.state` 为判定依据，`evidence_json` 内状态是旧快照；恢复完成后源计划仍 VERIFIED，隔离项及恢复计划才为 RESTORED；HOLD 限制 Purge，不能虚构后端 Restore 拒绝测试。

## 首组实际 GUI 证据（阶段记录，未最终通过）

- 正式 GUI 创建独立 happy 项目，只读核对登记源根与新夹具一致。
- 扫描自然 COMPLETED，文件 active=4、missing/unavailable=0；重复组1，两份独立物理副本完整 SHA 一致。已查看 temporary/normal 目录语境及保留理由。
- 保存草案、人工记录动作决定后，数据库仍 DRAFT，执行中心可执行数0。这里只证明 GUI 不提供执行资格，不冒称未发起的后端拒绝实测。
- 独立批准与 Dry Run：executed=0/skipped=1/failed=0，durable APPROVED；源4/4原始哈希一致、隔离区空、Journal/隔离项0、根外不变。
- 真实隔离：executed=1/failed=0，durable VERIFIED，唯一 QUARANTINED 项、唯一 done Journal。隔离目标 SHA/大小与初始清单一致，另外3个源文件及根外对照未变；已批准可执行列表归零。
- 恢复草案、审批、Dry Run 分开完成。恢复校验通过，restore plan APPROVED、restore Journal0，文件仍保持隔离后状态。
- Computer Use 坐标输入连续返回 `windowNotFoundAtPosition` / `noWindowsAvailable`；AX 将恢复试运行和执行合并为单元格，只能触发试运行。刷新绑定、Raise 和原生窗口菜单未解除定位问题；已请求用户点击当前唯一测试项的恢复执行按钮。工具阻断不能解释为产品 Restore FAIL 或 PASS。
- 当前采样：54标记/156编码变体；stdout/stderr、约366.8MB统一日志、11条 job_events、3条 operation_logs 均零命中。采集仍进行，未覆盖其余案例和重开后的新进程，不能作为最终隐私PASS。
- 上述工具阻断后，用户确认已点击唯一测试项的正式 GUI 恢复执行按钮。只读快照及实际页面确认源4/4的 SHA-256/大小均与初始清单一致，隔离区空，根外不变；源计划 VERIFIED，隔离项与恢复计划 RESTORED，恢复 Journal done。
- 正常退出并重新启动正式 App 后，重新打开 happy 项目，状态保持一致、执行可选数0、无恢复锁、无新增隔离项或 Journal。没有主动崩溃，也没有绕过 GUI 调用重复执行 API。
- 初始统一日志实时采集停止后存在恢复点击窗口的采集缺口，已通过系统 retained `log show` 补取该窗口；补取不能证明连续实时覆盖。stdout/stderr 保持捕获，新进程另开统一日志段；最终隐私判定须保留此限制。

## source-stale 与源根边界实际证据

- 全新 source-stale 项目正常扫描、独立复核、批准及 Dry Run；源4/4、隔离区空、Journal/隔离项0。
- 仅改变该计划的人工可丢弃 QUARANTINE 目标，备份保持原始内容。正式 GUI 点击执行后显示 executed=0/skipped=0/failed=1、`stale_detected`，可执行列表归零。
- durable 状态 DRAFT；审计包含 `approval_invalidated` 与 `stale_check`，无 Journal、无隔离项。改变的目标保持改变后的 SHA，另3个源文件与根外对照均未变。
- 全新 destination 项目批准后，输入与登记源不匹配的允许根，Dry Run 返回 `scope_validation_failed`、executed=0/failed=1，实际隔离按钮禁用；源4/4、隔离区空、Journal/隔离项0。随后输入正确源根，重新 Dry Run 后隔离成功：VERIFIED / 唯一 done Journal / QUARANTINED。
- 独立创建并批准恢复计划后，仅在原恢复目标位置创建人工冲突文件。恢复 Dry Run 返回 `destination_exists`；原隔离内容与冲突文件保持，restore plan APPROVED、restore Journal0。当时真实执行拒绝尚待观察，随后实际执行证据见下一节。
- 独立只读复核确认 happy 重开六张状态/Journal/审计表与恢复快照逐行一致，stale 的 DRAFT/审批失效/字节保护以及 destination 现场一致。GUI 汇总数字依据本任务实际 AX 观察，另存脱敏转录；转录并非原始截图或 GUI 事件导出，不能由 Journal0反推执行数字。
- 最新限定窗口采样：56标记、164编码变体，三组项目的33条 job_events、8条 operation_logs、两段 stdout/stderr、约373.4MB初始统一日志、约55.3MB新进程日志及约2.0MB补取日志均零命中。新进程日志尚未封存，另两组尚无运行证据，采集缺口与进程覆盖限制仍保留；不判最终隐私 PASS。

## 恢复冲突执行与隔离变化阶段

- 用户确认点击 destination 测试项的正式 GUI「执行」，页面新增 `destination_exists` 错误。随后的只读快照证明冲突文件 SHA 未变、原隔离文件 SHA/大小未变，restore Journal0、隔离项仍 QUARANTINED，源计划 VERIFIED；没有覆盖或新写入。这是预期安全拒绝，不是正常恢复闭环失败。
- 新 quarantine-stale 独立项目扫描及分阶段审批/Dry Run后正常隔离：VERIFIED、唯一 done Journal、QUARANTINED，另3个源文件与根外对照不变。
- 独立创建/批准恢复计划后，仅改变可丢弃隔离内容，原始备份未变。正式 GUI Dry Run 提示「计划已过期（文件自审批后发生了变化）。请重新生成计划并审批。」。随后用户确认点击蓝色执行并显示「执行恢复失败」；复查时 toast 已过期，不能直接确认实际执行的具体错误码。执行后只读快照确认变更隔离 SHA/大小不变、源目标仍缺失、其余3个源文件及根外不变、restore Journal0、隔离项 QUARANTINED、恢复计划 APPROVED。字节保护通过；精确错误码证据不足，不冒称完整错误分类验收 PASS。

## 受保护目录与收尾

- 第五个全新项目正常扫描4/4，受保护重复组为 critical，两项均 REVIEW/raw_source；GUI 明确显示严重风险 HOLD 需独立释放。
- 保存「起草动作」用户决定后仍无批准入口，执行中心可执行数0。只读数据库仍 critical/DRAFT，无 Journal/隔离项，源4/4原始 SHA/大小及根外不变。这里只证明正式 GUI 保护门禁，不声称调用过不可见的后端批准/执行接口，也没有测试隔离项 HOLD 的 Purge 行为。
- 五组原始备份、历史证据及现场均保留。异常用例保留人工冲突/变更内容作为证据；未清理、未 Purge、未主动触发 Crash Recovery。
- 当前正常闭环、状态持久化、stale 失效、恢复冲突、保护和源根 Dry Run 边界均有实际证据。隔离变化真实失败的精确错误码、连续日志采集缺口及更广进程覆盖仍限制最终结论。
- 最终封存日志采样：58标记、172编码变体；五组共55条 job_events、11条 operation_logs、两段 stdout/stderr、373,425,308字节与94,548,459字节实时统一日志及2,014,319字节 retained补取日志均零命中。两段实时日志不是可直接解析的完整 JSON 文档，按原始文本检查；补取 JSON 可解析为1754项。零命中只覆盖实际采集内容，不能证明缺口或全部相关进程无泄漏。
- 最后核对五组独立备份均4/4原始 SHA匹配、根外对照均未变。各组最后一致性快照 file_status 均 active4、missing/unavailable0、Purge Journal0。尝试统一重新打开旧项目 live DB 的只读查询遇到 `unable to open database file`，未改权限或数据库，改用此前各阶段保存的一致性快照；不把这些快照宣称为当前全部 live DB 的同时采样。
- 独立只读复审批准上述限定功能与文档结论；不是 Public Beta 发布批准。正式 App 保持打开，已停止诊断日志采集，没有取消或改变产品任务。

整体仍 INCONCLUSIVE（证据限制），Public Beta BLOCKED，Release Draft；不进入 Crash Recovery / Recovery Lock 正式故障验收。

## 第六组补证：连续窗口与实际执行通知

本节为后续补证，不改写五组历史结果或旧日志缺口；这仍是证据完整性阻断，未据此确认新的产品功能缺陷。正式 beta.7 与完整 RC、About 构建时间及通道再次核对一致，没有重建、修改版本、Tag、DMG 或发行资产。

- 创建第六组全新 disposable 文件与独立数据库，保留原始 SHA/大小及独立备份。正常扫描4/4，独立决定、批准、Dry Run 后实际隔离成功：durable VERIFIED、唯一 done Journal、QUARANTINED。
- 独立恢复草案及批准后，只改变该组人工隔离文件。CUA 此次坐标点击真正蓝色「执行」成功；再次相同安全拒绝的点击开始 `2026-10-10T03:07:41.225Z`，返回时间 `03:07:41.359Z`，同一工具调用即时取得完整 AX 和截图：`执行恢复失败 计划已过期（文件自审批后发生了变化）。请重新生成计划并审批。`。不再依赖用户通用失败报告或已经消失的 toast。
- 执行后隔离内容保持人为变更后的 SHA/大小，原目标缺失，另外3源文件与根外对照未变；恢复 Journal0，隔离项 QUARANTINED，恢复计划 APPROVED，源计划 VERIFIED。实际重复拒绝没有增加 Journal/隔离项。
- 这不是原始错误分类的直接证据：客户端会把含 `stale` 的错误翻译为上述通知；源码预期返回 `stale_detected` 且 StepFailed，但运行 stdout/stderr 和统一日志均未出现原始 `stale_detected` 或原始 status。中文通知与源码映射支持 stale 类拒绝的推论，不能宣称捕获了 `error_type=stale_detected / status=failed`。该严格证据项仍 INCONCLUSIVE。

### 新连续日志窗口

- 开始 `2026-10-10T03:05:01.090199Z`，结束 `03:08:23.922801Z`，203秒；先启动统一日志，再开启 stdout/stderr 重定向并启动正式 App。新 App PID84740，log PID84728；窗口内 App 未重启。
- predicate 包含 NDG 及 WebKit process/subsystem；每秒记录采集进程/App 存活、字节数、NDG 与 WebKit PID/PPID。193次采样全部存活，最大采样间隔1.139秒；实际恢复点击与只读结果核验均位于窗口内。结束为明确 STOP 后 SIGINT，log exit0，stderr空，未使用事后 `log show` 填补本窗口。
- NDJSON 为27,745,939字节，完整解析27,864条事件，结束记录 `count=27864 / finished=1`，另两行为过滤器头和空行。事件中 NDG26,928、WebKit GPU636、Networking300；WebContent 在 predicate 和进程存活清单中，但本窗口未取得它的独立日志事件，不能把零事件扩大为 WebContent 所有行为已通过隐私验收。
- 67测试标记、201编码变体对 stdout/stderr、统一日志及采集 stderr均零命中。原始日志、进程清单、AX转录、数据库与 SHA 证据仅存本机私有归档。该窗口关闭原先需要重新建立连续采集的缺口；不消除历史缺口，不声称全系统或长时隐私 PASS，1秒存活采样也不是底层日志绝无丢失的证明。

下一步仅补原始错误分类的可观测证据并独立复核；在原始分类未直接捕获前，正式 Quarantine / Restore Gate 仍 INCONCLUSIVE。Release Draft / Public Beta BLOCKED，Crash Recovery 不启动。
