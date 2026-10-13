# NDG 多渠道证据索引

快照时间：2026-10-12。公开索引只保存允许发布的 SHA、状态和回执 URL；原始私人截图、数据库、日志、Bookmark、主机路径与凭据留在本机私有归档。哈希是完整性标记，不证明证据作者或可信来源。

## 基线 / 总授权

- 总执行附件：完整 519 行正文入库 [主手册](NDG-MULTICHANNEL-MASTER.md)，源正文 SHA-256 见其来源说明。仓库许可和私人边界适用；附件实时快照差异另列，正文历史不覆盖。
- 主工作基线：main `584aafeb4cf1bb68cbda3bde2d26698b05555640`，PR #91 squash merge；本台账独立分支。用户明确授权连续开发、测试、PR、合规合并及台账；新公开、外部最终审批、生产写入和账户专属动作例外。

## 已公开 Developer ID beta.11

- [正式 Release](https://github.com/FNB2026/nas-data-governance/releases/tag/v0.5.0-beta.11)：API 只读核对 `draft=false`、`prerelease=true`、`published_at=2026-10-11T16:01:08Z`。不以 target_commitish 的字符串 `main` 代替 Tag 解引用身份。
- RC `791f19119542e69b62ee62dd358fed188000837e`；annotated Tag 对象 `395623c9ff0154d8ba7d0c63a109e1f8df2dccec`（本轮本地 Tag 对象及 peel 与冻结证据一致）。
- [发行工作流 38122101720](https://github.com/FNB2026/nas-data-governance/actions/runs/38122101720)：历史正式签名、公证、发行资产构建证据；不是新渠道批准。
- 用户明确授权回执：执行发布脚本前对“豁免新 Mac 离线首装并公开 beta.11”的确认，答复“确认豁免并授权公开 beta.11”。保存于当前用户任务历史；不伪造 GitHub 人类 Review。首装仅 WAIVED_BY_OWNER，现有证据不支持 PASS。
- [历史 beta.11 正式验收](../release/beta.11-formal-acceptance-go-no-go-2026-10-11.md) 与 [候选准备](../release/beta.11-release-candidate-2026-10-11.md) 保留当时 NO-GO/Draft/BLOCKED；本索引记录后续授权/公开，不覆盖原文。

| 冻结资产 | 2026-10-12 Release API digest（SHA-256） |
|---|---|
| NDG-0.5.0-beta.11-macos.dmg | `aa3d9dada7bd1f670f70e4385e91e16a0927b5a8855656394d99a9e4f1fe5273` |
| DMG SHA 文件 | `1f4ae3f04d95e59307c95d8e7ede669f0e187af6204e923490798e7b49227e06` |
| CycloneDX SBOM | `a545ffee13dc57f78123ca0b5634808b6d0b2ebd496347b1fcde1d3023b0c17d` |
| SPDX SBOM | `837c51e70f8212e3d2312b6922c699cc4a05f31510fbe70e3ffd5c30cf073778` |

四个 digest 与历史冻结记录一致；本轮未下载/重新签名/修改资产。Go SBOM 不证明 npm 完整覆盖。

## MAS 证据与限制

- [MAS 原型与 Phase 1A 总入口](../mas/README.md)，[Phase 1B 实际运行记录](../mas/phase1b-runtime-2026-10-12.md)，[Phase 1B 待办](../mas/phase1b-pending-tests-2026-10-12.md)。Phase 1A/1B 运行 INCONCLUSIVE；独立环境 BLOCKED，CarbonCore 历史 FAIL 永久保留。
- [PR #91](https://github.com/FNB2026/nas-data-governance/pull/91)：最终审查 HEAD `b368d2a1ebeb40545bfb5780a29a66fed815cc19`；用户指定原 HEAD `34066905100f80f9915d1959494662327ad3e040` 后仅修订注释/文档边界；merge `584aafeb4cf1bb68cbda3bde2d26698b05555640`。
- 独立 AI reviewer `/root/phase1c_review` 对上述 exact HEAD 出具范围限定报告；不是作者自批 GitHub APPROVE。tree `28a25e3dcdcbe3d93cca9bc9dda2afbfb1a6bee3`，base..HEAD diff SHA-256 `3df8c05b64272fd44e800afc340ff596944cfeeece3717c1ccb8a49063f74820`。
- [CI post-merge 38269775992](https://github.com/FNB2026/nas-data-governance/actions/runs/38269775992)：SUCCESS；[Security post-merge 38269821359](https://github.com/FNB2026/nas-data-governance/actions/runs/38269821359)：SUCCESS；两者 source SHA 为 merge SHA。两者最终结论已只读核对为 SUCCESS；仅 Phase 1C 预检原型 CLOSED。
- [Phase 1C 容器/兼容性原型证据](../mas/phase1c-container-and-compatibility-2026-10-12.md)：调用方必须证明 staging owner、ACL、容器和全调用稳定性；0700 不等于证明；未连 Wails，正式导入 NOT RUN / BLOCKED。
- 新 Required Reason API 专项与直接版 exporter 设计：[PR #92](https://github.com/FNB2026/nas-data-governance/pull/92)，exact HEAD `f61366acf6b9f6b9066e2dfb0229fc7794fbf713`。独立 `/root/phase1c_review` 研究范围 PASS；patch SHA-256 `3c213510023b1f1c227117cdfdae79ad0f809b7c24c2f3c331da88c9ad7ae716`（总控回执）。[CI 38270937225](https://github.com/FNB2026/nas-data-governance/actions/runs/38270937225) SUCCESS；[Security 38270937211](https://github.com/FNB2026/nas-data-governance/actions/runs/38270937211) SUCCESS；两者 headSha 与 exact HEAD 相同。尚未证明网络卷历史身份等价，未取得正式导入/商店运行认证；研究审查不替代运行门禁。合并提交 `4f9ed9762f5754e4073eba01230f021b05788f79`；合并后门禁独立执行中。

## 本工作包验证回执

本 PR 只新增管理文档，未更改产品、版本、Tag、发行资产、私有证据或部署状态。本工作树实际执行：`make test vet build public-check release-check version-check`、`go test -race -count=1 ./...` 均 exit 0 / PASS；对应可执行源码与 main `584aafeb4cf1bb68cbda3bde2d26698b05555640` 完全相同，仅五份本目录文档变化。全部附件正文比对（仅规范化行尾空格）、本目录私人路径检查、相对链接检查及 `git diff --cached --check` PASS。测试没有启动真实 NAS、MAS App、Docker 或 fnOS。

本机私有回执 SHA-256：required gates `79059d0e3776a7df32b0b68704a53502ff378fc94a2966364dfa5035a7d8f1fc`；fresh full race `af51328ceb1792aa4ff021284fd964fa045f20ad9ab71b300468a25c97fa1d91`。原始输出不入公共仓库。本 PR 的独立 review 和 GitHub CI/Security 待 exact HEAD 出具；本地 PASS 不替代这些门禁。

## Docker / fnOS

G4 独立实现工作包已启动，但无可发布 OCI digest、正式 Web GUI/容器 PASS 或 fnOS FPK。本机 Docker 未发现；官方 fnpack 1.2.3 已私有下载，help 与模板测试 PASS（总控工具回执，非 Developer ID 签名证明）。`codex/fnos-readonly-package` 已提交非可安装研究骨架与 fail-closed validator（`6025bf7f5f38df372bb7a232b59bd5b38eb0e9c9`，独立复审中），尚无固定 OCI digest 或 FPK 真机 PASS；工具 --help、交叉编译或源码测试不是部署证据。CI/实际架构、镜像、设备、SBOM 与 provenance 取得后按 source SHA/digest/测试层分别新增，不改写历史 NOT RUN。

- 本机签名环境（总控只读回执）：Developer ID Application identity=1；Apple Distribution / Mac Installer Distribution 未发现。该证据只覆盖当前机器可见 identity，账户资格、Profile 与 App ID 仍未证实。
- [fnOS 官方测试应用规范](https://developer.fnnas.com/docs/quick-started/test-application/) 2026-10-12 读取成功：手动安装只允许本地测试，不作为公开分发方式；外部分发政策需咨询，不能把生成 FPK 自动解释为可公开发布。

## 2026-10-13 增量（不覆盖历史快照）

main `15328e0054c1c3e476dd756436ce52d6f3c50c1a`。beta.11 Release API 再次核对
公开 Pre-release、发布时间及四个资产 digest 与上表完全一致；无历史资产改写。

- PR #92 merge `4f9ed9762f5754e4073eba01230f021b05788f79`：
  [CI post-merge 38272143547](https://github.com/FNB2026/nas-data-governance/actions/runs/38272143547)、
  [Security 38272175403](https://github.com/FNB2026/nas-data-governance/actions/runs/38272175403) SUCCESS。
- [PR #94](https://github.com/FNB2026/nas-data-governance/pull/94) 统一台账 merge
  `2064494786b4b64f851f4a645531f6684510518b`；exact HEAD
  `6f6283a12a47fc3de612ab9164dc5060cf75a68a` 独立文档复审、CI/Security PASS。
- [PR #93](https://github.com/FNB2026/nas-data-governance/pull/93) merge
  `356383115f43c5d75ed2827cf7520d3172ae055f`；exact HEAD
  `3dcd7a73b0b255cadc23d3dbda366b6c12cecab5`，tree
  `f87833ff80e0d371e04e5c65212a927a492dd454`；independent reviewer
  `/root/pr90_review` 多轮代码/补丁身份复审无阻断；PR patch SHA-256
  `84359d287435c1538829907b911b9218ef04d448286fbc126c134b27ea35fd00`。
  [原生 Ubuntu amd64/ARM64 38341940563](https://github.com/FNB2026/nas-data-governance/actions/runs/38341940563)
  合并后 SUCCESS；PR native run 38273652052 SUCCESS。实际 synthetic container
  7 文件、7 quick/6 full/3 groups/checkpoint completed、源哈希不变、写 API 403、
  第二 writer 拒绝、停止重启持久化、React 静态资产/CSP 及限定日志窗口通过。
  初始 CI 38272681883 FAIL 永久保留；它暴露 checkpoint summary 取错可恢复接口，
  修复不改变原 LastCheckpoint 的 Resume 语义。不是完整 G4 发行认证。
- [PR #95](https://github.com/FNB2026/nas-data-governance/pull/95) merge
  `15328e0054c1c3e476dd756436ce52d6f3c50c1a`；exact HEAD
  `6531d9b25187da0551bcc374f323e90a54f247c2`，tree
  `3737120b003bd0debe26923d4edc8c508f5ab192`；independent reviewer
  `/root/phase1c_review` 指出嵌套挂载 P2，修复后逐轮复审无阻断；最终 PR patch
  SHA-256 `f0bc447696124075e330b6285c5b9f562faf03288f739e42702325416d96bc6e`。
  [CI 38343732167](https://github.com/FNB2026/nas-data-governance/actions/runs/38343732167)、
  [Security 38343736081](https://github.com/FNB2026/nas-data-governance/actions/runs/38343736081)、
  [Linux native 38343738845](https://github.com/FNB2026/nas-data-governance/actions/runs/38343738845)
  均为此 merge SHA，SUCCESS。24 offline/mock fnOS tests PASS；build wrapper 即使
  receipt 宣称 PASS 仍拒绝生成包；真实 mount/FPK/device 仍 NOT RUN。

UX v2 附件 679 行 SHA-256 见实时台账。前端 frozen author commit
`214cabfd4d84297eca8f1d04957b27ad8d5635ad`，tree
`07c989fb2dd45955c4fc564efb37f6dde840172b`；独立复审中，所有真实数据库/正式 GUI/
理解测试状态分层保留。G6 OCI 证据实现尚未冻结，绝不预记 workflow PASS。

## 2026-10-13 新开发身份与证据限制

- UX1 [#96](https://github.com/FNB2026/nas-data-governance/pull/96) exact HEAD
  `8e6d1d1e69ed3b57a5edcd94fc0066221e103dc3`。本地 296 frontend tests /
  Go fresh race 和构建通过。独立 `/root/phase1c_review` 原始复审止于 `7fbfdcf`；
  [GitHub Codex review 请求](https://github.com/FNB2026/nas-data-governance/pull/96#issuecomment-6138979646)
  在 `3f679388` 产生实际三项发现。最新 delta 修正并补测试，尚未取得新的独立结论。
- UX2 [#98](https://github.com/FNB2026/nas-data-governance/pull/98) exact HEAD
  `e4dfaaadccb94ebf33e9d1be2fc48ba3e060f5e8`，实际产品代码 `0a60a727`，最新增量为 MAS 测试夹具及文档。
  原作者 `/root/linux_web_implementation` 停止后由 root 接手，原 lease 保留。
  真实 Go adapter/scanner/store/SQLite 七个临时源文件：自然完成、三组、六个 full
  hash、普通单例无 full hash、record-only 提交/重启/只读回执、源 SHA 不变。
  这不是原生 Wails GUI 的 PASS。fresh Go/race、Wails 2.13.0 dev build 及 SDK
  同步通过；早期 CI `70227525` 的 Gitleaks 和 Desktop Build FAIL 永久保留。
- G6 [#97](https://github.com/FNB2026/nas-data-governance/pull/97) exact HEAD
  `bbe9d35e3fa034824cf87889262982771c7beb75`。15 offline OCI tests 与 Go/race
  通过，全历史 Gitleaks v8.28.0 扫描 307 commits 无新发现。
  [独立 review 请求](https://github.com/FNB2026/nas-data-governance/pull/97#issuecomment-6138979994)
  仅得到 usage-limit 回复，不能算独立审查。新 dispatch collector 仍 NOT RUN；
  旧 Linux native 合成容器绿色不能替代新 collector/SBOM/漏洞结果。
- 台账先前全量 race 有一次 `TestListGroupDecisionsOrderingByUpdatedAt` 排序断言
  FAIL；20 次针对性复跑 PASS 只证明当前未复现，根因未定。原始日志私有保存，
  不把绿色复跑覆盖失败。所有 latest PR CI 必须按 exact HEAD 另外读取。

原始日志/截图只在私有归档；不在文档写入真实源路径、标记、凭据或数据库。
完整 UX3 batch discovery、物理未索引 stale、真正 GUI/签名层与理解测试尚未闭合。

- 后续本地台账 full required gate/fresh race 返回 0；历史排序断言 FAIL
  仍保留，不声称根因已修复。strict Gitleaks v8.28.0 / 308 commits 无新发现。
- #96 `8e6d1d1` 与 #97 `bbe9d35` 当前全部八项 CI/Security/Linux 检查 SUCCESS；
  并未取得最新独立审查。#98 `b1cdae9c` 的 distinct MAS fixture RED
  已以 `e4dfaaa` 补正确 sandbox 创建路径，真实授权/审批拒绝/持久 DRAFT
  定向测试以及默认完整 Go/race 通过；最新 CI 单独采集，不继承旧结果。


## 2026-10-13 后续实证

- #96独立reviewer fnos_official_audit，head a91610236f5ef958a2c91949629617edafeabe49、
  tree68ca56373b6b1c96ee671bd1b5ea6c72b6b9ff93、basebacc160；binary diff
  cdc928266f49c62dac1f06a9284640c23402040ccbe72f665dc52ac7ddab5a39。
  merge c95c55d588c7b9ede71e8931e54c492506080f08，8postmergechecks/local gates/race PASS。
- #98 head25f75ae451ca8f01f13e4844e819df8b6bf74084：独立phase1c_review完整布局/
  marker/WAL及真实hot-journal拒绝PASS；兼容页delta
  f4c2346dc3a04e5ba75fec8292c285c0d292ca167aec6699cfd14d77b07ceb69，297frontend/fullrace/8CI PASS。
  但已知候选audit时间排序需等待#102，不提前合并/记全P0 PASS。
- #100 headfbe32a0d1c8d3a27c3296bedb10e9b8629ebbd52，tree99819c47701e56929746788261820d1d0ccc5d31；
  independentlinux_web_implementation，fixdelta d0a1e96e0adb306acd384f1b6c2da31e752f735150b37613529d4f315d5ca66e。
  两P2真实RED→GREEN，309frontend/fullrace PASS。manual CI/security/Linux runs
  38391964183/38391969257/38391973843按SHA另核，不是已retargetmain。
- #97 mergebacc160，实际run38387062893：bounded runtime/scanPASS，source dev库存缺。
  #101 head063d0ee980ae8115cc92c0c9df0c5c43e540f14b/treef56e54626b060bb05b0c66b21e473f754a63e201；
  independentfnos_official_audit source/code及actualrun38391959321双架构逐ZIP/11files/
  content/runtime/207lock/202dev/205tuple/两SBOM PASS。source-map-js1.2.2存在；npm全级0、go192events0findings、Grype0。
  amd64OCI6d59909f00e540d93d5fc60a36754f08692f4dcecd251aaa5843b6d1e9999336，
  arm64OCI354cafa559305e47362e5f03a29fcf232842203b37c2cd9c9c4ede96aeb4b3cf。
  merge443beeb0e19fa6a737cd8fef5c20e0197bfe4392，postmerge另核；unsigned/repro/全G6未闭。
- fnOS只读设备inventory2026-10-13T11:54:10Z：user授权设备，Linuxx86/Docker28.5.2、
  12running、overlay2；无source读写/既有容器动作，test-area尚未建立。原输出和凭据
  只在本机私有归档，不在公共文档保存地址、账号、路径、环境变量或密码。

以上日期绑定证据不把旧FAIL覆写成PASS。MAS/正式native/真实SMB/FPK/其他NAS依赖仍保持。
