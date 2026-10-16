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

## 2026-10-16 真实设备补充：入口仍然 BLOCKED

候选源码 `f6e4097bcf9eca3459c9ab57d05716a86fbfb8d3` 已完成独立复审，未发现未关闭的 P0/P1/P2。该 SHA 的全部 11 项检查通过；外部链接检查按工作流条件跳过。原生 AMD64/ARM64 全线程测试及二进制构建见 [候选运行 38762218870](https://github.com/FNB2026/nas-data-governance/actions/runs/38762218870)。这些是代码／CI 证据，不是 fnOS 浏览器成功证据。

实际安装的 AMD64 CGI SHA-256 为 `8150e5c2638bb35dda0fa3fedd5445310928a0c2670d48ab57d99c6f1ef9ab42`。包独立于只读核心，无 Docker resource。启动配置及祖先为 root-owned、不可包用户写入。原生包首次安装成功；开发包 0.0.2 卸载后重新安装成功，二进制保持同一摘要。桌面入口改为相对 CGI 路径后，实际单击可进入正确 URL，随后仍返回固定 `request_rejected`。

真实 HTTPS 请求的有限诊断确认：CGI 环境没有 `HTTPS`、`REQUEST_SCHEME`、`X-Forwarded-Proto` 或 `X-Forwarded-Host`；`HTTP_HOST` 也不等于原始 HTTPS 入口 Host，`SERVER_PORT` 不是入口 HTTPS 端口。只读取了传输字段名称、枚举及布尔比较，未输出 NAS Cookie、凭据或全部环境内容。临时诊断文件已恢复为原始身份探针。**当前平台 CGI 转发不能满足候选要求的 HTTPS／Host 证明；拒绝是预期保护，不能删除门禁或把客户端自报 Header 当成可信证明。** 未修改 NAS Nginx、证书、SSH、Docker daemon、路由或防火墙。

另一次 `install-fpk` 对已有身份探针报告 installed，但安装后的 manifest 和文件摘要仍为旧值，实际未升级；保留为升级失败，不以 CLI 成功文字判定 PASS。0.0.2 的成功证据只覆盖卸载／重新安装，不覆盖原位升级。

### 独立只读核心的宿主侧验收

仅在新建 NDG 专属受保护目录、内部 Docker 网络和 10 个合成源文件上运行。容器无宿主端口映射，以 UID/GID 10001 运行，rootfs 只读、capabilities 全部移除、NoNewPrivs 开启，配置与两个源只读，只有独立状态目录可写。镜像原样使用已公开 Preview.1：

- OCI index：`sha256:49dada73f1eb9cc74485ed5d0b92023d7cb4c3df462d03e105f00158b91668e1`。
- 实际 AMD64 config：`sha256:2c38ca9493ccebc74cdf0cc84fb23e8a4c59babb1660d4892a2f9a50c12a96d4`；API 返回源码 `1db1e4ab9a1287d3f7f9b801b471b00bfc7c7d1e`。
- 源 A：7 个文件、7 quick／6 full、3 组；源 B：3 个文件、3 quick／2 full、1 组；均 completed，missing／unavailable 为零。
- 停止／启动容器后，两源快照保留；删除容器后状态目录完整保留，再创建容器后两源快照一致。
- 停止核心后复制独立状态目录，验证文件摘要；保留原状态目录，再从备份恢复，新进程读取的两源快照一致。没有复制正在写入的 SQLite。
- 前后 10 个源文件 SHA-256 全部相同。补查重建后的当前容器 stdout/stderr，二者均为零字节，未出现本次凭据、合成文件名或源路径。原生命周期脚本仅检查 stdout，已删除的初始容器日志未保留；这不认证完整生命周期的日志隐私门禁。
- 核心预检验证匿名状态请求 401、缺失 CSRF 403、Quarantine/Restore/Purge 403、危险能力关闭；NDG Cookie 具有 Secure／HttpOnly／SameSite=Strict。这些请求由 NAS 宿主直接访问内部核心，**没有经过浏览器 CGI 链路**。
- 前后原有 15 个容器的 ID、镜像、状态、启动时间、重启次数／策略、网络及挂载摘要逐项相同；仅本次 NDG 核心容器被重启／删除／重建。这不是全 NAS 文件系统审计。

仍待：可证明 TLS／Host 的客户端入口、真实 UI 登录／扫描／断点／键盘交互、浏览器 Cookie/CSRF 及隐私／资源占用、原位升级。手机实体浏览器、NAS 整机重启和全部 Quarantine/Restore/Purge 均未通过；不得标成 PASS。Linux Worker 的路径绑定、竞态及执行恢复仍须单独关闭。

在线刷新发现 [#111](https://github.com/FNB2026/nas-data-governance/issues/111) 已由 FNB2026 在 `2026-10-15T14:24:11Z` 关闭，未关联解决该证明缺口的新提交。保留所有者当前的 Issue 状态；当前源码仍拒绝 Darwin 新治理意图，安全能力保持 **BLOCKED**。Issue 关闭不等于正向核验 PASS。公开 Preview.1 和 Mac beta.11 均未变更。

## 2026-10-16 已授权的 Nginx 元数据实验：安全停止

所有者单独授权仅在既有 CGI 配置追加 NDG 路径白名单和服务端协议／Host 标记；要求原位语法核验后才平滑 reload，失败按摘要回滚。控制脚本及临时元数据探针经过独立静态复审，未发现该范围未关闭的 P0/P1/P2；这不是实际运行 PASS。

真实尝试在**候选配置写入前停止**。原生基线 `nginx -t` 返回成功；同期厂商文件监控报告受管理 Web 文件变化并请求重启 `trim_nginx`，原 master PID 改变，原 CGI 配置文件短暂消失。替换前的预期摘要检查返回 FileNotFoundError，因而没有写入候选。观察证明同期发生配置再生成／服务重启，尚不能证明厂商内部的确切触发原因。未设置厂商开发模式、关闭监控或修改厂商二进制。

临时 CGI 探针已恢复为上述原 Go 摘要。首次恢复记录因配置暂时缺失保留为 `recovery_incomplete`；随后独立只读核验确认原配置 SHA-256 为 `47e412618123ad51af9e46a7e18a0da340bd90951b3c84f21826f5d14a2774ef`，原配置及 Go 文件的 UID/GID/mode 与备份一致，HTTPS 健康，原有 15 个容器及 8 个网络逐项未变。实际 Chrome 刷新后管理桌面可用。未向新 master 发信号；**master 连续性未通过**。

本次没有执行候选原位语法核验、代理 reload 或 12 项传输矩阵；不能把它们标成 PASS。NDG 产品仍返回安全拒绝。当前配置内容保持原值，不自动重试或覆盖厂商再生成文件。需要可持续的应用自身入口机制后才能实施可信代理适配器，fnOS 客户端支持仍为 BLOCKED。已公开 Docker Preview.1、Mac beta.11 及其资产均未改变。

## 应用自身终止 TLS 的候选

厂商代理无法提供所需证明时，另行准备 `cmd/ndg-fnos-https`，在专用 HTTPS Origin 根路径提供同一只读 UI，转发到现有隔离核心。它核验真实 `Request.TLS`、Host 和 POST Origin；不从 Forwarded 或客户端自报 Header 构造 TLS 证明。CGI 的旧门禁不改变。该方案不复用 NAS 登录态：NDG 自身认证及 CSRF 仍由只读核心核验。

候选要求专用于 NDG 的 DNS 名称、匹配证书和私钥，绑定明确私有 IPv4／回环及非特权端口；禁止通配监听地址、公网 IPv4、IP Origin、无 SNI、错误 SNI、证书过期／不匹配及弱密钥。新域名还须实际验证解析与浏览器信任，且不应落入 NAS Cookie 的 Domain 范围；不同端口本身不能隔离 Cookie。不复制 NAS 私钥，不自动签发证书，不添加 DNS、端口映射或系统信任，也不提供忽略证书选项。

配置固定为 root-owned 元数据目录的 `https.json`，证书和密钥固定为同目录 `tls.crt`／`tls.key`。逐级 FD/openat2 拒绝符号链接、跨挂载及可写祖先；文件有大小界限，先检查打开的 FD 类型，FIFO 不会阻塞启动。私钥拒绝 world 权限、执行／特殊位、group 写权限和扩展访问 ACL。读取密钥的专属运行组仅应包含该应用账户。启动时仍要求全部线程的专属 UID/GID、零 capabilities 和不可撤销 NoNewPrivs；只能使用纯 Go Linux 构建。

适配器仍无源、数据库或 Docker 权限。只允许既有只读 API 白名单，64 个连接、固定 API body/response 上限、读取／写出／空闲超时，SIGTERM 有限等待后关闭。错误及 TLS 日志不包含请求、源路径或密钥。域名证书不是用户数据授权；目录选择、文件语境和危险操作仍未实现或未认证。

验证计划分层：本机 Go 测试使用新生成证书和真实 TLS socket，覆盖 Host／Origin／SNI、明文伪造 Header、危险路由、连接上限和关闭；原生 AMD64/ARM64 CI 另以专属非 root 账户运行实际候选二进制，检查 root-owned 配置／密钥、内核线程身份、真实 TLS 及五类启动拒绝。CI 脚本仅允许 GitHub-hosted 临时 runner，证书／私钥不进入发行或 CI 下载资产。这些不是实际 NAS、可信浏览器证书、核心端到端扫描或正式 FPK 生命周期验收。

原 unit 尝试结束时没有运行中的新 HTTPS 服务；下文保留当时的拒绝与回滚，随后记录修订策略的实际结果。既有 Nginx 配置不得重试覆盖。fnOS 正式支持、手机、NAS 整机重启、升级及完整治理仍未通过。

## 2026-10-16 已授权的原生 HTTPS 尝试：启动拒绝，完整回滚

所有者授权新增专用局域网 HTTPS 服务、安装专用证书和固定 `cde4c012393f6b58787aeed821101319680d7763` 候选，只切换隔离核心的 Origin；安全和实际 GUI 门禁通过前不启用开机启动。域名仍解析到私有局域网地址、Cloudflare 代理关闭。独立签发的专用证书有效期至 `2027-01-14T02:12:38Z`；没有自动续期。当前 Chrome 配置中父域没有网站数据或 Cookie，NAS IP Cookie 不匹配新 DNS Origin。这些不等于实际 NDG TLS 链路已通过。

实际 AMD64 二进制 SHA-256 为 `f73963c9e8381654654963f08f3622530dfda33dd4e05a6033b9215a3f37fbe8`，原部署 unit SHA-256 为 `1ce4d57316875e8de9b58fe7baadaafb3733e81a4df310f49550ff726cfd171d`。执行与回滚脚本独立复审发现并修复了部分安装、服务冲突、核心停止后恢复及清理并发问题，再进行受限真实尝试。**该 unit 启动退出，不是 GUI 验收 PASS。**

对自身候选进程的有界诊断定位到：全部线程 NoNewPrivs 设置成功后，第一个 `openat2` 返回 `ENOSYS`，尚未读取配置内容或创建监听。普通降权进程及无监听阶段探针在同一主机可以完成这些读取。systemd 252 的 [seccomp 实现](https://raw.githubusercontent.com/systemd/systemd/v252/src/shared/seccomp-util.c) 明确在 `RestrictSUIDSGID` 下拒绝 `openat2`，因为无法检查其间接结构中的 mode。原生 `setpriv` CI 没有运行这项 systemd 策略，不能替代服务沙箱验收。

每次尝试都完成守卫回滚：新增文件和监听移除，原 Origin 恢复、核心健康，10 个源文件 SHA-256 不变，原有 15 个容器和 8 个网络基线不变。没有启用开机启动、重启 NAS、修改厂商 Nginx 或开放治理写入。私有完整回执保留，诊断导出不含密钥或源内容；特权会话已退出。真实 NDG 浏览器登录、扫描、退出和重启验收均为 **NOT_RUN**。

### 替代沙箱仅进入封闭证明阶段

[候选 unit](../../packaging/fnos/readonly-https-candidate.service) 不是安装建议或已认证替代物。它研究用不可写的普通文件系统、独立用户 namespace、禁用运行中 FD 接收及匿名文件／namespace 入口来替代不兼容的 SUID 创建过滤；不允许回退到普通路径读取，也不裸删保护后标记成功。

独立审查指出两个必须真实覆盖的反例：同凭据 namespace 外进程的 `/proc/root/cwd/fd` 路径，以及 AF_UNIX／SCM_RIGHTS 传入的可写文件或 memfd。新的 [一次性 systemd 证明脚本](../../scripts/research/fnos-https-systemd-proof.py) 先要求这些反例实际修改人工对象，再验证新精确 profile 的拒绝、字节／模式／inode 守恒、真实线程身份、冻结 `cde` 二进制的 TLS 以及同设备同 inode 自绑定挂载拒绝。脚本只能在一次性 GitHub-hosted Linux 运行，明确拒绝 fnOS；它不会认证 NAS 或浏览器。

该证明在精确 HEAD `daf92497d6439795cef5bf06a3d44df2227f660d` 的 [原生运行 38877496884](https://github.com/FNB2026/nas-data-governance/actions/runs/38877496884) 双架构通过；26 份下载文件的摘要、journal 和回执已经独立审定。两端都先建立真实反例，再得到修订策略的 EACCES／EROFS／EPERM 拒绝和对象守恒；冻结二进制实际 TLS／线程身份、同设备同 inode 挂载拒绝及完整清理均通过。意外继承的只读 regular／memfd 也实际拒绝，失败日志独立于 fixture 保留。通过范围仅为 **GitHub-hosted native systemd proof**，不是 NAS、浏览器或发行验收。

原生证明完成当时尚未部署新策略；后续在所有者已授权的相同局域网、冻结二进制及自有隔离核心范围内继续兼容修复和现场验收，没有扩大永久服务、网络入口、数据访问或治理写入。公开 Docker Preview.1、Mac beta.11 和历史 RED 证据保持不变。

## 2026-10-16 修订策略实机与 Chrome：有限只读验收通过

现场保持原 `cde4c01` AMD64 二进制和专用证书不变，unit SHA-256 为 `fc4a6b5b0df855d375fcccaeb4ac6af1a68ef29d252fb22e92b11929f16a00d2`。r5 因 stderr 非空、r6 因诊断差异在静态核验阶段停止，均完整回滚，未运行探针或切换核心。核对原文后确认：无关系统 target 的 44 行厂商诊断之外，候选 Docker 依赖展开新增了 rc-local 的旧选项及 docker.service 可执行位两行厂商诊断；它们并非基线相同行的重复。原误述及更正保留。r7 同时固定两份完整 stderr 摘要并要求退出码零，任何变化拒绝；没有忽略 NDG 的未知属性／系统调用或修改厂商文件。

实际 fnOS systemd 252 下先运行无 TCP 监听的短命自有探针，仅持有新人工对象。普通 owner 的真实修改基线成功，精确策略随后拒绝 `/proc/root/cwd/fd` 路径、SCM_RIGHTS 入口、文件创建／截断／改权限、memfd、io_uring、ptrace 及新 namespace；字节／模式／inode 守恒，探针完整清理后才允许切换核心 Origin。实际网关全部线程以专属 UID/GID、零 capabilities、NoNewPrivs、seccomp 和独立用户 namespace 运行，根、临时目录及私有 dev 只读，没有普通文件／目录的遗留打开 FD。

真实客户端直连验证公有证书链、名称及 HTTPS 200。认证与会话失效、缺失／错误 CSRF、Host／Origin／跨站请求、编码越界、伪造转发头、明文及危险三路由均按预期拒绝。错误 SNI 收到明确的服务端 `TLSV1_ALERT_INTERNAL_ERROR`；客户端自行拒绝错主机名不计作服务端证明。Cookie 采用应用专属名称、Host-only、Secure／HttpOnly／SameSite=Strict；Chrome 首次访问前父域数据过滤为零，NAS IP Cookie 不匹配此 DNS Origin。

实际 Chrome 显示“连接是安全的”，未绕过警告；错误登录有明确文案，正常登录后拒绝密码保存提示。来源 A 实际经历扫描中到完成，7 文件／3 组；键盘切换来源 B 并实际扫描，3 文件／1 组。完成 checkpoint 的继续请求明确拒绝，**不认证扫描中途断点恢复**。重启自身 HTTPS 服务后会话和结果仍可用；重启隔离核心后旧会话安全失效，重新登录后两源快照保留。退出并完整刷新后仍未登录。UI 继续明确只读、最多 200 组且不显示文件名／路径。

该 Mac 的既有出站代理会让此局域网域名连接失败：直连 TLS 200，经代理 CONNECT 200 后 TLS 超时，Chrome 最初显示 ERR_CONNECTION_CLOSED。所有者另行明确授权仅临时追加该域名的 Wi-Fi 代理绕过项，验收后已核对并恢复原 8 项列表。**当前原代理仍不兼容此域名，后续客户端须具备获准的局域网直连路径；没有永久修改客户端或 NAS 网络。**

前后 10 个源文件 SHA-256 相同，原有 15 个容器和 8 个网络基线未变。当前核心 stdout／stderr 及本次网关进程应用日志为零字节；不补认证已删除旧容器的完整历史日志。小人工数据下网关 memory.current 约 11 MB、memory.peak 约 13.3 MB，核心约 6.1 MiB；不代表大规模负载、长期泄漏或 GUI 延迟认证。

最低权限、证书、Cookie、认证／CSRF、真实 GUI 和守恒门禁经独立审定无剩余安全阻断后，按原授权仅对同一服务执行 enable；前后 unit／二进制身份不变，实际 active／enabled，仍仅绑定原显式局域网 IP／端口。这只证明启用配置，不代表完成 NAS 整机重启测试。

当前仅为自有隔离环境中的只读开发候选。完整安装／原位升级、扫描中途断点恢复、手机、NAS 整机重启、完整 25 项 UX、长期性能及完整治理仍未认证，证书没有自动续期。保持实验性平台状态，不重写公开 Preview.1 的原验收与支持范围，也不开放 Quarantine／Restore／Purge。一次性 frozen systemd CI 仅在此候选分支或手动运行；其他分支跳过即为 NOT_RUN，不能沿用旧 PASS，也不能替代后续 unit／loader 变化所需的新证明。

## 2026-10-16 Compose 生命周期补充

后续已在同一人工安装完成实际容器删除重建、完整冷备份恢复到新的空 state 目录、
再回原状态目录运行。两个源快照及源 SHA 守恒，原容器/网络基线未变，入口进程不变；
重建后再次完整 wire 矩阵通过。首版测试误用跨对象 docker inspect 命中同名网络的
保护性 FAIL 与守卫恢复保留，改为 docker container inspect 后重新完整实测。
新准备工具和各层未认证范围见[独立记录](FNOS-READONLY-COMPOSE-2026-10-16.md)。
这不认证通用安装、原位升级、入口卸载、NAS整机重启或治理写入。
