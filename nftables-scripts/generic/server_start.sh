#!/bin/bash
# 在服务端启动后执行此脚本（nftables 版本）

INTERNAL_API="__INTERNAL_API__"
LOG_FILE="__WORKING_DIR__auth.log"
SERVER_ID="__SERVER_ID__"

SERVER_INTERFACE="__SERVER_INTERFACE__"
TABLE_NAME="openvpn_acl_$SERVER_ID"
ACL_CHAIN="acl"

# 设置mss。mss>100时启用 推荐在客户端使用mssfix参数调整mss 
MSS=0

if [ -x /usr/sbin/nft ]; then
    NFT="sudo /usr/sbin/nft"
else
    NFT="sudo nft"
fi

log_message() {
    local level="$1"
    local message="$2"
    echo "$(date '+%Y-%m-%d %H:%M:%S') [${level}] ${message}" >> "$LOG_FILE"
}

# ACL 与转发控制全部由 nftables 承担；集合是 nft 原生能力，无需 ipset。
# inet 表同时适用于 IPv4/IPv6：forward 基础链对两个地址族都生效，
# 各 ACL 组在 acl 链内分别以 ip/ip6 规则放行（集合类型 ipv4_addr / ipv6_addr）。
if ! $NFT list tables >/dev/null 2>&1; then
    log_message "WARN" "nft 不可用，ACL 与转发控制将无法生效（请安装 nftables 及相关内核模块）"
    exit 1
fi

# 删除并重建本服务器专用表：链 / 集合 / 规则随之全部回收，天然清理上次遗留
$NFT delete table inet "$TABLE_NAME" 2>/dev/null || true
$NFT add table inet "$TABLE_NAME"

# forward 基础链：priority -200 早于 fw4 等默认 filter(0) 链，保证面板的放行/丢弃先行生效。
# policy accept 表示不匹配本表接口的流量不受影响，交由系统其它防火墙处理。
$NFT add chain inet "$TABLE_NAME" forward '{ type filter hook forward priority -200 ; policy accept ; }'
$NFT add chain inet "$TABLE_NAME" "$ACL_CHAIN"

if [ "$MSS" -gt 100 ]; then
    $NFT add rule inet "$TABLE_NAME" forward iifname "$SERVER_INTERFACE" tcp flags '&' '(syn|rst)' == syn tcp option maxseg size set "$MSS"
    $NFT add rule inet "$TABLE_NAME" forward oifname "$SERVER_INTERFACE" tcp flags '&' '(syn|rst)' == syn tcp option maxseg size set "$MSS"
fi

$NFT add rule inet "$TABLE_NAME" forward ct state established,related accept
$NFT add rule inet "$TABLE_NAME" forward iifname "$SERVER_INTERFACE" jump "$ACL_CHAIN"
$NFT add rule inet "$TABLE_NAME" forward iifname "$SERVER_INTERFACE" drop
$NFT add rule inet "$TABLE_NAME" forward oifname "$SERVER_INTERFACE" accept

log_message "INFO" "nftables 表 $TABLE_NAME 已建立，接口 $SERVER_INTERFACE 的转发由面板 ACL 控制"

exit 0
