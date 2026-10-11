# beta.10 Crash Recovery — 正式实测与限定组合证据

## 身份与边界

- 正式 RC：`c1941f84e2d1c836b57cd0750db1c2554ba6c4c9`，版本 PR #78 merge；annotated tag `v0.5.0-beta.10` object `aca5b1293010d483f889c326d61442088e21a8bb`。旧 beta.8 / beta.9 Tag、DMG 与失败现场不变。
- PR #77 修复 HEAD `45fc0d6a1aa1512f72c2636ebf235172396c0c61` 独立 APPROVE，merge `72e29171ec4f86c4b65bcfa65e0354cd90a8b865`，post-merge CI `38089509917` / Security `38089511461` SUCCESS。
- PR #78 HEAD `ea995de495134ab3d24c1a98822cd3953d7f9df9` 独立 APPROVE，CI `38089580800` / Security `38089580779` SUCCESS；最终 RC post-merge CI `38089886582` / Security `38089895345` SUCCESS。
- 正式 Release workflow `38090124629` SUCCESS：签名、公证、Staple、Gatekeeper、DMG/校验文件与两种 SBOM；Release 为 Draft/prerelease。Tag 创建返回授权规则 bypass 通知，已记录，不代表豁免后续门禁。
- 官方下载 DMG 为 6,379,595 字节，SHA-256 `27f4651d192c856a6297a8ca8761a0ec9ae230d1124c7cafe52d97086d7ad60d` 与发布校验文件一致。安装独立 App 后再次验证签名、Gatekeeper；bundle `com.fnb.ndg`、Team `A2DYS82NLA`、hardened runtime。实际 GUI About 为 `0.5.0-beta.10` / 完整 RC / `2026-10-10T22:09:04Z` / beta。

四组全新人工夹具，每组四个文件：两份 512 MiB 重复内容、普通单例、受保护对照；独立备份与根外对照。GUI 分别完成扫描（4/4、失败 0）、目录语境、用户决定、审批、批准根目录及 Dry Run。数据库均由实际 App 打开的句柄和 source root 绑定确认。未使用真实 NAS、百万级项目或原失败数据库；不运行 Purge。

## 正式 App 结果

| 场景 | 实际证据 | 判定与限制 |
| --- | --- | --- |
| A：未确认普通隔离 | 真实 `EXECUTING` / pending 1，Journal target NULL；隔离部分输出 56,295,424 字节。终止前冻结验证、只读 DB 快照；重启后 GUI 两次恢复均 unresolved 1、rollback 0，扫描/执行禁用；源与备份 4/4、根外及部分输出 SHA/size 不变 | 限定保锁与未知输出保护 PASS。operation_logs=0，不声称拒绝恢复已有持久审计、不声称回滚或已解除 A 锁；保留现场 |
| B：无 Journal | 观察器采样到 `STALE_CHECKED`，但 SIGSTOP 后冻结谓词不成立；SIGCONT 后自然 VERIFIED，未 SIGKILL | 正式 INCONCLUSIVE，永久保留；不能把采样等同有效崩溃 |
| C：完成动作未结案 | 观察器采样到 `EXECUTING/done`，但冻结谓词不成立；随后自然 VERIFIED | 正式 INCONCLUSIVE，永久保留；未取得崩溃后的正式 GUI 回滚证据 |
| D：Restore 部分目标 | 实际 pending Restore，部分目标 95,453,184 字节，完整隔离副本 536,870,912 字节；冻结确认后 SIGKILL。两次 GUI Recover 返回安全阻断，APPROVED/pending/锁与两侧字节保持；再次启动仍锁 | 限定安全保护 PASS，不能因文件部分存在就宣布回滚 |
| D：人工保留与新审批闭环 | 单独核验完整隔离副本与原始部分目标；原样 rename 部分目标进入独立保留区（inode/dev/size/hash 不变），没有写 DB/Journal。GUI Recover 安全结案旧计划 ROLLED_BACK、清除原审批、保留隔离项；GUI 新草案→独立审批→Dry Run→实际 Restore，自然 RESTORED | 完整限定闭环 PASS：新旧 Restore 计划、Journal、隔离项和审计一致，源/备份 4/4 SHA/size、根外与保留部分输出不变；重载不复用旧审批 |

A/D 字节、只读 DB 与恢复锁证据已由独立复核确认。历史 GUI/中断/rename 时刻由原 Computer Use 与私有取证支持，独立复核未重新观察历史时刻；D 最终隔离输出已正常移走，不能事后重读其历史完整副本。这些限制不隐藏。

## B/C 经明确批准的组合契约

人工明确批准仅对 beta.10 B/C 使用组合证据；A/D 仍以正式签名 App 实测为要求。正式 B/C 均维持 INCONCLUSIVE，以下测试层不会改写它们。

[复现 runner](evidence/beta10-crash/run.py) 从精确 RC `git archive` 导出独立临时源码，仅添加两个 test-only carrier。394 个冻结文件运行前后 SHA-256 完全不变，未注入或修改正式 App、历史 DB 或文件。工具链 `go1.26.9 darwin/arm64`；实际 `-race -count=1` 输出与哈希见 [result.json](evidence/beta10-crash/result.json) 和 [race.log](evidence/beta10-crash/race.log)。

- B carrier 在真实 executor 的 SQLite CAS 已持久化 STALE_CHECKED 后、BeginJournal 前 panic 截断，避免模拟错误走正常失败回滚；不是手工构造正式运行 Journal。
- C carrier 在真实 MoveFile 与真实 MarkJournalDone 提交后、executor/app 最终结案前 panic 截断。真实文件操作完整、源缺失、done Journal 得到核验。
- 关闭/重开 SQLite 后，经真实 RecoveryService 恢复；B `reset_to_draft` / DRAFT / approval_invalidated，C `rolled_back` / ROLLED_BACK / rollback_status done / recovery_rolled_back。精确审计状态与终态相符，恢复锁清除。
- 新临时 DB trigger 拒绝 B 状态、B/C 审计、C Journal 持久化；另 C 隔离内容被改。均 skipped/有错误、保留原预约状态和恢复锁，未知内容不删除。真实执行服务拒绝写入；重开和再次恢复不虚报成功。
- 恢复后的主源/目标、完整计划、Journal、审计在重复恢复与原 APPROVED 请求后再次校验；旧审批执行为零。独立备份、非目标与根外文件 SHA 不变。
- Adapter 的另四例以新临时 DB 明确种子状态，经真实 Wails `RecoverSourcePlans`、`CheckRecoveryLock`、全部写能力与真实批准/执行请求验证传播及阻断。它们是数据库重开和新 API 服务重连，不能称为正式 App 进程崩溃重启。原不合格计划可以合法返回空结果/执行零；独立 APPROVED control 必须因锁拒绝且无新 Journal。

新增载体只用于测试，产品源码、版本和发行资产不变。载体开发中曾修正 absence 断言、空不合格请求响应期待与新 control task 准备方式；不能将载体准备错误说成产品缺陷，最终输出不替代独立审代码与注入位置。

## 限定隐私窗口

采集先于 App 启动，UTC `2026-10-10T22:12:10.516540Z` 至 `2026-10-11T00:12:16.183200Z`，约 7,205.667 秒。统一日志正常达到两小时 timeout，exit 0，**不是人工提前停止**。14,303 个健康样本均 log_alive，最大采样间隔 0.525 秒；App 21926→25776→26617→38891 的 PID 切换明确记录。A/D/B/C 正式动作均在窗口内。App 此后未继续故障验收。

选定 App/WebKit process/subsystem predicate 得到 124,835 条日志事件与 1 条结束记录（共 124,836 个 JSON 对象，另 1 条非 JSON 头），其中 NDG 115,499、WebKit GPU 4,864、Networking 2,444，其他匹配 subsystem 的进程单独计数。各 App 段 stdout/stderr 与 log stderr 均为空。67 个路径/文件名/业务标记、169 个编码变体在采集日志零命中。四组 job_events 各 11；operation_logs D=7、A=0、B=3、C=3，所读事件/审计零命中。

限定窗口日志与所读事件隐私 PASS；不是全系统、全部 WebKit 子进程覆盖、长时隐私或全新 Mac 离线首装 PASS。原始 GUI 输入框、操作数据库、截图及证据含必要路径，仅本机私有归档，不公开。关闭且无 WAL/SHM 的 D/A/B DB 只在确认无打开句柄后采用 immutable 只读，读取前后 DB SHA 不变；C 保留 WAL，正常 mode=ro 读取，不用 immutable。

## 最终裁定

当前等待本证据 PR exact HEAD 独立复核及 CI/Security。仅所有证据闭合后，按明确批准的组合标准裁定限定 Crash Recovery Gate；不扩展为 Public Beta 许可。

Issue #72 / #75 暂保持 OPEN，beta.8/beta.9 历史失败不转 PASS。Release Draft、Public Beta BLOCKED、Purge NOT RUN。其余门禁（全新 Mac 离线首装、依赖安全告警、其他隐私与首发范围）单独收口。
