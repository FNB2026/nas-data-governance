# NDG 双端完整治理能力与仓库公开入口统一升级执行手册

状态：2026-10-15，**工程委托/设计基线，不是已通过验收或发布授权**。
基线：GitHub main `ac6d56ca1563a1edbe832042618c1676d79a4336`；执行前必须重新读取 main、活跃 PR、Issue、Release、Tag 和实际测试状态。

## 0. 一句话目标和渠道分界

**让 Mac 桌面端及 Linux Docker 端均可安全执行：发现重复 → 用户人工决定 → 审批 → Dry Run → Quarantine 隔离 → Restore 恢复 → 满足保留期与单独授权后 Purge 永久清理。**

**当前真实现状（更新至 2026-10-16）：** Mac 公共 beta.11 具有有限 Quarantine/Restore；`internal/app/purge_service.go` 的 `PurgeReleaseAllowed()` 目前恒为 false。新 Governance UX v2 在 Mac 的源正向核验 [#111](https://github.com/FNB2026/nas-data-governance/issues/111) BLOCKED。Docker `0.5.0-docker-preview.1` 为只读服务，Compose 的源目录 `read_only: true` 且 Web 明确禁用治理意图、隔离、恢复和 Purge。fnOS 公开 Preview.1 的原 SSH 路径仍被主机禁止 TCP 转发；另行开发的 [独立只读 HTTPS 入口](FNOS-CGI-ENTRY-2026-10-15.md) 已完成有限真实 Chrome 验收，尚非完整安装发行或平台认证。

**不要**直接翻转 `PurgeReleaseAllowed()` 或将 Docker 源卷改成 RW 作为“功能完成”。旧公共镜像、Tag、DMG 和验收回执保持不变。

## 1. 自主执行模式

一个 Codex/TRAE 长工程主 Agent 负责持续推进，通常工程选择无需逐项请示。初次独立复核/复审可以使用与实现者不同的会话或代理，报告须与精确源码 SHA 绑定；不将作者自查称为独立审查。

允许在已授权的开发分支上实现、修复、测试、推送、建立 PR，并按正常仓库规则合并已审查的变更。不得绕过主干保护、自行批准自己 PR、强制覆盖现有公开资产或擅自开启真实 NAS 删除。

**需要用户重新明确授权的唯一关键动作：**
- 对现有生产 NAS 数据执行任何实际隔离/恢复/永久删除；
- 变更主机 SSH/Docker 配置、路由、防火墙或正式 NAS 的写入挂载；
- 发布新的有永久删除能力的公网版本或更改已发布镜像；
- 取消/下调既有安全门禁，或者引入无法解释的越权请求。

其他可验证的合成数据工作不要求反复询问。

## 2. Phase A — 公开首页和品牌入口（优先完成）

当前文档修订分支：`docs/refresh-homepage-dualchannel-20261015`。已更新根 `README.md`、`docs/README.md`、`docs/multichannel/RELEASE-STATUS.md` 与 `docs/multichannel/CAPABILITY-MATRIX.md`。

1. 对整个仓库（包括 README、指南、相对 Markdown 链接、图片、GHCR、Release、Actions、GitHub Issues、官方 fnOS/Apple 文档）制作可重复执行的链接检查。分别输出 LOCAL_PASS / HTTP_PASS / AUTH_REQUIRED / 429 / DEAD / REDIRECT / UNKNOWN；网络/鉴权故障不能写成链接已死。
2. 核查 ```README.md```、`SUPPORT.md`、`SECURITY.md`、`CONTRIBUTING.md`、`CHANGELOG.md`、`docs/README.md`、`docs/user-guide/NDG-用户指南.md`、`docs/multichannel/*` 和所有产品入口中的旧“未发布/Draft”等现时态信息。归档历史快照保留原句，但加明显“历史、以新发布为准”说明，不制造事实。
3. GitHub 仓库 metadata 需要单独更新：description 建议“NDG｜本地优先的 NAS 数据治理工作台；macOS Beta 与 Linux Docker Preview，安全扫描、隔离和恢复”，homepage 建议 `https://github.com/FNB2026/nas-data-governance/releases`；topics 可补 `docker`、`macos`、`file-management`（不超过 GitHub 限制）。通过具有权限的官方 GitHub Settings 或 `gh repo edit` 完成，先记录原值；没有权限则明确 BLOCKED，不伪称更新。
4. 当前规范图标源 `cmd/ndg-desktop/build/appicon.png`；旧首页使用 `docs/assets/ndg-logo.jpg`。盘点 macOS ICNS、README 标识、fnOS `ICON*.PNG`、Web favicon、安装包和截图是否统一。旧发行二进制图标**不得**改写；后续候选可从统一品牌源导出正确尺寸，核对缩放可读性、透明度、版权和安装后实际图标。特别检查 Docker Web 当前 favicon 401；修复后仅记入新版本，不修改已发布 Preview.1。
5. 文档状态与公开版本必须一致，README 同时列出两个发行渠道的区别、下载与安装链接、保密信息申报、许可边界和“完整治理能力尚未交付”的清晰文案。
6. 运行 `make public-check`、Markdown/link check、README 图片路径校验、Go/TS 相关版本一致性；产出单独文档/品牌 PR，复核后合并，避免与高风险执行代码混合。

**验收**：仓库首页能直接下载公开 Mac/Docker 版本；不误导用户认为 Docker 可删除；链接有复核证据；当前 Logo 与品牌来源明确；历史公开资产未变。

## 3. Phase B — 平台统一的治理事务合同

以现有 Go `internal/app/execution.go`、`quarantine.go`、`purge_service.go`、`recovery.go`、`write_gate.go`、`internal/executor/*`、`internal/store/*`、`internal/domain/*` 和现有 Decision/Plan/Journal/Approval 不变量为核心。

形成一次规范化状态机（不能把批准与执行合并）：

```text
SCAN → DUPLICATE_EVIDENCE → HUMAN_SELECTION
     → DRAFT_PLAN → APPROVE_PLAN → DRY_RUN
     → FRESH_SOURCE_RECHECK → QUARANTINE_EXECUTE
     → VERIFIED_QUARANTINE → RESTORE_PLAN/RESTORE_EXECUTE
                            ↘ AFTER_RETENTION → PURGE_DRAFT
                              → INDEPENDENT_PURGE_APPROVAL → PURGE_DRY_RUN
                              → EXPLICIT_HIGH_RISK_CONFIRM → PURGE_EXECUTE
```

- 执行前和提交前必须有根目录和成员身份核验、文件内容/元数据一致性、保护目录规则、计划 digest/CAS、单写者锁、Recovery Lock、审计/Journal 和取消边界。
- 删除功能**只允许清理已受管理且可证明身份的隔离文件**；不允许任意路径删除、扫描结果直接永久删除、UI 跳过隔离或自动全量删除。
- Purge 按现有单文件高风险政策逐项执行；批量 UX 可生成多个独立待审批项，但不得把一次按钮点击隐式批准全部永久删除。
- 至少 24 小时保留期为当前执行服务的已存在基线；是否提高默认期限由产品决策明确，不临时降低为零。
- Journal 或恢复锁状态未知、哈希不匹配、源根替换、权限不足、不同挂载/物理身份未确认，均 FAIL CLOSED。
- 跨设备隔离必须先复制到受控临时路径、完整验证且持久化关键证据，随后才能安全处理源；copy/rename/unlink 竞争窗口、崩溃与重复提交必须覆盖。
- Restore 不能覆盖用户后来创建的不同文件；无法确认输出、孤儿隔离、半完成 Purge 保留人工核对。

验证标准：原子性、幂等性、双击/重复请求、并发冲突、升级/回退、崩溃恢复、备份恢复、数据库 WAL/SHM 及原始源哈希有真实可重复证据。

## 4. Phase C — macOS 桌面完整治理执行

先复核 Mac beta.11 已有隔离与恢复的真实可用范围。新版本必须绑定当前源码、签名与公证身份，不能引用旧 beta.11 的运行证据证明未来 UX v2。

- 将新批量选择/治理意图合法关联已审核计划，记录真实 durable receipt。
- 针对 [#111](https://github.com/FNB2026/nas-data-governance/issues/111) 的 Darwin 同设备嵌套挂载、源根替换与 ABA 风险，独立审查公开系统 API 可支持的原子边界证明。不存在等价安全证明时保持新路径 BLOCKED，不换回弱 reader、不用 `st_dev` 相等冒充不跨挂载。
- 如果能明确证明某些受支持 Mac 本地文件系统范围，可定义**独立受限的支持合同**，经设计复审和用户批准后，仅对该可证范围放行；SMB/NFS/FUSE 不自动继承。
- 先完成 Quarantine/Restore 稳定事务，再另开 Purge 的发布政策评估；只有所有阻断消除才在**新发行版本**中设计显式 enable，旧发行 Purge 默认不变。
- 真实签名 App 验证文件交换、网络断开、恢复锁、操作审计、备份、异常提示、非开发者可理解性。

## 5. Phase D — Linux Docker 完整治理执行

Docker `Preview.1` 不改；开新可命名写入渠道的独立预览候选，例如 `docker-governance-preview`（实际版本名称由版本策略决定）。

- 保留扫描/Web 进程只读权限。单独的最小权限写入工作进程（或等效可审查权限分区）持有**明确授权**的单个源根 RW、受管理隔离区及私有状态目录；Web 不可直接决定文件系统路径。
- 写入 worker 不接受任意宿主路径，不挂载 Docker socket、宿主 `/`、设备或任意 Docker 管理权限；原则上非 root、drop all capabilities、no-new-privileges、read-only rootfs，只有明示数据目标 RW。
- 使用已验证的 Linux 根约束（如 `openat2` 的 BENEATH / NO_SYMLINKS / NO_XDEV）和 mount/namespace 身份；mount-on-file、同设备 bind、重挂载和 TOCTOU 都必须纳入真实负例。
- 保留认证、CSRF、Host/Origin、限额、无未经授权远程 API 和私有凭据，加入提交前的服务器端二次确认与幂等键。
- 先在独立 Linux 本地人工文件系统完成文件复制、隔离、恢复、留存期和逐项 Purge；再到隔离 fnOS 合成源测试。真实 NAS 的 SMB/NFS/FUSE 或生产数据无测试即不宣称支持。
- fnOS 客户端入口的 `AllowTcpForwarding=no` 问题另寻经授权且可验证的接入方案；不能用开启公开 Docker 端口作为偷偷的快捷修复。
- 真实容器验收必须覆盖停止/重建、state 保留、双 writer 拒绝、隔离卷意外脱载、取消、崩溃、无误删源、备份及隔离恢复。

## 6. Phase E — 界面与体验

Mac 和 Web 要有一致的用户术语、状态说明和操作可见性，但平台能力必须独立显示。

- 分清“保留（KEEP）/忽略或复核（REVIEW）/隔离（QUARANTINE）/恢复（RESTORE）/永久删除（PURGE）”。
- 批量选择、分组计数、显式范围、不跨筛选条件偷偷扩充对象；大规模风险操作显示具体项目数、字节数、保护原因、不可撤销性及有效保留副本数。
- 真实删除前至少两次不同阶段的授权：治理计划审批与高风险 Purge 明确确认（文本/机制与后端精确一致）。
- 结束状态不可将“部分成功”“未知锁”“执行计划已建立”误显示为“文件已清理成功”。
- 未提供的渠道按钮要诚实禁用并说明原因；不以“复杂技术术语”误导普通用户。

## 7. 最小跨平台验收矩阵

| 类型 | 桌面 Mac | Docker/Linux | 不能省略 |
| --- | --- | --- | --- |
| 扫描和重复组 | 已有 Beta 限定证据，未来发行物重测 | Preview.1 只读证据可参考，新写入版本重测 | 合成源 SHA 前后保全 |
| 隔离 | 签名原生 GUI + 真文件 | 独立工作进程 + 真文件/卷 | 明确根权限 + Journal/审计 |
| 恢复 | 同路径/目标已变化/跨卷 | 同上及容器重建 | 不覆盖后来新文件 |
| 永久清理 | 仅已管理隔离项 | 仅已管理隔离项 | 保留期、逐项审批、Dry Run、再次确认 |
| 竞争与挂载 | Darwin #111 边界证明 | openat2/namespace/mount 实际对抗负例 | 未证明平台维持 BLOCKED |
| 异常恢复 | 冷备份 + 断电/中断 | 冷备份 + 容器删除/重建 | 恢复锁与幂等性 |
| 对外发布 | Developer ID、公证、更新兼容、Gatekeeper | OCI digest、SBOM、漏洞、安全、GHCR 可拉取 | 各自独立发布裁定 |

每个阶段可以自主开发/修复/重跑 CI。失败归因和不确定项必须保留真实状态。仅限新建的人工数据进行实际删除测试。

## 8. 交付物与汇报格式

一次工程委托内持续完成可独立推进的工作，避免每个门禁都停下来请示：

1. 首页/品牌 PR：完整链接审计报告、README、文档索引、图标一致性和 GitHub 元数据差异。
2. 核心执行合同 PR：跨渠道业务不变量和可恢复原子操作回归。
3. Mac 执行候选 PR：#111 边界与真实签名 GUI 验收。
4. Docker 写入候选 PR：最小写入权限架构、真实 ARM64/AMD64 和 fnOS 合成源验收。
5. 各自 release candidate：安装/升级/备份/卸载/风险说明，不自动公开。

每轮最终只报告：**当前 main SHA、提交与 PR、完成的真实功能、PASS 与 NOT RUN/BLOCKED、下一项单一主阻断、是否需要所有者授权**。不要以文档完成次数代替用户真正可用的隔离、恢复和删除。
