# NDG v0.5 发布准备现状审计

> 审计日期：2026-07-30
> 基线：`origin/main` = `1bc0959a846336f9e333fe078679199387cc60f0`
> 分支：`release/v0.5-readiness-audit`
> 目标版本：`v0.5.0-beta.1`
> 审计性质：只读审计，不修改业务逻辑

---

## A. 产品版本

### A.1 各来源版本值

| 来源 | 字段 | 当前值 | 备注 |
|---|---|---|---|
| Git Tag | `v0.4.0` | `v0.4.0` | 仓库唯一标签，指向此前桌面扫描基线 |
| `cmd/ndg-desktop/wails.json` | `info.productVersion` | `1.0.0` | Wails 模板默认值，未更新 |
| `cmd/ndg-desktop/frontend/package.json` | `version` | `1.0.0` | 与 wails.json 一致，但与 tag 冲突 |
| `cmd/ndg-desktop/frontend/package-lock.json` | `version` | `1.0.0` | 与 package.json 一致 |
| `internal/version/version.go` | `Version` | `"dev"` | 默认占位值，构建时由 ldflags 覆盖 |
| `internal/version/version.go` | `Commit` | `"unknown"` | 默认占位值 |
| `internal/version/version.go` | `BuildTime` | `"unknown"` | 默认占位值 |
| `Info.plist`（构建产物） | `CFBundleVersion` | `1.0.0` | 从 wails.json productVersion 渲染 |
| `Info.plist`（构建产物） | `CFBundleShortVersionString` | `1.0.0` | 从 wails.json productVersion 渲染 |
| CLI `version` 命令 | 输出 | `nas-governance dev` | 开发构建未注入 ldflags |
| Makefile `VERSION` | 变量 | `git describe --tags --always --dirty` | CLI 构建解析为 `v0.4.0` |
| Makefile `desktop-build` | ldflags | **未注入** | 桌面端 Go 侧 Version 保持 `"dev"` |
| 设置页（SettingsPage.tsx） | 显示 | `version.version` / `version.commit` / `version.build_time` | 数据来自 GetVersion() 绑定 |
| 仓库根 VERSION 文件 | — | **不存在** | 执行手册 R1 要求新增 |

### A.2 版本不一致汇总

| # | 不一致项 | 涉及来源 | 严重度 |
|---|---|---|---|
| A-1 | Git tag `v0.4.0` vs 桌面产品版本 `1.0.0` | tag vs wails.json/package.json/Info.plist | BLOCKER |
| A-2 | 桌面 macOS Bundle `1.0.0` vs 桌面 Go 运行时 `dev` | Info.plist(1.0.0) vs version.go(dev) + Makefile desktop-build 不注入 ldflags | BLOCKER |
| A-3 | CLI 发布版本 `v0.4.0` vs 桌面产品版本 `1.0.0` | Makefile CLI 注入 v0.4.0 vs 桌面 productVersion 1.0.0 | BLOCKER |
| A-4 | 无 VERSION 单一来源文件 | 仓库根目录 | REQUIRED |
| A-5 | 无 `sync-version.sh` / `check-version-consistency.sh` | scripts/ 目录 | REQUIRED |
| A-6 | `wails.json` `frontend:install` 为 `npm install` 而非 `npm ci` | wails.json | REQUIRED |

### A.3 版本一致的部分

- `wails.json` productVersion、`package.json` version、`package-lock.json` version 三者均为 `1.0.0`，桌面前端侧内部一致。
- `Info.plist` 与 `Info.dev.plist` 除 dev 专用的 NSAppTransportSecurity 外，所有版本/标识键完全一致。
- `version.go` 的 `Get()` 函数与 CLI `runVersion`、桌面 `GetVersion` 绑定、前端 `SettingsPage` 展示字段一一对应，数据链路完整。

---

## B. macOS 身份与打包

### B.1 Bundle 身份

| 字段 | 源模板值 | 构建产物实际值 | 目标值 |
|---|---|---|---|
| CFBundleIdentifier | `com.wails.{{safeBundleID .Name}}` | `com.wails.ndg-desktop` | `com.fnb.ndg`（待用户确认） |
| CFBundleExecutable | `{{.OutputFilename}}` | `ndg-desktop` | `NDG` |
| CFBundleShortVersionString | `{{.Info.ProductVersion}}` | `1.0.0` | `0.5.0`（三段数字，Apple 要求纯数字与句点） |
| CFBundleVersion | `{{.Info.ProductVersion}}` | `1.0.0` | `1`（递增数字构建号，Apple 要求纯数字） |
| CFBundleName | `{{.Info.ProductName}}` | `NDG 数据治理工作台` | 一致 |
| NSHumanReadableCopyright | — | `Copyright © 2026 NDG` | 待用户确认主体 |
| LSMinimumSystemVersion | `10.13.0` | `10.13.0` | `13.0`（执行手册建议） |
| LSApplicationCategoryType | **缺失** | **缺失** | `public.app-category.utilities` |
| CFBundleDevelopmentRegion | `zh_CN` | `zh_CN` | 一致 |
| CFBundleLocalizations | `zh_CN`, `en` | `zh_CN`, `en` | 一致 |

> **Apple Bundle 版本映射规则**：`CFBundleShortVersionString` 和 `CFBundleVersion` 只能包含数字和句点，不得使用 SemVer 预发布后缀（如 `-beta.1`）。语义版本到 Bundle 版本的映射如下：
>
> | 语义版本（Git Tag / 应用内显示） | CFBundleShortVersionString | CFBundleVersion |
> |---|---|---|
> | `0.5.0-beta.1` | `0.5.0` | `1` |
> | `0.5.0-beta.2` | `0.5.0` | `2` |
> | `0.5.0-beta.3` | `0.5.0` | `3` |
> | `0.5.0`（正式版） | `0.5.0` | `4` |
>
> 应用内部展示版本（About 页、设置页）仍使用完整语义版本 `0.5.0-beta.1`，仅 macOS Bundle 元数据使用纯数字映射。

### B.2 签名状态

| 检查项 | 当前状态 |
|---|---|
| 签名类型 | **ad-hoc**（`flags=0x2(adhoc)`） |
| TeamIdentifier | **not set** |
| Developer ID 签名 | **无** |
| Hardened Runtime | **未启用** |
| Secure Timestamp | **未启用** |
| 公证状态 | **未公证** |
| Staple 状态 | **未 staple** |

当前 `.app` 仅为 Wails 构建时自动生成的 ad-hoc 签名，Gatekeeper 将拒绝运行。

### B.3 entitlements.plist

**不存在**。`cmd/ndg-desktop/build/darwin/entitlements.plist` 全仓库搜索无结果。

### B.4 图标

| 文件 | 状态 |
|---|---|
| `cmd/ndg-desktop/build/appicon.png` | 存在（Wails 默认图标） |
| `cmd/ndg-desktop/build/darwin/iconfile.icns` | **不存在**（仅构建产物内有 Wails 自动生成的 .icns） |
| 品牌图标 | **未确认**（执行手册要求不使用 Wails 默认图标） |

### B.5 DMG 生成能力

**无**。全仓库无 `hdiutil`、`create-dmg` 调用或任何 DMG 创建脚本。

### B.6 发布脚本

| 预期脚本 | 状态 |
|---|---|
| `scripts/release/build-macos-app.sh` | 不存在 |
| `scripts/release/sign-macos-app.sh` | 不存在 |
| `scripts/release/notarize-macos-app.sh` | 不存在 |
| `scripts/release/create-dmg.sh` | 不存在 |
| `scripts/release/verify-macos-release.sh` | 不存在 |
| `scripts/release/create-release-manifest.sh` | 不存在 |

`scripts/` 目录仅有 `check-public-boundary.sh` 和 `collect-third-party-licenses.sh`。

### B.7 Makefile 桌面构建目标

```makefile
desktop-build:
	cd $(DESKTOP_DIR) && wails build
```

- 仅执行裸 `wails build`，无签名、无公证、无 DMG。
- 无 `-trimpath`、无 `-ldflags` 版本注入。
- Wails CLI 安装使用 `@latest`，**未固定版本**。

### B.8 dist/desktop/ 目录

**不存在**。`dist/` 目录仅有 CLI 产物（tar.gz + SHA256SUMS + 第三方许可证）。

---

## C. 发布自动化

### C.1 CI 工作流（`.github/workflows/ci.yml`）

| 检查项 | 状态 |
|---|---|
| 触发条件 | PR 到 main、push 到 main、workflow_dispatch |
| Runner | `ubuntu-latest` |
| Go 版本 | `go-version-file: go.mod` |
| Node 版本 | `22`（硬编码） |
| npm ci | 是（在 `cmd/ndg-desktop/frontend` 目录） |
| 前端检查 | `make frontend-check`（frontend-test + frontend-build） |
| gofmt | 是 |
| go mod tidy | 是（diff check） |
| make public-check | 是 |
| go vet | 是 |
| go test -race | 是 |
| make build | 是（**仅 CLI**） |
| 桌面端构建 | **否** |
| macOS runner | **否** |
| Wails CLI 安装 | **否** |
| Wails 版本固定 | **否** |
| SHA256 生成 | **否** |
| lint (golangci-lint) | **否** |
| 覆盖率上报 | **否** |

### C.2 安全工作流（`.github/workflows/security.yml`）

| 扫描工具 | 版本 | 固定方式 | 扫描范围 |
|---|---|---|---|
| Gitleaks | `8.28.0` | 版本号 + SHA-256 校验 | 完整 Git 历史（`--all`），启用 `--redact` |
| Govulncheck | `v1.1.4` | `go run ...@v1.1.4` | 可达漏洞（`./...`） |

Actions 全部 SHA 固定（checkout@v5、setup-go@v6.5.0），权限最小化（`contents: read`）。

**缺口**：无 npm audit、无 CodeQL/SAST、无 SARIF 上传到 GitHub Security tab。

### C.3 桌面 Release Workflow

**不存在**。`.github/workflows/` 仅有 `ci.yml` 和 `security.yml`。

- 无 `release-desktop.yml` 或类似发布工作流。
- 无 `on: push: tags:` 触发器。
- 无 GitHub Release 自动创建。
- 无 `permissions: contents: write` 配置。

### C.4 Wails CLI 版本固定

| 位置 | 当前值 | 问题 |
|---|---|---|
| Makefile 注释 | `go install ...@latest` | 使用 `@latest`，未固定 |
| CI 工作流 | 未安装 Wails | 桌面端在 CI 中不可构建 |
| wails.json | 无版本字段 | — |

### C.5 Dependabot 配置

| 生态系统 | 目录 | 频率 | 状态 |
|---|---|---|---|
| gomod | `/` | weekly | 已配置 |
| github-actions | `/` | weekly | 已配置 |
| **npm** | **`cmd/ndg-desktop/frontend`** | — | **未配置** |

---

## D. 安全门槛

### D.1 Gitleaks 全历史扫描

**已配置**。security.yml 使用 Gitleaks 8.28.0，SHA-256 校验下载，扫描完整 Git 历史（`--all`），启用 `--redact`。

### D.2 Govulncheck

**已配置**。使用 `v1.1.4`，扫描 `./...` 可达漏洞。

### D.3 公开边界检查（`check-public-boundary.sh`）

**已配置**。作为 Gitleaks 之外的第二层扫描：
- 校验 go.mod 模块路径
- 扫描禁用产物（.db、.jsonl、.log、.pem、.key、bin/、dist/ 等）
- 正则扫描私钥、GitHub token、AWS 密钥、Slack token、本地用户路径

### D.4 open-source-boundary.md 门禁状态

| 检查项 | 状态 | 说明 |
|---|---|---|
| 请求 GitHub Support 清理旧 PR refs | `[x]` 已完成 | — |
| 获取 GitHub Support 确认并独立验证旧路径不可访问 | `[ ]` **未完成** | **BLOCKER** |
| 仓库公开 | 执行手册称"已公开"，但 boundary 文档第 35 行仍声明"须保持私有" | **矛盾** |
| 启用分支保护、secret scanning、Dependabot alerts、私有漏洞上报 | `[ ]` **未完成** | **BLOCKER** |
| 创建全新发布 | `[ ]` **未完成** | — |

### D.5 npm audit

**未配置**。`package.json` 无 audit 脚本，CI 无 npm audit 步骤。

### D.6 第三方许可证

| 覆盖范围 | 状态 |
|---|---|
| CLI Go 运行时依赖 | 已覆盖（`THIRD_PARTY_NOTICES.md` + `dist/third-party-licenses/`） |
| 前端 npm 依赖 | **未覆盖** |

### D.7 GitHub 仓库安全设置

以下设置无法从仓库文件审计，需在 GitHub UI 确认：

| 设置项 | 状态 |
|---|---|
| Branch Protection（main） | **待确认** |
| Required Status Checks | **待确认** |
| Secret Scanning | **待确认**（boundary 文档第 60 行未勾选） |
| Push Protection | **待确认** |
| Private Vulnerability Reporting | **待确认**（SECURITY.md 用词"when available"暗示未确认） |
| CODEOWNERS | 已配置（`@FNB2026`，覆盖高风险路径） |
| CODE_OF_CONDUCT.md | **不存在** |

### D.8 自定义 Gitleaks 配置

**不存在**。无 `.gitleaks.toml`，使用默认规则集。

---

## E. 产品可用性

### E.1 首次启动体验

| 检查项 | 状态 | 说明 |
|---|---|---|
| 新建项目入口 | 已实现 | ProjectStartCard 三入口（新建/最近/高级） |
| 目录选择器 | 已实现 | 系统原生选择器，中文环境 |
| 自动建库 | 已实现 | OS app-support 目录，0700/0600 权限 |
| 源目录只读声明 | 已实现 | UI 明确告知"扫描源保持只读" |
| Beta 安全声明 | **缺失** | 无任何"beta""声明""免责"提示 |
| 首次使用测试副本建议 | **缺失** | 无 |

### E.2 扫描功能

| 检查项 | 状态 |
|---|---|
| 启动扫描 | 已实现 |
| 取消扫描 | 已实现（红色醒目按钮） |
| 断点续扫 | 已实现（checkpoint 检测 + 继续扫描按钮） |
| 重试扫描 | 已实现 |
| 扫描进度 | 已实现 |
| 作业历史 | 已实现（筛选 + 无限滚动） |

### E.3 治理与执行

| 检查项 | 状态 |
|---|---|
| 重复结果查看 | 已实现 |
| 目录语境与证据 | 已实现 |
| 治理草案生成 | 已实现 |
| 人工决策与批准 | 已实现 |
| dry-run | 已实现 |
| 隔离执行 | 已实现（多重安全门禁） |
| 恢复 | 已实现 |
| Purge | 已实现（默认禁用，多重门控：草案→批准→试运行→逐字确认） |

### E.4 About / 设置页

| 检查项 | 状态 |
|---|---|
| 产品版本 | 已显示 |
| Commit | 已显示 |
| 构建时间 | 已显示 |
| 发布通道 | **未显示** |
| 许可证入口 | **未显示** |
| 项目主页 | **未显示** |
| 隐私说明 | 部分显示（外部AI/遥测/云上传均为"已关闭"） |
| 支持说明 | **未显示** |
| 路径脱敏模式 | 已实现（开关 + 实时预览） |

### E.5 错误信息

| 检查项 | 状态 |
|---|---|
| 页面级 try/catch | 已实现 |
| Toast 通知 | 已实现 |
| ErrorBoundary | 已实现（本地记录，不上报） |
| 无障碍（role="alert"） | 已实现 |
| 业务前置校验 | 已实现 |
| 错误分类体系 | **未实现**（执行手册 R4 要求分类：输入/权限/离线/数据库/中断/恢复锁/安全门控/未知） |

### E.6 用户文档

| 文档 | 状态 |
|---|---|
| `docs/install-macos.md` | **不存在** |
| `docs/quick-start-desktop.md` | **不存在** |
| `docs/upgrade-and-backup.md` | **不存在** |
| `docs/uninstall.md` | **不存在** |
| `docs/privacy.md` | **不存在** |
| `docs/beta-safety.md` | **不存在** |
| `docs/troubleshooting.md` | **不存在** |
| `docs/support-bundle.md` | **不存在** |
| `KNOWN_ISSUES.md` | **不存在** |
| `README.md` | CLI 优先，无桌面端快速开始 |
| `CHANGELOG.md` | 仅有 `Unreleased` 段，无版本化条目 |
| `CODE_OF_CONDUCT.md` | **不存在** |

### E.7 Issue 模板

| 模板 | 状态 | 质量 |
|---|---|---|
| `bug_report.yml` | 存在 | 字段完整，含隐私确认复选框 |
| `feature_request.yml` | 存在 | 含安全边界影响必填字段 |
| `config.yml` | 存在 | — |

### E.8 卸载后数据残留说明

**缺失**。无文档说明卸载 App 后项目数据库的位置和清理方法。

### E.9 支持包导出

**缺失**。无脱敏支持包或完整本地诊断包的导出功能。

### E.10 离线数据源处理

| 检查项 | 状态 |
|---|---|
| 数据源离线显示 | 已实现（存储列表显示状态） |
| 离线时不删除记录 | 已实现 |
| 重新挂载后恢复 | 已实现 |

---

## 审计结论

### 已具备的能力

当前桌面端产品主链已完整实现：新建/打开项目 → 注册数据源 → 启动扫描 → 取消/断点续扫 → 查看重复结果 → 查看目录语境 → 生成治理草案 → 人工决策与批准 → dry-run → 隔离执行 → 审计/恢复/保留期/永久清理。Purge 多重门禁设计严谨，默认禁用。

工程验证已覆盖：`go vet`、`go test -race`、`gofmt`、`go build`、`tsc --noEmit`、`vitest run`、`vite build`、`wails build`、GitHub CI（Verify）、Gitleaks、Govulncheck。

### 发布前必须解决的关键缺口

1. **版本体系分裂**：Git tag `v0.4.0`、桌面产品版本 `1.0.0`、桌面 Go 运行时 `dev` 三者互不一致，无 VERSION 单一来源。
2. **macOS 签名与公证完全缺失**：当前仅 ad-hoc 签名，无 Developer ID 签名、无 Hardened Runtime、无公证、无 DMG。
3. **发布自动化空白**：无桌面 Release Workflow、无 tag 触发构建、无 GitHub Releases 自动创建。
4. **公开历史门禁未关闭**：`open-source-boundary.md` 中 GitHub Support 清理确认门禁未勾选，与执行手册"仓库已公开"矛盾。
5. **用户文档全面缺失**：无安装、隐私、快速上手、卸载、已知问题文档，README 仍为 CLI 优先。
6. **Beta 安全声明缺失**：UI 无任何预发布风险告知。
7. **Wails CLI 版本未固定**：使用 `@latest`，构建可复现性无法保证。
8. **前端依赖安全盲区**：Dependabot 未监控 npm 生态，CI 无 npm audit。

### 下一步

按照执行手册 R1-R7 逐阶段推进。R0 审计完成，不修改业务逻辑，提交 Draft PR 等待审查。
