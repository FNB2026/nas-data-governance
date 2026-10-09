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
| 正式 App：真实 NAS 项目、索引、重复结果、目录语境与路径脱敏 | NOT RUN | 本阶段未在正式 App 打开真实项目或重新扫描 NAS。 | 开发版与历史 beta.4 证据不能替代此行。 |
| 正式 App：GUI 操作期间新增事件隐私 | NOT RUN | 尚未执行安全小范围扫描并检查新增持久化事件。 | 旧 creation event / 历史数据库原样保留；不推断新增事件合规。 |
| 正式 App：GUI 操作期间长时 stdout / stderr 与统一日志隐私 | NOT RUN | 当前仅正常 GUI 启动，没有本轮长时操作日志采集证据。 | 既有 8 秒启动窗口不能替代；不宣称零泄漏。 |
| 小范围 SMB 中断 → PAUSED_NETWORK → remount → Resume → COMPLETED | NOT RUN | 本阶段未卸载共享卷或发起扫描。 | 需安全小范围与明确中断时点；不得开展百万级扫描或把未见文件误判 missing。 |
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

**Public Beta：BLOCKED。** 静态资产、安装身份、Clean Install 与全新环境离线首启已通过；真实 NAS、GUI 事件 / 长时运行日志隐私、小范围 SMB Resume、disposable Quarantine / Restore 与 Crash Recovery 仍未验收。本次解除安装 / 离线首启两项阻断，不解除整体发布门槛。

下一唯一动作：在正式 beta.5 App 中以“只读打开”进入已有真实 NAS 项目，开启路径脱敏，查看重复结果中的一组目录语境和文件证据；不重新启动大目录扫描，不执行隔离、删除或恢复。


## 2026-10-09 人工反馈与身份闭环

| 场景 | 结果 | 证据 | 问题 / 限制 |
|---|---|---|---|
| 严格的全新环境断网首次启动 | PASS | 用户确认在未运行过 beta.5 的 Mac 上，下载后未在线启动 App，断网后首次安装并打开，未使用“仍要打开”或清除隔离属性；About 截图显示完整 RC SHA 与构建身份。 | 用户人工验收 + GUI 截图，不是代理现场网络状态测量；只关闭本场景。 |
| Mac 根目录范围包含当前项目数据库时的扫描启动拒绝 | PASS | 同轮用户报错截图与 About 身份闭环；报错精确对应 RC 源码 `internal/project/service.go` 的项目数据库 / WAL / SHM 包含检查及已有拒绝测试。 | PASS 仅表示预期保护拒绝得到证据，不表示合法子目录扫描已完成；报错截图见本轮对话，其原始图片文件现不可读取，未归档图片副本。 |

About 原始截图按字节一致复制保存，图片只含产品身份，没有本机路径、文件名、NAS 地址或凭据；报告不记录用户机器与源目录的实际路径。

2026-10-02 的 BLOCKED 来自 CLI 下载及已使用环境；2026-10-09 的人工反馈和身份截图补齐全新环境证据，现按场景关闭。Tag / RC / 发行资产不变，Draft 不发布。

下一唯一动作：正式 beta.5 App 只读打开已有 NAS 项目，检查一组重复结果 / 目录语境 / 文件证据，并以路径脱敏状态提供截图。
