# FULL_HASHING Resume 候选恢复修复 — 2026-10-09

## 发布边界与原始证据

- 正式 beta.5 基线：`8473dd630c4165389b32ffc85ae062145546a766`。
- 失败证据：[PR #56 的 SMB 专题记录（冻结 HEAD）](https://github.com/FNB2026/nas-data-governance/blob/96575a84882525cc1a96e775dae3676a52487ddc/docs/release/beta.5-smb-network-resume-acceptance-2026-10-09.md)。
- 六个 disposable SMB 文件，三对重复。FULL_HASHING 中断 → PAUSED_NETWORK → 重挂载 → Resume 自然 COMPLETED；文件保留 6/6，quick 6/6，full SHA-256 2/6，重复组 1/3。
- 网络状态链、checkpoint 复用、源文件保护已有证据；内容校验完成判断失败。原失败数据库、日志、夹具与正式 App 保留，没有用于本轮自动化测试。
- 原 beta.5 的 SMB Resume 结论仍是 **FAIL**；Public Beta **BLOCKED**；Release **Draft**；Quarantine / Restore **NOT RUN**。本修复不修改旧 Tag、版本文件或发行资产。

## 根因与旧实现失败证明

`ScanService.Scan` 从 checkpoint 后继续遍历，历史前缀只参与存在性 reconciliation。完整哈希分组原先仅使用本轮 `files`，且先排除已有完整哈希的成员。因此：

1. 前缀内四个未完成候选无法进入本轮队列；没有新增遍历文件时队列为空。
2. 跨 checkpoint 的候选可能因已有 full 的同组成员被排除而误成单例。
3. 没有未提交工作终态检查，完成 checkpoint 的错误原先也被忽略。

先新增 `TestResumeRestoresFullHashCandidatesWithoutNewTraversal`，在产品代码修改前执行：

```text
go test ./internal/app -run '^TestResumeRestoresFullHashCandidatesWithoutNewTraversal$' -count=1
FAIL: scan_full_resume_test.go:84: pending full hash not recovered
exit 1
```

该本机测试使用七个可丢弃文件（三对重复加一个普通单例）、正常 service/store API 和注入的内容读失败。它复现状态与数据库缺陷，不是实际 SMB 或正式发行物验收。

## 最小修复与完成态不变量

- 用活跃历史前缀 metadata 与本轮 fresh metadata 按路径合并，按 `size + nonempty quick_hash` 重建重复组。只有组内存在待完成 full 的重复组参与恢复；同组已完成成员也重新检查，以支持跨断点分组。
- 重新通过扫描器检查所选前缀文件；按祖先集合剪枝，不进入无关子树。不跟随链接、不跨设备、不越过任务根。路径去重，fresh metadata 覆盖缓存，最多访问每个历史路径一次；fresh quick 改变后可发现更多前缀同组成员。
- 保留分层哈希：普通单例不强制 full SHA-256。远端不可靠物理身份不授权缓存复用；变更文件先刷新 quick，再决定是否属于重复组。
- 恢复前缀不增加本轮 discovered / processed 或 checkpoint 的遍历计数。完整组含已完成成员，只有仍缺 full 的成员入队。
- 完整哈希候选显式经历 `pending → success / failed`。取消、未提交工作和网络暂停不能认证完成；成功哈希或已记录的可解释失败、结果持久化、coverage reconciliation 和 checkpoint outcome 写入完成后，才进入正常完成态。
- `COMPLETED` 在已有 API 中允许附带 count-only hash-failure warning；它不表示每个文件均获得 SHA-256。`CoverageState=complete` 表示遍历/存在性覆盖，不表示内容校验无失败。网络错误继续返回 PAUSED_NETWORK；无法检查的恢复候选使 checkpoint aborted，保持可恢复。
- 没有数据库迁移、新持久化队列表、新后端 API、前端绑定或破坏性操作。

### 恢复读取的安全边界

macOS/Linux 的 production recovered quick/full reads 使用 root-anchored `openat`：每级 `O_NOFOLLOW`、设备一致；叶子只允许普通文件；读前/后校验 metadata。读取后重新从当前 root 遍历整个相对路径，比较 root 身份及当前文件与已读取 FD，拒绝 leaf / ancestor rename replacement，避免将旧句柄内容写到新路径对象上。

注入 HashFunc 的故障测试使用前后 metadata 检查，不将其称为原子读取保障。其他平台 recovered reads fail closed；本记录不宣称其他平台功能实测 PASS。任何读取之后的外部变化仍需后续操作本身的 stale 检查，哈希不授予删除许可。

## 自动化覆盖与证据层

新增 service / scanner / fingerprint 回归覆盖：

- 无新增遍历时恢复三对候选、单例不 full；原 checkpoint / durable prefix、文件身份、计数保持一致。
- 再次网络中断、重复 Resume、取消、重试、跨 checkpoint 已完成同组成员。
- 远端不可靠缓存不得复用旧 full；文件变更、读取期间变更、叶子/祖先链接与 rename replacement。
- quick/full 可解释失败、事件仅输出脱敏计数；普通 quick 失败导致候选单例时保留失败摘要。
- final Upsert / completed checkpoint 写入失败不能宣告完成；Resume 后 missing/unavailable 无误判。
- 所选路径遍历剪枝及越根拒绝；大文件 quick/full 与原指纹算法一致。

本轮本地自动化实际通过：`go test -race -count=1 ./...`、`go vet ./...`、golangci-lint v2.12.2（0 issues）、`make frontend-check`（203 tests + build）、public/version consistency、模块 tidy 无差异、version mapping（11）、macOS scripts、release workflow（85）与 CLI dev build。npm ci 报告一个 high advisory，现有 CI 的 audit 为报告项；本修复不升级依赖或消除该风险。

CLI 生产默认也使用受保护的 `NewScanService`；测试在 `_test.go` 中显式替换 factory，避免正式构造因历史测试注入而绕开保护。CI、独立审查、Desktop Build 与合并后 Gate 以最终 SHA 的 PR 记录为准。自动化测试通过不替代正式签名 App 的 SMB 验收。

## CI 发现的工具链安全门禁

初始修复 HEAD `4a56e66` 的 Govulncheck [实际失败](https://github.com/FNB2026/nas-data-governance/actions/runs/37907743641)：当前 Go 1.26.6 的标准库报告五项可达漏洞 `GO-2026-6617 / 6613 / 6612 / 6611 / 6603`，全部要求 Go 1.26.9。这里的可达报告不等于已证实产品可被利用，但不能绕过强制安全门禁。

用单独提交将 `go.mod` 的 Go 补丁版本升到 1.26.9；产品版本与依赖库版本保持不变。[官方漏洞报告](https://pkg.go.dev/vuln/GO-2026-6611)列出相应修复边界。最终 HEAD 需重新完成 CI、构建与独立审查，初始失败记录保留。

## 新发行候选与正式复验建议（尚未执行）

1. 独立修复 PR 经最终 HEAD CI 和独立 Codex 复审通过后合 main，再跑合并后 Gate。
2. 从包含修复的 main 冻结新候选 SHA，使用新的版本/构建号和 Tag（建议 `0.5.0-beta.6`，实际准备时统一检查）。禁止替换 beta.5 DMG 或重指 beta.5 Tag。
3. 生成正式签名、公证、staple、DMG、SHA256、SBOM 与 Draft notes。只从新的 GitHub Draft 下载最终资产验收；About 必须匹配新版本与冻结 SHA。
4. 使用新的独立 disposable SMB 共享/项目，原失败副本只作历史证据。先正常基线，再在可观察 FULL_HASHING 阶段断开；必须实际观察 PAUSED_NETWORK 和 durable checkpoint。
5. 重挂载 → Resume 自然 COMPLETED；六个重复文件应 full 6/6、重复组 3/3、内容/计数一致、checkpoint completed、Resume 入口消失、无错误 missing/unavailable、源文件未改变。额外普通单例应保持分层哈希语义。
6. 同步采集 GUI 事件、stdout/stderr、统一日志，对测试路径、文件名与业务锚点标记做隐私检查。未触发中断、状态不可观察或未自然完成为 BLOCKED / INCONCLUSIVE，不能 PASS。
7. 正式候选这道 Gate 通过后再进入 Disposable Quarantine / Restore 与崩溃恢复验收；Public Beta 发布许可独立判定。
