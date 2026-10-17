# fnOS 只读 Compose 开发准备与生命周期实测

日期：2026-10-16。状态：受限人工数据开发候选；fnOS 整个平台仍为实验性。
该文件与新模板不属于已公开 Preview.1 的 15 个冻结资产，不改变 beta.11、Tag 或镜像。

## 当前可以证明什么

同一台 fnOS 1.2.0800、Linux 6.18.18、Docker 28.5.2、Compose 5.0.2 amd64 上，
已安装的核心本来由 Compose 管理，固定 internal bridge / IPv4、无宿主端口、
UID/GID 10001、只读根与来源、cap-drop ALL、no-new-privileges、1GiB / 128 tasks，
restart unless-stopped。独立 HTTPS 网关真实 TLS/Chrome/认证/CSRF/Cookie 验收见
[入口记录](FNOS-CGI-ENTRY-2026-10-15.md)。临时客户端代理例外已恢复；原代理路径仍不兼容，浏览器需要受支持的局域网直连。

本轮在已授权的十个人工源文件上实测：

1. 停止并确认自己的固定完整容器 ID，冷复制整个 state，包含 DB、WAL/SHM（若仍存在）、锁与 source-binding。新目录逐项 SHA-256、大小、模式、UID/GID 与原 state 相同；两个 SQLite quick_check 为 ok，WAL 已完全 checkpoint。
2. 删除该已停止的容器（无 `-v`），所有 bind 状态文件仍相同；用原固定 Compose、空 env、显式 project/working-dir 和 `--no-build --pull never --no-deps` 重新创建。来源 A 保留 7 文件/3组，B 保留3文件/1组，均 COMPLETED / completed checkpoint。
3. 再停删自己的核心，仅把 `/data` 指向另一个全新空目录中恢复的 state，其余 Compose 内容不变。启动后认证并读取两个快照，结果一致。
4. 停删恢复测试容器，回到原 Compose / 原 state，重新认证及快照一致。备份和恢复目录保留；来源与状态均未删除。

整个过程网关 PID 不变，原15个容器/8个网络基线和十个来源 SHA-256 不变。
最终新核心完整 ID `b2a3acc8cfafec12563b448dce0d36b44c98fb4b93bbfb52b9d17016d5f4d84d`。
重建后再次执行真实证书、登录/退出、会话、Host/Origin、CSRF、危险三路由拒绝、
明文及错误 SNI 的完整 wire 矩阵通过；它是 HTTP/TLS 证据，不冒充一次新的 GUI 验收。

原 r1 测试停止在重建前：通用 `docker inspect NAME` 匹配了同名网络，误判容器仍存在。
保护性停止后核验所有源/冷状态/配置及空网络，再用原 Compose 恢复。r1 保持 FAIL；
r2 统一使用 `docker container inspect` 并重新完整实测后才 PASS。没有删除该网络来“修复”。

证据身份：

- 核心源码 `1db1e4ab9a1287d3f7f9b801b471b00bfc7c7d1e`；OCI index `sha256:49dada73f1eb9cc74485ed5d0b92023d7cb4c3df462d03e105f00158b91668e1`；实际 AMD64 config `sha256:2c38ca9493ccebc74cdf0cc84fb23e8a4c59babb1660d4892a2f9a50c12a96d4`。
- 原现场 Compose SHA-256 `7c3738dfa974ed5fddc43542495ce2f64b89d757cb14f7f808bad368633dc7d6`。
- 独立复审并实测的 r2 执行器 SHA-256 `f95b1661c95ad5c4d65a19bf1ff97667f067bd1a29b3e6267ad0b735614317d9`。
- 私有24文件证据归档 SHA-256 `c9ddc44bcfadcc033f7c3123e4abb29492e1373c8d4410359878fd59f2c85fd6`，其中23项独立成员哈希在本机复核。源文件名、状态和凭据不公开。

这不认证断电、NAS整机重启、跨版本升级、FPK完整卸载、生产数据、手机、治理写入或长期性能。

## 新准备工具的使用边界

[初始化工具](../../scripts/packaging/init-fnos-readonly.py) 仅供 Linux amd64 可信管理员，
只创建全新随机命名的私有根，不启动 Docker、不安装服务、不改变网络或来源。
首次口令为32字节随机值，保存在新根 `config/credential`，0600/UID10001；不要放入
命令行、Compose 环境、普通日志或 Git。根0700/root，config/state0700/UID10001。
失败保留部分新根供人工检查，下次不会接管它或删除现场。

管理员先在受信任私有目录创建0600/root、无ACL、无符号链接的请求 JSON。
下面全为示例；network_id 必须替换为已核验的**全64位 Docker 网络 ID**。
请求文件是管理员的显式来源授权，不能由浏览器或原始向导文本直接生成并执行。

```json
{
  "boundary_root": "/var/lib/ndg-private",
  "parent": "/var/lib/ndg-private/installations",
  "origin": "https://ndg.example.test:24483",
  "network_id": "REPLACE_WITH_VERIFIED_64_HEX_ID",
  "core_ipv4": "10.253.188.2",
  "source_a": "/var/lib/ndg-private/source_a",
  "source_b": "/var/lib/ndg-private/source_b"
}
```

所有已存在的 parent/source 都须位于同一明确配置的 `boundary_root` 之下，
工具以此根 FD 的 `openat2(BENEATH|NO_SYMLINKS|NO_XDEV)` 核验，拒绝嵌套挂载、
同设备 bind 别名与源包含关系，然后才开始创建。boundary_root 自身可以是管理员
选定的本地挂载；此合同不支持该根内跨挂载来源。parent 与其全部祖先必须由 root
持有、无其他用户写权限或ACL。可信管理员不并发重挂载/替换这些根是执行前提；
这不是敌对 root 竞争或一般源文件删除证明，不能用 `st_dev` 相等替代。

```sh
# 先审查源码和请求；使用管理员私有终端，不记录来源或口令。
python3 scripts/packaging/init-fnos-readonly.py --create-new-installation /ROOT_PRIVATE_REQUEST.json
```

成功仅输出 `PREPARED_NOT_STARTED`、安装 ID 和新根位置。新根包含 compose.yaml、
installation.env、prepared.json、config/ 与空 state/。

## 分开的执行与验收步骤

准备回执的 network_verified / image_verified / gateway_installed 均为 false。
管理员在后续独立步骤必须核对：

- 网络为本安装独占的 bridge、Internal=true、完整 ID 匹配、无其他端点，固定 IPv4 在专属子网内且不冲突。不得使用 daemon、SSH 或公网配置变更修复。
- 已有本地镜像内容与上面的固定 index/架构/config/layers 相符；新模板 `pull_policy: never`，不自动拉取/构建或采纳可变标签。
- 通过 `docker compose --env-file NEW_ROOT/installation.env --file NEW_ROOT/compose.yaml config --format json` 审查最终配置，不能只看模板。私有源路径不得进入公开日志。
- 独立原生 HTTPS 网关的证书、非 root 身份、sandbox、固定上游/Origin、局域网绑定已经过审查和授权。此准备工具不会替用户安装或启用它；没有入口时仍不能浏览。
- 经限定授权后才启动本项目 core。实际 inspect 必须再次核验固定 image、UID、四个 bind 的读写属性、无端口与唯一 internal 网络，再验认证/GUI/源守恒；Compose 解析成功不等于运行通过。

新[开发模板](../../packaging/fnos/compose.lan-readonly.yaml) 使用 `bind.create_host_path: false`，
缺少目录时拒绝启动，避免隐式创建空来源或状态。含义见
[Docker Compose services](https://docs.docker.com/reference/compose-file/services/#long-syntax-5)；
[external 网络](https://docs.docker.com/reference/compose-file/networks/#external) 必须预先存在。

## 冷备份、恢复和卸载

先停止并核对本项目的精确容器 ID 与所有 state 写者；复制完整 state 到新的私有目录，
保留模式、UID/GID、WAL/SHM、绑定元数据和锁。源根/别名/身份保持不变。不要在线只拷
主 DB，不清理 Journal/Recovery Lock，不用同一 state 同时启动两个核心。
本次实际恢复仅在同版本、同源身份、全新空 state 目录成立；不认证跨版本或备份重新绑定。

卸载分为：停止自己的入口和核心、按身份删除自己的容器、核验来源/state/备份仍在。
不带 `-v`，不使用 prune 或默认删除数据，不能按名称前缀批量删除；专属网络仅在无
任何端点且独立确认归属后才考虑删除。本轮未删除网络或卸载入口，不据此认证整套平台卸载。

## 新准备工具的独立实测结果

源码 `7a841ac84b147af64e9a2802ba1a8621e3ead0e7` 的
[原生 CI38906780118](https://github.com/FNB2026/nas-data-governance/actions/runs/38906780118)
在独立 mount namespace 实际建立两种同设备、同 inode bind 别名，初始化均在首次
写入前拒绝。连同12种输入/权限拒绝、新私有根与口令权限、真实 Compose 解析/true
对照和来源 inode/SHA 守恒共18项通过。源码文件及下载的全部 artifact 摘要已核对。
准备证明限 amd64；ARM64 网关回归通过，但准备证明按合同跳过，不能称 ARM fnOS 安装通过。

首轮 `e7b036a` 的 CI38906158531 在解析断言处 KeyError：compose-go 的 bool
`omitempty` 会省略 false。失败保留；修正要求原四处显式 false、canonical 不能为
true，并把独立 control 改成 true 经同一真实解析器证明确有 true；未改安全配置或
弱化为忽略未知字段。此 JSON 格式差异不是运行中缺少路径拒绝的实测，后者仍待独立测试。

原生门禁通过后，在实际 fnOS 的既有授权人工隔离区仅创建一个新私有准备根，
实测错误 network 名、state 位于 source 内、HTTP Origin 三种请求在写前拒绝；
正常请求生成 root0700、config/state0700/10001、口令0600/10001且未输出。
同机 Compose 5.0.2 离线解析固定 image、四 bind、只读来源、无端口、完整 external
network ID 和新预留 IP。原核心、网关 PID、十个来源及其他容器/网络不变。
当前核心与该网关 PID 的应用日志仍0字节，不补认证已删除容器历史。

现场回执 SHA-256 `46275e7bb18014e5657e79f09681aa8433c6e63319c8428d5c1572d2c4a013b0`；
helper SHA-256 `cd1cf60004067579b1b17daf3cc50a55df6de89cd14dfdbd79155fa3a30cee0e`，
模板 SHA-256 `42d9b7707340f66d63dcb963f6e0ce270f6a0698fa8eb3eacdc6bfd897807366`。
真实结论为 **PREPARED_NOT_STARTED**，network_verified/image_verified/gateway_installed
仍为 false；没有借用旧核心的安装 PASS，没有启动新容器/网络/服务。新的随机口令仅
保留在 NAS 私有准备目录，不在证据导出、仓库或日志中；特权验收会话已显式退出。

## 仍待验收

下一步是可审查的首次引导和授权服务安装链路，之后再认证缺少绑定路径的实际启动
拒绝、原位升级、完整卸载、手机与整机重启。Quarantine、Restore、Purge 继续关闭，
完整 fnOS Governance 未交付。

## 2026-10-17 生命周期核验工具（R4 Task D，离线层）

[fnos-install-lifecycle.py](../../scripts/packaging/fnos-install-lifecycle.py)
把本文"分开的执行与验收步骤"与"冷备份、恢复和卸载"两节的人工核对矩阵产品化：
证据进、类型化裁决出。它**自身不执行任何 docker/compose 命令**（测试用静态
不变量证明源码无 subprocess/os.system/popen/eval/exec），只核验管理员提供的
`docker image inspect`、`docker compose config --format json`、
`docker container inspect` 与备份证据 JSON，并产出下一步的精确命令计划：

- `verify-image`：只接受 digest 引用锚定在冻结的 OCI index/amd64 config 上；
  仅标签、错误架构、未锚定 digest 一律拒绝。
- `verify-mounts`：resolved compose 配置必须与冻结身份逐项相等（四 bind、
  `create_host_path: false` ×4、只读属性、无端口、无 build、无 environment、
  10001:10001、read_only 根、`pull_policy: never`、external 完整 64 位网络 ID），
  且 prepared.json 的 network_verified / image_verified / gateway_installed
  必须仍为 false——准备回执不得自我预认证。
- `plan-backup` / `verify-backup`：容器必须以精确 ID 停止；冷拷贝计划含逐文件
  SHA-256/mode/uid/gid 证据与 quick_check；证据缺哈希、DB 损坏、无 DB 备份拒绝。
- `plan-restore`：拒绝恢复进原 state 目录（必须全新空目录）；重建命令固定
  `--no-build --pull never --no-deps`。
- `plan-upgrade`：备份验证必须排在镜像重建之前；同镜像"升级"拒绝；回滚=
  恢复已验证的备份回执。
- `plan-uninstall`：仅按容器身份删除；`-v`/`--force`/`prune`/按名称前缀批量
  删除全部列为 forbidden；要求事后核验来源/state/备份仍在。

离线证明
[test-fnos-lifecycle.py](../../scripts/packaging/test-fnos-lifecycle.py)
24 项（`make fnos-lifecycle-test`，已接入 CI）：接受矩阵 + 拒绝矩阵 +
"核验器无执行路径"静态不变量。CI 步骤名为
`Check fnOS install-chain lifecycle verifier matrix (offline)`。

边界不变：这是核验与计划层，不是运行时验收。缺少绑定路径的实际启动拒绝、
原位升级、完整卸载、手机与整机重启仍需在真实 fnOS 上按本文件既有流程实测；
本工具不改变任何既有工具的合同，也不覆盖 Preview.1 的 15 个冻结资产。

## 2026-10-17 可执行安装链（R5；独立 Linux 实测，非 fnOS 验收）

同一支 [fnos-install-lifecycle.py](../../scripts/packaging/fnos-install-lifecycle.py)
新增 `plan` / `authorize` / `run` / `status` 四个子命令，把"证据核验"升级为
"可实际操作"。原有 `verify-*` / `plan-*` 的输出与拒绝词保持不变。

### 三道前置件，缺一不可

1. **冻结计划**：`plan` 读取结构化请求 JSON，校验 origin、网络全 ID、IPv4、
   边界根、源目录、备份目录与 14 步参数，输出规范化 SHA-256（`plan_digest`）
   到私有 receipt（0700，plan.json 0600）。计划不写安装根。
2. **逐步授权**：`authorize --steps a,b,c --confirm <digest 前 12 位>` 生成
   0600 的 `authorization.json`，只包含与**该摘要**绑定的步骤；已存在即拒绝覆盖
   （换计划必须换 receipt 目录）。
3. **显式执行**：`run --execute`。缺任一件即拒绝；计划被改动一个字节，
   `plan_digest_mismatch`。

### 14 步与真实动作

`compat`（Linux/amd64/root/内核/Compose）→ `sources`（目录身份 + 源基线）→
`image`（冻结 index/架构/config）→ `init`（新根、config/state 0700/10001、
口令 0600、installation.env、compose、prepared 全 false）→ `gateway`（仅生产：
单元/证书复核 + systemctl）→ `start`（解析后配置逐项核验、独占 internal bridge、
固定 IP、启动、健康、容器身份复核）→ `browser`（200/CSP/nosniff/app shell）→
`access`（登录、CSRF、扫描、checkpoint=completed、源字节不变、退出）→
`gui`（服务端能力声明必须诚实；出现 `data-ndg-enabled="true"` 的危险控件即拒绝）→
`backup`（按 ID 停止、冷拷贝、逐文件 SHA-256、WAL 一并 quick_check）→
`upgrade`（候选镜像必须不同且本地存在；改 compose 前必须先有已验证备份）→
`rollback`（旧 state 改名保留、从备份恢复到新 state、逐文件比对、回滚镜像）→
`uninstall`（按 ID 停止/删除、网络仅在无端点且 `network_owned` 时删除、
来源/state/备份字节复核）→ `reboot`（restart policy + 进程重启后快照一致）。

### 安全不变量（由测试而非注释保证）

- **无 shell**：所有外部动作是 argv 列表；AST 断言每个 `subprocess.*` 调用都有
  字面 `shell=False`，且首参不是字符串/拼接/f-string；禁止
  `os.system/os.popen/os.exec*/os.spawn*/pty.*/eval/exec`。
- **目录身份**：`O_NOFOLLOW` 打开、`chmod` 后复读断言（umask 无法静默过滤模式）、
  Linux `openat2(BENEATH|NO_SYMLINKS|NO_XDEV)`、设备/inode 快照在
  `init/start/backup/rollback` 前复核；路径含 `\n`/NUL、`..`、任何符号链接
  组件一律拒绝。
- **Journal**：append-only、每条 fsync；`start` 无终态即崩溃足迹，
  仅声明可对账且后置复核确实通过的步骤允许 `--reconcile`，否则
  `manual_reconciliation_required`，不猜成功、不删不明文件。
- **范围隔离**：`DISPOSABLE_LINUX_ACCEPTANCE` 在 plan/authorize/run 三处都要
  `--acceptance`，只允许 core 明文入口且安装根必须在 `ndg-chain-` 临时父目录；
  `PRODUCTION` 必须 HTTPS DNS origin + 网关参数 + 冻结镜像；两者不可混用。
  `uninstall` 永不删除来源、state 或备份。

### 实测证据

- 本地 `make fnos-lifecycle-test`：54 项。原 24 项证据矩阵之外，新增计划/授权/
  Journal 拒绝矩阵、真实文件复制（空格与中文文件名、模式/UID 保持）、
  umask 0077 下的模式断言、真实 HTTP 请求与 Origin、argv 含空格不被拆分、
  篡改计划与未授权步骤拒绝。
- 独立 Linux `make fnos-install-chain-test`（GitHub-hosted ubuntu-latest amd64，
  root，真实 Docker）：匿名拉取冻结 index 镜像 → 创建本安装独占 internal
  bridge → 真实执行 compat/sources/image/init/start/browser/access/gui →
  `docker restart` 后快照一致 → 冷备份 → 按身份卸载 → 来源字节、state、
  备份逐文件保留，容器与自有网络按身份删除。CI 步骤名为
  `Real install/uninstall chain on the pinned image (amd64, not NAS acceptance)`。

### 仍未主张

这不是 fnOS 验收：未安装 HTTPS 网关（`gateway` 在 acceptance 范围类型化拒绝）、
未做整机重启（`reboot` 仅进程重启）、未做原位升级（无第二个候选镜像时
`upgrade` 拒绝）、未在真实 NAS 上写入或删除任何数据。Q/R/P 仍关闭，
`PurgeReleaseAllowed=false` 不变，公开 API 仍只读。真实 fnOS 人工源验收仍需
所有者授权；逐项授权清单、通过判据与中止条件见
[人工源验收申请](FNOS-MANUAL-ACCEPTANCE-APPLICATION-2026-10-17.md)，
在该申请获批前不执行任何真实 NAS 写入。
