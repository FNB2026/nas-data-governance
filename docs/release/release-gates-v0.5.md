# NDG v0.5 发布门槛清单

> 基于 `readiness-audit-v0.5.md` 审计结果
> 审计日期：2026-07-30
> 基线：`origin/main` = `1bc0959a846336f9e333fe078679199387cc60f0`

## 状态定义

```
BLOCKER    不解决不得公开发布
REQUIRED   公开 Beta 前必须完成
RECOMMENDED 可在 Beta 周期内完成
DEFERRED   明确延后到 1.0 或后续平台
```

---

## BLOCKER

| # | 门槛 | 当前状态 | 对应阶段 | 审计项 |
|---|---|---|---|---|
| B-01 | 版本单一来源：Git tag、wails.json、package.json、Info.plist、version.go 全部一致 | 三处不一致（v0.4.0 / 1.0.0 / dev） | R1 | A-1, A-2, A-3 |
| B-02 | macOS Developer ID 签名 + Hardened Runtime + Secure Timestamp | 仅 ad-hoc 签名 | R2 | B.2 |
| B-03 | Apple 公证（notarytool）通过 + Staple | 未公证 | R2 | B.2 |
| B-04 | DMG 签名、公证、Staple 通过 | 无 DMG 创建能力 | R2 | B.5 |
| B-05 | `open-source-boundary.md` GitHub Support 清理确认门禁关闭 | 未勾选确认，文档状态矛盾 | R0 人工 | D.4 |
| B-06 | GitHub 原生 Secret Scanning + Push Protection 启用 | 待确认（boundary 文档未勾选） | R5 人工 | D.7 |
| B-07 | GitHub Private Vulnerability Reporting 启用 | 待确认（SECURITY.md 措辞暗示未启用） | R5 人工 | D.7 |
| B-08 | main 分支保护启用（require PR、status checks、block force push） | 待确认 | R5 人工 | D.7 |
| B-09 | Release Artifact 从 Git Tag 构建，SHA-256 可验证 | 无 Release Workflow | R3 | C.3 |
| B-10 | Wails CLI 版本固定（禁止 `@latest`） | Makefile 使用 `@latest` | R2 | C.4 |
| B-11 | Bundle ID 为项目自有标识（非 `com.wails.*`） | `com.wails.ndg-desktop` | R1 | B.1 |
| B-12 | 所有发行物从明确 Git Tag 构建 | 无 tag 触发工作流 | R3 | C.3 |

---

## REQUIRED

| # | 门槛 | 当前状态 | 对应阶段 | 审计项 |
|---|---|---|---|---|
| R-01 | VERSION 单一来源文件 | 不存在 | R1 | A-4 |
| R-02 | `sync-version.sh` 版本同步脚本 | 不存在 | R1 | A-5 |
| R-03 | `check-version-consistency.sh` 版本一致性校验脚本 | 不存在 | R1 | A-5 |
| R-04 | `wails.json` `frontend:install` 改为 `npm ci` | 为 `npm install` | R1 | A-6 |
| R-05 | `entitlements.plist` 最小权限配置 | 不存在 | R2 | B.3 |
| R-06 | `LSApplicationCategoryType` 设置为 `public.app-category.utilities` | 缺失 | R1 | B.1 |
| R-07 | `LSMinimumSystemVersion` 设置为 `13.0` | 为 `10.13.0` | R1 | B.1 |
| R-08 | 桌面 Release Workflow（tag 触发，4 job：verify/build-unsigned/sign-notarize/draft-release） | 不存在 | R3 | C.3 |
| R-09 | GitHub Release Environment `release-macos` 配置 required reviewer | 不存在 | R3 人工 | C.3 |
| R-10 | 首次启动 Beta 安全声明（用户确认后才能继续） | UI 无任何 beta 提示 | R4 | E.1 |
| R-11 | Purge 默认隐藏或禁用（Beta 安全模式） | 已实现（多重门控） | R4 | E.3 |
| R-12 | 错误信息分类体系（输入/权限/离线/数据库/中断/恢复锁/安全门控/未知） | 未实现 | R4 | E.5 |
| R-13 | 脱敏支持包导出功能 | 不存在 | R4 | E.9 |
| R-14 | `docs/install-macos.md` | 不存在 | R5 | E.6 |
| R-15 | `docs/quick-start-desktop.md` | 不存在 | R5 | E.6 |
| R-16 | `docs/privacy.md` | 不存在 | R5 | E.6 |
| R-17 | `docs/upgrade-and-backup.md`（含数据库迁移备份与回滚） | 不存在 | R5 | E.6 |
| R-18 | `docs/uninstall.md`（含数据残留说明） | 不存在 | R5 | E.6, E.8 |
| R-19 | `docs/beta-safety.md` | 不存在 | R5 | E.6 |
| R-20 | `KNOWN_ISSUES.md` | 不存在 | R5 | E.6 |
| R-21 | README 重构为桌面优先 | CLI 优先 | R5 | E.6 |
| R-22 | CHANGELOG 版本化条目 | 仅有 Unreleased | R5 | E.6 |
| R-23 | About 页显示许可证入口、项目主页、隐私说明、支持说明 | 部分缺失 | R1 | E.4 |
| R-24 | 品牌图标（非 Wails 默认图标） | 使用默认图标 | R1 | B.4 |
| R-25 | `outputfilename` 改为 `NDG` | 为 `ndg-desktop` | R1 | B.1 |
| R-26 | 桌面构建 ldflags 注入 Version/Commit/BuildTime/Channel | desktop-build 不注入 | R1 | A.2 |
| R-27 | 发布脚本（build/sign/notarize/dmg/verify/manifest） | 全部不存在 | R2 | B.6 |
| R-28 | `release-manifest.json` 生成 | 不存在 | R2 | B.8 |
| R-29 | 第三方许可证覆盖前端 npm 依赖 | 仅覆盖 CLI Go 依赖 | R5 | D.6 |
| R-30 | Dependabot 监控 npm 生态 | 未配置 | R5 | C.5 |
| R-31 | 数据库迁移前自动备份 + 失败回滚 | 未实现 | R5 | E.6 |
| R-32 | 发布构建可复现（固定 Go/Node/Wails 版本） | Wails 未固定 | R2 | C.4 |
| R-33 | `CODE_OF_CONDUCT.md` | 不存在 | R5 | D.7 |
| R-34 | `docs/release/github-settings-checklist.md`（GitHub UI 操作清单） | 不存在 | R5 | D.7 |
| R-35 | `docs/release/release-checklist.md` | 不存在 | R5 | E.6 |
| R-36 | `docs/release/release-notes-template.md` | 不存在 | R5 | E.6 |
| R-37 | `docs/release/beta-test-guide.md` | 不存在 | R5 | E.6 |
| R-38 | `docs/troubleshooting.md` | 不存在 | R5 | E.6 |
| R-39 | `docs/support-bundle.md` | 不存在 | R5 | E.6 |
| R-40 | 语义版本到 macOS Bundle 版本映射（CFBundleShortVersionString 纯三段数字，CFBundleVersion 递增数字构建号，禁止 `-beta.N` 后缀） | 未定义映射规则 | R1 | B.1 |

---

## RECOMMENDED

| # | 门槛 | 当前状态 | 对应阶段 | 审计项 |
|---|---|---|---|---|
| M-01 | CI 增加 `wails build` 桌面端回归 | CI 不构建桌面端 | R3 | C.1 |
| M-02 | CI 增加 macOS runner | 仅 ubuntu-latest | R3 | C.1 |
| M-03 | CI 增加 npm audit | 未配置 | R3 | D.5 |
| M-04 | CI 增加 golangci-lint | 未配置 | R3 | C.1 |
| M-05 | 自定义 `.gitleaks.toml` 配置 | 使用默认规则集 | R3 | D.8 |
| M-06 | CI 增加覆盖率上报 | 未配置 | R3 | C.1 |
| M-07 | SBOM（CycloneDX 或 SPDX）生成 | 不存在 | R3 | C.1 |
| M-08 | 应用内"检查更新"功能（访问 GitHub Releases） | 不存在 | R5 | E.4 |
| M-09 | `docs/troubleshooting.md` NAS 品牌协议矩阵 | 不存在 | R5 | E.6 |
| M-10 | CODEOWNERS 增加 `/cmd/ndg-desktop/` 路径 | 未列 | R5 | D.7 |
| M-11 | 源码目录独立 `.icns` 文件 | 仅构建产物内有 | R1 | B.4 |

---

## DEFERRED

| # | 门槛 | 说明 | 延后到 |
|---|---|---|---|
| D-01 | Windows 正式安装包 | 首发仅 macOS arm64 | 1.0+ |
| D-02 | Intel Mac (x86_64) 支持 | 首发仅 Apple Silicon | 1.0+ |
| D-03 | Linux 桌面 GUI | 首发不支持 | 后续 |
| D-04 | Mac App Store 上架 | 首发不走 App Store | 后续 |
| D-05 | 后台自动升级 | 首个 Beta 暂缓，提供手动升级 | 1.0+ |
| D-06 | 应用内安全检查更新（自动） | 首个 Beta 暂缓 | 1.0+ |
| D-07 | Universal Binary (arm64 + x86_64) | 首发仅 arm64 | 1.0+ |
| D-08 | CodeQL / SAST 集成 | 已有 Gitleaks + Govulncheck + public-boundary | 1.0+ |
| D-09 | SARIF 上传到 GitHub Security tab | 可在 Beta 后续增加 | 1.0+ |

---

## 统计

| 状态 | 数量 |
|---|---|
| BLOCKER | 12 |
| REQUIRED | 40 |
| RECOMMENDED | 11 |
| DEFERRED | 9 |
| **合计** | **72** |

---

## 人工操作项（无法通过代码完成）

以下操作需要用户在 GitHub UI 或 Apple Developer Portal 完成：

1. **GitHub Support 清理确认**：核实旧 PR refs 已清理，旧提交与私有路径不可访问（B-05）
2. **GitHub 仓库安全设置**：启用 Secret Scanning、Push Protection、Private Vulnerability Reporting（B-06, B-07）
3. **GitHub 分支保护**：main 分支 require PR、required checks（Verify/Gitleaks/Govulncheck）、block force push（B-08）
4. **GitHub Release Environment**：创建 `release-macos` 环境，配置 required reviewer（R-09）
5. **Apple Developer 证书**：获取 Developer ID Application 证书、Team ID（B-02）
6. **Apple 公证凭据**：配置 App Store Connect API Key 或 Apple ID 应用专用密码（B-03）
7. **品牌图标源文件**：提供正式品牌图标（R-24）
8. **产品身份确认**：确认 Bundle ID、公司名、版权主体、支持邮箱（B-11, R-23）
