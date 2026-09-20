# OpenWrt / ImmortalWrt 部署说明

本文说明如何在 **新版 OpenWrt 或 ImmortalWrt** 上，以“手动放置文件”的方式运行 openvpn-pannel。

---

## 1. 适用环境与依赖

- 系统：**新版 OpenWrt** 或 **ImmortalWrt**。
- 防火墙：可以使用 **fw4 + nftables**，已实测兼容。
- 需要保证系统内存在以下命令：

  | 命令 | 用途 | 缺失时的行为 |
  | --- | --- | --- |
  | `bash` | 资源脚本使用 bash 语法 | 脚本无法运行 |
  | `ipset` | ACL 聚合放行 | 自动回落到逐条 `iptables` 规则（功能可用、性能较差） |
  | `tc` | 带宽限速 | 记录日志并自动跳过限速 |
  | `iptables` | 下发 ACL 规则 | 无法放行 |
  | `flock` | 上下线脚本串行化 | 并发时可能相互覆盖规则 |

安装依赖：

```sh
opkg update
opkg install bash ipset tc iptables flock
```

> 旧版 **fw3 + iptables 未经测试**，行为可能与本文不同。

---

## 2. 目录约定

设程序部署在 `/opt/openvpn-pannel`：

```
/opt/openvpn-pannel/
├── openvpn-pannel            # Go 二进制
├── config.json               # 配置文件
└── workdir/                  # 运行目录（证书 / 日志 / socket，由 working_dir 指定）
/etc/init.d/openvpn-pannel    # procd 启动脚本
```

默认 `config.json` 的关键项：

- `listen`: `0.0.0.0:8081`（Web 面板端口）
- `sqlite_db`: SQLite 数据库路径
- `working_dir`: 运行时目录
- `internal_api_listen`: `127.0.0.1:59003`（内部 API，供脚本调用）

---

## 3. 部署步骤

### 3.1 放置程序与配置

把二进制 `openvpn-pannel` 与 `config.json` 放进 `/opt/openvpn-pannel/`，并确保 `working_dir` 指向的目录存在且可写（建议放在 /tmp 或独立可写分区，不要放进只读的 squashfs，也不要放在闪存分区里，避免日志写坏闪存）。

### 3.2 安装启动脚本

本目录下的 **`openvpn-pannel`** 就是启动脚本（procd），复制到 `/etc/init.d/`：

```sh
cp openvpn-pannel /etc/init.d/openvpn-pannel
chmod +x /etc/init.d/openvpn-pannel
/etc/init.d/openvpn-pannel enable
```

它托管运行的命令是：

```
/opt/openvpn-pannel/openvpn-pannel -config=/opt/openvpn-pannel/config.json run
```

若你的实际路径不同，请修改脚本中的该行。

脚本已设置 `term_timeout 60` 与 `pidfile`，并在 `stop_service()` 中主动发 `SIGINT` 并等待面板退出，确保停止服务时能把所有 openvpn 实例一起关掉（详见 6. 常见问题）。

首次部署需初始化数据库：

```sh
/opt/openvpn-pannel/openvpn-pannel -config=/opt/openvpn-pannel/config.json migratedb
```

### 3.3 启动服务并打开面板

```sh
/etc/init.d/openvpn-pannel start
```

浏览器访问 `http://<路由器IP>:8081/`（端口以 `config.json` 的 `listen` 为准）。

### 3.4 更新资源文件（换成 OpenWrt 版本）

进入面板的**资源文件**管理，把默认资源替换为**本目录下的 OpenWrt 版本**：

| 资源 | 本目录文件 | 作用 |
| --- | --- | --- |
| 客户端上线脚本 | `client_online.sh` | 上线时放行 ACL（ipset）+ 设置下载限速 |
| 客户端下线脚本 | `client_offline.sh` | 下线时回收 ACL 与限速 |
| 达量限速更新脚本 | `ratelimit.sh` | 达量限速命中或解除时，对变化的客户端重设/移除 tc 限速 |
| 服务端启动脚本 | `server_start.sh` | 初始化 ACL 链 / ipset |
| 服务端退出脚本 | `server_exit.sh` | 回收 ACL 链 / ipset |
| 杂项配置 | `misc` | 路径与文件名（`openvpn_path`、`shell_path` 等） |

其余资源（`auth.sh`、OpenVPN `config` 模板、`client-config` 模板、`help`）保持默认即可。

> - **`ratelimit.sh` 是达量限速的运行时脚本**：面板每隔一段时间检查在线客户端的生效限速，只有当某个客户端的限速发生变化时，才把变化的客户端通过标准输入交给该脚本重设 tc。首次上线时的限速仍由 `client_online.sh` 设置。
> - **`openvpn-pannel` 不是资源文件**，它是系统启动脚本，放到 `/etc/init.d/`（见 3.2）。
> - 资源文件中形如 `__INTERNAL_API__`、`__WORKING_DIR__`、`__SERVER_ID__`、`__SERVER_INTERFACE__` 的占位符，由面板在保存/下发时自动填充，**请勿手动修改**。
> - OpenWrt 版资源与通用版的主要差别：直接调用 `/usr/sbin/iptables`（或 `/sbin/tc`）等绝对路径、不依赖 `sudo`、用 `/sys/class/net/<iface>` 判断接口是否存在、`openvpn_path=/usr/sbin/openvpn`、`shell_path=/bin/bash`，且 tc 限速与 ACL 操作放入 `flock` 临界区串行化。

### 3.5 新建防火墙区域

在「网络 → 防火墙 → 区域」中新建区域 **`openvpn`**：

- **入站（Input）**：
  - 不希望 OpenVPN 客户端访问路由器本机 → 选 **丢弃** 或 **拒绝**；
  - 希望客户端能访问路由器本机 → 选 **允许**。
- **转发（Forward）**：选 **拒绝**。
- **出站（Output）**：选 **拒绝**。
- **转发区域**: 不需要选任何区域转发到此区域，也不需要选择此区域能转发到任何区域。

### 3.6 新建接口

在「网络 → 接口」中新建接口 **`openvpnxxxx`**（名字可自定）：

- **协议**：**不配置协议**（unmanaged）。
- **设备**：选择 **openvpn 进程刷出来的那个接口**（如 `ovpns0`、`tun0` 之类，此接口名称是你在面板中设置的openvpn接口名称，可用 `ip link` 或接口列表确认）。
- 归属防火墙区域：选上面新建的 **`openvpn`**。

点击「应用」后，你会发现该 openvpn 接口的地址“没了”——**这是正常现象**（UCI 接管接口后不再由 netifd 配置地址）。

此时**重启一次服务器进程**，地址即会恢复：

- 面板里重启对应的服务器，或
- ```sh
  /etc/init.d/openvpn-pannel reload
  ```

重启后即完全正常工作。

> 为什么要建这个接口：OpenWrt/fw4 需要有对应的 UCI 接口记录，才能把 openvpn 的动态接口绑定到 `openvpn` 防火墙区域。选「不配置协议」表示不让 OpenWrt 管理它的地址，地址仍由 openvpn 进程自行配置。

### 3.7 完成

至此，客户端即可连接，并按面板中的用户 / 用户组 / ACL 规则进行放行与限速。

---

## 4. 工作原理：面板规则与 fw4 (nftables) 的匹配顺序

在 **fw4 + nftables** 模式下，数据包会 **优先匹配面板脚本下发的 `iptables` 规则**（ACL 放行、限速等），之后才匹配由 **fw4 维护的 `nft` 规则**。

因此：

- 客户端能访问哪些目标、是否限速，由**面板的 ACL** 决定。
- fw4 区域的入站/转发/出站策略主要决定默认策略与是否放行到路由器本机（见 3.5）。

> 旧的 **fw3 + iptables** 未测试。

---

## 5. 本目录文件清单

| 文件 | 放置位置 | 说明 |
| --- | --- | --- |
| `openvpn-pannel` | `/etc/init.d/openvpn-pannel` | procd 启动脚本 |
| `client_online.sh` | 面板 → 资源文件 | 上线：ACL 放行 + 限速（`flock` 串行化） |
| `client_offline.sh` | 面板 → 资源文件 | 下线：回收 ACL + 限速（`flock` 串行化） |
| `ratelimit.sh` | 面板 → 资源文件 | 达量限速变化时重设 / 移除 tc 限速（`flock` 串行化） |
| `server_start.sh` | 面板 → 资源文件 | 服务端启动：初始化 ACL 链 / ipset |
| `server_exit.sh` | 面板 → 资源文件 | 服务端退出：回收 ACL 链 / ipset |
| `misc` | 面板 → 资源文件 | 路径 / 文件名配置 |

---

## 6. 常见问题

- **限速不生效**：确认已安装 `tc`，且内核包含 `sch_htb`、`cls_u32`、`act_police`、`sch_ingress`；并在面板「资源文件」中把 `ratelimit.sh` 替换为本目录的 OpenWrt 版本。缺失时脚本会记录日志并跳过限速，不影响连接；达量限速在运行中变化时依赖 `ratelimit.sh` 重设 tc。
- **ACL 未生效**：确认已安装 `ipset`。缺失时脚本会回落到逐条 `iptables` 模式（功能仍可用）。
- **应用接口后地址消失**：属正常现象，见 3.6，重启服务器进程即可恢复。
- **修改了资源脚本**：需在面板重新保存并重启服务，使新脚本生效。
- **改了 `config.json`**：同样需要重启服务才生效。
- **停止面板后 openvpn 进程残留**：
  - 请使用 `/etc/init.d/openvpn-pannel stop` 正常停止。procd 默认发 `SIGTERM`，面板会据此优雅关闭所有 openvpn 实例（每个最多等 8 秒，`term_timeout` 给了 60 秒，见 3.2）。
  - **不要用 `kill -9` 直接杀面板进程**，否则面板来不及关闭子进程，openvpn 会残留为孤儿进程，需要手动 `kill` 或重启设备。
  - 若使用旧版面板（只处理 `SIGINT` 不处理 `SIGTERM`），升级到本目录脚本并重建二进制即可；本目录的启动脚本同时补发 `SIGINT`，对旧二进制也有效。
