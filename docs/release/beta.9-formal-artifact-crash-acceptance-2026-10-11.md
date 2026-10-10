# beta.9 正式发行物 Crash Recovery 验收 — 2026-10-11

## 冻结身份与证据层

- Issue #72 修复 [PR #73](https://github.com/FNB2026/nas-data-governance/pull/73)，exact HEAD `ae1644ef88863784238cb3a845dc481ed66f2bc6` 独立 APPROVE；CI `38067959502` / Security `38067959358` SUCCESS。
- 修复 merge `40cd859afe4bb7eed5e80036fd052ce11bb43ba2`，post-merge CI `38068259112` / Security `38068342372` SUCCESS。
- 版本 [PR #74](https://github.com/FNB2026/nas-data-governance/pull/74)，exact HEAD `5f5c0b7b5519dcdfbd33253721117edfb94a0aea` 独立 APPROVE；CI `38068347454` / Security `38068347407` SUCCESS。
- 最终 RC `af4c5902eb431833e72d9024ccdee6e6ea678f97`；post-merge CI `38068649407` / Security `38068652302` SUCCESS。
- 新 annotated Tag `v0.5.0-beta.9` object `2d5f3c91094d3256331dd45a88f1e0a89fc6a0f3`，peel 到 RC。Tag 创建使用仓库现有 authorized bypass 权限；未移动任何旧 Tag。
- 正式 Release workflow `38068923209`：SUCCESS（Verify、Build、Developer ID 签名、公证、Staple 与 Draft 上传）。

## 安全执行边界

全新人工 disposable 四文件夹具：一对重复候选各 536,870,912 字节、一个普通单例、一个保护控制；独立备份哈希与源一致，隔离目录初始为空，项目必须由正式 GUI 新建。原 beta.8 数据库、部分目标、隔离副本、备份、截图与日志保持原样。

全部原始路径、文件名、隐私标记、数据库快照和日志留于本机私有归档。恢复执行与文件人工保留分开处理；不伪造 Journal、不清锁、不删除未确认输出。正式故障注入在只读观察到 pending 与部分输出后，仅对已核验的测试 App PID SIGSTOP/SIGKILL；不是自发闪退。

## 正式发行物身份

- 官方 Draft DMG：6,378,850 字节，SHA-256 `6c3b301256dcb81142c1af79225c6d92447e7f7acaa65ad7e3c0ad3997c137aa`，与官方 checksum 资产一致；CycloneDX / SPDX JSON 资产存在且可解析。
- DMG 签名、Staple 验证、Gatekeeper open 检查 PASS；从只读挂载安装独立 App 后，deep/strict 签名及 Gatekeeper execute PASS。
- App Identifier `com.fnb.ndg`，Developer ID Team `A2DYS82NLA`，hardened runtime；Gatekeeper 显示 Notarized Developer ID。
- 正式 GUI About：`0.5.0-beta.9`、完整 RC `af4c5902eb431833e72d9024ccdee6e6ea678f97`、构建时间 `2026-10-10T16:49:13Z`、channel beta。
- 不将本机安装启动扩大为全新 Mac 离线首装 PASS；未声称 App 具有独立 Staple ticket。

## 正式场景状态

| 场景 | 结果 | 实际证据 |
| --- | --- | --- |
| 发行物身份/签名/公证/Gatekeeper/About | PASS | 官方 Draft DMG 与实际签名 GUI 身份一致 |
| D 实际部分 Restore 目标 | PASS（触发） | pending + 8,421,376 字节目标 + 536,870,912 字节完整隔离副本，观察器仅终止测试 App |
| D 保锁/重开/重复 Recover/脱敏审计 | PASS | 两次 GUI Recover 均未结案；项目重开、App 重启仍 pending / APPROVED / 禁写 |
| D 独立保留部分目标后安全结案 | PASS | 原样 rename 保留，同 inode/字节/hash；GUI Recover 后 durable ROLLED_BACK / rolled_back / 清审批 / pending=0 / 解锁 |
| D 新审批完整 Restore 闭环 | FAIL（GUI 入口） | 隔离项仍 QUARANTINED，但操作列只有“已回滚”，无法创建新草案或重新审批 |
| A pending / 未确认动作 | NOT RUN | 本轮尚未针对性正式复验 |
| B 无 Journal / 中断计划 | NOT RUN | 本轮尚未针对性正式复验 |
| C 确认动作 / 未最终结案 | NOT RUN | 本轮尚未针对性正式复验 |
| 两个限定连续日志窗口隐私 | PASS（限定范围） | 67 个标记变体零命中；两个窗口之间存在明确不覆盖区间 |

### D 实际安全结果

全新 GUI 项目自然扫描完成 4 个文件、1 个重复组；只隔离一个批准的重复候选，普通单例及保护控制未被写入。正常隔离后原目标不存在，完整隔离 SHA-256 与独立备份一致。

恢复计划 `restore-20506a84467b2dbd5102fdb8` 经 GUI 草案、审批、Dry Run 后实际执行。在持久化 Journal pending 且目标部分写入时暂停并终止测试 App；这是授权故障注入，不是产品自发闪退。

正式 GUI 两次恢复均报告“确认回滚 0、未完成 1”，恢复锁保持。实际 GUI 审计可见 `manual_reconciliation_required / failed / APPROVED`。只读 SQLite 与哈希复核：pending 保留，两侧文件未被改动；项目重开及 App 重启后仍禁止新扫描和执行。

人工保留与产品 Recover 分开执行：先在全新独立保留目录内原样保存部分目标（同文件 inode、大小与 SHA-256），确认完整隔离副本和批准目标缺失；没有直接修改数据库或 Journal，也没有删除部分输出。随后正式 GUI Recover 报告“确认回滚 1、未完成 0、恢复锁已解除”。只读数据库证实 Restore Plan ROLLED_BACK、Journal rolled_back、approval_digest 空、approved_at NULL、pending=0；隔离项 QUARANTINED、隔离副本完整、保留部分目标不变。

### 新 GUI 阻断（[Issue #75](https://github.com/FNB2026/nas-data-governance/issues/75)）

在正式 App 的“隔离与恢复”页面，唯一隔离项状态为“已隔离”，但操作列仅显示“已回滚”，没有“创建恢复草案”、审批或执行入口。没有通过 API 注入、修改数据库或旧审批复用来绕过。

冻结源码 `ExecutionCenterPage.tsx` 对存在任意 restorePlan 的隔离项隐藏创建入口；仅 DRAFT / APPROVED 显示后续按钮，ROLLED_BACK 只显示终态。这支持入口阻断的原因分析，不能替代上述实际 GUI 观察。后端自动化的新计划路径通过，不等于正式 GUI 闭环通过。

## 日志与隐私边界

统一日志采集先于 App 启动，predicate 覆盖 NDG 及匹配的 WebKit process/subsystem；stdout/stderr 按 App PID 分段。健康采样每约 0.5 秒记录日志进程存活、App PID 和文件增长。

- 窗口 1：UTC `2026-10-10T16:52:17.966392Z` 至 `18:52:45.594211Z`；14,265 次健康采样均见采集存活，最大间隔 0.578 秒，38,116 条统一日志，两个 App PID 段。到预设两小时上限正常退出，包含实际中断及首次阻断恢复。
- 窗口 2：UTC `2026-10-10T21:34:35.036798Z` 至 `21:36:59.093962Z`；286 次健康采样均见采集存活，最大间隔 0.506 秒，12,904 条统一日志；明确 STOP，采集退出 0。覆盖 App 重启、部分目标保留、正式 Recover 结案及入口阻断观察。
- 两个窗口之间没有连续统一日志证据；不能靠事后补取填补。第二次重复恢复与关闭/重开项目的操作有 GUI/DB/文件证据，但不纳入窗口 1 的连续日志声明。
- 两段采集内容对实际测试路径、文件名、标记及编码变体共 67 项检测零命中。仅是选定进程/子系统及限定时间窗口的隐私 PASS，不是全系统或长时 PASS。
- GUI 非编辑显示采用路径脱敏；批准根目录编辑框中的实际测试输入属于私有原始证据，不能声称所有 GUI 输入均隐藏。公开记录不含实际路径、文件名、测试业务锚点或原始日志。

## 当前结论

Issue #72 后端“部分目标被错误结案并解除锁”的原缺陷在 beta.9 正式 App 中未复现，保锁与经验证的安全结案通过。但同一正式 GUI 无法从 ROLLED_BACK 创建新恢复草案，完整 D 闭环 FAIL；不能关闭 Issue #72 或将 Crash Recovery Gate 判为 PASS。A/B/C 仍 NOT RUN。

beta.8 历史 D = FAIL，保留原始现场和资产。beta.9 资产保持不可变；新入口缺陷应独立最小修复、重新冻结候选并复验，不改写本次失败结论。Public Beta BLOCKED，所有 Release Draft，Purge NOT RUN。
