# ADR-0007：项目生命周期下沉与多实例协调

- 状态：Accepted
- 日期：2026-07-30

## 背景

首次启动项目创建曾集中在 Wails Binding，导致 CLI 或其他桌面壳无法复用源目录验证、项目元数据、最近项目和失败回滚规则。同时，固定临时文件名和“先检查再创建”无法安全协调两个 NDG 实例。

## 决策

新增 `internal/project.Service` 作为项目生命周期应用服务，负责：

- `CreateFromSource` 与原子项目目录抢占；
- `ValidateScanSource`、`RegisterScanSource`；
- `ProjectMeta`、逻辑项目 ID 与旧任务作用域兼容；
- `RecordRecent`、`ListRecent`；
- `RollbackProjectCreation`。

Wails Binding 只保留原生目录选择、DTO 映射和应用服务装配。

多实例协调使用三层机制：

1. 使用 `os.Mkdir` 原子抢占项目目录，冲突实例自动尝试带序号 ID；
2. 原子写使用同目录唯一临时文件，再 `fsync` 和 rename；
3. `recent.json` 的读改写使用 OS 文件锁，避免进程间丢失更新。

## 约束

- 扫描源保持只读，项目数据库仍只位于 Application Support；
- 回滚只能删除本次原子抢占且位于 projects 根目录内的目录；
- 不把路径、文件名或内容写入普通日志；
- 旧数据库继续使用历史绝对路径查询既有 Job，但对外 `ProjectInfo.project_id` 始终不是路径。

## 验证

并发测试使用多个独立 `Service` 实例同时创建同名项目并更新最近项目，验证目录 ID 不冲突、清单不丢记录、无临时文件残留。原有 Wails 集成测试继续覆盖创建、默认命名、源验证、回滚和重开。
