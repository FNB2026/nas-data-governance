# beta.8 Crash Recovery 专项 — 2026-10-10

## 身份与门禁

历史证据 PR #68 合并 `a1a142cc577203bb0272656fc7cafc94017e3efb`；其 INCONCLUSIVE 及 beta.7 GUI 反馈 FAIL 保留。原数据库、部分输出、截图、日志未修改。

Issue #67 最小反馈修复 PR #69：精确 HEAD `92453e10340eac41ce6f14ed484e08910a2960f9`，独立 APPROVE，CI `38044521671` / Security `38044521670` SUCCESS；merge `b7d40101384778832225c13e8e4e813113a778e3`，post-merge CI `38044753874` / Security `38044756384` SUCCESS。

版本 PR #70：精确 HEAD `1413a8c5895c5c4196af3d751ab4611745cb3009`，独立 APPROVE，CI `38044813168` / Security `38044813173` SUCCESS。最终冻结 RC / merge `6644c2ba520173be27a33c53c241e95241563c25`；post-merge CI `38045031593` / Security `38045038213` SUCCESS。新 annotated `v0.5.0-beta.8` Tag object `55c7c2f6ad994e058b6f08b4bc9f4a5cfa7b25be`，发行流水线 `38045311272`。远端既有 actor bypass 权限允许创建此新 Tag，push 返回 creation restriction bypass 提示；没有调整仓库规则或移动旧 Tag。

正式签名、公证、Gatekeeper、资产与 About：待流水线及本机核验。Release 保持 Draft，Public Beta BLOCKED。

## 独立测试层补证

新增 `internal/executor/recovery_acceptance_test.go`，仅使用新临时 SQLite、人工字节和独立备份，不进入冻结 RC、不访问历史现场。`go test -race -count=1 -v ./internal/executor -run TestRecoveryDurableAcceptance` 四案例 PASS。

- A pending/partial：重开数据库及重复恢复后 EXECUTING、lock count 1、pending Journal、未回滚；没有完成审计，部分输出及源/备份完整。
- B 无 Journal：EXECUTING 和实际 pre-Journal STALE_CHECKED 两种状态均 durable DRAFT，审计 failed / DRAFT / approval_invalidated，旧 APPROVED 条件请求被拒绝，二次恢复无任务。
- C done 未结案：durable ROLLED_BACK，done Journal 的 rollback_status=done，审计 ok / ROLLED_BACK / recovery_rolled_back；源 SHA-256/大小及独立备份一致，隔离输出消失，二次幂等，执行锁依据清零。

独立复核 APPROVE 文件 SHA-256 `b9a582b86c0311df107722edb590dd14cc543fde6bcdb23029deeaca0230f081`；本机目标日志 SHA-256 `f7543a3c409461d4258fad8956b1993dad02045f521713492603444793d49851`。原始输出保存在本机，以上仅记录脱敏结论。

D 既有 `TestRestoreRecoveryCrashContract` 五形态 PASS：仅隔离、仅恢复目标、两处、两处不匹配、部分恢复目标。两处/不匹配保锁；仅一处完整按既有语义回滚。部分目标被保留且现有后端回滚并解锁，这项测试只陈述当前语义，不能作为完整正式数据一致性 PASS。

## 正式运行层

| 场景 | 状态 | 证据限制 |
| --- | --- | --- |
| A pending / 部分输出 | NOT RUN | 需新 App 实际中断、重启保锁及 GUI 反馈 |
| B 无 Journal | NOT RUN | 需实际短窗口、重审反馈与审批失效 |
| C done 未结案 | NOT RUN | 需实际短窗口、回滚、审计与解锁 |
| D Restore 中断 | NOT RUN | 需正式 GUI、实际部分输出及恢复一致性 |
| 连续限定窗口隐私 | NOT RUN | 需 stdout/stderr、统一日志、GUI、采集健康与进程范围 |

全新 disposable 夹具和独立备份已准备，尚未交给正式 App 执行。测试层四案例不替代正式运行层。无法自然捕获短窗口时保留 INCONCLUSIVE；没有获准采用组合证据关闭 Gate。不启动 Purge、不增加功能、不发布 Release。
