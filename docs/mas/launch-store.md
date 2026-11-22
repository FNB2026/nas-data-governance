# NDG Mac App Store 首发资料

待审核资料。首发渠道独立于 GitHub DMG，使用 `com.fnb.ndg.mas`，版本建议 `0.5.0`、构建 `1`、分类 Utilities、免费。编号以 App Store Connect 中该平台记录为准。

| 字段 | 简体中文 | English |
|---|---|---|
| 名称 | NDG | NDG |
| 副标题 | 重复文件治理与可恢复隔离 | Duplicate governance and recoverable quarantine |
| 简介 | 扫描你授权的文件夹，核对重复证据，人工批准后可恢复地隔离副本。 | Scan folders you authorize, review duplicate evidence, and quarantine copies recoverably after explicit approval. |
| 关键词 | 重复文件,查重,文件扫描,NAS,SMB,目录,哈希,隔离,恢复,归档 | duplicates,files,scanner,NAS,SMB,hash,quarantine,restore,archive |
| 分类 | 工具 | Utilities |
| 支持 | [NDG 用户支持](https://github.com/FNB2026/nas-data-governance/issues) | [NDG Support](https://github.com/FNB2026/nas-data-governance/issues) |
| 隐私政策 | [Mac App Store 版隐私政策](https://ndg.fnbapp.net/mac-app-privacy) | [Mac App Store Privacy Policy](https://ndg.fnbapp.net/mac-app-privacy) |

## 商店描述

NDG 帮你看清本机资料和已挂载存储中的重复文件。通过 macOS 系统选择器授权一个目录，使用分层指纹筛选候选，由完整内容哈希确认重复组。查看文件所在目录的语境、物理副本和保留理由，再为重复组记录决定。扫描只读取文件；只有在你逐项复核、明确批准并完成执行前校验后，隔离才会把副本移动到应用容器内的隔离区。隔离可随时恢复，恢复同样需要人工批准。永久清理在本发行版中不开放。

项目、扫描进度、决定、审批、隔离与恢复记录保存在本机。可退出后重新打开，也可继续符合条件的中断扫描。异常中断后应用会先锁定写入并要求恢复，未确认完成状态的操作不会被静默当作成功。

NDG helps you understand duplicate files in local folders and mounted storage. Choose a folder in the macOS picker. NDG uses layered fingerprinting to shortlist candidates and a full content hash to confirm duplicate groups. Compare folder context, physical copies and retention evidence, then record a decision for each group. Scanning only reads files. Quarantine moves a copy into the app container's quarantine area only after itemized review, explicit approval and a pre-execution check. Quarantined copies can be restored, also with explicit approval. Permanent purge is disabled in this release.

Projects, scan progress, decisions, approvals and quarantine/restore records stay on your Mac. Reopen a project later or resume an eligible interrupted scan. After an unexpected interruption, NDG locks further writes until recovery is confirmed; incomplete operations are never silently treated as successful. The current app interface is in Simplified Chinese.

## App Review 审核说明

无需账号或服务器。准备一个临时目录，在其中创建 `cache/pair-a-1.bin`、`cache/pair-a-2.bin`（内容相同的两个小文件）和一个内容不同的文件。首次启动选择「新建扫描项目」，在 macOS 系统选择器中授权该目录（读写），开始扫描。

在「重复治理」选择该重复组，把主要决定设为「制定处理方案」并保存。在「查看单计划复核」生成并保存系统建议，选择草案并批准。在「任务中心」勾选已批准计划，先执行「执行前试运行」，通过后执行「执行隔离」。应用容器隔离目录默认为本项目私有路径，无需第二次授权。随后在「隔离与恢复」创建恢复草案、批准并执行恢复，源文件哈希应恢复一致。「清理」页展示永久清理当前不可用。

扫描不写入源目录。隔离和恢复只在人工批准后写入已授权目录或应用容器隔离目录。可选用 Finder 中已挂载的共享；首轮本地验证无需网络共享。永久清理在本发行版中保持禁用。

No account or server is required. Prepare a test folder with two identical small files under `cache/` plus one different file. Choose the folder in the system Open panel with read-write access, start the scan, record a plan decision in 重复治理 (Duplicate Governance), generate and approve the plan in 查看单计划复核 (Plan Review), then run the dry run and quarantine in 任务中心 (Task Center). The app-owned quarantine folder needs no second authorization. Create, approve and execute a restore in 隔离与恢复 (Quarantine & Restore) to confirm the original hash. 清理 (Purge) is visible but disabled in this release.

Scanning does not write to the source. Quarantine and restore write only after explicit approval, into the authorized folder or the app container quarantine area. A network share, if used, is mounted in Finder before folder selection. Permanent purge stays disabled in this release.

## 隐私披露与素材

- App Privacy 拟填 **Data Not Collected / 不收集数据**。依据为首发本地处理架构，无账号、遥测或上传代码路径。发版负责人须按最终包确认；用户主动打开外部支持链接由浏览器处理。
- 图标复用项目 1254×1254 注册素材 `cmd/ndg-desktop/build/appicon.png`。打包脚本生成 macOS ICNS，其中含 1024×1024 表示。
- 在最终签名包重拍商店截图，使用合成数据，依次展示重复列表、目录语境、治理决定、计划批准、任务中心隔离/恢复及项目重开。中文 UI 的英语商品页可用准确英文图注。避免系统选择器中的个人目录、路径和测试账号。截图规格以 Apple 当前要求为准。 [截图规范](https://developer.apple.com/help/app-store-connect/reference/app-information/screenshot-specifications)
- GitHub README 和官网均应保留渠道“未发布”状态直到负责人授权。隐私 URL 已加入网站源码，正式提交前应验证部署结果。

## 安装与授权 / Installation and access

从 Mac App Store 安装 NDG，启动后选择目录并授予读写访问，然后开始扫描。对 NAS，先在 Finder「连接服务器」挂载共享，只授权准备治理的子目录。失去授权或共享不可用时，重新挂载并在数据源页重新选择目录；旧只读授权不会被静默升级，需要重新授权。要继续扫描，打开原项目并选择对应未完成任务。索引和隔离区由应用容器独立保存。

Install NDG from the Mac App Store once available. Select a folder in the system dialog with read-write access and start the scan. For NAS files, connect to the share in Finder first, then authorize only the folder you want to govern. If access is lost, reconnect and choose the folder again on the Sources page; an old read-only grant is never silently upgraded. Reopen the same project to continue an eligible paused scan. The index and quarantine area are kept in the app container.
