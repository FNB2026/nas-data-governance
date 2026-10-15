# Docker Preview — 2026-10-15 真实验收

候选 `0.5.0-docker-preview.1`，源码 `1db1e4ab9a1287d3f7f9b801b471b00bfc7c7d1e`，
tree `23affa62ffa2685e7bced588cf90676e7c6242c2`。
[双架构构建、SBOM 与漏洞回执](https://github.com/FNB2026/nas-data-governance/actions/runs/38729064851)
成功。Mac `VERSION` 和 beta.11 资产不变；该版本尚未获得对外公开授权。

| 层 | 真实证据 | 裁定 |
|---|---|---|
| AMD64 / ARM64 原生 CI | 精确源码、OCI、镜像/源双格式 SBOM、npm 全 lock / 可达 Go / Grype runtime 扫描 | PASS，仍是构建层 |
| 独立 Linux 主机 | 全新 Ubuntu ARM64 VM，独立 kernel、Docker 29.1.3/containerd；非 GitHub runner | 主机验收 PASS |
| 源保护 | 4,000 人工文件 SHA-256 前后相同；符号链接不跟随；真实同设备嵌套 bind mount 未被遍历，DB 无边界外文件 | PASS，Linux 专属证据 |
| 生命周期 | 真实取消和断点；删除容器后恢复；重启；关闭后完整 state 备份、恢复和重扫 | PASS |
| 安全 | 非 root UID 10001、只读 root/source/config、EROFS 实测、无 capabilities/no-new-privileges、唯一 internal 网络、无默认路由 | PASS |
| HTTP | 认证、cookie、CSRF、Host/Origin/别名/任意路径拒绝、危险 API 拒绝、嵌入 Web 资产 | PASS |
| 真实 Chromium | SSH 入口登录、扫描 202、自动刷新 4,000 文件结果、可见控件和键盘注销清空结果 | PASS，有限主流程 |
| fnOS Docker | Linux x86_64 6.18.18.c1126-trim，Docker 28.5.2/overlay2、Compose 5.0.2；7 人工文件、3 组、6 full；安装、单写者拒绝、重启、Compose 删除后重建、保留状态及卸载 | 宿主链路 PASS；客户端入口 BLOCKED，平台仍实验性 |
| fnOS 保护 | 15 个既有容器的限定状态字段前后相同；仅 nonce 自有容器/网络/镜像被移除；人工源与 state 保留 | PASS，非全系统审计 |

Docker 29 此现场的 inspect ID 为 manifest descriptor；未将其强行等同于 OCI config。
重新导出实际运行镜像，逐字节验证配置 SHA-256 及所有 uncompressed layer diff IDs，
再与已验证 OCI 对照。fnOS 28 的 config ID 与原 OCI 一致。完整身份见 Draft 资产
`release-candidate.json`、两个已脱敏主机回执和 SHA256SUMS。

最终多架构 OCI index：
`sha256:49dada73f1eb9cc74485ed5d0b92023d7cb4c3df462d03e105f00158b91668e1`。
AMD64 manifest：`sha256:bb6db808bc5a17cf27b951b07406136c6bba3056c89d2aee0268042a683f5be4`。
ARM64 manifest：`sha256:4e13ba64c9b85d8e293b7c544c1a877d73c1a09b56323699e1a84464ed8ed216`。

## fnOS 客户端入口阻断（最终裁定）

宿主 HTTP 和 Compose 安装/重建/保留/卸载结果不变。随后检查实际 SSH 客户端入口：
`sshd -T` 返回 `allowtcpforwarding no`，真实本地转发连接返回
`administratively prohibited: open failed`。未改动 sshd/Docker daemon、防火墙或既有服务，
不提供策略绕过。fnOS 平台仍为 **EXPERIMENTAL / CLIENT_INGRESS_BLOCKED**，不升级为
已验证完整支持。此阻断不影响独立 Ubuntu Linux 的 SSH 浏览器链路和 Docker Preview。
需要单独确定符合现有主机策略的客户端入口，或获得范围明确的配置变更授权后再验。

## 发现、修复与保留限制

真实截图发现 Web 入口漏载 tokens 导致按钮白底不可见，已在候选源码修复并重新构建、
重新执行两条主机链路及浏览器检查。较早 `d809e13` 镜像不进入最终候选。
测试驱动的 namespace root FD、人工文件所有权、取消时序和 Compose 重建时的
exclusive 文件写入问题分别修正；失败记录保留在私有证据中，不追认为产品 PASS。

旧 fnOS `loopback_binding_changed` 历史失败保留。当前方案不发布容器端口，
不增加外联网络；由已验证 internal endpoint 接受主机 HTTP，客户端通过 SSH 转发。
Compose 模板分为普通独立 internal 网络和 fnOS 明确指定已核对 internal network ID。
未更改 Docker daemon、SSH 服务配置或任何既有容器。

Web 当前是有限聚合技术预览，原始 JSON 结果最多 200 组；没有路径、完整目录职责、
分页治理或任何写入执行。favicon 请求仍返回 401（已知非阻断项）；不称为零控制台错误。
未签名、未证明重现性；未认证 SMB/NFS/FUSE、反向代理、生产 NAS 数据、原生 FPK、
App Store、完整 UX 25 项及非开发者走查。#111 Darwin 缺口保持 BLOCKED。
#112 仍需独立复审，当前单 Agent 的作者自查不充当独立审查。

安装、停止后备份、恢复和卸载方法见 [安装说明](DOCKER-READONLY-PREVIEW.md)。
本记录只放行固定字节的发行候选；公开 GitHub Release 和注册表镜像仍须所有者授权。
