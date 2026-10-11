# beta.8 Crash Recovery 专项 — 2026-10-10

## 身份与门禁

历史证据 PR #68 合并 `a1a142cc577203bb0272656fc7cafc94017e3efb`；其 INCONCLUSIVE 及 beta.7 GUI 反馈 FAIL 保留。原数据库、部分输出、截图、日志未修改。

Issue #67 最小反馈修复 PR #69：精确 HEAD `92453e10340eac41ce6f14ed484e08910a2960f9`，独立 APPROVE，CI `38044521671` / Security `38044521670` SUCCESS；merge `b7d40101384778832225c13e8e4e813113a778e3`，post-merge CI `38044753874` / Security `38044756384` SUCCESS。

版本 PR #70：精确 HEAD `1413a8c5895c5c4196af3d751ab4611745cb3009`，独立 APPROVE，CI `38044813168` / Security `38044813173` SUCCESS。最终冻结 RC / merge `6644c2ba520173be27a33c53c241e95241563c25`；post-merge CI `38045031593` / Security `38045038213` SUCCESS。新 annotated `v0.5.0-beta.8` Tag object `55c7c2f6ad994e058b6f08b4bc9f4a5cfa7b25be`，发行流水线 `38045311272`。远端既有 actor bypass 权限允许创建此新 Tag，push 返回 creation restriction bypass 提示；没有调整仓库规则或移动旧 Tag。

正式流水线 `38045311272` SUCCESS。从 Draft 下载的 DMG SHA-256 `6f8d9ee26e970a629547cbbbdfd00c2933f9c03508439c224e0c4d4907394d2d` 与发布校验文件一致；CycloneDX / SPDX SBOM 均存在。Developer ID 签名、hardened runtime、App codesign strict/deep、App / DMG Gatekeeper、DMG staple validation 通过。正式 GUI About：`0.5.0-beta.8` / Commit `6644c2ba520173be27a33c53c241e95241563c25` / `2026-10-10T10:37:20Z` / beta。App 本体没有独立 stapled ticket；本轮证明的是已公证并 staple 的 DMG 与本机在线 Gatekeeper，不扩大为全新 Mac 离线首装 PASS。Release 保持 Draft，Public Beta BLOCKED。

## 独立测试层补证

新增 `internal/executor/recovery_acceptance_test.go`，仅使用新临时 SQLite、人工字节和独立备份，不进入冻结 RC、不访问历史现场。`go test -race -count=1 -v ./internal/executor -run TestRecoveryDurableAcceptance` 四案例 PASS。

- A pending/partial：重开数据库及重复恢复后 EXECUTING、lock count 1、pending Journal、未回滚；没有完成审计，部分输出及源/备份完整。
- B 无 Journal：EXECUTING 和实际 pre-Journal STALE_CHECKED 两种状态均 durable DRAFT，审计 failed / DRAFT / approval_invalidated，旧 APPROVED 条件请求被拒绝，二次恢复无任务。
- C done 未结案：durable ROLLED_BACK，done Journal 的 rollback_status=done，审计 ok / ROLLED_BACK / recovery_rolled_back；源 SHA-256/大小及独立备份一致，隔离输出消失，二次幂等，执行锁依据清零。

独立复核 APPROVE 文件 SHA-256 `b9a582b86c0311df107722edb590dd14cc543fde6bcdb23029deeaca0230f081`；本机目标日志 SHA-256 `f7543a3c409461d4258fad8956b1993dad02045f521713492603444793d49851`。原始输出保存在本机，以上仅记录脱敏结论。

D 既有 `TestRestoreRecoveryCrashContract` 五形态 PASS：仅隔离、仅恢复目标、两处、两处不匹配、部分恢复目标。两处/不匹配保锁；仅一处完整按既有语义回滚。部分目标被保留且现有后端回滚并解锁，这项测试只陈述当前语义，不能作为完整正式数据一致性 PASS。

## 正式运行层

全部使用新人工数据、独立项目、独立备份和正式签名 beta.8。通过 GUI 分离扫描、复核、审批、Dry Run、实际执行与恢复；读取 SQLite 时使用只读连接。观察器只在已批准测试操作的真实 Journal / 文件系统窗口冻结并终止对应 App PID，不伪造 Journal、不修改数据库。

| 场景 | 状态 | 实际证据与限制 |
| --- | --- | --- |
| A pending / 部分输出 | 功能 PASS | pending Journal 与部分隔离输出的真实窗口中断；重启后两次 GUI 恢复均提示安全阻断和人工核对；EXECUTING / pending / 锁仍保留，四个源文件完整，部分输出未变；无虚报成功 |
| B 无 Journal | PASS | 真实 STALE_CHECKED / Journal 0 窗口；恢复后 durable DRAFT，approval_invalidated 审计，GUI 要求重新审查；执行中心无已批准可执行计划，源四文件及隔离空目录未变化 |
| C done 未结案 | PASS | 真实 EXECUTING / done 1 / pending 0 窗口；正式 GUI 回滚后四个源文件 SHA-256 / 大小一致，隔离输出消失；ROLLED_BACK、rollback_status=done、recovery_rolled_back 审计与解锁一致；重开项目仍一致 |
| D Restore 中断 | FAIL | 人工点击正式 GUI Execute，真实 pending + 7,634,944 字节部分目标窗口中断。重启先正确保锁；正式 Recover 随后报告回滚成功并解锁，但部分目标仍在。完整 536,870,912 字节隔离副本、其它三文件和四份独立备份完整；不得称原始内容丢失 |
| 连续限定窗口隐私 | 限定窗口 PASS | 第二窗口 61 个路径、文件名及标记编码变体零命中；不覆盖第一窗口结束后的 A 中断，不证明 D 尚未执行步骤，不扩大为全进程或长时隐私 |

### 采集连续性限制

第一窗口于 `2026-10-10T10:41:09.966640Z` 开始，因 45 分钟上限于 `11:26:11.376324Z` 结束。A 的实际中断发生在该窗口结束后，不能以其日志或事后补取宣称覆盖 A 全流程。A 功能结论与该窗口隐私 INCONCLUSIVE 分开记录。

第二窗口先启动统一日志，再启动正式 App，覆盖 B、C 实际中断及重启，以及 D 的准备。采集包含 App stdout/stderr 与按 NDG / WebKit process 或 subsystem predicate 的统一日志；PID 切换记录在私有归档。第二窗口 `2026-10-10T15:27:12.225331Z` 至 `15:43:38.659029Z` 主动结束，log exit 0；1,958 个健康样本全部 log_alive，最大采样间隔 0.658044 秒。App PID 88442 → 91117 → 93245 记录完整。App stdout/stderr、采集 stderr 与统一日志针对 61 个原始/URL 编码/JSON 编码标记变体零命中。GUI 原始 AX 与截图保存在私有归档，显式配置输入不等同于普通日志泄漏。此 predicate 不能证明全部 WebKit 子进程或全系统事件均被覆盖，不扩大为全系统或长时隐私。

原 beta.7 与各 beta.8 崩溃现场分别保留。原始数据库、源文件、部分输出、截图和日志只在本机私有归档；此文不包含真实路径、文件名或隐私标记。不启动 Purge、不增加功能、不发布 Release。整体 Crash Recovery Gate 尚未关闭。

### D 独立源码复核风险（非正式运行结论）

冻结 RC 的 `internal/executor/restore.go` 159–186 将隔离副本完整、恢复目标不匹配归入同一分支，保留目标字节后调用 MarkRestoreRolledBack。`internal/store/restore.go` 172–195 持久化 ROLLED_BACK 并将 Journal 从 pending 排除；Wails recovery status 在其它 pending 类别均为空时解除锁。源目标缺失、部分写入、内容变化和读取错误不能仅由“不匹配”证明回滚已完整完成。

独立只读复核：这是源码层已确认风险，正式 D 尚未实际观察。若运行中出现部分目标保留同时 ROLLED_BACK、解锁和成功提示，则 D 的回滚一致性与不确定态保护为 FAIL；完整隔离副本仍在可以单独证明源内容可恢复，不能扩大为完整回滚 PASS，也不能据此称原始字节丢失。不在 Issue #67 的最小 GUI 修复中扩大修改后端；正式证据取得后另立修复任务。

## 2026-10-11 D 正式补验

重新开启独立第三采集窗口后打开 D 项目，重新设置批准源根 / 隔离根、Dry Run 通过；人工点击正式蓝色 Execute。观察器冻结确实处于 pending Journal 且部分输出存在的 App PID 24210，保存快照后主动 SIGKILL。这是预先授权的故障注入，不是产品自发闪退。部分目标为 7,634,944 字节（SHA-256 `870e6b2346a69046adb64c5116e9960e18889266b6c8cf20b201b2c1ae141be5`）。

采集内重启 PID 25851。正式 GUI 首先显示恢复锁 1、隔离还原待办 1，并禁用新扫描与执行。随后 GUI Recover Quarantine Restore 返回“确认回滚 1 / 未完成 0 / 已确认回滚 / 恢复锁已解除”及绿色成功。

只读数据库：restore plan ROLLED_BACK，restore Journal rolled_back；同一部分目标大小与 SHA-256 未变化，完整隔离副本仍为 536,870,912 字节，SHA-256 `c0ccba495bbd4ac34bb240e0ec346c5ea5defed492dbd2824fa585e37311196e` 与基线相符。其它三个源文件、四份独立备份和源根外 sentinel 均未变化。恢复前目标不存在，因此这份部分输出属于未撤销的恢复副作用。

**D 回滚一致性、未确认状态保锁和成功反馈 FAIL；完整隔离副本保护 PASS。** 不删除部分目标、不修改 Journal / 失败数据库，不复用审批继续写入，现场保留。此新后端问题与已合并 Issue #67 最小 GUI 分类修复分开归档。整体 Crash Recovery FAIL，Release Draft / Public Beta BLOCKED。

第三窗口：`2026-10-10T16:01:07.798905Z` 至 `16:03:14.756275Z`（北京时间 2026-10-11 00:01:07 至 00:03:14），App 启动前开启采集，覆盖 Execute、中断、重启、锁及正式恢复核验。252 个健康样本全部 log_alive，最大间隔 0.507343 秒，主动结束 log exit 0；19 个 D 路径/文件名/业务标记变体在 stdout/stderr 及选定统一日志零命中。限定 D 窗口隐私 PASS；不证明全部进程、长时窗口或全新 Mac 离线首装。

独立只读复核确认 D 的快照状态转换、原始 AX 成功/解锁及未变化的部分目标和隔离副本，结论为新的后端 Release Blocker；Issue #67 GUI 是按后端错误终态分类，不应将本轮直接归为 #67 修复未完成。恢复审计列表仍仅原三条 quarantine / KEEP / stale_check，Restore recovery 审计闭环未证明。临时运行摘要 expected_bytes=null 是取错字段的采集瑕疵；canonical restore plan expected_size 与 Journal file_size 均为 536,870,912，原摘要保留不改造，以只读 DB 证据为准。

独立修复任务：[Issue #72](https://github.com/FNB2026/nas-data-governance/issues/72)。保留 beta.8 不可变发行物和实际失败证据，后续修复与新候选复验单独执行。
