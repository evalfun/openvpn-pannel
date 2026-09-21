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

log_message "INFO" "nftables 表 $TABLE_NAME 已删除，接口 $SERVER_INTERFACE 的转发控制已回收"

exit 0
