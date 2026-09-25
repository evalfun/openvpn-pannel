#!/bin/bash
# 在服务端停止后执行此脚本（nftables 版本）

INTERNAL_API="__INTERNAL_API__"
LOG_FILE="__WORKING_DIR__auth.log"
SERVER_ID="__SERVER_ID__"

SERVER_INTERFACE="__SERVER_INTERFACE__"
TABLE_NAME="openvpn_acl_$SERVER_ID"

NFT_BIN=""
for _nft in /usr/sbin/nft /sbin/nft /usr/bin/nft /bin/nft; do
    if [ -x "$_nft" ]; then NFT_BIN="$_nft"; break; fi
done
[ -z "$NFT_BIN" ] && NFT_BIN="nft"

log_message() {
    local level="$1"
    local message="$2"
    echo "$(date '+%Y-%m-%d %H:%M:%S') [${level}] ${message}" >> "$LOG_FILE"
}

# 删除整张表即可回收本服务器的全部基础链 / 规则链 / 子链 / 集合 / 规则
$NFT_BIN delete table inet "$TABLE_NAME" 2>/dev/null || true

# 回收本服务器遗留的下载限速对象：先删主接口 ingress 上的 redirect/police 过滤器，
# 再删 ingress qdisc，最后删除 ifb 设备（ifb 名见 client_online.sh：ovpnrl<服务器ID>）。
TC_BIN=""
for _tc in /sbin/tc /usr/sbin/tc /usr/bin/tc /bin/tc; do
    if [ -x "$_tc" ]; then TC_BIN="$_tc"; break; fi
done
[ -z "$TC_BIN" ] && TC_BIN="tc"

IP_BIN=""
for _ip in /sbin/ip /usr/sbin/ip /usr/bin/ip /bin/ip; do
    if [ -x "$_ip" ]; then IP_BIN="$_ip"; break; fi
done
[ -z "$IP_BIN" ] && IP_BIN="ip"

IFB_DEV="ovpnrl${SERVER_ID}"
if [ -n "$SERVER_INTERFACE" ]; then
    $TC_BIN qdisc del dev "$SERVER_INTERFACE" ingress >/dev/null 2>&1 || true
fi
if $IP_BIN link show "$IFB_DEV" >/dev/null 2>&1; then
    $IP_BIN link del "$IFB_DEV" >/dev/null 2>&1 || true
    log_message "INFO" "已回收下载限速设备 $IFB_DEV"
fi

log_message "INFO" "nftables 表 $TABLE_NAME 已删除，接口 $SERVER_INTERFACE 的转发控制已回收"

exit 0
