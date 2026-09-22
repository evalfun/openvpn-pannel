# OpenVPN 面板

一个自托管的 OpenVPN 管理面板。统一管理 **用户**、**用户组** 与 **OpenVPN 服务器实例**，并通过
`iptables` / `ipset` 做访问控制、通过 `tc` 做带宽限速。面板负责“编排”，真正的连接与转发交给 OpenVPN。

- 用户 / 用户组 / 服务器 / 证书 / 资源 / 日志，一站式 Web 管理
- 达量限速：按周期统计流量，超出阈值自动降速，超额禁止连接并踢下线（防止滥用）
- 基于用户组与用户权限的精细化访问控制（ACL）
- 客户端上线自动下发 ACL（优先 `ipset` 聚合，缺失时回落逐条 `iptables`）
- 客户端上线自动 `tc` 限速，支持按用户组或固定带宽，区分上传/下载
- 内置证书管理：用 Go 生成/签发 CA、服务器与客户端证书，支持导入与引用
- 一键导出客户端 `.ovpn`（内联证书密钥），服务器地址与端口按实例记忆
- 服务器日志按容量自动轮换，内置事件审计
- 单个二进制，前端资源内嵌，支持 MySQL 或 SQLite

## 工作方式

```
            ┌────────────┐   内部 API(仅本机)     ┌──────────────────┐
  浏览器 ──▶│  面板 Web  │ ◀───────────────────▶│   认证/上下线脚本 │
            └─────┬──────┘                       └──────┬───────────┘
                  │ 管理/配置                            │
                  ▼                                     ▼
            ┌────────────┐                ┌──────────────────────────────┐
            │  MySQL/    │                │ OpenVPN 实例 + 工作目录       │
            │  SQLite    │                │ iptables/ipset nftables · tc │
            └────────────┘                └──────────────────────────────┘
```

- 内部 API 只监听 `127.0.0.1`，供 OpenVPN 的认证、上线、下线脚本回调查询权限、ACL 与限速。
- 每个服务器实例有独立工作目录 `working_dir/<服务器ID>/`（证书、`server.conf`、日志、脚本）。

## 快速开始

```bash
# 1) 生成一份示例配置
./openvpn-pannel democonfig > config.json

# 2) 修改config.json配置文件

# 3) 初始化数据库（首次部署、或升级版本后执行）
./openvpn-pannel -config=config.json migratedb

# 4) 创建管理员用户：  用户名  密码  描述
./openvpn-pannel -config=config.json createuser admin 'ADMIN_PASSWORD_111222333' 管理员

# 5) 创建 admin 用户组（名称必须为 admin）
./openvpn-pannel -config=config.json creategroup admin

# 6) 把管理员加入 admin 组
./openvpn-pannel -config=config.json addusertogroup admin admin

# 7) 启动面板
./openvpn-pannel -config=config.json run
```

浏览器访问 `http://<主机>:8081`，使用 `admin` 登录。服务器实例的证书与参数在“服务器管理”中配置。

> 运行依赖：`openvpn`、`iptables`；建议安装 `ipset`（聚合访问控制，提升性能）与 `tc`（带宽限速）。

> 更多命令：`migratedb` / `run` / `createuser` / `creategroup` / `addusertogroup` / `democonfig` / `version`。

## 配置

`config.json` 常用字段：

| 字段 | 说明 |
| --- | --- |
| `listen` | 面板监听地址，如 `:8081` |
| `sqlite_db` | SQLite 文件路径；**非空时优先使用 SQLite**，忽略 MySQL |
| `mysql_addr` / `mysql_user` / `mysql_pass` / `mysql_db` | MySQL 连接信息 |
| `passwd_salt` | 密码盐，随机字符串，**设定后不要更改** |
| `session_secret` | 会话密钥，建议 32 字符随机串 |
| `working_dir` | 工作目录绝对路径 |
| `internal_api_listen` | 内部 API 地址，建议 `127.0.0.1:59003` |
| `client_page_listen` | 客户端自助页面监听地址（如 `10.8.0.1:8088`），为空则不启用 |
| `max_log_size_kb` | 单实例日志容量上限（KB），`<=0` 关闭轮换 |
| `allow_edit_resource` | 是否允许在线编辑脚本资源，**默认 false** |

```json
{
  "listen": ":8081",
  "sqlite_db": "/opt/openvpn-pannel/data.db",
  "passwd_salt": "CHANGE_ME_random_string",
  "session_secret": "CHANGE_ME_32_byte_random_secret",
  "working_dir": "/opt/openvpn-pannel/workdir",
  "internal_api_listen": "127.0.0.1:59003",
  "client_page_listen": "10.8.0.1:8088",
  "max_log_size_kb": 20480,
  "allow_edit_resource": false
}
```

## 客户端自助页面与 MFA（TOTP）

- 在 `config.json` 配置 `client_page_listen`（如 `10.8.0.1:8088`）后，面板会在该地址额外监听一个
  只读页面，仅面向已连接的 VPN 客户端（按 HTTP 来源 IP 识别，非 VPN 客户端访问返回错误）。
  页面展示：服务器名称、用户名称、证书名称、公网/内网 IP、可访问网络、已用流量、当前限速。
- 在“用户管理 → 编辑”中可为用户启用 **TOTP（MFA）**，保存后显示密钥与 `otpauth://` 链接供绑定。
- 启用后：VPN 认证通过但不下发任何 ACL；用户在自助页面输入 6 位动态验证码通过后才放行网络，
  登出时回收。无头设备可用 `curl http://<页面地址>/login/<6位验证码>` 与 `curl http://<页面地址>/logout`。
- 面板登录同样受 TOTP 保护：登录页在需要时会弹框要求补充动态验证码。

## 权限与访问控制

服务器权限分“用户权限”和“用户组权限”，各有允许/拒绝，优先级为：

```
用户拒绝 > 用户允许 > 组拒绝 > 组允许
```

未设置任何匹配规则时默认拒绝。登录后：

- **用户级允许**：下发该用户**所有**所属组的 ACL（不再看组规则）；
- **组级允许**：仅下发“服务器允许的组”与“用户所属组”的**交集** ACL；
- 只有用户权限、没有任何组时，可登录但无 ACL，无法访问受控网络。

用户组通过 ACL（`4`=IPv4 / `6`=IPv6 + CIDR，如 `10.0.0.0/24`）声明允许访问的网段，客户端上线时
自动写入防火墙，下线回收。安装 `ipset` 时采用聚合模式，规则数与在线人数无关，性能更好。

## 带宽限速

客户端上线时按策略自动下发 `tc`：**上传**（服务器→客户端，出方向）与**下载**（客户端→服务器，入方向）
分别控制，单位 KB/s，`0` 表示不限速。

用户限速策略四选一，默认为“依据活跃用户组的最低速率”：

| 策略 | 说明 |
| --- | --- |
| 不设置任何限速策略 | 完全不限速 |
| 依据活跃用户组的最低速率（默认） | 活跃组限速取最低，忽略 `0` |
| 依据活跃用户组的最高速率 | 活跃组限速取最高，任一为 `0` 则不限速 |
| 固定限速 | 使用该用户自己的上传/下载值 |

**活跃用户组** = 用户所属且被该服务器允许连接的组。用户组限速默认 `0`（不限速）。

## nftables 脚本（可选，需自行替换）

默认资源脚本基于 `iptables` / `ipset`。仓库 `nftables-scripts/` 目录额外提供了一套纯
`nftables` 实现，**不会内嵌进二进制，也不会自动生效**，仅供需要 nftables 的用户自行替换：

- `nftables-scripts/generic/`：普通 Linux（脚本内使用 `sudo`），对应二进制内嵌的默认资源；
- `nftables-scripts/openwrt/`：OpenWrt / ImmortalWrt（不使用 `sudo`、使用绝对路径），对应 `doc-openwrt/`。

替换方式：在面板「资源文件」中，把 `client_online.sh`、`client_offline.sh`、
`server_start.sh`、`server_exit.sh` 分别替换为对应目录下的同名文件后重启服务。
启用 TOTP（MFA）时，还需把 `acl_add.sh`、`acl_del.sh` 替换为对应目录下的同名文件。
`ratelimit.sh`、`misc`、`auth.sh` 等不涉及防火墙，保持原样即可。

> nftables 版本为每台服务器创建独立 `inet` 表 `openvpn_acl_<服务器ID>`，基础链
> `forward` 使用 `priority -200`（早于 fw4 的默认 `filter` 链），由面板脚本直接 `drop`
> 实现访问控制。因此在 OpenWrt 上使用时，请把 `openvpn` 防火墙区域的**转发策略设为
> 允许**，真正的放行/拒绝由面板 ACL 完成（详见 `doc-openwrt/README.md`）。

## 证书管理

面板内置证书管理，证书与私钥由 Go 标准库生成，**不需要 openssl**（适合 OpenWrt 等无 openssl 的环境）。

密钥类型支持 **RSA**（2048/3072/4096）与 **ECDSA**（P-256/P-384/P-521），创建时选择。

服务器编辑处，CA 证书 / 服务器证书 / 服务器私钥既可**直接粘贴 PEM**，也可从证书管理**选择引用**；

**客户端配置导出**：在“服务器管理”点“导出客户端配置”，即可下载内联 `<ca>/<cert>/<key>/<tls-auth>` 的单个 `.ovpn` 文件。

## 部署

以非 root 运行时，给 OpenVPN 授予网络管理能力，并在 `sudoers` 放行所需命令：

```bash
sudo setcap cap_net_admin+ep /usr/sbin/openvpn
```

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

## 从源码构建

需要 Go、Node.js 与 `go-bindata`（前端资源会被内嵌进二进制）：

```bash
# 1) 构建前端
cd front && npm install && npm run build && cd ..

# 2) 将前端与默认脚本打包进程序
go-bindata -o internal/assets/assets.go -pkg assets -prefix "front/dist" front/dist/...
go-bindata -o internal/ovpn_server/assets.go -pkg ovpnserver \
  -prefix "internal/ovpn_server/default_resource" internal/ovpn_server/default_resource/...

# 3) 编译后端
go build -ldflags "-X 'main.buildDate=$(date '+%Y-%m-%d %H:%M:%S')'" ./cmd/openvpn-pannel/

# 可选：剥离符号
strip ./openvpn-pannel
```

## 文档

完整的部署、证书管理、脚本与变量、限速计算示例、常见问题等，请查看面板内的 **帮助信息** 页面
（对应资源 `help`；客户端导出模板对应资源 `client-config`，开启 `allow_edit_resource` 后可自行修改）。

如果需要在OpenWRT下部署，请查看`doc-openwrt/README.md`