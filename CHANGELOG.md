# Changelog

本项目的显著变更按里程碑组织。每个里程碑对应 [开发路线](knowledge/maps/roadmap.md) 中的章节。

## 0.5.0-beta.9 — Restore Recovery 安全发行候选

- **不确定目标保锁**：Restore 中断后，只要恢复目标仍存在或证据不可信，保留 pending 与恢复锁；不再仅因隔离副本完整而报告回滚完成（#73 / #72）。
- **事务与审计一致性**：恢复结案需要验证完整隔离副本与目标确实不存在，并原子提交 Journal、计划终态、审批失效和脱敏审计；持久化失败不宣告成功。
- **失败执行保护**：正常 Restore 完成状态持久化失败时，保留实际输出与 pending，禁止未经身份证明的自动反向移动或删除。

### 候选状态与必需复验

- 新的不可变 Draft 候选；beta.8 Tag、DMG、失败数据库和 D 场景 FAIL 证据保留。
- 源码修复、自动化与独立审查不替代签名 App 验收。正式 D 需证明部分目标保锁、人工独立保留后的安全结案、新审批及恢复闭环；A/B/C 进行针对性回归。
- 当前批准根目录仍为信任边界；不同设备子目录被拒绝，但 Dev 检查不证明识别同设备 bind mount。无强制解锁功能。
- Issue #72 在正式复验完成前保持 OPEN；Public Beta BLOCKED、Release Draft，不启动 Purge。

## 0.5.0-beta.8 — 恢复结果反馈发行候选

- **恢复反馈分类**：普通执行、隔离还原与清理恢复依据明确终态和错误分类显示结果；skipped、未知、错误附带成功状态不再报恢复完成（#69 / #67）。
- **草案与未知锁**：退回 DRAFT 明确要求重新审查和审批；恢复响应或请求失败均刷新锁，未知或矛盾锁状态禁止新写入。
- **隐私与结果留存**：只显示固定安全错误文案，恢复反馈不插入原始错误或敏感路径；解锁后仍保留结果与分类计数。

### 候选状态与必需复验

- 新的不可变 **Draft 候选**。beta.7 Tag、DMG 及正式 Crash Recovery **INCONCLUSIVE** 保留；源码修复不改变历史发行物结论。
- 正式 App 需要复验恢复反馈及受影响场景，使用全新 disposable 项目、文件、独立备份，不修改原崩溃现场。
- pending、无 Journal、done 未结案与 Restore 中断分别记录实际状态、文件、Journal、审计及恢复锁证据；不能把源码故障注入测试当作正式运行 PASS。组合证据关闭 Gate 需要明确批准。
- Public Beta **BLOCKED**，Release **Draft**；不启动 Purge，不新增产品功能。

## 0.5.0-beta.7 — 执行状态一致性发行候选

- **执行终态可靠持久化**：成功隔离仅在 VERIFIED、隔离登记与审计事务提交后报告成功，重启与重复执行不复用旧执行资格（#64 / #62）。
- **stale 审批失效**：文件变化明确计为拒绝失败，durable DRAFT 与审批失效审计同步提交，要求人工复审；不再把 stale 拒绝计为 executed。
- **执行与恢复保护**：条件状态更新、权威计划重读、共享执行 owner 和完整恢复锁；未知 Journal、回滚/持久化失败保留锁，并停止批次后续写入。COPY 恢复保护变化目标。

### 候选状态与必需复验

- 新的不可变 **Draft 发行候选**；beta.6 Tag、DMG 和正式 Quarantine / Restore **FAIL** 保留。源码修复与自动化通过不代表旧发行物通过。
- 正式 App 必须复验隔离/恢复文件闭环、durable VERIFIED、stale executed=0/failed=1/DRAFT、审批失效、重复执行安全、GUI/数据库/Journal 一致性与限定窗口隐私。
- 使用全新人工 disposable 文件与独立项目数据库，不修改真实 NAS、原失败数据库或日志。不进入 Purge，不主动触发正式 Crash Recovery。
- Public Beta 仍 **BLOCKED**；正式候选与必要门禁通过之前不得发布 Release。Crash Recovery / Recovery Lock、全新 Mac 离线首装、长时隐私和剩余安全项继续单独留证。

## 0.5.0-beta.6 — 完整哈希断点恢复发行候选

- **完整哈希待办恢复**：修复 FULL_HASHING 网络中断后 Resume 遗漏持久化遍历前缀中尚未完成的重复候选（#57）；恢复同组已完成成员的分组语境，覆盖无新增遍历文件与跨 checkpoint 的情况。保留分层哈希，普通单例不强制完整 SHA-256。
- **完成态与持久化约束**：完整哈希候选必须达到成功或已记录的可解释失败；取消、网络暂停及未提交待办不认证完成，最终结果和 checkpoint 写入失败不能静默完成。恢复前缀不重复累加遍历计数。
- **恢复读取边界**：桌面与正式 CLI 的恢复读取使用 no-follow 目录句柄和设备/metadata 校验，读取结束再次检查当前路径绑定，拒绝链接或文件/父目录替换。
- **工具链安全补丁**：Go 1.26.6 → 1.26.9，通过 Govulncheck 的标准库安全门禁（#57）。

### 候选状态与必需复验

- 本版本是新的正式签名 **Draft 发行候选**，不是公开发布许可；beta.5 Tag、DMG 和 SMB Resume 失败证据保留。
- 源码回归、独立复审及工程门禁通过，不能替代本版本发行物的真实 SMB 中断、重挂载、Resume、完整哈希覆盖与日志隐私验收。
- SMB 复验必须实际观察 FULL_HASHING → PAUSED_NETWORK → Resume → 自然 COMPLETED；三对重复候选 SHA-256 6/6、重复组 3/3，普通单例仍按分层哈希，missing/unavailable=0 且源内容未变。
- Disposable Quarantine / Restore、Crash Recovery 和最终 Public Beta Gate 在前述复验通过后继续，当前尚未执行。

## 0.5.0-beta.5 — 首个公开 Beta 候选

- **断点续扫修复**：网络源不可用时安全暂停并保留可续扫边界，不再把未读取文件推断为删除；恢复完成后 Resume 入口消失，原有前缀记录保持 `active`（#48）。
- **事件隐私与失败摘要**：`job:created` 事件不再携带项目标识（历史记录中该字段曾写入项目数据库完整路径），持久化层同时禁止 `project_id` 键；扫描存在错误或部分覆盖时，改为持久化**仅含计数**的摘要（遍历错误数、快速/完整哈希失败数、覆盖状态、missing/unavailable 计数），不含路径、文件名或原始错误文本（#48）。
- **依赖安全收口**：前端开发依赖 `undici` 8.9.0 → 8.11.2；该依赖的 10 条安全告警在 GitHub 上全部转为 `fixed`，开放告警为 0（#47）。
- **发布门禁修复**：release 脚本测试断言改为跨 grep 方言一致（此前 GNU/BSD 差异会造成"同一提交 CI 绿、本地假红"），并收紧两条可被注释或报错文本满足的空断言（#54）。

### 当前限制与覆盖口径（必读）

- **"扫描完成"不等于"每个文件内容都完成哈希校验"。** 扫描结果按三个互不混淆的维度陈述：
  - **遍历完成**：任务自然完成，检查点为 `completed`。
  - **文件存在状态**：`active` / `unavailable` / `missing` 三态；`unavailable` 表示扫描时无法读取，**不等于**数据丢失。
  - **内容校验覆盖**：快速哈希覆盖全部条目；**完整内容 SHA-256 按分层设计仅作用于重复候选**，因此不得表述为"全部内容已校验"。
- 已知限制：历史版本未逐项持久化哈希失败原因，个别历史失败的原因不可追溯；不可读条目的原始时刻错误不可回溯。
- 本版本仍为 Beta：仅支持 Apple Silicon Mac；重要数据请保留独立备份。

## 0.5.0-beta.4 — 发布链修复候选

- 修复 notarization submission ID 解析：`notarize-macos-app.sh` 改用 `notarytool --output-format json` 结构化解析提交 ID 与公证状态，替代对带缩进的人类可读文本（`  id: UUID`）的脆弱 `grep '^id:'`，避免 `set -euo pipefail` 下静默退出。
- 补足 beta.3 验证：notarization 上传提交（`NDG-0.5.0-beta.3-macos.dmg`）确认已在 Apple 侧成功接收。

## 0.5.0-beta.3 — 发布链修复候选

- 修复 macOS 发布签名链：`Sign & Notarize` 的 Untar 步骤在全新 checkout 中解压到 git-ignored 的 `cmd/ndg-desktop/build/bin` 时因目标目录缺失失败（`tar: could not chdir`）；现在先 `mkdir -p` 再解压，并保持可执行位验证。附带回归测试（行为测试 + workflow 结构断言）。
- 补足 beta.2 的发布凭据配置：完成 Apple Developer ID Application 证书的创建、安装与 P12 导出，配置 App Store Connect Team API Key，并在 `release-macos` 环境补齐六项发布 Secret。

## 0.5.0-beta.2 — 首个公开 Beta 候选

- 完成桌面端 UI Final Polish：统一应用框架和状态反馈，补齐目录语境、治理决策、执行风险边界、恢复路径、设置与首次启动引导。
- 在真实只读 SMB 数据源及隔离测试夹具完成扫描恢复、路径脱敏、审批边界、隔离恢复、Purge 与 Recovery Lock 验收；未对真实 NAS 源文件执行写入、隔离或删除。
- 修复 macOS 版本同步脚本的 BSD `sed` 兼容性，确保 package-lock 与 Info.plist 可由同一机制同步。
- 升级 Echo、Vitest 及其间接 Nano ID 依赖，前端锁文件高危审计结果为零。

## Unreleased

- 开源前安全收口：非 dry-run `execute` 强制要求 SQLite `--db`，Journal 初始化或动作完成记录失败即停止并回滚；核心索引、计划、审计和 SQLite 产物统一为目录 `0700`、文件 `0600`，并收紧既有宽权限文件。
- 源根目录和隔离根目录必须是互不重叠的真实目录且不能是符号链接；根目录自身也纳入执行前符号链接检查。
- Go module 统一为 `github.com/FNB2026/nas-data-governance`；构建写入版本、提交和构建时间，增加 `version` 子命令及 macOS/Linux 兼容的 SHA-256 发布校验。
- README 将首次体验限制为只读分析和 `execute --dry-run`，真实写操作移入高级风险章节并禁止默认使用全量审批。
- 扫描日志安全收口：普通错误输出只保留阶段与聚合数量，SQLite upsert 错误使用行号而非源路径；`scanner.Stats.FormatErrors` 不再输出路径。新增真实 SMB 高位 inode 回归，确认无符号索引与 SQLite 按位存储数量一致。
- 新增 `diagnose-paths` 私有只读兼容性诊断：在任务根目录内检查历史失败路径、NFC/NFD 变体和规范化同名目录项，拒绝符号链接、跨挂载点与越界路径，不读取文件内容、不自动重命名。
- 新增 `diagnose-merges` 私有门控报告：统计兄弟目录对、名称相似对及 Jaccard 0.10/0.25/0.50 分档；保持生产合并阈值 0.5 不变，近似候选只供人工复核。
- P2 审慎治理建议：新增 `diagnose-governance` 私有只读报告，合并完全重复 DRAFT 计划、零字节保守分类和大容量音视频格式/编码/版本/派生/侧车/目录职责证据。报告固定 `execution_authorized=false`，拒绝非 DRAFT 结果，不持久化审批或调用执行器。
- 零字节文件无论命中占位、失败输出、潜在临时产物或无法解释分类，都只生成 `KEEP_AND_REVIEW`；理论重复容量不等于可删除容量。
- P1 分析准确率：新增 `diagnose-formats` 私有复核报告，聚合大型 unknown、扩展名/文件头冲突和媒体元数据缺口；报告强制 `0600`，普通日志不输出路径。
- 媒体只读解析扩展：WAV/AIFF/FLAC 时长与编码，MP4/MOV/M4V 尾部 `moov` 的时长/尺寸/编码，AVI 时长/尺寸/编码，MPEG 帧尺寸；修复 AVI RIFF 子类和 M4A 检测顺序。`analyze --refresh-metadata` 只重试当前能力范围内的缺口。
- 目录职责规则升级为 `builtin-v2`：新增录音/素材/母带、制作/后期/剪辑/设计、成品/播出版等明确语义；音乐/资料等模糊词继续保留 unknown。
- P0 真实数据分析收敛：超大资产组以 10,000 成员为安全上限分层拆分并强制人工复核，evidence 只保留聚合规则说明、不重复成员路径；新增 AIFF/AIFC、OLE DOC/XLS/PPT、PSD 与工程侧车识别；`analyze` 新增断点复用、unknown 定向刷新、SQLite 分批持久化和聚合进度。
- 建立侧车依赖保护：XMP/CPR/SESX/PSD 及 PEAK/PKF/PEK/CFA/MPGINDEX 默认不参与自动清理；可再生成标记不等于可删除。
- 新增哈希失败闭环：扫描有限重试、失败记录保留、`0600` 私有清单，以及带根目录/符号链接/挂载点/stale 校验的 `retry-hashes` 局部补扫；补扫只生成新索引，数据库补录继续由独立的 `import-index` 完成。
- 格式分析数据库告警改为仅输出聚合数量，报告文件强制使用 `0600` 权限，避免普通日志和宽松文件权限暴露敏感文件名与路径。
- 新增 `import-index`：先校验完整 JSONL，再分批幂等导入 SQLite 并重建目录语境，全程不访问 NAS。
- 支持高位 SMB device/inode 在 SQLite 中无损往返。
- 扫描错误日志脱敏，并收紧资产分组与版本关系识别。

## v1.0.0 — 私有内部里程碑（未公开发布）

此版本号对应历史重写前的私有里程碑；Release 与标签已经删除，不得重建或复用。首个公开版本将使用新的预发布版本号。

首个生产就绪版本。完成 M1–M6 全部里程碑，提供可在真实 NAS 上运行的安全索引、分析、计划、执行与学习闭环。

### M6 生产化收束

- **P0-1 持久化执行日志与崩溃恢复**（`633c9ac`）：`execution_journal` 表记录每个写动作生命周期；`Recover()` 在启动时回滚未完成动作或重置中断计划；CLI `recover --db`。
- **P0-2 增量扫描与断点续扫**（`c7a1ddc`）：size+mtime+inode 三元组哈希缓存复用；`scan_checkpoints` 表断点续扫；已删除文件标记为 missing 而非物理删除；自定义 BFS 遍历支持 context 优雅中断。
- **P0-3 真实 NAS 故障演练**（`e3363d9`）：隔离临时目录中 4 个只读场景；报告脱敏，临时目录前缀替换为 `<tmp>`。
- **P1-4 任务队列与资源控制**（`b1667d2`）：`internal/runner` worker pool，semaphore 并发控制；CLI `--workers N`。
- **P1-5 人工复核管理界面**（`fd4124a`）：`review` 子命令覆盖 plans/rules/merges/conflicts 四条线；规则全链路 draft→probation→approved。
- **P1-6 补齐底层测试**（`c81df09`）：6 个包 34 个新测试，覆盖此前未触达分支；全量 `go test ./... -race` 17 包通过。
- **P2-7 文档状态同步**（`2e5eb5d`）：roadmap/README/ADR/知识卡全部与代码实现对齐。

### M5 资产关系与智能整理

- 资产组识别（业务锚点或路径前缀聚类）、版本关系、派生关系、目录合并建议。
- L1 本地规则模型 + 生命周期 + SQLite 持久化。
- L2 本地统计学习：遍历 SQLite 索引统计目录名频次，生成规则草案。
- L3 行业资料学习：读取 TXT/MD/DOCX/PDF 提取术语，CJK n-gram 分词。
- L4 决策反馈学习：统计历史 plan 偏差，生成权重调整建议。

### M4 常见格式分析

- magic bytes 格式检测（图片/视频/音频/PDF/压缩包）；RIFF 子类型区分；OOXML 分流。
- 只读元数据提取（PNG/JPEG/GIF/BMP/WebP 尺寸、PDF 页数、ZIP 条目数、MP4/MOV/MKV 时长、MP3 时长）。

### M3 安全执行

- 执行前 stale 复核（path/size/mtime/inode/hash 五项比对）。
- Plan 状态机（DRAFT→APPROVED→STALE_CHECKED→EXECUTING→VERIFIED/ROLLED_BACK）。
- 隔离区路径管理；跨卷复制-校验-删除源；回滚机制。
- MOVE/COPY/RENAME、DELETE→隔离；CLI approve/execute。

### M2 目录语境与去重计划

- 目录角色分类（敏感/原始/备份/系统/项目/缓存/归档等）；上级目录链（1—6 层）；业务锚点（项目代号、年份目录）。
- 完整保留评分（Authority/Stability/PathDepth/RoleBonus）；冲突复核。

### M1 安全索引

- 只读文件扫描；SHA-256 分层哈希；完全重复报告。
- 不跟随符号链接、不跨挂载点、可处理中断与权限错误。
