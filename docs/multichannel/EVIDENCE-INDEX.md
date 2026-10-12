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
- 新 Required Reason API 专项与直接版 exporter 设计：[Draft PR #92](https://github.com/FNB2026/nas-data-governance/pull/92)，exact HEAD `f61366acf6b9f6b9066e2dfb0229fc7794fbf713`。独立 `/root/phase1c_review` 研究范围 PASS；patch SHA-256 `3c213510023b1f1c227117cdfdae79ad0f809b7c24c2f3c331da88c9ad7ae716`（总控回执）。[CI 38270937225](https://github.com/FNB2026/nas-data-governance/actions/runs/38270937225) 本次只读采样 in_progress；[Security 38270937211](https://github.com/FNB2026/nas-data-governance/actions/runs/38270937211) SUCCESS；两者 headSha 与 exact HEAD 相同。尚未证明网络卷历史身份等价，未取得正式导入/商店运行认证；研究审查不替代门禁。

## 本工作包验证回执

本 PR 只新增管理文档，未更改产品、版本、Tag、发行资产、私有证据或部署状态。本工作树实际执行：`make test vet build public-check release-check version-check`、`go test -race -count=1 ./...` 均 exit 0 / PASS；对应可执行源码与 main `584aafeb4cf1bb68cbda3bde2d26698b05555640` 完全相同，仅五份本目录文档变化。全部附件正文比对（仅规范化行尾空格）、本目录私人路径检查、相对链接检查及 `git diff --cached --check` PASS。测试没有启动真实 NAS、MAS App、Docker 或 fnOS。

本机私有回执 SHA-256：required gates `79059d0e3776a7df32b0b68704a53502ff378fc94a2966364dfa5035a7d8f1fc`；fresh full race `af51328ceb1792aa4ff021284fd964fa045f20ad9ab71b300468a25c97fa1d91`。原始输出不入公共仓库。本 PR 的独立 review 和 GitHub CI/Security 待 exact HEAD 出具；本地 PASS 不替代这些门禁。

## Docker / fnOS

G4 独立实现工作包已启动，但无可发布 OCI digest、正式 Web GUI/容器 PASS 或 fnOS FPK。本机 Docker 未发现；官方 fnpack 1.2.3 已私有下载，help 与模板测试 PASS（总控工具回执，非 Developer ID 签名证明）。`codex/fnos-readonly-package` 正在实现非可安装研究骨架与 fail-closed validator，尚无固定 OCI digest 或 FPK 真机 PASS；工具 --help、交叉编译或源码测试不是部署证据。CI/实际架构、镜像、设备、SBOM 与 provenance 取得后按 source SHA/digest/测试层分别新增，不改写历史 NOT RUN。

- 本机签名环境（总控只读回执）：Developer ID Application identity=1；Apple Distribution / Mac Installer Distribution 未发现。该证据只覆盖当前机器可见 identity，账户资格、Profile 与 App ID 仍未证实。
- [fnOS 官方测试应用规范](https://developer.fnnas.com/docs/quick-started/test-application/) 2026-10-12 读取成功：手动安装只允许本地测试，不作为公开分发方式；外部分发政策需咨询，不能把生成 FPK 自动解释为可公开发布。
