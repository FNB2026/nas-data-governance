# Docker Read-only Public Preview

候选版本：`0.5.0-docker-preview.1`。当前为工程候选，尚未授权公开。
Mac 的 `VERSION` / beta.11 资产不变；容器内部通道标识继续为 `web-experimental`。

## 最小支持范围

Linux amd64 / arm64，本地受支持文件系统、单实例、人工明确配置的源根目录。
Web 提供认证、源别名、只读扫描、取消/断点恢复和有限聚合结果；不提供文件路径、
治理意图记录、Quarantine、Restore、Purge 或计划执行。不得称为完整治理产品。
SMB/NFS/FUSE、公开互联网入口、反向代理、跨版本数据库迁移、FPK 和 App Store
均不在此预览的已验证支持范围。fnOS Docker 28.5.2 / Compose 5.0.2 的宿主侧人工源隔离链路已测；但现有 SSH
禁止 TCP 转发，客户端入口实际拒绝，因此平台仍为实验性 / 客户端入口 BLOCKED。
不认证生产 NAS 数据或其他版本。详见 [实测记录](DOCKER-PREVIEW-RESULTS-2026-10-15.md)。

## 安装边界

下载对应架构的 `.docker.tar.gz`、`SHA256SUMS`、`release-candidate.json` 和
`.evidence.tar.gz`。先核对 SHA-256（例如 `sha256sum -c SHA256SUMS --ignore-missing`），
然后导入固定字节的候选：

```sh
gzip -dc ndg-0.5.0-docker-preview.1-linux-arm64.docker.tar.gz | docker load
# AMD64 主机将文件名中的 arm64 换为 amd64。
docker image inspect ndg:0.5.0-docker-preview.1
```

核对架构和 source revision / tree / version。记录实际 inspect ID，作为 `NDG_IMAGE`。
传统 image store 的 ID 是 config digest；本次 Docker 29/containerd 现场返回 manifest
ID，因此不能只比较 inspect ID 与 config digest。发行前已对实际运行镜像执行
`docker image save`，验证导出 config SHA-256 和每一层 diff ID 与 OCI 一致；相关值见
清单。SHA256SUMS 将下载归档绑定到这次实测字节，不要自行转换/重建后沿用结论。
多架构 OCI 归档保留原始每架构 manifest，作为后续注册表原样推广候选。

复制 `packaging/docker/compose.yaml` 到全新私有部署目录。建立 `private-config`
和 `private-state`，权限 0700、所有者 UID/GID 10001。生成至少 32 字节随机口令，
存入 `private-config/credential`（0600），不要放入命令行、Compose 环境或日志。
从 `server.example.json` 建立 `private-config/server.json`（0600），配置：

```json
{
  "listen": "0.0.0.0:8080",
  "origin": "http://127.0.0.1:18084",
  "state": "/data",
  "credential_file": "/config/credential",
  "sources": {"source_a": "/sources/source_a"}
}
```

设置 `NDG_IMAGE` 为已核对的本地 image ID，`NDG_DISPOSABLE_SOURCE` 为明确的绝对
本地源目录。首次只使用人工数据；确保 UID 10001 有读取及目录搜索权限。应用不自动
chmod/chown 源目录。`docker compose up -d` 后检查容器 user、只读根文件系统、
capabilities、no-new-privileges、三个挂载的读写属性及唯一 internal 网络。
不得挂载 Docker socket、宿主根目录、设备或生产数据。不得为修复端口发布增加外联网络。

此配置不发布端口。Docker 的 internal-only 网络在部分版本会忽略 published ports，
历史 fnOS 窗口因此在 HTTP 前失败。保留该历史失败。当前入口是从客户端建立 SSH
本地转发到该容器的 internal IP：

```sh
# 在 Docker 主机查询此 Compose 项目自己的容器 IP，记录并核对 ID/网络归属。
docker inspect --format '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' "$(docker compose ps -q ndg)"
# 在客户端替换下面的 HOST 和已核对的 IP；SSH 账号需已有授权。
ssh -N -L 127.0.0.1:18084:CONTAINER_IP:8080 USER@HOST
```

fnOS 宿主侧实验使用 `compose.fnos.yaml`（本次设备禁止 SSH TCP 转发，尚无已验证客户端入口）：先用 `docker network create --internal NAME`
创建一个全新项目专用 bridge，记录返回的完整 ID，核对 `docker network inspect ID`
中的 `Internal=true`、`Driver=bridge` 和无其他 endpoint，再将 ID 设置为
`NDG_INTERNAL_NETWORK`。模板与实测相同，使用该 ID 作为 network_mode，无 published
ports、无 Compose default 网络。卸载只删除本项目容器和该确切专用网络，保留 bind
目录；不修改既有网络、daemon 或其他 Compose 项目。此模板的宿主验收不代表客户端可用，
不得据此更改现有 sshd 策略或建立绕过通道。

浏览器打开 `http://127.0.0.1:18084` 并输入口令。容器重建后 IP 可能变化，必须重新
核对并建立转发。仅在受信任的主机与 SSH 隧道中使用 HTTP；不配置远程明文入口。
此方案不修改 SSH/Docker daemon 配置，不提供宿主自动代理进程。

## 数据保留与备份恢复

源目录只读；扫描结果与断点仅存入 `private-state`。先停止容器并确认进程退出，
再在独立执行步骤复制整个 state 目录（包含所有数据库文件），保留 UID/GID 和模式。
备份放入私有目录。恢复到新的空目录，保持源根身份和别名，再启动单个实例。
不要在线仅复制主 DB，不要删除恢复锁，不要将同一 state 同时交给两个进程。
恢复验收需核对已完成快照/断点与重新扫描结果；容器删除不等于删除 bind-mounted state。
卸载先停止并删除本项目容器、仅删除本项目网络；保留 state、备份和源目录。
不使用 `docker system prune`、卷清理或广泛删除命令。

## 放行证据（各层分别裁定）

- 同一源码 SHA / tree 的 amd64、arm64 OCI、config image ID、manifest digest、归档 SHA-256。
- 镜像及源依赖 SBOM、真实漏洞扫描、工具及构建材料，未签名/未重现性必须如实标注。
- 独立 Linux 主机版本、实际运行 image ID、真实 HTTP 和 Web 浏览器交互。
- 认证、CSRF、Host/Origin/路径/别名拒绝，危险 API 拒绝、非 root、只读挂载和网络隔离。
- 合成源文件扫描前后 SHA-256、取消/断点恢复、重启、删除容器后持久化、离线备份恢复。
- 有限窗口日志隐私、源边界负例、资源保留与本项目清理回执。

CI 的合成容器结果不替代独立主机结果。任何未运行项保留 NOT RUN，安全失败保持 BLOCKED。
完成候选和上述最低证据后才集中申请对外授权；不覆写 beta.11 或已有发行资产。

网络依据：[Docker internal 网络](https://docs.docker.com/engine/network/drivers/bridge/)、
[端口发布](https://docs.docker.com/engine/network/port-publishing/)、
[Moby internal-only 端口问题](https://github.com/moby/moby/issues/44986)。
