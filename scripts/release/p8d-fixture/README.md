# P8-D Release-Test Fixture

该工具只为 RC 的 K5、K6 人工桌面验收创建一次性项目。它不接收任何路径参数；每次运行均通过 `os.MkdirTemp` 创建独立临时根目录、文件和 `governance.db`。不得用于生产项目、NAS 或用户数据。

```bash
go run ./scripts/release/p8d-fixture k5
go run ./scripts/release/p8d-fixture k6
```

命令输出 JSON manifest，其中的 `database_path` 是唯一需要在 NDG 的“高级：打开已有数据库”中使用的值。

## K5：Purge 风险确认

fixture 通过 `store` API 建立一条完成的 QUARANTINE 历史：`quarantined_at` 为当前时间前 25 小时，`retain_until` 为当前时间前 1 小时。因此它满足最低 24 小时保留期，并以已到期的历史状态验证真实 UI，不改变产品默认 30 天设置。

在 NDG.app 中依次执行：生成清理草案、批准、试运行、输入错误确认文本（按钮必须禁用）、输入精确确认语句。只有在人工确认后才能点击“永久清理”。随后检查文件不存在、`quarantine_items.status=PURGED`、`purge_plans.state=PURGED` 和 Purge Journal 为 `committed`。

## K6：Recovery Lock

fixture 通过 `store` API 创建一个 `EXECUTING` 的普通执行计划及一条 `pending` Journal。它没有任何已完成的文件写入，源文件保持原位。

在 NDG.app 读写打开该项目后，应显示 Recovery Lock：扫描和执行入口禁用，审计与恢复入口可用。点击“恢复普通执行”后，计划应重置到 `APPROVED`，锁清除，源文件哈希不变。
