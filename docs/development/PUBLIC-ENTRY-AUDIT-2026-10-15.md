# 公开入口与品牌核查（2026-10-15）

本轮接管现有 `docs/refresh-homepage-dualchannel-20261015`，保留既有首页修订，没有建立第二套 README。产品入口同时区分公开 Mac beta.11、Docker 只读 Preview.1 与尚未可用的 fnOS；完整 Quarantine/Restore/Purge 仍为开发目标。

## 已核对

- 两个公开 Release 的 Tag 对象、解引用源码 SHA、发行 ID、全部资产 ID/摘要已重新读取，记录在 [公开发行身份](../public-channels.json)。Mac 是 annotated tag，Docker 是 lightweight tag，核验分别保留 Tag 对象和源码身份。
- 本机在线身份核对 `check-public-channels.py --online` 为 LIVE_IDENTITY_PASS；该结果只证明公开服务身份与文档对应，不是主机/GUI 或删除能力验收。
- 全部受 Git 跟踪的 Markdown 的本地链接、图片及已引用锚点检查通过。可复现命令为 `python3 scripts/docs/check-links.py`。检查器排除 fenced code 示例、保留行号，拒绝逃出仓库或指向未跟踪资产；外部页面片段锚点不在自动认证范围。
- 当前机器的公共域名 DNS 由网络环境返回非公网地址。外部 HTTP 审计因此拒绝连接，记录 DNS_NOT_PUBLIC_NOT_FETCHED，不能写为死链或 HTTP_PASS。新增手动 GitHub Actions 外部审计在独立 runner 上运行并保留完整分类报告；实际执行回执另记，不以离线成功代替网络结果。
- 仓库简介及 Homepage 已通过 GitHub API 修改并读回；保留原 Topics，新增 docker、macos、file-management。Homepage 指向公开 Releases，简介明确 Docker 为只读 Preview。
- 规范图标 `cmd/ndg-desktop/build/appicon.png` 已实际查看，为蓝色 NDG 字母图标。README 使用此规范源；fnOS 研究包 256/64 图标已查看/核对尺寸，与该视觉源一致。旧 `docs/assets/ndg-logo.jpg` 是不同图案，保留为历史资产，不再作为当前首页标识。

## 自动检查及保留限制

`Public documentation` 在 PR/main 上检查本地链接、README 图像与公开渠道清单；手动 external 模式核对实时 Release/GHCR 身份并分类 HTTP 链接。鉴权、429、DNS、TLS、临时网络错误分别保留 UNKNOWN/受限状态；404/410 仅标为 DEAD_CANDIDATE，需进一步确认，不自动大规模改写历史文档。脚本不携带浏览器或 NAS 凭据，不访问私网目标，重定向逐跳核验。

README / 文档索引 / 能力矩阵 / 发布台账的历史段落仍保持历史属性。仓库中的 Docker 安装说明已更新为已公开状态并补充 digest 拉取方式；原 Release 的 15 个冻结文件没有替换。

Web 的 favicon 401 尚未修复；后续 fnOS/Web 构建和安装后的品牌一致性仍待实际验证。旧 DMG/镜像图标不改写；此记录不声称新 Mac 签名包、所有渠道图标或完整客户端支持已经通过。

#112 已经独立限定复审、更新后八项检查并正常合并为 `65b49a8541ab819c30fa782cab0f0e11afc4f958`；#111 仍 OPEN/BLOCKED，不因文档或开发 profile 合并而放行。
