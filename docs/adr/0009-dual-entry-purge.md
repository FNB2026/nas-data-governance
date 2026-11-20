# ADR-0009 · 双入口永久删除

状态：实施中。产品范围见 [Issue #140](https://github.com/FNB2026/nas-data-governance/issues/140)。

本决策沿用 ADR-0005 的隔离区清理，并增加单独的源文件直删入口。两个入口都按文件生成独立计划、审批、Dry Run、提交与回执。源文件直删只允许数据库中当前有效的扫描文件实例，并验证一个独立物理对象作为保留副本。已有的 `delete_to_quarantine` 继续表示可恢复隔离。

Linux 执行层使用已核对的根、父目录和挂载身份，将对象同卷移入私有暂存，再验证对象、持久化提交标记并清理。提交标记存在或证据不足时保留材料，交给独立恢复步骤。普通删除不承诺介质安全擦除。Mac 的实际执行取决于 Darwin 挂载边界证明。

The Linux integration candidate requires an exclusive Worker UID and GID.
The purge root and every target or keeper parent within it must be owned by
that subject, mode `0700`, without an extended ACL. Eligible files must
also belong to that subject and must not grant writes to group or other
users. Only the synthetic candidate fixture supplies this namespace today.
A shared source tree or another service using the Worker UID falls outside
this deployment contract.
