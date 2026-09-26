# OpenVPN 面板

一个自托管的 OpenVPN 管理面板：统一管理 **用户**、**用户组** 与 **OpenVPN 服务器**，
提供访问控制、带宽限速、证书管理与审计日志。单二进制、前端内嵌，开箱即用。

## 功能特性

**用户与权限**
- 用户 / 用户组管理：批量创建（CSV 导入）、搜索分页、备注
- 精细访问控制（ACL）：用 CIDR 声明可访问网段（IPv4 / IPv6），客户端上线自动下发、下线自动回收；
  安装 `ipset` 时聚合下发，规则数不随在线人数增长
- **IPv6 支持**：ACL、带宽限速、流量统计与客户端自助页面均支持 IPv6（iptables 与 nftables 两套脚本
  均已支持，含 OpenWrt）；是否开启服务器 IPv6 建议按业务实际情况决定（详见面板内帮助）

**服务器管理**
- 多 OpenVPN 实例统一管理：端口、协议、网段、路由、客户端配置集中维护，支持开机自启动
- 进程保活看门狗：实例异常退出自动拉起，并记录“服务器已恢复”事件

**带宽与流量**
- 带宽限速：区分上传/下载，支持“活跃用户组最低/最高速率”与固定速率，可配各用户/用户组
- 达量限速：按周期统计流量，超出阈值自动降速，超额禁止连接并踢下线
- 流量统计：实时查看在线客户端及其已用流量

**证书**
- 内置 CA 与证书管理：生成 / 签发 / 导入 / 引用，支持 RSA 与 ECDSA，无需 openssl
- 一键导出客户端 `.ovpn`（内联证书密钥），服务器地址与端口按实例记忆

**认证与安全**
- 双因素认证（TOTP）：扫码或手动绑定，登录与动态验证码均有频率限制
- 密码使用 bcrypt 存储，历史密码登录时自动升级；支持可信反向代理白名单
- 客户端自助页面：查看连接信息、输入动态验证码放行网络

**审计与运维**
- 事件审计：客户端上下线、认证成功/失败、ACL 变更、证书操作，并记录操作人
- 服务器日志按容量自动轮换
- 高可用：面板重启或崩溃时可不中断已连接的客户端（可选）
- 单个二进制、前端内嵌，支持 MySQL / SQLite

## 优势

- **开箱即用**：单个二进制 + 一份配置文件即可运行，前端资源已内嵌，无需单独部署 Web 服务；
  默认使用 SQLite，零外部数据库依赖。
- **脚本可灵活配置**：认证、上下线、ACL、限速等逻辑全部由脚本资源驱动，可在面板内在线查看与替换；
  仓库同时提供 `iptables` 与 `nftables` 两套脚本，按系统自由切换。
- **运行环境要求低**：纯 Go 实现，证书由标准库生成，**无需 openssl**；可运行于 x86 / ARM，
  兼容 OpenWrt、ImmortalWrt 等嵌入式系统。
- **占用资源少**：单进程、内存占用低，适合路由器、VPS 等资源受限的环境。
- **企业级防火墙体验**：用户/用户组级别的允许与拒绝策略、CIDR 粒度 ACL、实时限速与达量限速、
  连接跟踪清理，配合完整的事件审计，管控粒度接近专业防火墙。

## 依赖

- **必需**：`openvpn`、`iptables`（或 `nftables`，见下）、`bash`（内置脚本使用 bash 语法，
  由配置项 `shell_path` 指定，默认 `/bin/bash`；请勿改为 `sh`/`dash`，否则认证与 ACL 脚本会失效）。
- **建议**：`ipset`（聚合 ACL，规则数与在线人数无关，性能更好）、`tc`（带宽限速）、
  `conntrack`（启用 MFA 时登出后立即清理连接跟踪，缺失则自动跳过）。
- **下载限速（tc 入方向）依赖内核模块**：优先使用 `ifb` + `act_mirred`（推荐，OpenWrt 常见），
  不可用时回退 `act_police`；两者都缺失时会记录明确 WARNING 并跳过下载限速（上传限速不受影响）。
- **数据库**：SQLite（默认，需 CGO）或 MySQL。
- 证书由 Go 标准库生成，**无需 openssl**。

> 面板默认使用 `iptables` / `ipset` 脚本。若系统已不再提供 `iptables`（如较新的发行版或 OpenWrt），
> 请在资源管理中使用nftables脚本。

## 快速开始

```bash
# 1) 生成示例配置并修改
./openvpn-pannel democonfig > config.json

# 2) 初始化数据库（首次部署或升级后执行）
./openvpn-pannel -config=config.json migratedb

# 3) 创建管理员并加入 admin 组
./openvpn-pannel -config=config.json createuser admin 'ADMIN_PASSWORD' 管理员
./openvpn-pannel -config=config.json creategroup admin
./openvpn-pannel -config=config.json addusertogroup admin admin

# 4) 启动面板
./openvpn-pannel -config=config.json run
```

浏览器访问 `http://<主机>:8081`，使用 `admin` 登录。之后在“服务器管理”中配置
服务器证书与参数即可。

> 其他命令：`run` / `migratedb` / `createuser` / `creategroup` / `addusertogroup` / `democonfig` / `version`。

### 开始使用

1. 创建vpn用户
2. 创建用户组，并给用户组设置ACL（vpn用户可以访问的网段）
3. 将vpn用户加入用户组内
4. 创建证书。依次创建CA证书，服务器证书，客户端证书。
5. 创建VPN实例，选择刚才创建的证书，填写配置。给VPN实例加入用户组授权。
6. 导出客户端配置文件，即可导入客户端连接到服务器。 

## 配置

`config.json` 的最小示例（完整字段说明见面板内 **帮助信息**）：

```json
{
  "listen": ":8081",
  "sqlite_db": "/opt/openvpn-pannel/data.db",
  "passwd_salt": "CHANGE_ME_random_string",
  "session_secret": "CHANGE_ME_32_byte_random_secret",
  "working_dir": "/opt/openvpn-pannel/workdir",
  "internal_api_listen": "127.0.0.1:59003"
}
```

使用 MySQL 时填写 `mysql_addr` / `mysql_user` / `mysql_pass` / `mysql_db` 并留空 `sqlite_db`。

## 部署

以非 root 运行时，先给 OpenVPN 授予网络管理能力：

```bash
sudo setcap cap_net_admin+ep /usr/sbin/openvpn
```

并在 `sudoers`（如 `/etc/sudoers.d/openvpn-pannel`）放行面板所需的网络命令：

```sudoers
username ALL=(ALL) NOPASSWD: /usr/sbin/iptables
username ALL=(ALL) NOPASSWD: /usr/sbin/ip6tables
username ALL=(ALL) NOPASSWD: /usr/sbin/ipset
username ALL=(ALL) NOPASSWD: /usr/sbin/tc
```

systemd 单元（`/etc/systemd/system/openvpn-pannel.service`）：

```ini
[Unit]
Description=openvpn-pannel
After=network.target nss-lookup.target

[Service]
User=openvpn
AmbientCapabilities=CAP_NET_ADMIN CAP_NET_BIND_SERVICE
CapabilityBoundingSet=CAP_IPC_LOCK CAP_NET_ADMIN CAP_NET_BIND_SERVICE CAP_NET_RAW CAP_SETGID CAP_SETUID CAP_SYS_CHROOT CAP_DAC_OVERRIDE CAP_AUDIT_WRITE
WorkingDirectory=/opt/openvpn-pannel
ExecStart=/opt/openvpn-pannel/openvpn-pannel -config /opt/openvpn-pannel/config.json run
Restart=on-failure
RestartPreventExitStatus=23
ReadWritePaths=/opt/openvpn-pannel

[Install]
WantedBy=multi-user.target
```

> 若希望面板重启/崩溃时不中断已连接的客户端，把配置项 `stop_instances_on_exit` 设为 `false`，
> 并给上面的 systemd 单元加上 `KillMode=process`（否则服务重启会连带杀掉同 cgroup 内的 OpenVPN 进程）。

需要在 **OpenWrt / ImmortalWrt** 部署时，请查看 [`doc-openwrt/README.md`](doc-openwrt/README.md)。

## 从源码构建

需要 Go、Node.js 与 `go-bindata`（前端资源会被内嵌进二进制）：

```bash
cd front && npm install && npm run build && cd ..
go-bindata -o internal/assets/assets.go -pkg assets -prefix "front/dist" front/dist/...
go-bindata -o internal/ovpn_server/resource_sets_assets.go -pkg ovpnserver \
  -prefix "internal/ovpn_server/resource_sets" internal/ovpn_server/resource_sets/...
bash scripts/rename_resource_set_assets.sh internal/ovpn_server/resource_sets_assets.go
go build -ldflags "-X 'main.buildDate=$(date '+%Y-%m-%d %H:%M:%S')'" ./cmd/openvpn-pannel/
```

> 内置资源已改为**资源集（resource set）**机制：`internal/ovpn_server/resource_sets/` 下预置 4 套
> （`linux-iptables`、`linux-nftables`、`openwrt-iptables`、`openwrt-nftables`），每套包含全部 13 个
> 资源文件（脚本、配置模板、帮助文档与客户端自助页面 `client-page.html`）。面板「资源管理」页可
> 查看/编辑每套资源并切换当前启用的资源集（切换后需重启服务器进程生效）。也可以直接运行
> `make assets` 重新生成内嵌资源。

## 文档

部署细节、证书管理、脚本与变量、限速计算示例、常见问题等，请查看面板内的 **帮助信息** 页面。
