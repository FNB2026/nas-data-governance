# beta.5 最终发行物验收记录（初始 2026-10-02；更新 2026-10-09）

本记录保存正式发行物证据；实时进度与发布决策仍由 [唯一执行手册](NDG-v0.5-Beta-RC-Release-Execution-Manual.md) 管理。未执行场景不默认通过，开发 App / CI / 静态资产检查不能替代真机与 NAS 场景。

## 发行身份与证据来源

- 本轮开始时，本地 HEAD / origin/main / RC / tag peel 均为 `8473dd630c4165389b32ffc85ae062145546a766`；工作树干净。后续本轮提交仅更新文档，发行身份保持该 SHA。
- `VERSION=0.5.0-beta.5`，`BUNDLE_BUILD_NUMBER=5`。
- Annotated tag `v0.5.0-beta.5`：object=`3b519f6b8c38f5f35ed313ef9fac1d8f0feac2ec`；远端 peel 与 RC 严格一致。
- [Release run 37005096647](https://github.com/FNB2026/nas-data-governance/actions/runs/37005096647)：event=push，head_sha 与 RC 一致，completed / success；Verify、Build Unsigned .app、Sign & Notarize、Draft GitHub Release 均 success。
- Release ID=`401795345`，名称 `NDG 0.5.0-beta.5`；`draft=true`、`prerelease=true`、`published_at=null`。
- 当前 #45 / #46 均 OPEN，依赖判定继续沿用 [既有报告](dependency-prs-45-46-judgment-2026-10-02.md)，不进入本候选。
- 本轮资产通过 `gh release download v0.5.0-beta.5` 从正式 Draft 下载，未使用本地构建或 Actions 临时资产；于 2026-10-02 12:45 UTC 再核对资产与 Release notes。

| 正式资产 | Asset ID | Bytes |
|---|---|---:|
| NDG-0.5.0-beta.5-macos.dmg | 605556564 | 6,342,011 |
| NDG-0.5.0-beta.5-macos.dmg.sha256 | 605556572 | 93 |
| sbom.cyclonedx.json | 605556565 | 45,554 |
| sbom.spdx.json | 605556563 | 80,459 |

DMG 官方 digest 与下载后重新计算的 SHA-256 严格相等：

```text
e6d5ad3ad2ae87eeff9d558d646a3dd67a3a56c9d48f12f28ce77afae37881e5
```

## 逐场景结果

结果仅使用 `PASS / FAIL / BLOCKED / NOT RUN`。每行的证据只支持该行范围。

| 场景 | 结果 | 证据 | 问题 / 限制 |
|---|---|---|---|
| 源码 / Tag / workflow / Draft 身份核对 | PASS | `git rev-parse`、远端 `ls-remote` 与 GitHub API；身份如上，四个 job 成功，四资产存在。 | CI success 不代表最终场景通过。 |
| 正式下载 DMG 完整性 | PASS | 大小 6,342,011；本地 SHA-256 与官方 digest 一致；`shasum -a 256 -c` 显示 OK，exit=0。 | 仅验证这份发行文件。 |
| QA-4：正式 DMG staple | PASS | `xcrun stapler validate`：The validate action worked，exit=0。 | 不推断离线首次启动结果。 |
| 包内 App 签名 / Gatekeeper | PASS | codesign deep / strict exit=0：valid on disk、satisfies Designated Requirement；spctl execute exit=0：accepted、source=Notarized Developer ID；Team ID=A2DYS82NLA，runtime flag 开启。 | 包内检查不能代替安装后或全新环境检查。 |
| 正式 App 的 Finder 安装与启动 | PASS | 安装目标原先不存在；通过 Finder 复制包内 App 到 Applications 后，用安装路径启动，GUI 正常出现。安装后二进制 SHA-256 与正式包内二进制相等。 | 本机已有 NDG 使用历史；实际安装为 Finder 复制，不能声称拖动操作成功或干净环境首次启动通过。 |
| 安装后 App codesign / Gatekeeper | PASS | codesign deep / strict exit=0；spctl execute exit=0，accepted、source=Notarized Developer ID。 | 当前在线且已经评估过该资产。 |
| 安装后 GUI About 身份 | PASS | 真机 About：Version=0.5.0-beta.5；Commit=`8473dd630c4165389b32ffc85ae062145546a766`；Build time=2026-10-02T12:13:59Z；Channel=beta。正式二进制中完整 RC SHA 恰出现一次。 | 初始窗口 About Commit=`bcab0413ac49`，不匹配 RC，已排除该窗口并关闭空闲旧构建；未把其结果算作正式发行物证据。 |
| QA-1：Clean Install / 首次 Gatekeeper GUI 路径 | PASS | 2026-10-09 用户人工验收：未运行过 beta.5 的 Mac，下载后未在线启动，断网首次安装并打开正常，未使用“仍要打开”或清除隔离属性；[About 截图](evidence/beta5-offline-first-launch-about-20261009.png) 身份严格匹配正式 RC。 | 用户实际操作反馈与截图，非代理现场操作；未在该 Mac 单独读取 quarantine 属性或复算下载文件 checksum。2026-10-02 CLI 下载与已使用环境的启动证据不用于替代本场景。 |
| 全新环境断网首次启动 | PASS | 2026-10-09 用户确认全新 beta.5 环境、下载后无在线启动、断网首次安装 / 打开、无 Gatekeeper 绕过；About 版本=0.5.0-beta.5，Commit=`8473dd630c4165389b32ffc85ae062145546a766`，构建时间=2026-10-02T12:13:59Z，通道=beta。 | 用户人工验收与身份截图共同支持本场景；不替代 NAS、事件 / 运行日志隐私、Resume 或恢复场景。 |
| Release notes / SBOM 资产读取 | PASS | GitHub Release body 去首尾空白后与 CHANGELOG beta.5 段正文逐字相等；未使用占位回退。CycloneDX JSON 可读取，59 components；SPDX-2.3 JSON 可读取，55 packages。 | JSON 读取不代表穷尽 SBOM 完整性审计。 |
| 正式 App：真实 NAS 项目、索引、重复结果、目录语境与路径脱敏 | PASS | 2026-10-09 Computer Use：正式安装 App 只读打开已有 NAS 验证项目；Finder 重挂载原共享后，注册根目录可读且位于 SMB。52,319 组可读取；抽样两副本组显示 backup / 受保护 / 必须人工复核，业务锚点已隐藏，物理身份不可靠并保守估算。 | 本行只覆盖已有项目读取与 UI 抽样；不是正式 beta.5 新扫描、网络 Resume 或完整写操作闭环。截图在本轮对话可见，未另存图片。 |
| 正式 App：GUI 操作期间新增事件隐私 | PASS | 2026-10-09 正式 App GUI 创建独立本机合成项目并扫描 3 文件：COMPLETED，discovered=processed=3，failed=0，checkpoint completed / 3；新增 11 条事件的测试锚点命中 0，job:created 仅含 {"job_type":"scan"}。 | 仅覆盖本轮成功扫描新增事件；失败摘要、网络异常等未在此场景触发。历史数据库不清洗、不回填。 |
| 正式 App：GUI 操作窗口 stdout / stderr 与统一日志隐私 | PASS | 2026-10-09 捕获 252 秒真实 GUI 创建项目、扫描、重复结果、审计与错误保护路径；stdout=stderr=0 bytes；log show exit=0，2,872 条统一日志，测试路径 / 文件名等锚点命中 0。 | 有界操作窗口，不宣称全日志零泄漏或长时压力验收通过；详细范围见下节。 |
| 正式 App：小范围网络异常事件 / stdout / stderr / 统一日志隐私 | PASS | 2026-10-09 独立 SMB 全流程约 765 秒：95 条事件载荷、stdout/stderr=0 bytes、3,951 条统一日志，测试路径 / 文件名等锚点 0 命中；GUI 暂停摘要只含计数。见 [专题记录](beta.5-smb-network-resume-acceptance-2026-10-09.md)。 | 有界故障窗口；夹具无非空业务锚点，不代替长时扫描或全部异常类型。 |
| 正式 App：长时运行日志隐私 | NOT RUN | 尚未覆盖长时压力扫描日志。 | 765 秒 SMB 故障窗口和此前 252 秒 GUI 窗口均不能代替长时场景。 |
| 小范围 SMB 中断 → PAUSED_NETWORK → remount → Resume 与结果完整性 | FAIL | 正式 App、独立只读 SMB / 项目：6/6/0 基线，完整哈希阶段真实中断后 paused_network / 6；重挂载 Resume 自然 COMPLETED、checkpoint completed / 6、入口消失，active=6、missing=unavailable=0。 | 未完成完整哈希未恢复：SHA-256 仅 2/6，预期 3 组实际 GUI 1 组。状态链与前缀安全通过，不足以判整体 PASS。见 [失败证据](beta.5-smb-network-resume-acceptance-2026-10-09.md)。 |
| Disposable Dry Run → Quarantine → 校验 → Restore | NOT RUN | 本阶段未执行写操作。 | 只可使用明确可丢弃夹具，不能使用真实唯一资料。 |
| Disposable Crash Recovery / Recovery Lock | NOT RUN | 本阶段未执行恢复夹具。 | 历史 K6 不能替代正式 beta.5 发行物验证。 |

## QA-4 文档决策

治理说明：**PASS WITH DOCUMENTATION CORRECTION**；这是已接受的要求修正，不是独立验收场景状态，也不代表 QA-4 所有子场景或 Public Beta 已通过。

当前流水线只公证并 staple DMG，包内 App `stapler validate` exit=65（无独立 stapled ticket）。记录这一实现事实，不单独判定 beta.5 FAIL，不由此推断离线首次启动的结果。正式 DMG staple、安装后 codesign 与 spctl 仍必须通过；全新环境断网首次启动已于 2026-10-09 由用户人工验收与 About 截图取证为 PASS，详情见本记录。

不更改产品、版本、流水线、Tag 或正式资产，也不发布 Draft。

## 历史覆盖口径（非本轮正式发行物验收结果）

继续沿用 [覆盖与失败口径封口](coverage-and-failure-criteria-2026-10-02.md)：

- D1：历史开发版 checkpoint completed，scanned_count=1,308,951；Resume session discovered=processed=1,648,814，failed=3。
- D2：active=2,957,765 / unavailable=26 / missing=0，total=2,957,791；26 条分为三个父目录组。当前代表探针 ENOENT 只证明当前不可读，不回推原始扫描错误。
- D3：active 快速哈希=2,957,765 / 2,957,765；完整 SHA-256=197,170，约占 active 的 6.67%，按设计只作用于重复候选。
- 3 条历史 hash failure 原因、26 条 unavailable 原始时刻错误不可追溯；不清洗、不猜测、不回填。

“扫描完成”不等于“每个文件内容都完成哈希校验”。

## 发布结论与下一动作

**Public Beta：BLOCKED。** 静态资产、安装身份、Clean Install、全新环境离线首启、已有 NAS 索引只读 UI、成功扫描与本次 SMB 异常窗口隐私通过。小范围 SMB 场景已执行并 FAIL：Resume 完成但遗漏 4 个未完成完整哈希候选，预期 3 个重复组只剩 1 个。长时日志、disposable Quarantine / Restore 与 Crash Recovery 仍未验收。

下一唯一动作：独立审查本次前缀完整哈希恢复遗漏，确定后续候选修复与复验路径；保持 beta.5 Tag / 资产和 Draft 不变。失败现场保留，不进入 Disposable Quarantine / Restore。


## 2026-10-09 人工反馈与身份闭环

| 场景 | 结果 | 证据 | 问题 / 限制 |
|---|---|---|---|
| 严格的全新环境断网首次启动 | PASS | 用户确认在未运行过 beta.5 的 Mac 上，下载后未在线启动 App，断网后首次安装并打开，未使用“仍要打开”或清除隔离属性；About 截图显示完整 RC SHA 与构建身份。 | 用户人工验收 + GUI 截图，不是代理现场网络状态测量；只关闭本场景。 |
| Mac 根目录范围包含当前项目数据库时的扫描启动拒绝 | PASS | 同轮用户报错截图与 About 身份闭环；报错精确对应 RC 源码 `internal/project/service.go` 的项目数据库 / WAL / SHM 包含检查及已有拒绝测试。 | PASS 仅表示预期保护拒绝得到证据，不表示合法子目录扫描已完成；报错截图见本轮对话，其原始图片文件现不可读取，未归档图片副本。 |

About 原始截图按字节一致复制保存，图片只含产品身份，没有本机路径、文件名、NAS 地址或凭据；报告不记录用户机器与源目录的实际路径。

2026-10-02 的 BLOCKED 来自 CLI 下载及已使用环境；2026-10-09 的人工反馈和身份截图补齐全新环境证据，现按场景关闭。Tag / RC / 发行资产不变，Draft 不发布。

## 2026-10-09 Computer Use：NAS 只读 UI 与合成扫描隐私

实际操作对象为 `/Applications/NDG.app` 正式安装 App。GUI About 再次显示版本 `0.5.0-beta.5`、完整 Commit `8473dd630c4165389b32ffc85ae062145546a766`、构建时间 `2026-10-02T12:13:59Z`、通道 beta；路径脱敏已开启，外部 AI / 遥测 / 云上传显示关闭。

### 已有 NAS 项目：只读证据

- 从高级入口点击“只读打开”，页首与页尾均显示只读；扫描页禁止新扫描，执行中心禁用且给出只读原因。未点击最近项目的读写快捷入口。
- 初始无 SMB 挂载时，只把页面结果视为缓存索引。之后通过 Finder 重挂载原共享，再以低负载只读检查确认原注册根目录可读且位于 SMB；没有遍历或完整哈希大文件。
- 只读 SQLite 聚合：active=2,957,765，unavailable=26，missing=0；checkpoint 1 为 completed / 1,308,951，updated_at=`2026-10-02T00:33:09.249256Z`。这是历史开发版扫描的持久化结果，不归入本轮正式 App 扫描成绩。
- 重复结果总数 52,319；两副本抽样组显示受保护 backup、人工复核理由与保留得分，业务锚点显示“已隐藏”；物理证据为不可靠 / 待确认，未被表达成可靠硬链接或可直接删除。截图可见目录语境与物理证据同时存在，未弱化物理证据区。
- 审计页无恢复锁且零审计 / Journal；本轮对 NAS 源未执行扫描、隔离、清理或恢复写操作。

### 独立本机合成项目：GUI 与持久化事件

临时目录由程序自行创建，包含一对相同内容文件与一个唯一文件。正式 App GUI 选择该目录、创建独立项目、点击开始扫描并自然完成，随后查看重复结果、证据检查器、审计与设置；未使用 mock、测试 binding 或开发 App。

- 任务自然 COMPLETED / FINALIZING，发现 3、处理 3、失败 0；active=3，checkpoint completed / 3，生成 1 个重复组；合成源三文件内容保持不变。
- 新增 11 条持久化事件；creation payload 精确为 `{"job_type":"scan"}`；载荷对临时源 / DB 路径、文件名 canary、用户主目录、挂载卷前缀、project_id 与 Application Support 等锚点为 0 命中。检查只读取聚合，不输出载荷中的私密数据。
- 脱敏后的扫描根输入、重复组路径与文件名正确遮蔽；目录语境仍能表达保护规则。系统目录选择器与主动编辑路径输入不作为分享截图，未将其内容公开。
- 在合成项目尝试以包含其自身数据库的目录开始扫描，真实 GUI 拒绝，错误仅为 `project: scan source must not contain the current project database or its WAL/SHM files`，不带实际路径；没有启动该扫描。
- 同一正式二进制的 252 秒 GUI 操作采集窗口：stdout/stderr 均 0 bytes；统一日志读取 exit=0，JSON 2,872 条 / 3,638,391 bytes；上述测试锚点为 0 命中。原始日志与本机测试 DB 留在本机私有临时目录，不纳入公共仓库。

**范围限制：** 本轮新增事件只有成功扫描；没有触发 hash failure 摘要或 SMB 中断。252 秒是有界 GUI 操作窗口，不等于长时扫描 / 异常压力测试，也不能证明未列入锚点的所有数据绝无泄漏。NAS 只读视图与合成扫描分别取证；不把本机合成扫描当作真实 SMB 扫描闭环。操作结束后，App 已恢复到已有 NAS 项目的只读模式。

以上为此前成功扫描窗口的证据。后续小范围 SMB 故障验收已完成取证并发现阻断项，现行结论见 [2026-10-09 SMB 专题记录](beta.5-smb-network-resume-acceptance-2026-10-09.md)：网络状态链 / 源安全 / 本次异常隐私 PASS，完整哈希待办恢复 FAIL。Draft 保持未发布，产品 / VERSION / Tag / 正式资产均不改动。
