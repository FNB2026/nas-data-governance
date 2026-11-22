# NDG Mac App Store 首发资料

待审核资料。首发渠道独立于 GitHub DMG，使用 `com.fnb.ndg.mas`，版本建议 `0.5.0`、构建 `1`、分类 Utilities、免费。编号以 App Store Connect 中该平台记录为准。

| 字段 | 简体中文 | English |
|---|---|---|
| 名称 | NDG | NDG |
| 副标题 | 只读重复文件分析 | Read-only duplicate analysis |
| 简介 | 扫描你授权的文件夹，查看重复文件、目录语境和保留理由。 | Scan folders you authorize and inspect duplicates, folder context, and retention evidence. |
| 关键词 | 重复文件,查重,文件扫描,NAS,SMB,目录,哈希,存储,归档 | duplicates,files,scanner,NAS,SMB,hash,folders,storage,archive |
| 分类 | 工具 | Utilities |
| 支持 | [NDG 用户支持](https://github.com/FNB2026/nas-data-governance/issues) | [NDG Support](https://github.com/FNB2026/nas-data-governance/issues) |
| 隐私政策 | [Mac App Store 版隐私政策](https://ndg.fnbapp.net/mac-app-privacy) | [Mac App Store Privacy Policy](https://ndg.fnbapp.net/mac-app-privacy) |

## 商店描述

NDG 帮你看清本机资料和已挂载存储中的重复文件。通过 macOS 系统选择器授权一个目录，使用分层指纹筛选候选，由完整内容哈希确认重复组。查看文件所在目录的语境、物理副本和保留理由，再决定如何在你的日常工作中处理资料。项目、扫描进度和结果保存在本机，可退出后重新打开，也可继续符合条件的中断扫描。

本版扫描源为只读。NAS 路径须先在 Finder 中挂载，再通过系统选择器授权。首发不提供隔离、恢复、永久删除或备份导入。

NDG helps you understand duplicate files in local folders and mounted storage. Choose a folder in the macOS picker. NDG uses layered fingerprinting to shortlist candidates and a full content hash to confirm duplicate groups. Compare folder context, physical copies and retention evidence. Projects, scan progress and results stay on your Mac. Reopen a project or resume an eligible interrupted scan.

Source access is read-only. Mount network shares in Finder before authorizing a folder. Quarantine, restore, permanent deletion and backup import are outside this edition. The current app interface is in Simplified Chinese.

## App Review 审核说明

无需账号或服务器。用两个相同的小文件和一个不同文件创建临时目录。首次启动选择「选择数据目录」，在 macOS 系统选择器中授权该目录，点击「开始扫描」，然后在「重复结果」打开文件组。正常退出后重新打开，在「最近项目」选回项目。源文件仅以只读权限打开。App Sandbox 内的数据库、扫描检查点及 app-scoped bookmarks 用于继续操作。可选择 Finder 中已挂载的共享，首轮本地验证无需网络共享。

No account or server is required. Prepare a test folder with two identical files and one different file. Select the folder in the system Open panel, start the scan, and open the duplicate group in 重复结果 (Duplicate Results). Quit and use 最近项目 (Recent Projects) to reopen the saved project. The source is read-only. The database, scan checkpoints and app-scoped bookmarks stay in the app container. A network share, if used, is mounted in Finder before folder selection.

## 隐私披露与素材

- App Privacy 拟填 **Data Not Collected / 不收集数据**。依据为首发本地处理架构，无账号、遥测或上传代码路径。发版负责人须按最终包确认；用户主动打开外部支持链接由浏览器处理。
- 图标复用项目 1254×1254 注册素材 `cmd/ndg-desktop/build/appicon.png`。打包脚本生成 macOS ICNS，其中含 1024×1024 表示。
- 在最终签名包重拍商店截图，使用合成数据，依次展示重复列表、目录语境、扫描进度及恢复、项目重开。中文 UI 的英语商品页可用准确英文图注。避免系统选择器中的个人目录、路径和测试账号。截图规格以 Apple 当前要求为准。 [截图规范](https://developer.apple.com/help/app-store-connect/reference/app-information/screenshot-specifications)
- GitHub README 和官网均应保留渠道“未发布”状态直到负责人授权。隐私 URL 已加入网站源码，正式提交前应验证部署结果。

## 安装与授权 / Installation and access

从 Mac App Store 安装 NDG，启动后选择目录，然后开始扫描。对 NAS，先在 Finder「连接服务器」挂载共享，只授权准备扫描的子目录。失去授权或共享不可用时，重新挂载并在数据源页重新选择目录。要继续扫描，打开原项目并选择对应未完成任务。索引由应用容器独立保存。

Install NDG from the Mac App Store once available. Select a folder in the system dialog and start the scan. For NAS files, connect to the share in Finder first, then authorize only the folder you want to scan. If access is lost, reconnect and choose the folder again on the Sources page. Reopen the same project to continue an eligible paused scan.
