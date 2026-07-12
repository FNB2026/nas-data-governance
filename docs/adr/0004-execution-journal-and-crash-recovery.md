# ADR-0004：执行动作落盘日志与崩溃恢复

状态：已接受

## 决策

每个文件系统写动作（MOVE/COPY/RENAME/DELETE→隔离）在执行前后向 `execution_journal` 表落盘一条记录，包含动作 ID、计划 ID、操作类型、状态（pending→done/failed→rolled_back）与实际目标路径。启动时 `Recover()` 扫描未完成的动作：对已完成但未校验的动作逆序回滚，对中断的计划重置为 APPROVED 等待重试。CLI `recover --db` 触发恢复。

## 原因

白皮书 K-007 要求操作可验证、可审计、可恢复。M3 的执行器已有 stale 复核和单次回滚，但若进程崩溃或断电，内存中的动作状态会丢失。没有落盘日志就无法区分"已完成的动作"和"未开始的动作"，恢复时可能重放已完成动作或遗漏未完成动作，破坏源路径与目标路径的一致性。

## 后果

- 执行器注入 `Journal` 接口，`BeginJournal/MarkJournalDone/MarkJournalFailed/MarkJournalRolledBack` 在每个动作前后同步落盘；nil journal 保持向后兼容（测试可用）。
- `Recover()` 是只读分析 + 必要时回滚，不产生新动作；回滚失败的动作标记为 failed，进入人工复核。
- 日志记录动作 ID、计划 ID、操作类型、状态与目标路径；不记录文件内容、哈希或敏感路径片段（遵循 AGENTS.md 审计不泄露路径）。
- 增量扫描（P0-2）与断点续扫独立于此 ADR，分别由 `scan_checkpoints` 表和 `file_instances.file_status` 列管理。
