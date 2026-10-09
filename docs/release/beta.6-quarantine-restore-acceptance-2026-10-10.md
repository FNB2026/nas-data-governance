# NDG beta.6 — Disposable Quarantine / Restore 正式验收

## 结论与身份

**总体 FAIL；Public Beta BLOCKED；Release 保持 Draft。** 实际文件隔离及原路径恢复成功，但执行计划的持久化终态和陈旧拒绝的结果计数不一致。独立修复任务：[Issue #62](https://github.com/FNB2026/nas-data-governance/issues/62)。修复需独立 PR、回归与复审，并用下一不可变正式候选复验；不能将本次 beta.6 FAIL 改写成 PASS。

- 正式 App：官方 Draft DMG 安装的 `0.5.0-beta.6`，About 完整 Commit `42397a9196ae5c0621aacec3913970b7269bae24`，构建时间 `2026-10-09T11:00:44Z`，channel `beta`。
- 文档基线 main `29af17975771df0b6e7c4e0df322f880cc766ea4`；该文档提交不是发行身份。
- 本轮再次核对正式 App 身份和 deep/strict 签名。为采集 stdout/stderr 正常重启同一个已安装签名执行文件，没有改 bundle 或使用开发版。
- 操作实际窗口 UTC **2026-10-09 14:12:31–16:19:39**（CST 跨至 10 月 10 日）。本记录日期为结案日期，不将后续文档操作编造为此前运行证据。
- 全程没有修改产品代码、数据库记录、版本、Tag、发行资产或真实 NAS 文件；未执行 Purge，未主动触发 Crash Recovery。

## 安全执行边界

四份独立本机 APFS 可丢弃夹具及 GUI 新建项目数据库，分别为正常闭环、恢复目标已存在、源文件变化、隔离内容变化。不是 SMB Quarantine 实测，也不是原百万级项目。所有文件由测试执行层人工生成，在产品写入前保存 SHA-256、大小、相对路径及独立字节备份；每个项目有独立 SourceRoots、QuarantineRoot 和源根外哨兵。

正常夹具五文件：一对 temporary 重复文件、一对受保护目录重复文件及一个普通单例。其余三个负例各两份 temporary 重复文件。实际 registered root 与夹具根目录通过只读 SQLite 核对后才扫描。

初始 App Support 内夹具被 GUI 项目创建边界拒绝，未执行文件动作；复制到新的可丢弃临时根并验证 SHA-256 后再进行验收。源变化负例最初的目录命名被内置规则识别为 raw_source，属于 **INCONCLUSIVE 夹具语境**；未批准、未隔离，保留该项目，另建中性目录夹具。没有修改保护规则来强行执行。

## 实际 GUI 工作流与结果

扫描、目录语境、系统建议、保存草案、记录用户决定、批准、Dry Run、隔离、恢复草案、恢复批准、恢复试运行、执行恢复均使用正式 GUI，独立步骤操作。SQLite 仅用 mode=ro 查询/一致性 backup，未直接构造审批或执行结果。

| 场景 | 实际结果 | 判定 |
|---|---|---|
| 正常扫描与目录语境 | 5/5 自然完成；两对重复，一对 temporary，一对敏感/受保护，单例不进入隔离建议 | PASS |
| 未批准执行入口 | 两个草案保存后执行中心批准计划数 0，执行入口不可用 | PASS（GUI Gate；未声称调用后端拒绝实测） |
| 受保护/HOLD 建议 | critical 草案要求 HOLD 独立释放，无批准按钮，未隔离受保护文件 | PASS（计划保护 Gate；不等于 managed HOLD item 恢复实测） |
| SourceRoots 范围拒绝 | 用独立源根外哨兵目录作允许根，Dry Run 返回 scope_validation_failed；源五文件哈希不变，隔离为空 | PASS |
| 正常 Dry Run | executed 0 / skipped 1 / failed 0；五个源文件哈希不变，隔离为空，执行 Journal 0 | PASS |
| 正常实际隔离 | 指定副本从源消失，隔离区有一份原始 SHA-256/大小一致的文件；其他四源文件、哨兵不变；Journal done，item QUARANTINED | PASS（文件与 Journal） |
| 正常隔离计划终态 | GUI VERIFIED、executed 1 / failed 0，DB operation_plans 仍 APPROVED，执行中心仍可选 | **FAIL** |
| 正常恢复闭环 | 分别创建 DRAFT、批准 APPROVED、试运行与实际恢复；五源文件全部恢复原 SHA-256/大小，隔离文件 0；item/restore plan RESTORED，Restore Journal done；无额外源副本 | PASS（文件与恢复状态）；原隔离计划仍 APPROVED 的缺陷不解除 |
| 源文件变化 | 已批准且 Dry Run 成功后，只修改有独立备份的可丢弃目标；执行 stale_check 拒绝动作，源未移动，隔离/Journal/item 均 0，非目标与哨兵不变 | PASS（源保护） |
| 源变化结果与批准失效 | GUI 最终 DRAFT，却 toast/summary executed 1 / skipped 0 / failed 0；DB 仍 APPROVED | **FAIL** |
| 恢复目标已存在 | 隔离与恢复批准后，在原路径创建不同内容的可丢弃冲突文件；试运行及实际恢复均 destination_exists；冲突文件及隔离原文件哈希不变，Restore Journal 0 | PASS |
| 隔离内容变化 | 独立隔离并批准恢复后，只改变有备份的可丢弃隔离文件；试运行及实际恢复均提示计划过期/文件变化；源路径仍缺失，隔离文件保留变化后 SHA-256，Restore Journal 0 | PASS（拒绝写入） |
| Audit / Journal 一致性 | GUI 审计可见隔离 done 和 VERIFIED 审计详情，路径遮罩；恢复完成/拒绝以只读 DB 补充。但隔离计划 durable APPROVED 与审计 VERIFIED 不一致 | **FAIL（总体状态一致性）** |

正常计划 `dup-892454dba3ba`，隔离项 `q-4fd98ea0390885a88739baff`，恢复计划 `restore-54c4e42482a71ae790febef0`。恢复目标碰撞与隔离内容变化负例保留各自 APPROVED 恢复计划及 QUARANTINED 项，这是拒绝后的实际现场，不是完成状态。人为改变的负例字节、原始备份和失败 DB 全部保留，未删除/覆盖现场来制造闭环通过。

## 缺陷定位与修复边界

正式 RC 的 `internal/executor/executor.go` 在 stale 分支仅将内存计划转回 DRAFT，返回时未设置 Err/ErrorType。`internal/app/execution.go` 按 Err 是否为空累计 Executed，并没有将返回的最终计划状态持久化。这与两份独立 GUI/数据库证据一致：

1. 成功隔离后，Journal/item 有正确实际状态，但计划 durable state 未变为 VERIFIED。
2. 陈旧目标拒绝写入后，被错误计为成功，计划 durable state 未失效。

修复任务要求显式结果分类、终态持久化与错误处理、陈旧失效后人工复审、重复执行/重载/持久化失败回归，并保持 Journal-before-write、源根/符号链接检查、回滚和隐私语义。没有在这份文档 PR 中修改代码。

## 限定窗口隐私

采集同一正式 App PID **82840** 的 stdout/stderr、统一日志，并读取四份最终一致性快照中的持久化 GUI job events 和 operation_logs。检查 **76** 个路径/文件名/业务 canary 标记及 **262** 个编码、转义变体。

| 表面 | 实际覆盖 | 命中 |
|---|---:|---:|
| App stdout | 0 bytes，持续重定向 | 0 |
| App stderr | 0 bytes，持续重定向 | 0 |
| App PID 统一日志 | 162,461,915 bytes，完整 JSON 解析 **140,800** records | 0 |
| log stream stderr | 0 bytes | 0 |
| 四项目持久化 GUI job events | 44 rows | 0 |
| 四项目 operation_logs | 10 rows | 0 |

**限定窗口运行日志标记检查 PASS。** 空 stdout/stderr 是实际空流，错误证据来自 GUI/DB，不能宣称测试过不存在的输出。只覆盖指定 App PID，不代表 WebKit 子进程、全系统进程或长时隐私 Gate 已通过。

GUI 计划、文件证据、Journal 显示遮罩路径；执行中心可编辑 SourceRoots/QuarantineRoot 输入必须展示操作员填入的路径，私有原始截图/AX 也含这些输入。不得将输入框的路径与私有 operational DB 路径字段称为日志泄漏，也不能声称所有 GUI 表面均无路径。原始截图、AX、DB、源备份、测试标记、日志及证据索引仅保存在本机 owner-only 档案；公开记录仅包含匿名场景、数量、状态与证据摘要哈希。

| 私有证据匿名标签 | SHA-256 |
|---|---|
| 正常恢复最终 DB | `0b98330db97c70ebd72d5952415452f087b96bd8f8cf0284285861ad34ed1a56` |
| 正常恢复文件/状态快照 | `3a369cb29840f4dcb9a40c12c4a65143dca9dd167c26328fc2595ec456969d69` |
| 源变化拒绝最终 DB | `cb285c1f1307ff580ab8d2db70caa148d0f7bae35da4b868090be3f9f3eac26f` |
| 恢复目标冲突最终 DB | `7bacfaaa78c2261bf30c514b9d9093575721ac8d48a6b3a28069e10f9cf68d04` |
| 隔离内容变化拒绝最终 DB | `c3a045bba86fa1607cfe52a9bcc01ca2c797517ca68d16b30248a01d2be4c46b` |
| 正常恢复 GUI 截图 | `6dd1f4bae5bdefe26ef3eaaf62cd560ba39a1b2f3f5c5bc24d8206f52c1766b4` |
| App PID 统一日志 | `eb18396fa287796ebc63f40e907473441f0f0d2ea9321e07efcdf03d84fb8381` |
| 隐私检查汇总 | `d19b1b563207a3cad2ad7e6b31d8e1e0fb04e19a9a0e62b48fec2150aeb41bbc` |

## 限制与后续门禁

- 未修改数据库来模拟审批 digest 失效；GUI 未提供时间型过期审批操作，本次该子场景 **NOT RUN**。源变化导致的批准失效要求已实际暴露 FAIL。
- 未直接构造 managed HOLD item，其恢复流程 **NOT RUN**；后端允许 HOLD 项恢复，HOLD 限制永久清理。当前 GUI 仅对 QUARANTINED 项显示创建恢复草案；受保护计划 Gate 与它分开记录，不将允许的 HOLD 恢复误判为应拒绝操作。
- 恢复拒绝没有开始文件动作，因此无 Restore Journal；未声称 GUI 审计列出不存在的恢复失败 Journal。实际拒绝以私有 GUI 文本/截图、前后文件哈希和 DB 验证。
- 本轮结论限于本机可丢弃 APFS 工作流；正式 SMB Resume 的此前 PASS 另见[独立记录](beta.6-formal-artifact-smb-acceptance-2026-10-09.md)，不能混作 SMB 隔离/恢复证据。
- Crash Recovery / Recovery Lock **NOT RUN，前置验收 FAIL 阻断**；Purge **NOT RUN**。下一步仅推进 Issue #62 的独立修复，不提前崩溃测试或清理。
- beta.6 全新 Mac 离线首装仍 **BLOCKED**，长时及更广进程覆盖 **NOT RUN**，`source-map-js` high advisory 安全处置仍未关闭。本轮无依赖修改或安全例外判定。
- Release **Draft**，Public Beta **BLOCKED**。beta.5 历史 FAIL、beta.6 不可变 Tag/DMG及所有失败证据保留。

## 独立复核

本轮使用独立只读审查核对正式 RC 源码、私有 GUI 记录、各项目一致性快照、文件哈希及隐私结果。最终复核结论与这份文档提交身份记录在验收 PR；源码分析、自动化门禁与实际发行物证据分别列示，不以 CI 代替正式 GUI 验收。
