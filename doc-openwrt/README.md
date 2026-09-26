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
  | `kmod-ifb` + `kmod-sched-act-mirred` | 下载（入方向）限速（**自编译固件必选**，详见 3.4.2） | 回退 `kmod-sched-act-police`；两者都缺失则下载限速跳过并记录 WARNING |
  | `kmod-sched-act-police` | 下载限速的备用方案（**自编译固件通常没有此模块**） | 若同时无 ifb/act_mirred，则下载限速跳过并记录 WARNING |
  | `iptables` | 下发 ACL 规则 | 无法放行 |
  | `flock` | 上下线脚本串行化 | 并发时可能相互覆盖规则 |
  | `conntrack` | 登出时清空该客户端的连接跟踪（状态表） | 记录日志并跳过，已建立的连接会继续放行到超时 |

安装依赖：

```sh
opkg update
opkg install bash ipset tc iptables flock conntrack kmod-ifb kmod-sched-act-mirred
```


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
- `client_page_listen`: `0.0.0.0:8088`（客户端自助服务和MFA二次认证登录页面）


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

### 3.4 选择资源集（OpenWrt 版脚本）

进入面板的**资源管理**页，选择并启用适合你的**资源集**即可，**不再需要逐个替换文件**。

面板内置 4 套资源集，每套都包含全部资源文件（脚本、配置模板、帮助文档与客户端自助页面）：

| 资源集 | 适用系统 | 防火墙 | 说明 |
| --- | --- | --- | --- |
| `linux-iptables`（默认） | 普通 Linux 发行版 | iptables/ipset | 下载限速用 ingress police |
| `linux-nftables` | 普通 Linux 发行版 | 纯 nftables | 无需 ipset/iptables |
| `openwrt-iptables` | OpenWrt / ImmortalWrt | iptables/ipset | 下载限速优先 ifb、回退 police |
| `openwrt-nftables` | OpenWrt / ImmortalWrt | 纯 nftables | 下载限速优先 ifb、回退 police |

在**资源管理**页顶部的「资源集」区域：选择要查看的资源集 → 点「启用该资源集」即可切换。
切换后**请重启相关服务器进程**（或在面板重启对应服务器 / `/etc/init.d/openvpn-pannel reload`）使其生效。

每套资源集是一个**可编辑的工作区**：首次使用时面板会把该套内置默认内容写入数据库，之后你在
「资源管理」里对某个文件的修改只作用于当前查看的资源集，切走再切回仍保留；点「重置为内置默认值」
可恢复该文件的出厂内容。

本目录（`doc-openwrt/`）中各资源文件即 `openwrt-iptables` 资源集的来源，`nftables-scripts/openwrt/`
为 `openwrt-nftables` 资源集的来源，`nftables-scripts/generic/` 为 `linux-nftables` 资源集的来源。
各资源文件的作用一览：

| 资源 | 作用 |
| --- | --- |
| 客户端上线脚本（`client_online.sh`） | 上线时放行 ACL + 设置下载限速 |
| 客户端下线脚本（`client_offline.sh`） | 下线时回收 ACL 与限速 |
| TOTP 放行脚本（`acl_add.sh`） | 二次认证通过后动态放行 ACL（仅启用 MFA 时用到） |
| TOTP 回收脚本（`acl_del.sh`） | 登出时动态回收 ACL（仅启用 MFA 时用到） |
| 达量限速更新脚本（`ratelimit.sh`） | 达量限速命中或解除时，对变化的客户端重设/移除 tc 限速 |
| 服务端启动脚本（`server_start.sh`） | 初始化 ACL 链 / ipset |
| 服务端退出脚本（`server_exit.sh`） | 回收 ACL 链 / ipset |
| 杂项配置（`misc`） | 路径与文件名（`openvpn_path`、`shell_path` 等） |
| 客户端自助页面（`client-page.html`） | 客户端自助服务页面的 HTML，可自行修改界面 |

> - **`ratelimit.sh` 是达量限速的运行时脚本**：面板每隔一段时间检查在线客户端的生效限速，只有当某个客户端的限速发生变化时，才把变化的客户端通过标准输入交给该脚本重设 tc。首次上线时的限速仍由 `client_online.sh` 设置。
> - **`openvpn-pannel` 不是资源文件**，它是系统启动脚本，放到 `/etc/init.d/`（见 3.2）。
> - 资源文件中形如 `__INTERNAL_API__`、`__WORKING_DIR__`、`__SERVER_ID__`、`__SERVER_INTERFACE__` 的占位符，由面板在保存/下发时自动填充，**请勿手动修改**。
> - OpenWrt 版资源与通用版的主要差别：直接调用 `/usr/sbin/iptables`（或 `/sbin/tc`）等绝对路径、不依赖 `sudo`、用 `/sys/class/net/<iface>` 判断接口是否存在、`openvpn_path=/usr/sbin/openvpn`、`shell_path=/bin/bash`，且 tc 限速与 ACL 操作放入 `flock` 临界区串行化。
> - 资源编辑/切换默认受 `config.json` 的 `allow_edit_resource` 控制，需设为 `true` 才可修改。

> **nftables 版本**：启用 `openwrt-nftables`（或普通 Linux 用 `linux-nftables`）后，请把 3.5 中
> `openvpn` 区域的**转发（Forward）策略设为允许**：面板会在 fw4 之前（`priority -200`）用自身规则
> 直接 `drop` 未放行的流量，从而实施真正的访问控制；若仍设为拒绝，fw4 会把面板已放行的流量一并拒绝。

### 3.4.1 关于服务器 IPv6（是否开启）

面板支持为服务器开启 IPv6（“服务器管理”里的**服务器 IPv6 网段**，对应 OpenVPN 的 `server-ipv6`）。
**是否开启请按业务实际情况决定：**

- **开启 IPv6 的好处**：即便你没有任何 IPv6 业务，也建议开启。否则接口上没有 IPv6 地址，
  客户端会认为“本机没有 IPv6 网络”，可能导致其正常的 IPv6 出公网流量异常（本机 IPv6 流量
  不会经由 VPN 转发，行为不符合预期）。
- **开启 IPv6 的坏处**：极少数**完全禁用了 IPv6 的主机**，由于无法为其接口配置 IPv6 地址，
  会出现连接失败。

因此推荐默认开启；只有在明确知道客户端主机全部禁用了 IPv6 时，才关闭（把该字段留空）。

> 四个脚本目录（二进制内置的默认资源、`doc-openwrt/`、`nftables-scripts/generic/`、
> `nftables-scripts/openwrt/`）均已支持 IPv6 的 ACL、限速与流量统计。

### 3.4.2 下载限速方案实测（自编译 OpenWrt 固件必读）

自编译固件通常**不带 `kmod-sched-act-police`，也不带 `kmod-ifb` / `kmod-sched-act-mirred`**，
因此需要先确认可用方案。下面是真实设备（ImmortalWrt SNAPSHOT、内核 6.18.39、x86_64）在
OpenVPN 隧道接口上的实测结论。

面向**下载方向（客户端 → 服务器，即服务器的入方向）**，常见做法有以下几类：

| 方案 | 原理 | 需要的内核模块 | 本设备实测 |
| --- | --- | --- | --- |
| **ifb + HTB** | ingress 上把匹配的包 `mirred egress redirect` 到 `ifb`，在 ifb 上做 HTB 整形 | `ifb`、`act_mirred`、`sch_ingress`、`sch_htb`、`cls_u32` | ✅ **可用**，8Mbit 限到 ~7.5–8 Mbits/sec |
| **ifb + TBF** | 同上，整形器换成 TBF | `ifb`、`act_mirred`、`sch_ingress`、`sch_tbf`、`cls_u32` | ✅ **可用**，8Mbit 限到 ~7.7 Mbits/sec |
| **ifb + CAKE** | 同上，整形器换成 CAKE | `ifb`、`act_mirred`、`sch_ingress`、`sch_cake`、`cls_u32` | ✅ **可用**，8Mbit 限到 ~7.6 Mbits/sec |
| **ingress + police** | 在 ingress 过滤器的 action 里直接 `police rate ... drop` | `sch_ingress`、`cls_u32`、**`act_police`** | ❌ **不可用**（`RTNETLINK answers: No such file or directory`），自编译固件普遍缺 `act_police.ko` |
| **iptables/nft 的 `limit`/`hashlimit`** | 用防火墙匹配模块按速率丢包 | 对应 xt/nft match | ⚠️ 只能做粗粒度丢包，**不是整形**，会重传、抖动大，**不推荐**做带宽限速 |
| **应用层 / OpenVPN `shaper`** | OpenVPN 自带 `--shaper`（仅出方向、全局） | 无 | ⚠️ 仅能限**上传**且是全局总量，无法按客户端（详见下方说明） |

**结论：**

- 本类自编译固件**能用的下载限速方案是「ifb + HTB/TBF/CAKE」这一族**，前提是固件里编进了
  `kmod-ifb` + `kmod-sched-act-mirred`。若这两个也没有，则**没有任何内核级下载限速方案**，
  脚本会记录 WARNING 并跳过下载限速（上传限速不受影响）。
- 面板脚本的默认实现是 **ifb + HTB**；`ratelimit.sh`（达量限速）同样基于 ifb。
- 上传方向（服务器 → 客户端，服务器的出方向）用常规 egress qdisc（HTB/TBF/CAKE）即可，
  不需要 ifb，本设备同样可用。

**如何检查固件带不带这些模块**（有对应 `.ko` 文件或能 `modprobe` 即可）：

```sh
ls /lib/modules/$(uname -r)/ | grep -E 'ifb|act_mirred|act_police|sch_htb|sch_tbf|sch_cake|cls_u32|sch_ingress'
modprobe ifb && echo "ifb OK"
modprobe act_mirred && echo "act_mirred OK"
modprobe act_police && echo "act_police OK"   # 一般会失败
```

**手动验证 ifb + HTB 是否生效**（把 `DEV` 换成你的 openvpn 隧道接口）：

```sh
DEV=tunudp1300
ip link add ifb9 type ifb && ip link set ifb9 up
tc qdisc add dev $DEV handle ffff: ingress
tc filter add dev $DEV parent ffff: protocol ip prio 1 \
    u32 match ip src <客户端隧道IP>/32 action mirred egress redirect dev ifb9
tc qdisc add dev ifb9 root handle 1: htb default 9999
tc class add dev ifb9 parent 1: classid 1:2 htb rate 8000kbit ceil 8000kbit
tc filter add dev ifb9 parent 1: protocol ip prio 1 \
    u32 match ip src <客户端隧道IP>/32 flowid 1:2
# 之后在客户端跑: iperf3 -c <服务器隧道IP> ，应降到约 8 Mbits/sec
```

**注意（探测顺序）**：在 ingress 上挂规则前，必须**先创建 `ffff:` ingress qdisc**
（`tc qdisc add dev $DEV handle ffff: ingress`）。否则 `tc filter add ... parent ffff:`
会返回 `RTNETLINK answers: Invalid argument`，被误判为“限速不可用”。

**清理顺序**（避免 `Resource busy` 残留）：

```sh
tc filter del dev $DEV parent ffff: protocol ip prio 1
tc qdisc del dev $DEV ingress
tc class del dev ifb9 classid 1:2
tc qdisc del dev ifb9 root
ip link del ifb9
```

> **脚本中的实现**：OpenWrt 版资源脚本（`doc-openwrt/` 与 `nftables-scripts/openwrt/`）
> 已按本节结论处理，无需手动操作：
>
> - 下载限速的 ifb 设备名统一为 **`ovpnrl<服务器ID>`**（加长并加前缀，避免与系统
>   自带的 `ifb0`/`ifb1` 或其它程序的 ifb 设备重名）。
> - `client_online.sh` / `ratelimit.sh` 在探测方案前**先创建主接口的 `ffff:` ingress qdisc**；
>   若探测失败且 ingress 是本次新建的，会把它删除，避免留下空 qdisc。
> - `server_start.sh` 启动时会清理上次异常退出遗留的 `ovpnrl<服务器ID>` 设备与主接口 ingress；
>   `server_exit.sh` 停止时会删除主接口 ingress（含其上过滤器）并删除 `ovpnrl<服务器ID>` 设备。

**两套脚本的下载限速策略（默认行为）**

> **普通 Linux 发行版默认用 `police`，`ifb` 方案仅在 OpenWrt 脚本中启用。**

| | 普通 Linux（默认资源 / `nftables-scripts/generic/`） | OpenWrt（`doc-openwrt/` / `nftables-scripts/openwrt/`） |
| --- | --- | --- |
| **默认方案** | ingress + **police**（纯丢包限速，无探测） | 探测后 **ifb + HTB** 优先，**police** 回退 |
| **依赖模块** | `sch_ingress`、`cls_u32`、`act_police` | ifb 分支需 `ifb` + `act_mirred` + `sch_htb`；police 分支需 `act_police` |
| **设备/状态** | 无额外设备，一行 filter 即完成 | 每个服务器多一个 `ovpnrl<服务器ID>` 虚拟设备 + 主接口 ingress |

**两种方案的优势与缺点：**

- **ingress + police（普通 Linux 默认）**
  - ✅ 优势：实现最简单，**无状态**（不需要创建/维护/清理 ifb 设备），普通发行版内核普遍自带 `act_police`，开箱即用。
  - ❌ 缺点：只是**超速丢包**，不是整形；被限速的连接会触发 TCP 重传、速率抖动大，实测接近但不如整形平滑。
- **ifb + HTB（OpenWrt 脚本默认优先）**
  - ✅ 优势：是真正的**整形**（排队、平滑），限速更稳、抖动小；不依赖 OpenWrt 常缺的 `act_police`。
  - ❌ 缺点：需要固件编入 `kmod-ifb` + `kmod-sched-act-mirred`；脚本需**探测**方案、创建/设置/清理 ifb 设备与 ingress qdisc，逻辑更复杂，异常退出时可能残留设备（已在 `server_start.sh`/`server_exit.sh` 中回收）。

> 如果你希望**一定**具备下载限速能力：在编译固件时选中 `kmod-ifb` 与
> `kmod-sched-act-mirred`（以及 `kmod-sched-htb`、`kmod-sched-core`）。
> 若你确实无法加入这些模块，那么请接受“无下载限速”这一现状，面板会明确记录 WARNING。

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


---

## 5. 本目录文件清单

| 文件 | 放置位置 | 说明 |
| --- | --- | --- |
| `openvpn-pannel` | `/etc/init.d/openvpn-pannel` | procd 启动脚本 |
| `client_online.sh` | 资源集 `openwrt-iptables` | 上线：ACL 放行 + 限速（`flock` 串行化） |
| `client_offline.sh` | 资源集 `openwrt-iptables` | 下线：回收 ACL + 限速（`flock` 串行化） |
| `acl_add.sh` | 资源集 `openwrt-iptables` | TOTP 二次认证通过后动态放行 ACL（仅启用 MFA 时用到） |
| `acl_del.sh` | 资源集 `openwrt-iptables` | TOTP 登出后动态回收 ACL（仅启用 MFA 时用到） |
| `ratelimit.sh` | 资源集 `openwrt-iptables` | 达量限速变化时重设 / 移除 tc 限速（`flock` 串行化） |
| `server_start.sh` | 资源集 `openwrt-iptables` | 服务端启动：初始化 ACL 链 / ipset |
| `server_exit.sh` | 资源集 `openwrt-iptables` | 服务端退出：回收 ACL 链 / ipset |
| `misc` | 资源集 `openwrt-iptables` | 路径 / 文件名配置 |

> 以上文件是内置资源集 `openwrt-iptables` 的来源，面板已内置，无需手动放置。
> 需要纯 `nftables` 时，在「资源管理」页启用 `openwrt-nftables` 资源集即可（详见 3.4）。

---

## 6. 常见问题

- **限速不生效**：确认已安装 `tc`，且内核包含 `sch_htb`、`cls_u32`、`sch_ingress`。**下载（入方向）限速**优先依赖 `ifb` + `act_mirred`（安装 `kmod-ifb`、`kmod-sched-act-mirred`），不可用时回退 `act_police`；两者都缺失时脚本会记录 WARNING 并跳过下载限速（上传限速不受影响）。**自编译固件通常既无 `act_police`、也可能未编入 `ifb`/`act_mirred`，请按 3.4.2 实测确认你的固件到底能用哪种方案**。同时请在「资源管理」页确认已启用 OpenWrt 版资源集（`openwrt-iptables` 或 `openwrt-nftables`）。缺失时脚本会记录日志并跳过限速，不影响连接；达量限速在运行中变化时依赖 `ratelimit.sh` 重设 tc。
- **ACL 未生效**：iptables 资源集确认已安装 `ipset`。缺失时脚本会回落到逐条 `iptables` 模式（功能仍可用）。若启用 nftables 资源集，则无需 `ipset` / `iptables`，但需安装 `nftables`，并确认服务端启动脚本成功建立了 `openvpn_acl_<服务器ID>` 表。
- **启用 MFA 后客户端一直无法上网**：确认已在客户端自助页面完成动态验证码验证；nftables 资源集的 `acl_add.sh`、`acl_del.sh` 已一并启用，无需单独替换。
- **应用接口后地址消失**：属正常现象，见 3.6，重启服务器进程即可恢复。
- **修改了资源脚本**：需在面板重新保存并重启服务，使新脚本生效。
- **改了 `config.json`**：同样需要重启服务才生效。
- **停止面板后 openvpn 进程残留**：
  - 请使用 `/etc/init.d/openvpn-pannel stop` 正常停止。procd 默认发 `SIGTERM`，面板会据此优雅关闭所有 openvpn 实例（每个最多等 8 秒，`term_timeout` 给了 60 秒，见 3.2）。
  - **不要用 `kill -9` 直接杀面板进程**，否则面板来不及关闭子进程，openvpn 会残留为孤儿进程，需要手动 `kill` 或重启设备。
  - 若使用旧版面板（只处理 `SIGINT` 不处理 `SIGTERM`），升级到本目录脚本并重建二进制即可；本目录的启动脚本同时补发 `SIGINT`，对旧二进制也有效。
