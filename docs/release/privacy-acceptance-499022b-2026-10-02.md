# 事件隐私与失败摘要验收 — main `499022b`（2026-10-02）

> 验收对象：主干 `499022b04ae57b9d82bfd58743aee214cc7d7c4c`（#49 squash 合并后；含 #48 的 Resume 修复与事件隐私/失败摘要修复）
> 验收性质：手册「当前下一步」第 1 项 —— 新构建 App 的事件隐私与失败摘要验收
> 结论：**代码级 + 启动运行窗口取证全部 PASS；历史记录按证据治理原则原样保留；GUI 真机复核项移交最终发行物验收**
> 本文不含任何真实本机路径；真实数据库路径一律以应用内目录名指代。

## 1. 构建与身份（已取证）

| 项 | 证据 |
| --- | --- |
| 构建来源 | `COMMIT="$(git rev-parse HEAD)" make desktop-build`，HEAD = `499022b04ae57b9d82bfd58743aee214cc7d7c4c` |
| 注入 Commit | 二进制内 `version.Commit=499022b04ae57b9d82bfd58743aee214cc7d7c4c`（完整 40 位；`strings` 计数 = 1） |
| VERSION | `0.5.0-beta.4`（未提升；新候选身份按规则待后续决定） |
| 工作树 | 构建后干净；`wailsjs/go/models.ts` 无漂移 |
| 主干 CI | run `36967806900` SUCCESS，headSha 与 `499022b` 严格绑定 |
| 修复内容核对 | `79bbc05`（#48 squash）：creation event 不再携带 `project_id`（`internal/jobs/manager.go`）；持久化层 `SanitizePayload` 将 `project_id` 列入敏感键（`internal/events/events.go`）；扫描失败/部分覆盖时持久化仅含计数的 `scan_summary` 警告（`internal/app/scan_job.go`）。#48 squash 前的身份 `33594b9` 内容已全部包含于 `79bbc05`。 |

## 2. 事件管道探针（已取证 — 真实持久化管道，代码级）

方法：临时探针测试（运行后删除，不入正式套件；永久回归由 #48 自带 `events_test.go` / `manager_test.go` / `scan_job_test.go` 承担）。探针走完整桌面管道 `ScanJobRunner → JobManager → 事件消毒层 → SQLite 项目 DB`，使用真实隐私锚点：project identity = 深层绝对 `governance.db` 路径（历史泄漏向量）；一个 `chmod 000` 文件触发真实快速哈希失败；一对重复文件触发完整哈希阶段。

持久化事件全表（12 条，`go test -v` 实录）：

```text
seq=01 type=job:created   stage=DISCOVERING   state=QUEUED     payload={"job_type":"scan"}
seq=02..04 type=job:stage stage=DISCOVERING   state=RUNNING    payload={}
seq=05 type=job:stage     stage=QUICK_HASHING state=RUNNING    payload={}
seq=06 type=job:stage     stage=FULL_HASHING  state=RUNNING    payload={}
seq=07..08 type=job:stage stage=FINALIZING    state=RUNNING    payload={}
seq=09 type=job:warning   stage=FINALIZING    state=RUNNING    payload={"category":"scan_summary","coverage_state":"complete","full_hash_failures":0,"missing":0,"quick_hash_failures":1,"scan_errors":0,"unavailable":0}
seq=10..11 type=job:progress stage=FINALIZING state=RUNNING    payload={"bytes_processed":0,"discovered":3,"failed":1,"processed":3,"total":0}
seq=12 type=job:completed stage=FINALIZING    state=COMPLETED  payload={}
```

断言全部通过：

- `job:created` 载荷**恰为** `{"job_type":"scan"}` —— 历史泄漏向量（`project_id`=项目 DB 完整路径）已封闭；
- 12 条载荷对以下锚点**全部 0 命中**：临时目录路径、项目 DB 路径、源目录路径、`Application Support`、`governance.db`、`/Users/`、`project_id`、三个源文件名；
- 真实哈希失败（EACCES）被计数（`failed=1`、`quick_hash_failures=1`）且任务 `COMPLETED`、`warning_count=1`；
- 失败摘要**只含计数**，无路径、无文件名、无原始错误文本。

## 3. 运行期 stdout/stderr（已取证 — 启动窗口）

方法：真实 `.app` 二进制直接启动，`HOME` 重定向到全新隔离目录（不接触真实项目数据），运行 8 秒后终止，全量捕获 stdout/stderr。

- 输出总量 **35 字节**，内容仅为一行关闭提示（`Ctrl+C detected. Shutting down...`）；
- 六类路径锚点 0 命中：`/Users/<user>`、`/Volumes/`、`governance.db`、`Application Support`、`project_id`、`.db`；
- 隔离 HOME 内应用仅创建 `recent.json.lock`，未触碰任何真实项目数据。

## 4. 历史 DB 基线（已取证 — 只读聚合，不输出载荷）

对两个真实项目库做 `sqlite3 -readonly` 聚合查询，只取计数与时间戳；查询前后文件 mtime 不变，零写入。

| 库 | 事件总数 | 含 `project_id` 键的事件 | 说明 |
| --- | --- | --- | --- |
| 验证副本（`resume-fix-validation-20260928/governance.db`，6.3 GB） | 522,085 | **10**，全部为 `job:created`，时间跨 2026-09-24 → 09-30（最后一条即 09-30 18:25 CST Resume 任务创建事件，由修复前 dev 构建产生） | 10 个任务：5 CANCELLED / 1 COMPLETED / 4 PAUSED_NETWORK |
| 原始 NAS 只读验收项目库 | 250,974 | **8**，同类历史 `job:created` | 5 CANCELLED / 3 PAUSED_NETWORK |

- 含 `/Users/`、`/Volumes/` 样式的载荷共 2 条，**全部**落在上述 10 条 `project_id` 事件内（其 `project_id` 值本身即路径）；泄漏集合之外为 0；
- 修复提交落地时刻之后新增泄漏事件：**0**；
- 历史记录**原样保留、不做清洗** —— 与手册口径一致（正确的证据治理方式；历史泄漏的存在不构成对新代码的 FAIL 判定）。
- 注：Resume 验收报告中"136,354 条事件"指单个历史任务（`job-6027…`）的事件数；本文 522,085 / 250,974 为全库各任务总和，两者不矛盾。

## 5. 三态汇总

| 项 | 状态 |
| --- | --- |
| 新构建身份与 main/CI 绑定 | 已取证 |
| 新 creation event 不含 `project_id` / 项目 DB 路径 | 已取证（真实持久化管道） |
| 失败摘要仅含脱敏计数、无路径/文件名/原始错误 | 已取证（真实哈希失败触发） |
| 事件消毒层纵深防御（`project_id` 列入敏感键） | 已取证（含于上述管道与 #48 单测） |
| 运行期 stdout/stderr 无路径泄漏 | 已取证（**启动窗口 8 秒**；见未验证项） |
| 历史 DB 旧记录原样保留、未被清洗 | 已取证（只读聚合 + mtime 不变） |
| App GUI 打开真实项目并触发新扫描/事件后检查新事件 | **未验证（待真机）** —— 无法自动化点击 GUI；并入最终发行物验收（Clean Install + 真实 NAS 闭环） |
| 长时运行 / 异常路径下的 stdout/stderr | **未验证（待真机）** —— 本轮仅捕获启动窗口；建议真机从终端启动 App 执行一次小扫描后检查终端输出 |
| 系统统一日志（os_log / console）取证 | **未验证** —— 应用无文件日志、stdout 干净；如需可后续用 `log show --predicate` 复核 |

## 6. 遗留与移交

1. 真机 GUI 复核清单（最终发行物验收时执行）：About 显示 Commit = 候选 SHA → 打开真实项目 → 小范围扫描 → 审计/任务历史确认新事件无路径 → 终端启动捕获全程 stdout/stderr。
2. 历史 10 + 8 条泄漏记录为已知历史证据，保留不清洗；若未来提供 DB 导出/分享功能，须先处理该历史键。
3. 下一步按手册顺序：第 2 项"失败解释/覆盖率口径封口"（active=2,957,765 / unavailable=26 / missing=0；3 条历史哈希失败无单项原因记为已知限制）→ 第 3 项 #47 undici 处置 → 第 4 项 #45/#46 独立判定。
