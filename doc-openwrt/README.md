# OpenWrt / ImmortalWrt 部署说明

> **本文档内容已整合进面板内置帮助与项目完整文档，本文件仅作指引。**

OpenWrt / ImmortalWrt 的完整部署说明（适用环境与依赖、目录约定、procd 启动脚本、资源集选择、
IPv6 建议、下载限速方案实测、防火墙区域与接口设置、常见问题）现位于：

- **面板内**：启用 `openwrt-iptables` 或 `openwrt-nftables` 资源集后，打开「帮助信息」页查看；
- **仓库内**：[`../full-doc.md`](../full-doc.md) 的「13.1 部署（OpenWrt / ImmortalWrt，procd）」章节。

本目录仍保留可直接使用的 **`openvpn-pannel`** procd 启动脚本（保存为 `/etc/init.d/openvpn-pannel`），
其内容也已内嵌在上述帮助信息中，可直接复制。
