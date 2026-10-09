# beta.5 正式发行物：小范围 SMB 中断 / Resume 与异常隐私验收

## 结论

**本场景 FAIL；Public Beta 继续 BLOCKED。** 网络暂停与恢复状态链、持久化前缀安全和本次异常日志隐私得到实际证据；但完整哈希阶段中断后，Resume 没有补做前缀中的未完成重复候选。任务自然 `COMPLETED`，却只生成预期 3 个重复组中的 1 个。不能仅凭 terminal state 判定本场景通过。

本轮仅验收及更新文档，没有修改产品代码、版本、Tag、正式资产或 Release，没有开始 Disposable Quarantine / Restore。

## 身份与隔离

- 文档起点：PR #56 HEAD `fee5323db798d9fe1a400c3d74662a8c2b32781d`。
- 实际操作对象：已安装的正式签名 App；GUI About 为 `0.5.0-beta.5` / `8473dd630c4165389b32ffc85ae062145546a766` / `2026-10-02T12:13:59Z` / beta。签名 deep / strict 复核 exit=0。没有使用开发 App。
- 独立临时 SMB2 服务仅绑定 loopback 的专用端口，导出自行创建的可丢弃目录；使用临时工具环境中的 Impacket 0.13.1。没有更改系统 SMB 服务、网络配置、权限或真实 NAS 共享。
- 服务端 share 只读，客户端 smbfs 挂载也为只读；6 个普通文件，每个 1 MiB，总计 6 MiB，三对相同内容，无符号链接。创建夹具后冻结内容，验收结束逐文件 SHA-256 与夹具清单一致。
- 正式 GUI 创建独立本机项目和 Storage `src_3ccf01cb4a0c`；数据库位于本机项目目录，不在扫描根内。没有修改原百万级项目数据库。
- 中断只停止本轮拥有的测试 SMB 服务。既有 NAS 挂载在中断和恢复前后均保持原样；未卸载真实共享，未操作其他共享会话。
- 原始日志、夹具清单及测试服务脚本保留在本机私有验收归档，带 SHA-256 清单；公共文档只包含脱敏聚合。GUI 截图与操作树在验收对话中可见，未另存图片资产。

## 实际时序（2026-10-09，CST）

| 时点 / 任务 | GUI 与持久化证据 |
|---|---|
| 16:11:54，基线 `job-cb9dc8739bf66a2f732b6ee36b10dab1` | 正常扫描自然 COMPLETED / FINALIZING，发现 6、处理 6、失败 0；checkpoint 1 completed / 6，active=6。 |
| 16:13:08，中断对象 `job-557d23c2dfcae59cb203ddb4fde5ef7c` | GUI 启动全量扫描，并发 1。测试服务对每个 SMB READ 延迟 500 ms，以保证阶段可观察；不是 mock 或伪造数据库状态。 |
| 16:13:56.188 | GUI 与只读 DB 已观察到 RUNNING / FULL_HASHING，发现 6、处理 6；此时停止独立测试 SMB 服务。 |
| 16:14:12.212 | 自然 PAUSED_NETWORK / FINALIZING；GUI 显示“网络中断，已暂停”及“继续扫描”。发现 6、处理 6、失败 2，警告 1；checkpoint 2 paused_network / 6，active=6、missing=0、unavailable=0。 |
| 重挂载 | 重启同一测试服务、取消测试延迟；专用挂载在服务断开后已消失，重新 mount 成功、同一根目录可读。没有用“重启 App”代替重挂载。 |
| 16:15:51，Resume `job-d7ca3f0e5622e7e56f03e64bb0e7a645` | GUI 取消全量扫描选项并点击“继续扫描”，先观察到读取已保存进度，随后自然 COMPLETED；没有点击新扫描，没有取消任务。checkpoint 2 completed / 6，updated_at=`2026-10-09T08:15:51.553188Z`；Resume 入口消失。 |

Resume 是新 job，但复用 checkpoint 2。本轮发现 / 处理 / 失败均为 0，持久化进度为 `{}`：所有 6 个文件都在已保存遍历前缀内，没有新增遍历文件。有效文件覆盖为前缀 6 + 本轮新增 0 = 6，不能把计数相加解释成 12 个文件。

## 分项结果

| 验收项 | 状态 | 实际证据 / 限制 |
|---|---|---|
| 正式发行身份与安全隔离 | PASS | 正式 GUI About + 签名；独立 loopback 服务、只读 share / mount、独立项目，既有挂载未变。 |
| 正常扫描基线 | PASS | 6 / 6 / 0，自然完成，active=6。未额外保存基线时刻完整哈希 / 重复组数据库快照，不反推该快照。 |
| 实际中断与暂停 | PASS | 在完整哈希阶段断开服务，实际观察 PAUSED_NETWORK 和 paused_network / 6；没有靠取消模拟断网。 |
| 重挂载、Resume 自然终态 | PASS | 同一 Storage / 根目录，复用 checkpoint 2；COMPLETED、checkpoint completed、Resume 入口消失。仅为状态链通过。 |
| 文件覆盖与源安全 | PASS | 6 个不同索引路径、active=6、总大小 6,291,456 bytes；missing=unavailable=0，源六文件 SHA-256 清单全匹配。没有源写操作或错误 missing 推断。 |
| 未完成完整哈希 / 重复候选恢复 | FAIL | quick_hash=6/6；完整 SHA-256=2/6，已有两条哈希正确，另 4 个重复候选仍为空；快速哈希候选为 3 组，完整哈希组与实际 GUI 都仅为 1 组，夹具预期 3 组。 |
| 重复结果与证据 UI 可访问 | PASS | 可进入唯一现存组；目录语境 raw_source / 受保护 / 人工复核，物理身份不可靠并保守估算。可访问不代表重复结果完整。 |
| 本次异常事件 / 运行日志隐私 | PASS | 下节有界窗口及测试标记检查；不宣称未覆盖标记或所有异常场景均无泄漏。 |
| 长时压力 / 真实 NAS 全部故障类型 | NOT RUN | 仅小范围本机 SMB2 中断，不代替长时扫描、物理 NAS 断网、协议差异或所有网络错误。 |

## 异常期间隐私证据

采集窗口 `16:09:11–16:21:56 CST`，约 765 秒，覆盖正式 App GUI 项目创建、基线扫描、完整哈希中断、暂停、重挂载、Resume、重复组检查和暂停任务详情。实际 GUI 时间线与只读事件表相互对应。

- 新增持久化事件 95 条：created=3、stage=20、progress=68、warning=1、paused_network=1、completed=2。
- 事件载荷对临时源 / 项目 DB 完整路径、canonical 路径、文件名标记、主目录、挂载路径前缀、share 名、project_id 与 Application Support 等测试锚点均 0 命中。
- GUI 异常摘要只含：`category=scan_summary`、`coverage_state=partial`、`full_hash_failures=2`、`quick_hash_failures=0`、`scan_errors=0`、`missing=0`、`unavailable=0`；没有源路径或文件名。
- stdout=0 bytes，stderr=0 bytes；按正式 App PID 读取统一日志 exit=0，JSON 3,951 条 / 5,005,666 bytes，上述锚点命中 0。
- 开启路径脱敏：源输入、重复结果路径 / 文件名和证据检查器没有显示完整测试路径或文件名标记。此夹具未产生非空业务锚点，不能把本次结果外推为所有业务锚点异常日志场景通过；既有真实项目业务锚点脱敏证据另见最终发行物记录。
- SQLite 的私有结构化文件路径、项目定位字段仍用于正常功能，不把“事件载荷无泄漏”误写成“数据库不含路径”。原始日志不上传公共仓库，不清洗历史数据库。

## 根因核对与待办

实际 RC 与本轮文档起点的相关产品源码相同，已用 `git diff` 核对。以下是对源码与本轮 GUI / DB 结果的解释，不是另一次修复验证：

1. `internal/app/scan.go:278` 恢复最后未完成 checkpoint，读取遍历前缀与计数。
2. `internal/scanner/scanner.go:185` 跳过 `path <= ResumePath` 的文件；本例六文件全部被跳过。
3. `internal/app/scan.go:303` 的本轮 `files` 初始为空；`:459` 的完整哈希候选仅从该本轮集合生成，没有加入已持久化前缀中缺失完整哈希的候选。
4. `internal/app/scan.go:525` 将缓存前缀加入存在状态 reconciliation，正确保护了六文件不被误判 missing；`:547` 根据源可用性将 checkpoint 记为 completed。存在状态正确不能补齐内容哈希。

这是明确的重复候选恢复遗漏，不是“所有非重复文件都必须完整哈希”的新要求。夹具的六文件本来都属于三对重复候选。

下一动作：独立审查并处理“完整哈希中断后的前缀待办恢复”阻断项，再使用后续不可变候选重新验收；不得改写 beta.5 Tag 或覆盖正式资产。当前文档 PR 不修产品、不解除发布 Gate，也不以一次重新全量扫描掩盖失败现场。失败项目、事件与日志保留，可复核。
