# NDG 多渠道风险登记

快照：2026-10-13，main `15328e0054c1c3e476dd756436ce52d6f3c50c1a`。状态含义依照 [主手册](NDG-MULTICHANNEL-MASTER.md)；无新证据不得提升，历史 FAIL 永久保留。

| ID | 风险与实际状态 | 约束 / 推进条件 | 责任方 |
|---|---|---|---|
| R01 | 附件/历史文档仍称 beta.11 Draft/未授权 | 当前 Release 已公开 Pre-release；单独校准，不覆盖历史 NO-GO；其他渠道不得复用发布许可 | 总控 |
| R02 | 全新 Mac 离线首装未实测，WAIVED_BY_OWNER | 负责人已接受该次风险；绝不记 PASS，其他版本不自动豁免；限定支持说明保留 | 发行负责人 |
| R03 | MAS Required Reason 卷身份用途尚未合规闭合，BLOCKED | 官方最新文档/等价身份验证；不使用不真实 Reason；必要时明确 DTS 问题 | 兼容性 writer + 独立 reviewer |
| R04 | 不确定卷身份继承旧授权/checkpoint | 网络卸载本地挂载点、不同端点同路径、卷 ID 缺失/不稳定须 fail-closed；正式环境未测不放行 | MAS writer |
| R05 | Phase 1C staging 所有权/容器/TOCTOU 未解决 | 0700 不证明 owner/ACL/container；信任调用方与稳定 ancestry 限定原型；正式 importer hard BLOCKED | 导入设计负责人 |
| R06 | exporter 来源、全项目锁与闭合 SQLite 备份未建立 | 不复制 live DB/共享实时 WAL；校验 hash 不是真实性证明；独立设计来源契约/快照/不可变备份 | exporter 设计负责人 |
| R07 | MAS Phase 1A/1B INCONCLUSIVE；独立环境 BLOCKED | 不接管已有 QEMU、不卸载未知 SMB；新环境身份可证后补实际场景 | 环境负责人 |
| R08 | CarbonCore 严格零命中历史 FAIL；来源未闭合 | 三窗口对照与 App/WebKit/stdout 分层归因；Apple 名称不自动豁免；不声称全系统无痕 | 隐私 reviewer |
| R09 | MAS 注册/签名/Profile/外部审核未知，NO-GO | 当前本机仅见 Developer ID Application=1，未见两项 MAS identity；不推定账户无资格；设计 ID 不等于已注册；账户资格、协议和最终送审批准到实际需要时集中办理 | 账户持有人 |
| R10 | Docker 两架构合成容器 PASS，但无正式公开镜像/部署闭环 | PR #93 原生 Ubuntu 两架构回执；限定本地合成源，不替代真实网络源、公开供应链或正式宿主认证 | G4 writer |
| R11 | Web 任意路径、认证/CSRF/rebinding/日志暴露 | 固定服务端别名，权限/路径校验、无默认密码、只读策略 API 负例；默认回环 | API writer / reviewer |
| R12 | Linux DB 多 writer/网络 WAL/重启或升级损坏 | 私有本地状态卷；全进程所有权锁；持久化/恢复锁/快照回滚独立验证 | G4 writer |
| R13 | fnOS 官方 fnpack help/template PASS；平台/真机/OCI 依赖未就绪 | G4 固定 digest 优先；研究骨架不是可安装 FPK；工具证据不是签名证明；无 FPK 不称上架；生产宿主仅授权独立测试区 | G5负责人 |
| R14 | 新渠道开启破坏性操作扩大认证范围 | MAS Quarantine/Restore/Purge 禁止；Docker/fnOS 首发只读；新写能力需要分离执行和正式门禁 | 总控 / 安全 reviewer |
| R15 | 多 IDE/agent 并发碰同工作树或历史现场 | 原子 lease + 单 writer + 独立分支/数据；不抢 lease、不 reset/clean；交接 exact diff | 总控 |
| R16 | 公开仓库出现私人路径、日志、凭据或 Bookmark | raw仅私有归档；每 PR public-check/secret gate + 人工边界复核；manifest/provenance 不带 secret | 各 writer/reviewer |
| R17 | AI review 被误当作者批准，或 CI跨 SHA | exact HEAD reviewer与实际 ruleset；不 admin bypass；每次主干合并后重新绑定 gates | 总控 |
| R18 | fnOS 公开自安装 FPK 路线存在外部政策阻断 | [官方规范](https://developer.fnnas.com/docs/quick-started/test-application/) 限手动安装于本地测试；不得以主手册旧快照自安装 Beta 表述公开分发；向平台咨询可获批准渠道和开发者提交权限 | 平台负责人 / 总控 |

若发现字节损坏、错误解锁、虚假回滚、源根逃逸或审计不一致，立即对应渠道 FAIL / SECURITY_BLOCKER，保护现场；不得清理真实数据取得 PASS。旧 beta.11、历史数据库、资产及失败记录保持不可变。

## UX v2 新增风险（当前开发线）

- R19：旧单组决定的无 CAS/幂等性/审计事务及 MAS/恢复锁入口缺口。由独立
  UX2 工作包先复现再修复，禁止前端 legacy mutation loop 隐藏问题。
- R20：UI 将意图当执行、未知当低风险、同内容当删除许可。统一词汇明确未批准/
  未执行；未知物理身份不报告可回收空间；系统建议和用户决定区分。
- R21：跨页选集、晚到响应、部分成功或丢失回执导致重复记录。显式版本快照、
  后端项目作用域/CAS/幂等、可查询回执；客户端未知结果禁止新 key 重放。
- R22：旧 binary 不参与新 writer lock；本轮锁不能追溯保证 beta.11 并发互斥。
  测试只使用独立项目，旧 writer 必须清洁关闭；不自动迁移历史现场。
- R23：重启后的 GUI 批次发现仍缺少契约。已知 key 的后端查询不等于用户界面
  能重新找到请求；完整 UX3 接入前仍未认证，不能靠 mock 绿色关闭。
