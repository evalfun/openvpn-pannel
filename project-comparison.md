# OpenVPN 面板项目对比

本文对比四个 OpenVPN 管理面板/项目，帮助做选型与功能借鉴。

| 项目 | 地址 | 来源 |
|---|---|---|
| **openvpn-pannel**（本项目） | <https://github.com/evalfun/openvpn-pannel> | 自研 |
| **ovpnmanager** | <https://github.com/slc14211421/ovpnmanager> | 开源 |
| **openvpn-ui** | <https://github.com/d3vilh/openvpn-ui> | 开源 |
| **ovpn-admin** | <https://github.com/palark/ovpn-admin> | 开源 |

> 为便于阅读，下文表格统一使用简称：**pannel** = openvpn-pannel，**ovpnmanager** = ovpnmanager，**openvpn-ui** = openvpn-ui，**ovpn-admin** = ovpn-admin。

---

## 一、基本情况与技术栈

| 维度 | pannel（本项目） | ovpnmanager | openvpn-ui | ovpn-admin |
|---|---|---|---|---|
| 来源 | 自研 | 开源 | 开源 | 开源 |
| 后端语言/框架 | Go 1.25 + Gin | Python + FastAPI | Go 1.26 + Beego v2 | Go 1.24 + 标准库 net/http |
| 前端 | React 19 + MUI（SPA） | Vue 3 + Element Plus（SPA） | 服务端模板 AdminLTE/Bootstrap/jQuery | Vue 2 + bootstrap-vue |
| 前后端分发 | 单二进制，前端内嵌 | 前后端分离 + Nginx | 单二进制 + 静态资源 / Docker | 单二进制内嵌 / Docker / Helm |
| 数据库/存储 | SQLite / MySQL（GORM） | SQLite（SQLAlchemy） | SQLite（Beego ORM） | 文件系统 / **K8s Secrets** |
| 配置方式 | `config.json` | `.env` | `conf/app.conf` + DB | flags / 环境变量 |
| 核心外部依赖 | **无 openssl** | EasyRSA/openssl、systemctl | EasyRSA/openssl、Docker、oathtool、qrencode | EasyRSA/openssl、bash、openvpn-user |
| 后端代码量 | ~15k 行 | ~2.7k 行 | ~3.3k 行 | ~3k 行 |
| 前端代码量 | ~11.5k 行 | ~1.5k 行 | ~2.6k 行模板 | ~0.5k 行 |
| 测试 | 有（~5k 行 Go 单测） | 少量（276 行） | 无 | 无 |
| 社区活跃度 | —（自研） | 低 | 高（持续发布） | 高（CI 活跃） |

---

## 二、功能对比

| 功能 | pannel（本项目） | ovpnmanager | openvpn-ui | ovpn-admin |
|---|---|---|---|---|
| 多 OpenVPN 实例 | 支持（多实例托管） | 不支持（单实例） | 不支持（单实例） | 部分（多 mgmt 接口） |
| 进程托管/保活 | 支持（看门狗） | 不支持（靠 systemd） | 不支持（靠容器） | 不支持（不管理服务） |
| 服务启停/重启 | 支持（直接托管） | 支持（systemctl） | 支持（重启容器） | 不支持 |
| 用户 + 用户组 | 支持（组 + 有效期/禁用 + CSV 批量） | 部分（仅面板账号） | 部分（仅面板账号） | 不支持（仅证书用户） |
| 证书生命周期 | 大部分支持（生成/签发/导入/引用，RSA/ECDSA）不支持吊销 | 支持（生成/吊销/CRL） | 支持（生成/续期/吊销/删除/PKI 维护） | 支持（生成/吊销/恢复/轮换/删除） |
| 客户端 .ovpn | 支持（内联导出） | 支持（内联导出） | 支持（内联导出） | 支持（模板渲染） |
| CCD（静态 IP/路由） | 支持 | 支持 | 部分（仅静态 IP） | 支持（静态 IP + 自定义路由） |
| 附加密码认证 | 支持（+ MFA） | 不支持 | 支持（服务端 2FA） | 支持（openvpn-user） |
| **ACL 访问控制** | 支持（CIDR v4/v6 + ipset） | 不支持 | 不支持 | 不支持 |
| **带宽/达量限速** | 支持 | 不支持 | 不支持 | 不支持 |
| 在线状态/踢下线 | 支持 | 支持（management） | 支持（management） | 支持（management） |
| 流量统计 | 支持（实时） | 部分（mgmt 字节） | 部分（load-stats） | 部分（bytes 指标） |
| 审计日志 | 支持（结构化事件） | 支持 | 部分（运行日志） | 不支持 |
| 日志查看/轮换 | 支持 / 支持 | 不支持 | 支持（查看） | 不支持 |
| 服务器配置编辑 | 部分（模板生成） | 部分（读写 server.conf） | 支持（表单 + 原文编辑） | 不支持 |
| 客户端自助页 | 支持 | 不支持 | 不支持 | 不支持 |
| 内存模式（护闪存） | 支持 | 不支持 | 不支持 | 不支持 |
| Prometheus 指标 | 不支持 | 不支持 | 不支持 | 支持（原生） |
| master/slave 同步 |  不支持 | 不支持 | 不支持 | 支持（证书/CCD 同步） |
| Kubernetes 集成 | 不支持 | 不支持 | 不支持 | 支持（LB + Secrets + Helm） |
| OpenWrt/嵌入式 | 支持（procd） | 不支持 | 不支持 | 不支持 |
| Grafana Dashboard | 不支持 | 不支持 | 不支持 | 支持 |

---

## 三、认证与安全

| 维度 | pannel（本项目） | ovpnmanager | openvpn-ui | ovpn-admin |
|---|---|---|---|---|
| 面板登录认证 | 支持（Session + bcrypt） | 支持（JWT Token） | 支持（Session + passlib） | **无认证** |
| MFA/TOTP | 支持（面板 + 自助页） | 不支持 | 支持（证书 + OTP 接入） | 不支持 |
| LDAP / OAuth | 不支持 | 不支持 | 支持（LDAP + Google OAuth） | 不支持 |
| 登录频率限制 | 支持 | 部分 | 不支持 | 不支持 |
| 可信代理白名单 | 支持 | 不支持 | 不支持 | 不支持 |
| 权限模型 | 支持（用户组/权限） | 部分（超管/普通） | 部分（管理员/普通） | 不支持 |
| 运行权限 | setcap/sudoers 最小化 | systemd 用户 | privileged + docker.sock | privileged / root |
| 可否公网暴露 | 可以（有认证） | 建议反代 HTTPS | 建议私有/容器内 | 仅限私有网络 |

---

## 四、部署与适用场景

| 维度 | pannel（本项目） | ovpnmanager | openvpn-ui | ovpn-admin |
|---|---|---|---|---|
| 典型部署 | systemd / procd 单文件 | uvicorn + Nginx | docker-compose 双容器 | Docker / Helm / 二进制 |
| 平台 | x86/ARM/OpenWrt | 标准 Linux | Linux + Docker | Linux / Docker / K8s |
| 上手难度 | 中（概念较多） | 低 | 中 | 低 |
| 最适合 | 企业级管控、多实例、审计合规 | 单机快速可视化 | Docker 生态、LDAP/OAuth、PKI 维护 | K8s/监控栈、证书与路由管理 |
| 一句话 | 自包含的企业级控制平面 | 轻量 CRUD 运维面板 | 成熟的 Docker 化 Web UI | 无认证的证书/用户管理器 |

---

## 五、选型速查

| 需求 | 推荐 |
|---|---|
| 需要登录认证、审计、权限、可公网暴露 | pannel（本项目） |
| 需要 ACL、带宽/达量限速、多实例托管 | pannel（本项目） |
| 部署在路由器/嵌入式（OpenWrt、ARM） | pannel（本项目） |
| 单台 OpenVPN，想快速获得 Python/Vue 可视化 CRUD | ovpnmanager |
| 已有 EasyRSA + systemd，只需管理面板 | ovpnmanager |
| Docker 生态，需要 LDAP / Google OAuth、PKI 维护界面 | openvpn-ui |
| 需要服务端 2FA 直接用于 VPN 接入 | openvpn-ui |
| Kubernetes 环境、Prometheus/Grafana 可观测性 | ovpn-admin |
| 需要 master/slave 证书同步、Helm 部署 | ovpn-admin |
| 私有网络、无面板认证也可接受、要 rotate/unrevoke | ovpn-admin |

---

## 六、核心结论

- **pannel（本项目）功能最全**：唯一同时具备 **面板认证 + MFA + 用户组 + ACL + 限速 + 多实例 + 审计 + 自建 CA + OpenWrt** 的项目，适合企业级与嵌入式场景。
- **ovpnmanager 最轻**：FastAPI 实现，功能基础，适合单机快速上手。
- **openvpn-ui 集成最强**：LDAP/OAuth、服务端 2FA、PKI 维护界面、Docker 生态成熟。
- **ovpn-admin 云原生最强**：Prometheus、Kubernetes、master/slave、证书生命周期最完整，但**无面板认证**，仅限私有网络。
