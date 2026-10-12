# NDG 多渠道风险登记

快照：2026-10-12，main `584aafeb4cf1bb68cbda3bde2d26698b05555640`。状态含义依照 [主手册](NDG-MULTICHANNEL-MASTER.md)；无新证据不得提升，历史 FAIL 永久保留。

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
| R09 | MAS 注册/签名/Profile/外部审核未知，NO-GO | 设计 ID 不等于已注册；账户资格、协议和最终送审批准到实际需要时集中办理 | 账户持有人 |
| R10 | Docker 容器/两架构运行没有证据 | 本机无 Docker 不伪造运行；先源码安全 MVP，再隔离宿主或 CI 两架构/可信仿真实测 | G4 writer |
| R11 | Web 任意路径、认证/CSRF/rebinding/日志暴露 | 固定服务端别名，权限/路径校验、无默认密码、只读策略 API 负例；默认回环 | API writer / reviewer |
| R12 | Linux DB 多 writer/网络 WAL/重启或升级损坏 | 私有本地状态卷；全进程所有权锁；持久化/恢复锁/快照回滚独立验证 | G4 writer |
| R13 | fnOS 工具/平台/真机/OCI 依赖未就绪 | G4 固定 digest 优先；官方模板校准；无 FPK 不称上架；生产宿主仅授权独立测试区 | G5负责人 |
| R14 | 新渠道开启破坏性操作扩大认证范围 | MAS Quarantine/Restore/Purge 禁止；Docker/fnOS 首发只读；新写能力需要分离执行和正式门禁 | 总控 / 安全 reviewer |
| R15 | 多 IDE/agent 并发碰同工作树或历史现场 | 原子 lease + 单 writer + 独立分支/数据；不抢 lease、不 reset/clean；交接 exact diff | 总控 |
| R16 | 公开仓库出现私人路径、日志、凭据或 Bookmark | raw仅私有归档；每 PR public-check/secret gate + 人工边界复核；manifest/provenance 不带 secret | 各 writer/reviewer |
| R17 | AI review 被误当作者批准，或 CI跨 SHA | exact HEAD reviewer与实际 ruleset；不 admin bypass；每次主干合并后重新绑定 gates | 总控 |

若发现字节损坏、错误解锁、虚假回滚、源根逃逸或审计不一致，立即对应渠道 FAIL / SECURITY_BLOCKER，保护现场；不得清理真实数据取得 PASS。旧 beta.11、历史数据库、资产及失败记录保持不可变。
