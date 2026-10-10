# beta.7 Restore 错误分类证据契约评估

状态：方案 A 已由用户在 2026-10-10 明确认可，分类补证已执行，最终裁定见正式验收记录。本文保留提案时的缺口与采用条件，不将测试称为正式 App 原始字段直取，不解除 Public Beta BLOCKED；Release 保持 Draft。

## 身份与规范

- 正式 RC：`b09166f4ef5f7a4c9fa899a3c1370163fa373e62`；评估时 PR #66 HEAD：`34acf9fa5b740de30376aa7a213b1620b03d228d`。两者仅相差正式验收文档，产品源码一致。
- 执行手册 QA-8 要求实际隔离、SHA-256、Journal、隔离项及原路径恢复闭环和 HOLD 状态，未明确要求 GUI 直接显示原始错误码。
- beta.7 候选文档要求正式 GUI 安全拒绝与字节保护，源码测试不能替代正式操作。
- 用户要求必须证明实际返回 stale_detected，不能仅凭通用失败提示判通过；PR #66 当前选择直接取得原始 `error_type=stale_detected / status=failed` 作为关闭方法。事实要求与取证方法应区分，现有证据不能证明通用规范已将原始两个字段直取规定为不可变要求。方案 A 改变当前关闭方法仍须负责人明确认可，不能自行解释为已批准，并须保留历史 INCONCLUSIVE。

## 接口链核对

冻结源码中，RestoreExecutor 的 ValidateRestore 以 StepFailed 开始；隔离内容大小或 SHA 不符返回 stale_detected，ExecuteRestore 在错误时立即返回，尚未 BeginRestore 或移动文件。

QuarantineService 保留 RestoreResult，同时返回包含分类的 Go error。Wails ExecuteRestore 的错误分支返回空 DTO 和 Go error；正常 DTO 的 status/error_type 在此分支不会作为结构化响应返回。前端 ApiError 保留 raw，但 GUI 显示 friendlyError；其 `includes("stale")` 映射只能证明 stale 类提示，不能唯一证明原始枚举及 status。普通运行日志没有保证记录这两个字段。扩大日志采集不是可靠补证方法。

## 已有证据与缺口

| 层次 | 已取得 | 不能据此宣称 |
| --- | --- | --- |
| 正式 App | 第六组实际 GUI 执行的即时 AX/截图及 stale 中文提示 | 直接捕获原始错误码/status |
| 文件及持久化 | 变更隔离字节不变、原目标缺失、非目标与根外不变；Journal0、QUARANTINED、恢复 APPROVED、源计划 VERIFIED；重复拒绝无新增项 | 由 Journal0 反推出特定错误分类 |
| 限定隐私 | 203 秒连续采集，27,864 事件、201 变体零命中 | 历史缺口被消除、全系统/长时或 WebContent 所有行为通过 |
| 静态源码 | 上述分类及错误传输链 | 同一次正式运行实际返回了特定原始字段 |
| 现有自动化 | Restore 正常执行/Dry Run、错误 digest、缺参和完成持久化失败回滚测试 | 隔离内容变化的实际 ExecuteRestore 精确分类已被回归测试覆盖 |

提案阶段全仓库测试搜索发现，现有明确断言 stale_detected 的 executor、app consistency 和 drill 测试针对源隔离执行，不能借用为 Restore 内容变化分类证明。当时尚未找到 Restore 内容变更负例同时断言 ErrorType、StepFailed 及写入前拒绝的测试。测试总体成功不能关闭这一缺口；后续新证据专用测试见下一节，未修改签名 App、原失败数据库或现场。

## 方案 A 的采用条件

拟将该单项标准修订为“实际正式 GUI 拒绝及字节/状态保护 + 冻结产品源码的独立后端分类回归 + 错误传输映射审查”。该组合证明不同层次，不宣称直接捕获原始正式运行字段。

采用前须满足：

1. 验收负责人明确批准仅此证据标准变更，记录批准时间、范围及限制；独立审查批准技术充分性不代替负责人采用决定。
2. 独立、可复现的测试绑定冻结 RC 产品源码，使用新临时文件与新数据库，真实调用 ExecuteRestore（非只调用 ValidateRestore）。覆盖大小变化和同大小内容变化，断言非空错误、stale_detected、StepFailed、源目标仍不存在、隔离及非目标字节不变、无新 Journal/状态误完成，并覆盖重复拒绝。
3. 服务层测试验证真实执行错误与保留 Result 分类一致；Wails/前端证据明确空 DTO、错误传播及本地化的限制。不得把 mock 或静态分支阅读称为正式 App 原始码取证。
4. 测试仅作为分类层补充，正式 GUI、文件保护、审批/状态与限定隐私仍依据 PR #66 实测；保留旧候选 FAIL、历史日志缺口和 beta.7 原始字段 NOT_CAPTURED。
5. 独立复核上述组合及精确测试源码身份。只有所有条件闭合，才按修订标准重新裁定该限定 Gate；这不是 Public Beta 发布许可。

测试可放入独立证据测试载体，针对冻结源码运行；不得注入正式 App 或修改既有夹具/数据库。若产品源码改变，必须采用新的不可变发行候选，不能把改变后的测试对象归为 beta.7。

## 方案 B

若负责人坚持“同一次正式运行直接观察原始字段”为强制标准，应保留 beta.7 INCONCLUSIVE，通过独立最小接口修复 PR 使拒绝返回可观察的结构化 status/error_type，再生成新候选并正式复验。不能覆盖 beta.7 Tag/DMG，不在本评估中实施该方案。

## 决策建议

提案时优先建议方案 A，但当时既缺标准变更批准，也缺精确 Restore 负例分类回归，不能立即转 PASS。用户随后明确答复“认可方案 A，继续补独立分类测试”。批准范围是上述多层证据契约，不是宣称已直接捕获正式 App 原始码、发布许可或更广隐私通过。

独立只读复审认可上述方案的条件与测试缺口，并要求区分用户的实际分类事实要求和 PR 文档当前选用的直接取证方法。此认可仅为提案技术复审，不是方案采用、正式 Gate PASS 或发布批准。

## 独立分类补证

- 可复现载体：[测试](evidence/beta7-restore-classification/restore_contract_test.go.txt)、[运行脚本](evidence/beta7-restore-classification/run.py)、[脱敏结果](evidence/beta7-restore-classification/result.json)。脚本导出冻结 RC，添加仅测试文件，关闭 Go workspace 和 GOFLAGS 覆盖，显式使用 Go1.26.9；原始369个导出文件在运行后哈希全部不变。
- 真实 executor / service 各覆盖大小变化、同大小内容变化、未变正控制，共6子例。每个负例执行两次真实 ExecuteRestore（service DryRun=false），精确断言非空错误、stale_detected、StepFailed、FinalState APPROVED；源目标不存在、隔离字节不变、每次拒绝非目标/根外不变。只读 SQLite 证明 restore Journal0，源 Journal/隔离项/审计数量不增加，完整批准恢复计划与隔离项不变化、源计划保持 VERIFIED。
- 服务层同时断言 `restore failed: stale_detected` 和保留 Result。两个正控制真实恢复原字节，隔离路径消失、恢复计划与隔离项 RESTORED、唯一身份匹配的 done Restore Journal。避免“任何恢复都拒绝”的假阳性。
- 根执行最终载体：UTC `06:15:06.552103–06:15:11.365111`，go test -race -count=1，6/6 PASS、exit0；没有仅用 cached PASS。测试载体与 runner 的完整 SHA 在 result.json。
- 环境隔离加固时一次 GOTOOLCHAIN=local 使用系统 Go1.26.2，因低于冻结 module 的1.26.9而在测试前失败，0子例；修正为显式1.26.9后成功。该失败和此前运行日志均私有保留，不隐藏失败，不把它描述为产品缺陷。
- 测试是冻结源码的测试二进制，不是正式签名 App，不触碰历史验收数据库或真实 NAS。原始 stdout/stderr 仅私有归档。最终限定 Gate 还需对完整证据进行独立复核。
- 独立复核最终载体并再次运行：[独立结果](evidence/beta7-restore-classification/independent-result.json)，UTC06:15:21.358206–06:15:26.262426，6/6 PASS、race exit0、369原文件不变。复核 APPROVE，并认可采用方案 A 后正式 GUI/字节/状态与独立分类测试可共同关闭限定 Restore 分类门禁；不是正式 App 原始字段直取或 Public Beta 许可。正式判定见验收报告末节。
