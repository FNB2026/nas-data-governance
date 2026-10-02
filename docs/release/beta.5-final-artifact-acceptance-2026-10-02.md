# beta.5 最终发行物验收记录（2026-10-02）

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
| QA-1：浏览器下载后 Clean Install / 首次 Gatekeeper GUI 路径 | BLOCKED | 已完成安装与 About 子检查；本轮 `gh` 下载及安装的 App 未携带 com.apple.quarantine 标记。 | 没有清除 quarantine，但 CLI 下载未走浏览器下载标记路径；本机也非全新环境，不能把正常启动记为完整 Clean Install PASS。需浏览器下载且不绕过保护的独立场景。 |
| 全新环境断网首次启动 | BLOCKED | 已向用户提供独立 Mac / VM 验收步骤。 | 尚无未运行过 beta.5、未在线评估过该资产的环境证据；未断开当前 NAS / 网络，也未模拟干净环境。 |
| Release notes / SBOM 资产读取 | PASS | GitHub Release body 去首尾空白后与 CHANGELOG beta.5 段正文逐字相等；未使用占位回退。CycloneDX JSON 可读取，59 components；SPDX-2.3 JSON 可读取，55 packages。 | JSON 读取不代表穷尽 SBOM 完整性审计。 |
| 正式 App：真实 NAS 项目、索引、重复结果、目录语境与路径脱敏 | NOT RUN | 本阶段未在正式 App 打开真实项目或重新扫描 NAS。 | 开发版与历史 beta.4 证据不能替代此行。 |
| 正式 App：GUI 操作期间新增事件隐私 | NOT RUN | 尚未执行安全小范围扫描并检查新增持久化事件。 | 旧 creation event / 历史数据库原样保留；不推断新增事件合规。 |
| 正式 App：GUI 操作期间长时 stdout / stderr 与统一日志隐私 | NOT RUN | 当前仅正常 GUI 启动，没有本轮长时操作日志采集证据。 | 既有 8 秒启动窗口不能替代；不宣称零泄漏。 |
| 小范围 SMB 中断 → PAUSED_NETWORK → remount → Resume → COMPLETED | NOT RUN | 本阶段未卸载共享卷或发起扫描。 | 需安全小范围与明确中断时点；不得开展百万级扫描或把未见文件误判 missing。 |
| Disposable Dry Run → Quarantine → 校验 → Restore | NOT RUN | 本阶段未执行写操作。 | 只可使用明确可丢弃夹具，不能使用真实唯一资料。 |
| Disposable Crash Recovery / Recovery Lock | NOT RUN | 本阶段未执行恢复夹具。 | 历史 K6 不能替代正式 beta.5 发行物验证。 |

## QA-4 文档决策

治理说明：**PASS WITH DOCUMENTATION CORRECTION**；这是已接受的要求修正，不是独立验收场景状态，也不代表 QA-4 所有子场景或 Public Beta 已通过。

当前流水线只公证并 staple DMG，包内 App `stapler validate` exit=65（无独立 stapled ticket）。记录这一实现事实，不单独判定 beta.5 FAIL，不由此推断离线首次启动的结果。正式 DMG staple、安装后 codesign 与 spctl 仍必须通过；全新环境断网首次启动另行实际验收。

不更改产品、版本、流水线、Tag 或正式资产，也不发布 Draft。

## 历史覆盖口径（非本轮正式发行物验收结果）

继续沿用 [覆盖与失败口径封口](coverage-and-failure-criteria-2026-10-02.md)：

- D1：历史开发版 checkpoint completed，scanned_count=1,308,951；Resume session discovered=processed=1,648,814，failed=3。
- D2：active=2,957,765 / unavailable=26 / missing=0，total=2,957,791；26 条分为三个父目录组。当前代表探针 ENOENT 只证明当前不可读，不回推原始扫描错误。
- D3：active 快速哈希=2,957,765 / 2,957,765；完整 SHA-256=197,170，约占 active 的 6.67%，按设计只作用于重复候选。
- 3 条历史 hash failure 原因、26 条 unavailable 原始时刻错误不可追溯；不清洗、不猜测、不回填。

“扫描完成”不等于“每个文件内容都完成哈希校验”。

## 发布结论与下一动作

**Public Beta：BLOCKED。** 静态资产与安装身份检查已通过；完整 Clean Install、独立环境离线首启和其余真机 / NAS / 隐私 / 恢复场景仍缺证据。没有新增产品或资产回归判定，也没有解除最终发布门槛。

下一唯一动作：用户准备未运行过 beta.5 的 Mac / VM，通过浏览器从正式 Draft 下载 DMG，核对上述 SHA-256 后先不要启动；断网、挂载、Finder 安装并首次双击，记录 Gatekeeper 与 About。禁止 `xattr -cr` 或“仍要打开”绕过。若无该环境，保持 BLOCKED，由用户明确后再推进下一阶段。
