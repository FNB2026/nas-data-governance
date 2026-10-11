# beta.11 首发阻断修复候选准备 — 2026-10-11

## 后续冻结与正式验证

版本 PR #83 已合并，最终 RC 为 `791f19119542e69b62ee62dd358fed188000837e`。
新的 annotated `v0.5.0-beta.11` 和签名 Draft 资产已生成；正式 Release
workflow 38122101720 SUCCESS。发行身份、实际 GUI 与限定日志证据及
仍未关闭的门禁见 [beta.11 正式验收与 Go/No-Go](beta.11-formal-acceptance-go-no-go-2026-10-11.md)。
下面保留版本准备时的要求；准备要求不等于全部发布门禁已通过。

本 PR 仅同步版本 0.5.0-beta.11 / Bundle build 11、CHANGELOG 和发行准备文档。
最终 RC 必须是本版本 PR 合并后的 main SHA；不得用修复 PR 的 merge SHA
代替正式发行身份。新 annotated Tag 与 Draft 签名资产不可覆盖 beta.10。

## 修复来源

- PR #58：exact e3144eaa0b73a09c25f267c77f9566c41d743425，锁文件仅
  source-map-js 1.2.1→1.2.2 / resolved / integrity。独立 APPROVE，CI
  38120717063 / Security 38120717070 PASS；merge
  f0a7c107c3253746e4f9e955ee98dcfa1d5ac176，post-merge CI38120923212 /
  Security38120924183 PASS，完整 audit 0。
- PR #82 / Issue #80：exact 4354b17a74a8a8baddde8bfb743388e5b6e501b9，
  独立 APPROVE，CI38121436651 / Security38121436708 PASS；merge
  f599cf764641ff970293d1ea46c40c64c8785f0b。新 Purge Create / Approve /
  Dry Run / Execute 在 Service / Wails / CLI 副作用之前固定拒绝，无普通
  参数、环境或 channel 开关。保留历史列表及既有异常恢复，不认证未验收的
  历史未知 staged 内容身份；Quarantine / Restore / Recovery 能力独立。

## 必需门禁

修复 main post-merge、版本 exact CI/Security、独立复审、版本 merge 后门禁
全部通过，才冻结 RC / annotated Tag。使用既有正式 Release 工作流完成
Developer ID、Notarization、Staple、Gatekeeper、DMG、SHA-256、CycloneDX
和 SPDX。所有产物绑定同一 RC。

从 Draft 下载资产核验哈希、签名、公证、About完整身份，在新 disposable
项目和人工文件进行 GUI Purge禁用 / 隔离-还原正常字节、计划、Journal、
审批一致性复验。启动前采集明确窗口 stdout/stderr 和统一日志，保留实际
采集覆盖范围，不扩大为长时或全系统隐私 PASS。不得实际执行 Purge。

beta.10 限定 Crash Gate PASS 的组合证据契约保留：正式 B/C 永久
INCONCLUSIVE，正式 A 仅未知部分输出保锁且审计限制保留；不以此版本
准备或 GUI 正常回归消除限制。原失败现场、Tag和资产不变。

## 发布边界

Release Draft、Public Beta BLOCKED。全新 Mac 离线首次安装暂缓仍 BLOCKED，
没有设备不等于豁免。正式 beta.11 完成后才裁定首发隐私/人工备份支持范围，
形成最终 Go/No-Go；公开发布需要用户另行明确授权。
