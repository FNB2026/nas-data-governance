# NDG Mac App Store 构建与交付

GitHub DMG 继续使用自己的构建、Bundle ID、版本和 Developer ID 签名。MAS 路径选择既有 Wails 桌面入口、`desktop,production,mas` 标签及专用前端构建变量。首发 App Store 版本暂定 0.5.0 (1)，正式 ID 为 `com.fnb.ndg.mas`，本地验收 ID 为 `com.fnb.ndg.mas.launchqa`。本地测试数据仅写入其独立容器。

## 本地沙盒构建

在 macOS、对应 Xcode SDK、仓库锁定的 Go 及受依赖支持的 Node 版本上运行。

```sh
bash scripts/mas/build-launch.sh local
NDG_MAS_QA_UNIVERSAL=1 bash scripts/mas/build-launch.sh local
```

脚本输出独立目录和 `NDG.app`，用本机沙盒签名启动验证。首发上架包要求最终签名链。

## 账号持有人操作

1. 在发行团队中选择或注册准确 App ID `com.fnb.ndg.mas`，并在 App Store Connect 建立相同 Bundle ID 的 macOS 应用记录。确认应用名称、团队和版本号可用。[注册 App ID](https://developer.apple.com/help/account/identifiers/register-an-app-id/) · [App Store Connect](https://appstoreconnect.apple.com/)
2. 在构建机的钥匙串中提供带私钥的 Apple Distribution 或 Mac App Distribution，以及 Mac Installer Distribution，保持两者同一批准团队。创建与此 Bundle ID 和应用签名证书匹配的 App Store Connect 分发 Profile。[证书类型](https://developer.apple.com/help/account/certificates/certificates-overview/) · [发行 Profile](https://developer.apple.com/help/account/provisioning-profiles/create-an-app-store-provisioning-profile/)
3. 处理 [Required Reason 核验](required-reason-volume-identity-2026-10-12.md) 中的未决用途，审查最终产物和依赖，确认 `PrivacyInfo.xcprivacy`。不要填报项目并未使用的容量用途。若当前 Apple 文档无法确定 `statfs`/SQLite 该用途的口径，向 Apple Developer Support 提交 API、具体调用位置和实际处理目的。[Apple Developer Support](https://developer.apple.com/contact/) · [Required Reason 说明](https://developer.apple.com/documentation/bundleresources/describing-use-of-required-reason-api)

## 正式包

输入路径留在构建机本地，证书私钥及 Profile 不进入仓库。App Store 专用脚本校验身份类型，绑定 profile、沙盒 entitlements 与 Bundle ID，生成 arm64 + x86_64 应用，签署 Installer Package 并核对签名。

```sh
NDG_MAS_PROFILE=/private/path/NDG-MAS.provisionprofile \
NDG_MAS_APP_IDENTITY='Apple Distribution: <approved team>' \
NDG_MAS_INSTALLER_IDENTITY='Mac Installer Distribution: <approved team>' \
NDG_MAS_PRIVACY_MANIFEST=/private/path/PrivacyInfo.xcprivacy \
NDG_MAS_VERSION=0.5.0 NDG_MAS_BUILD=1 \
bash scripts/mas/build-launch.sh distribution
```

核验并保存签名、Sandbox、架构、资源和 SHA-256 输出。在最终构建上做安装运行验收，更新真实截图和 [双语商店资料](launch-store.md)，验证公开隐私及支持 URL。负责人批准后，可用 Transporter 校验/上传最终 `NDG.pkg` 至对应 App Store Connect 记录，选择 Build，另行批准 Submit for Review。[Mac 安装包](https://developer.apple.com/documentation/xcode/packaging-mac-software-for-distribution) · [上传构建](https://developer.apple.com/help/app-store-connect/manage-builds/upload-builds/)
