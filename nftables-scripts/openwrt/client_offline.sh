#!/bin/bash

INTERNAL_API="__INTERNAL_API__"
LOG_FILE="__WORKING_DIR__auth.log"
SERVER_ID="__SERVER_ID__"

SERVER_INTERFACE="__SERVER_INTERFACE__"
LOCK_FILE="__WORKING_DIR__acl.lock"

# nftables：表 / 链 / 集合结构见 client_online.sh 顶部说明。
NFT_BIN=""
for _nft in /usr/sbin/nft /sbin/nft /usr/bin/nft /bin/nft; do
    if [ -x "$_nft" ]; then NFT_BIN="$_nft"; break; fi
done
[ -z "$NFT_BIN" ] && NFT_BIN="nft"

TABLE_NAME="openvpn_acl_$SERVER_ID"
ACL_CHAIN="acl"
HASH_LEN=16

log_message() {
    local level="$1"
    local message="$2"
    echo "$(date '+%Y-%m-%d %H:%M:%S') [${level}] ${message}" >> "$LOG_FILE"
}

# 与 client_online.sh 完全一致，用于从存储的 connect-time ACL 还原同一组哈希
list_v4_cidrs() {
    tr -d '\r' \
        | sed -n 's/^[[:space:]]*4#//p' \
        | sed 's/[[:space:]]*$//' \
        | grep -v '^[[:space:]]*$' \
        | sort -u
}

log_message "INFO" "用户下线 CertificateName $common_name User $username Virtual IP: $ifconfig_pool_remote_ip Client IP: $untrusted_ip:$untrusted_port Bytes Received: $bytes_received Bytes Sent: $bytes_sent Duration: $time_duration seconds"
encoded_username=$(echo -n "$username" | base64 -w 0 | tr -d '\n')
CLIENT_IP="$ifconfig_pool_remote_ip"

# 执行下线命令
request_body="server_id: $SERVER_ID
username: $encoded_username
real_ip_addr: $untrusted_ip:$untrusted_port
client_cert_name: $common_name
virtual_ip_addr: $ifconfig_pool_remote_ip
bytes_received: $bytes_received
bytes_send: $bytes_sent"

curl -s -X POST -d "$request_body" http://$INTERNAL_API/user/offline

# ===== 清理带宽限速 (tc) =====
# classid/prio 由虚拟 IPv4 末两段推导，与上线脚本一致；无论是否设置过限速都尝试清理。
# OpenWrt：无需 sudo，直接使用 tc。
TC_BIN=""
for _tc in /sbin/tc /usr/sbin/tc /usr/bin/tc /bin/tc; do
    if [ -x "$_tc" ]; then TC_BIN="$_tc"; break; fi
done
[ -z "$TC_BIN" ] && TC_BIN="tc"

TC_DEV="$SERVER_INTERFACE"
if [ -n "$CLIENT_IP" ] && [ -d "/sys/class/net/$TC_DEV" ]; then
    # 与上线共用同一把 flock，串行化清理，避免与并发上/下线进程互相覆盖 qdisc/class/filter
    (
    if ! flock 9; then
        log_message "WARNING" "用户 User $username VirtualIP: $CLIENT_IP 获取限速锁超时，跳过 tc 清理"
        exit 0
    fi
    _c=$(printf '%s' "$CLIENT_IP" | cut -d. -f3)
    _d=$(printf '%s' "$CLIENT_IP" | cut -d. -f4)
    MINOR=$(( (${_c:-0} << 8) | ${_d:-0} ))
    [ "$MINOR" -le 0 ] && MINOR=1
    [ "$MINOR" -ge 65535 ] && MINOR=65534
    [ "$MINOR" -eq 9999 ] && MINOR=9998

    $TC_BIN filter del dev "$TC_DEV" parent 1: protocol ip prio $MINOR u32 2>/dev/null || true
    $TC_BIN class del dev "$TC_DEV" classid 1:$MINOR 2>/dev/null || true
    $TC_BIN filter del dev "$TC_DEV" parent ffff: protocol ip prio $MINOR u32 2>/dev/null || true
    log_message "INFO" "用户 User $username VirtualIP: $CLIENT_IP 已清理 tc 限速 classid 1:$MINOR"
    ) 9>"$LOCK_FILE"
fi

# 取回该客户端上线时存入的 ACL（/user/acl/del 会返回并删除 AddedServerACLRecord）
request_body="server_id: $SERVER_ID
username: $encoded_username
real_ip_addr: $untrusted_ip:$untrusted_port
virtual_ip_addr: $ifconfig_pool_remote_ip"
result=$(curl -s -X POST -d "$request_body" http://$INTERNAL_API/user/acl/del)

log_message "DEBUG" "acl内容 $result"

CIDRS=$(printf '%s\n' "$result" | list_v4_cidrs)
if [ -z "$CIDRS" ]; then
    log_message "WARNING" "用户 User $username VirtualIP: $CLIENT_IP 未能还原IPv4 ACL记录，跳过清理(无IPv4 ACL或记录缺失)"
    exit 0
fi

if ! $NFT_BIN list table inet "$TABLE_NAME" >/dev/null 2>&1; then
    log_message "WARNING" "nftables 表 $TABLE_NAME 不存在，跳过清理"
    exit 0
fi

HASH=$(printf '%s\n' "$CIDRS" | sha256sum | cut -c1-$HASH_LEN)
SRC_SET="ov${SERVER_ID}_s_${HASH}"
DST_SET="ov${SERVER_ID}_d_${HASH}"
SUB_CHAIN="ov${SERVER_ID}_c_${HASH}"

# 临界区：从源集移除该客户端；源集空了则按“删主链跳转 -> flush子链 -> 删子链 -> 删set”的顺序回收
(
    if ! flock 9; then
        log_message "WARNING" "用户 User $username VirtualIP: $CLIENT_IP 获取ACL锁超时，跳过nft清理"
        exit 0
    fi

    if ! $NFT_BIN list set inet "$TABLE_NAME" "$SRC_SET" >/dev/null 2>&1; then
        log_message "WARNING" "源集 $SRC_SET 不存在(可能已清理或哈希不匹配) 用户 User $username VirtualIP: $CLIENT_IP"
        exit 0
    fi

    $NFT_BIN delete element inet "$TABLE_NAME" "$SRC_SET" "{ $CLIENT_IP }" 2>/dev/null || \
        log_message "WARNING" "从 $SRC_SET 移除 $CLIENT_IP 失败(可能已不存在)"

    # 源集已空时 nft list set 不再输出 elements 段
    REMAIN=1
    $NFT_BIN list set inet "$TABLE_NAME" "$SRC_SET" 2>/dev/null | grep -q 'elements' || REMAIN=0

    log_message "INFO" "用户 User $username VirtualIP: $CLIENT_IP 下线(nft) src=$SRC_SET 组 $HASH 剩余=$REMAIN"

    if [ "$REMAIN" = "0" ]; then
        # 顺序关键：acl 链规则引用源集、子链规则引用目标集，需先删规则再删子链与集合。
        # 通过 handle 精确定位 acl 链中引用该源集的跳转规则后删除。
        HANDLE=$($NFT_BIN -a list chain inet "$TABLE_NAME" "$ACL_CHAIN" 2>/dev/null \
            | awk -v s="@$SRC_SET" 'index($0,s){for(i=1;i<=NF;i++) if($i=="handle"){print $(i+1); exit}}')
        [ -n "$HANDLE" ] && $NFT_BIN delete rule inet "$TABLE_NAME" "$ACL_CHAIN" handle "$HANDLE" 2>/dev/null || true
        $NFT_BIN flush chain inet "$TABLE_NAME" "$SUB_CHAIN" 2>/dev/null || true
        $NFT_BIN delete chain inet "$TABLE_NAME" "$SUB_CHAIN" 2>/dev/null || true
        $NFT_BIN delete set inet "$TABLE_NAME" "$SRC_SET" 2>/dev/null || true
        $NFT_BIN delete set inet "$TABLE_NAME" "$DST_SET" 2>/dev/null || true
        log_message "INFO" "组 $HASH 已无在线客户端，回收 src=$SRC_SET dst=$DST_SET chain=$SUB_CHAIN"
    fi
) 9>"$LOCK_FILE"

exit 0
