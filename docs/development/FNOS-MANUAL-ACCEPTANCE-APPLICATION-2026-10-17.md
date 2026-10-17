# fnOS 人工源验收申请（R5，待所有者授权）

日期：2026-10-17。状态：**申请，不是验收记录**。在所有者逐项批准前，不得在真实 NAS
上创建网络、容器、服务单元，或写入/删除任何人工数据。

## 一、申请做什么

在一台已授权的 fnOS 测试机（Linux 6.x、Docker 28.x、Compose 5.x、amd64）上，
用新的私有安装根完成一次**独立安装 → 浏览器只读操作 → 冷备份 → 原位升级 →
回滚 → 安全卸载 → 整机重启后恢复**，并把每一步的证据绑定到确定的提交与镜像摘要。

明确不做：

- 不接触现有生产核心、网关、容器、网络、来源与 state；
- 不删除任何真实文件；Quarantine / Restore / Purge 保持关闭；
- 不修改系统代理、防火墙、Docker 全局配置或其他应用；
- 不把验收结果用于解除 `PurgeReleaseAllowed=false`。

## 二、需要所有者逐项确认的授权

| # | 授权项 | 具体内容 | 不批准的后果 |
| --- | --- | --- | --- |
| A1 | 测试机与时间窗 | 机器标识、可占用时段、允许重启次数（建议 1 次整机重启） | 无法进行运行时验收 |
| A2 | 私有根位置 | 一个**新建**目录作为 `boundary_root`（root 0700，无 ACL/符号链接/嵌套挂载），其下 `parent`、`source_a`、`source_b` 均为人工测试目录 | `plan` 在写前拒绝 |
| A3 | 专属 internal bridge | 允许创建 1 个 `--internal` bridge，使用未占用的 /24 与固定 IPv4；网络 ID 在创建后核验 | `start` 拒绝（必须是完整 64 位 ID 且无其他端点） |
| A4 | 镜像 | 允许从 `ghcr.io/fnb2026/ndg-readonly-preview@sha256:49dada73…68e1` 拉取/使用冻结镜像（amd64） | `image` 拒绝 |
| A5 | HTTPS 网关 | 允许安装/启用**新的** `ndg-readonly-https.service`（独立用户、只读根、固定上游），证书与私钥路径、单元与二进制 SHA-256 预先登记 | `gateway` 拒绝；只能用 core 明文做开发验收，不能称浏览器验收 |
| A6 | 浏览器操作 | 允许在局域网内用受支持的 Chrome 访问新 Origin，完成登录、扫描、查看聚合结果、退出 | 无法回答"真实浏览器完成操作" |
| A7 | 冷备份目录 | 一个新的 0700 私有目录，用于整目录冷拷贝与逐文件 SHA-256 | `backup` 拒绝 |
| A8 | 原位升级候选 | 提供**第二个**已在本地的 digest 固定镜像（不同 image ID），并允许改该安装根的 `compose.yaml` | `upgrade` 类型化拒绝（不会假装升级） |
| A9 | 整机重启 | 允许在备份完成后重启一次测试机，并在重启后核对容器/快照/恢复锁 | `reboot` 只能证明进程重启，不能证明整机 |
| A10 | 卸载范围 | 仅允许按容器 ID 删除**本安装**的容器；自有网络在无端点后删除；来源/state/备份一律保留 | `uninstall` 拒绝删除网络 |

## 三、执行方式（全部为结构化参数，不执行任何生成的 Shell 字符串）

```sh
# 0) 先审查源码与请求文件；请求文件 0600/root，不得由浏览器或向导文本生成
python3 scripts/packaging/fnos-install-lifecycle.py plan /ROOT_PRIVATE/request.json \
  --receipt /ROOT_PRIVATE/receipt          # 生产范围不需要 --acceptance
# 1) 逐项授权（只授予本次要执行的步骤；确认串为计划摘要前 12 位）
python3 scripts/packaging/fnos-install-lifecycle.py authorize \
  --plan /ROOT_PRIVATE/receipt/plan.json --receipt /ROOT_PRIVATE/receipt \
  --steps compat,sources,image,init,gateway,start,browser,access,gui,backup,upgrade,rollback,uninstall,reboot \
  --confirm <PLAN_DIGEST_12>
# 2) 执行（缺 --execute 不动作）
python3 scripts/packaging/fnos-install-lifecycle.py run \
  --plan /ROOT_PRIVATE/receipt/plan.json --receipt /ROOT_PRIVATE/receipt --execute
# 3) 只读回读
python3 scripts/packaging/fnos-install-lifecycle.py status --receipt /ROOT_PRIVATE/receipt
```

工具只使用 argv 列表调用外部命令，不含 shell；测试以 AST 断言每个
`subprocess` 调用都带字面 `shell=False` 且首参不是字符串。目录身份在每次变更前
复核（O_NOFOLLOW、chmod 后复读、Linux `openat2`、设备/inode 快照）。
Journal 逐条 fsync；中断步骤只有后置复核确实通过才允许 `--reconcile`。

## 四、每一步的通过判据（证据必须绑定确定的提交与摘要）

| 步骤 | 通过判据 |
| --- | --- |
| compat | Linux/amd64/root；内核 ≥ 5.10；Docker 与 Compose 版本可解析 |
| sources | 四个路径无符号链接组件、无 `..`/控制字符、root 持有且无组/其他写；`openat2` 证明同一挂载内无嵌套；源文件基线 SHA-256 落盘 |
| image | `RepoDigests` 锚定冻结 index；架构 amd64；image ID 为 config 摘要 |
| init | 新根 0700；config/state 0700/10001；口令 0600/10001；`prepared.json` 三个标志全 false；模板哈希未变 |
| gateway | 单元与二进制 SHA-256 匹配；证书 0644/私钥 0600；`systemctl is-active` 为 active；HTTPS `/healthz` 200 |
| start | 解析后配置与冻结身份逐项相等；网络 Internal、完整 ID、启动前无端点、固定 IPv4 在专属子网且非网关；容器 image/UID/只读根/四 bind/无端口/唯一网络全部复核；健康检查 200 |
| browser | 200；CSP 与 nosniff 存在；页面为真实 app shell |
| access | 登录 200、CSRF、扫描后 `checkpoint=completed` 且 `scanned` ≥ 预期；扫描前后源文件 SHA-256 不变；退出成功 |
| gui | `/api/capabilities` 的 quarantine/restore/purge 必须为 false；页面不得出现 `data-ndg-enabled="true"` 的危险控件 |
| backup | 按容器 ID 停止且确认已停；整目录冷拷贝；逐文件 SHA-256/模式/UID/GID；DB 含 WAL 的 `quick_check=ok` |
| upgrade | 备份已验证；候选镜像不同且本地存在；改用候选摘要后容器 image 等于候选；健康检查通过 |
| rollback | 旧 state 改名保留（不删除）；从备份恢复到新 state 并逐文件比对；镜像回到原摘要 |
| uninstall | 按 ID 停止/删除；无同名残留；自有网络无端点后删除；来源、state、备份逐文件不变 |
| reboot | restart policy 为 `unless-stopped`；整机重启后健康检查通过、认证快照与重启前一致、无遗留恢复锁 |

## 五、中止条件（任一出现立即停止并保留现场）

- 任一步骤返回非零，或 `status` 出现 `failed`；
- Journal 出现无终态的 `start`（崩溃足迹），且后置复核不通过；
- 源文件、state 或备份 SHA-256 与基线不一致；
- 容器/网络/服务单元的身份与计划不符；
- 需要执行计划外的删除、`-v`、`prune`、按名称前缀批量操作。

中止后不自动清理、不猜测成功；由所有者与执行人对照 receipt 中的
`journal.jsonl`、`actions.jsonl`、`sources.json`、`backup-receipt.json` 人工对账。

## 六、本次申请不覆盖的内容

- 跨版本迁移与备份重新绑定；
- 手机浏览器、外网访问、代理路径；
- 生产数据、长期性能、断电；
- Q/R/P 的写入执行与 Purge 发布。

这些需要单独设计与独立复审，不能由本申请顺带批准。
