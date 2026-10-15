# fnOS CGI 入口候选与真实身份实验（2026-10-15）

状态：开发候选，尚未完成 NDG 客户端安装／扫描验收。不是 fnOS 正式支持，不修改已经公开的 Docker Preview.1。

## 有界实验及真实结果

设备为 fnOS 1.2.0800、Linux x86_64 6.18.18.c1126-trim、Docker 28.5.2、Compose 5.0.2。实验前记录了 15 个现有容器及 8 个网络；使用新建 NDG 专属原生应用，无源目录、数据库、Docker 权限或后台服务。

1. `install-local` 初次因缺少 `cmd/install_init` 返回 10111。补齐八个无副作用 hooks 后返回 10237（执行 `install_init` permission denied）。仅放宽本次无凭据源根至 0755 后仍失败；未修改平台目录、SSH、Docker daemon、网络配置或业务应用。失败原因尚未确证，保留失败。
2. 官方 fnpack 1.2.3（本机工具 SHA-256 `d40cb00896cb2a5d211357d255750ed0cbe7f2d141df671c2b717afb4e74bf77`，没有发布方 checksum 认证）打包同一探针；FPK SHA-256 `10c6e370854f107c021f78a519dca94f9a804b98efc5fb199ff142ce541f1315`，`install-fpk` 成功。这不是完整 NDG FPK 安装验收。
3. 在实际 Chrome 中登录 NAS HTTPS 管理界面并打开探针 CGI。输出为 **REJECTED**：预设门禁错误地要求 passwd 主 GID 与运行 GID 相同。实际 real/effective/saved/filesystem UID 均为专属 966，GID 均为专属组 967，附加组为空，effective/permitted capabilities 为零，NoNewPrivs 为 0；账户主 GID 是平台公共组 901。同名组和账户均由本次首次安装创建，先前不存在。保留原 REJECTED，未改写为产品 PASS。
4. 独立复审认可门禁改为可信配置指定的专属 UID/GID，不应套用 passwd 主 GID；仍须检查全部身份、附加组、inheritable/permitted/effective/ambient capabilities 与 NoNewPrivs。实际安装后的元数据、生命周期 hooks 为 root-owned、包用户不可写；CGI 代码位于包用户可写的 target，不能称为代码防篡改。

NAS 当前证书是自签名，Chrome 已有用户保留的站点例外。本轮核对浏览器证书与经 SSH 从实际 HTTPS 监听器读取的证书指纹一致后完成登录；未关闭新的浏览器警告、未改系统信任或 NAS 证书。此证据不是可信 CA 或首次安装 TLS 无障碍 PASS。

探针生成器为 `scripts/research/fnos-cgi-identity-probe.py`，只在本地新建 stage，不连接或安装 NAS。实验私有身份清单、原始平台回执保存在本任务隔离证据目录，不公开业务路径或登录凭据。

## 受限候选边界

`cmd/ndg-fnos-cgi` 是每次请求运行的短 CGI，不是执行 Worker。采用官方 [CGI 入口](https://developer.fnnas.com/docs/core-concepts/index-cgi/) 和 [包用户权限](https://developer.fnnas.com/docs/core-concepts/privilege/)；暂不承担 WebSocket、流式传输或长请求。

- 必须 `CGO_ENABLED=0`。`syscall.AllThreadsSyscall6(PR_SET_NO_NEW_PRIVS)` 同步所有 Go 线程，再逐线程核验四类 UID/GID、附加组及四类 capabilities；未知、非零或漂移都拒绝。后续线程继承该不可撤销标记。cgo 构建返回不支持并拒绝；单次普通 prctl 不视为进程级证明。
- 启动配置必须位于 root-owned、不可包用户写入的元数据树。逐级 FD / openat2 解析，拒绝可写祖先、符号链接、跨挂载或不明类型。配置只含固定入口、包身份和核心 IP，不含 NAS 凭据、源路径、数据库路径或 Docker 权限。
- 强制 HTTPS CGI 环境、精确 Host/Origin、POST Origin 与只读端点白名单。不透传 NAS Cookie、Authorization、X-Trim、Forwarded 或任意客户端目标。独立 NDG cookie 限定实例前缀、Secure、HttpOnly、SameSite=Strict；核心仍独立校验 NDG 认证及 CSRF。
- 核心固定 IPv4 私有地址和 8080 端口，不使用环境代理或跟随重定向。请求体 4 KiB、响应 256 KiB、核心调用 12 秒；整个 CGI 20 秒硬进程截止覆盖停顿 stdin。拒绝／错误只返回固定分类，不输出 backend 原始错误。
- 原生包生命周期不得以 root 消费 target 内 `.env`、Compose、脚本或动态配置；原生 CGI 包本身无 Docker resource。核心 Compose 必须另在 NDG 专属受保护目录运行，且不得自动发现包用户可写配置。
- NAS 原生应用共享管理站点 Origin，存在平台应用互信边界；不能把 Cookie Path 或包 UID 宣称为不同 Web Origin 的隔离。内部 Docker 网络同样不证明对宿主服务无访问能力。当前候选没有取得完整主机沙箱认证。

## 验证分层与待办

当前代码单测、前端状态回归和构建均不替代实际设备验收。独立代码审查没有发现 P0/P1，发现入站时间上限 P2；已增加全 CGI 进程截止，修复复审另记。`fnOS CGI candidate` 在 AMD64/ARM64 原生 Linux 上执行全线程 NoNewPrivs 测试、Web 回归并生成带源码 SHA／摘要的候选二进制，**不发布**。

仍待：精确候选审查、真实 Linux 线程测试、fnOS HTTPS CGI 参数、固定核心私网连接、真实 UI 登录／扫描／断点／重建、匿名拒绝、Cookie/CSRF、资源占用、安装升级卸载及现有服务前后保护对照。手机实体浏览器、NAS 整机重启和全部 Quarantine/Restore/Purge 均未通过；不得标成 PASS。Linux Worker 的路径绑定、竞态及执行恢复仍须单独关闭；#111 继续 OPEN/BLOCKED。
