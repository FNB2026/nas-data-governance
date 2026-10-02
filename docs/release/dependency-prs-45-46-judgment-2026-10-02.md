# #45 / #46 独立判定 — 2026-10-02

> 对象：`#45`（go 依赖组，4 项升级）、`#46`（npm 依赖组，4 项升级）
> 前置纪律：**两者都先 rebase 到最新 main（`d503e24`）再取证**，不沿用旧 head / 旧 base 的分析与旧 CI 结论
> 结论：**两个 PR 均不进入首个 Public Beta**，延后到 Beta Stabilization / post-beta dependency maintenance；两个 PR 保持 OPEN，不需要关闭

## 0. 为什么必须"先更新事实"

本轮直接印证：**重基会改变更新集合本身**。`#45` 重基前目标是 `modernc.org/sqlite v1.59.0 / libc v1.75.7`，重基后变为 **`v1.60.0 / v1.77.1`**（另有 `cc/v4 4.29.2→4.29.7`、`ccgo/v4 4.35.0→4.36.1`、`x/tools 0.49.0→0.50.0`）。

若沿用重基前的 diff 下结论，分析的是一组**已被取代的版本**。因此：**旧 head 的绿灯与分析都作废，只认新 head SHA 绑定的检查结果。**

## 1. 重基后的落点核对（两个 PR）

| PR | 新 head | merge-base | 领先提交 | 变更文件 | 重基后 diff 指纹 |
| --- | --- | --- | --- | --- | --- |
| #45 | `e689fef38d8a6d9e8aa3525dc89cca5b2a6031a5` | `d503e24`（= 最新 main） | 1 | `go.mod`、`go.sum`（无源码改动） | `b2235416…`（与重基前不同 — 版本集合已变） |
| #46 | `5b87733ca20fbcd30c811c9e2b11509b3f11999e` | `d503e24`（= 最新 main） | 1 | `cmd/ndg-desktop/frontend/package.json`、`package-lock.json` | `e4ec8be1…`（与重基前逐字一致） |

两者的 CI 与 Security 均已在新 head 上重跑并通过（run 绑定关系见表末）：

- `#45`：CI run `36976657916`（Verify ✓ 3m25s / Desktop Build (macOS) ✓ 2m46s）、Security run `36976657899`（Gitleaks ✓ / Govulncheck ✓）
- `#46`：CI run `36976667729`（Verify ✓ 3m17s / Desktop Build ✓ 1m40s）、Security run `36976667728`（Gitleaks ✓ / Govulncheck ✓）
- 两个 PR 当前 `mergeStateStatus = CLEAN`

> **CI 绿是"可编译 / 测试集合通过"的证据，不是"应当合并"的结论。** 下面的判定不以此为依据。

## 2. #45（go 依赖组）——排除于首个 Public Beta

### 2.1 升级内容

| 类型 | 模块 | 版本 |
| --- | --- | --- |
| 直接 | `github.com/wailsapp/wails/v2` | `2.13.0` → **`2.16.0`** |
| 直接 | `modernc.org/sqlite` | `1.56.0` → **`1.60.0`** |
| 直接 | `golang.org/x/sys` | `0.47.0` → `0.48.0` |
| 直接 | `golang.org/x/text` | `0.40.0` → `0.42.0` |
| 间接 | `golang.org/x/crypto` / `x/net` | `0.52.0→0.53.0` / `0.55.0→0.56.0` |
| 间接 | `modernc.org/libc` / `memory` | `1.74.4→1.77.1` / `1.11.0→1.12.1` |

模块集合变化：`go.sum` 中 **14 个模块仅换版本，净新增 0 / 净移除 0** —— 没有引入任何新的第三方模块，也没有模块退出构建。

### 2.2 是否进入发布二进制（判据：`go list -deps ./cmd/ndg-desktop`，无 dev tag）

| 模块 | 命中包数 |
| --- | --- |
| `github.com/wailsapp/wails/v2` | **17** |
| `modernc.org/sqlite` | 2（另含底层 `modernc.org/libc` **24**） |
| `golang.org/x/sys` | 1 |
| `golang.org/x/text` | 4 |
| 发布依赖总包数 | 281 |

**结论：四个直接升级项全部进入最终桌面二进制**，并携带 SQLite 引擎与其 C 运行时垫片（libc）一起参与编译。这不是"仅开发期依赖"。

### 2.3 构建链耦合（本轮新发现）

| 位置 | 事实 |
| --- | --- |
| `Makefile` | `WAILS_VERSION := v2.13.0`；`wails-check` 在 **CLI 版本 ≠ 该 pin 时直接失败**；`desktop-build` 与 `desktop-dev` 都依赖 `wails-check` |
| `.github/workflows/ci.yml` | `go install github.com/wailsapp/wails/v2/cmd/wails@v2.13.0` |
| `.github/workflows/release.yml` | 同上，安装 `wails@v2.13.0` |
| 本机开发环境 | Wails CLI `v2.13.0` |

后果有两层：

1. 若把库升到 `2.16.0`，**必须同步修改** Makefile 的 pin、CI 与 release workflow 的安装版本，并升级每位开发者的本地 CLI —— 这是**工具链变更**，不是单纯的依赖刷新；
2. 更需注意：`wails-check` 只比对 **CLI 版本 ↔ Makefile pin**，**不比对 CLI ↔ go.mod 里的库版本**。因此 `#45` 的 CI 绿，实际是"用 CLI 2.13.0 构建库 2.16.0"这一**上游未必验证过的组合**通过了编译 —— 绿灯不能替代工具链一致性。

### 2.4 回归面

- **桌面运行时**：窗口 / WebView / IPC / 绑定生成 / 前端资源装载（3 个 minor 版本跨度）；
- **数据层**：内嵌 SQLite 引擎 `1.56.0 → 1.60.0`（含 libc、memory 连带升级）——直接影响项目数据库的读写路径；
- **系统调用层**：`x/sys`；
- **文本/文件名处理**：`x/text`（Unicode 表变化会影响文件名规范化与匹配行为）。

### 2.5 判定

**不进入首个 Public Beta。** 理由：

1. 触及**运行时 + 数据层**，其回归面必须重走 RC 级验收（Clean Install、真实 NAS 扫描、治理闭环）才能下结论；
2. 与冻版策略一致：**RC 前只合单包 PR，不合分组升级 PR**；
3. 升级后产生**三方版本一致性义务**（CLI pin / CI / 本地 CLI ↔ 库），属工具链治理范畴；
4. CI 绿只证明可编译，**不证明** SQLite 引擎行为与既有项目库（数百万行）的兼容性。

**后续**：延后到 Beta Stabilization / post-beta dependency maintenance。届时作为独立工作项，与 Wails CLI pin、CI、本地工具链**同步升级**，并重跑 Clean Install + 真实 NAS 验收。

## 3. #46（npm 依赖组）——排除于首个 Public Beta

### 3.1 升级内容（全部在 devDependencies）

| 包 | 范围 |
| --- | --- |
| `@testing-library/jest-dom` | `^7.0.0 → ^7.0.1` |
| `@testing-library/react` | `^16.3.2 → ^16.3.3` |
| `@types/node` | `^26.1.2 → ^26.6.3` |
| `jsdom` | `^30.0.1 → ^30.1.1` |

`dependencies`（`react` / `react-dom`）**未改动**。

### 3.2 lockfile 全量分类

- 条目数：`208 → 207`，移除 `node_modules/symbol-tree`（`jsdom`/`whatwg-url`/`w3c-xmlserializer` 链上的传递依赖，随 jsdom 升级被去掉）；
- 版本变化条目：18 个，**全部 `dev: true`**（无任何生产依赖条目变动）；
- 连带升级族：`@csstools/*`、`@asamuzakjp/*`、`whatwg-url`、`bidi-js`、`undici-types` 等（均为测试/类型工具链）；
- **Node 下限收紧**（环境契约变化）：
  - `@asamuzakjp/css-color`、`@asamuzakjp/dom-selector`、`w3c-xmlserializer`：要求 `^22.22.2 || ^24.15.0 || >=26.0.0`；
  - `html-encoding-sniffer`：`^22.13.0 || >=24.0.0`；
  - workflow 使用 `node-version: 22`（浮动），本机为 22.22.2，均满足；但**恰好**踩在新下限上。

### 3.3 判定

**不进入首个 Public Beta**（同样延后）。理由：

1. 纯 `devDependencies`，**不进入发布产物**，前端构建产物与 React 运行时不变 —— 无数据安全或运行时风险；
2. 但它**改变了开发/测试环境契约**（Node 下限、jsdom 测试环境代数），属于冻版期之外的常规维护变更；
3. 与 `#47` 的安全修复性质不同：`#47` 有明确告警驱动且只动一行 lockfile；`#46` 是分组工具链升级，按既有策略不并入 RC。

**后续**：延后到 Beta Stabilization；届时单独合入，并复核前端测试全绿与 Node 下限契约。

## 4. 明确不做

- **不关闭** `#45` / `#46`（保留 OPEN，日后可取最新事实重判）；
- **不因为 CI 绿而合并**，也不为"减少开放 PR 数量"而合并；
- 不为这两个 PR 改动 `VERSION`，不触碰 `beta.4` 的 tag / Draft / 资产。

## 5. 三态汇总

| 项 | 状态 |
| --- | --- |
| 两 PR 重基落点 == 最新 main、各领先 1 提交 | 已取证 |
| 两 PR 新 head 的 CI + Security 结果 | 已取证（run 与 headSha 绑定一致） |
| `#45` 四个直接升级项进入发布二进制 | 已取证（`go list -deps` 包数） |
| `#45` 模块集合净增 0 / 净移除 0 | 已取证（go.sum 版本级比对） |
| `#45` 与 wails CLI pin 的三方一致性义务 | 已取证（Makefile / ci.yml / release.yml / 本机 CLI 均为 v2.13.0） |
| `#45` 数据层（SQLite 1.60）与既有项目库的兼容性 | **未验证** —— 需在候选构建上做真实 NAS 验收 |
| `#46` 全部改动落在 devDependencies | 已取证（package.json + lockfile 分类） |
| `#46` 测试环境与 Node 下限变化 | 已取证（engines 字段对比） |
| 两者的处置结论 | 已判定：均不进首个 Public Beta，延后维护窗口 |

## 6. 主线位置

```text
Step 3  #47 CLOSED（undici，告警 open=0）
        ↓
#45 / #46 独立判定   ← 本文（两者均排除于首个 Public Beta）
        ↓
RC Freeze
        ↓
v0.5.0-beta.5 候选身份（届时才改 VERSION / 打标签）
```
