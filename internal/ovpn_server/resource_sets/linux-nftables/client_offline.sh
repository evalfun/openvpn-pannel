#!/bin/bash

INTERNAL_API="__INTERNAL_API__"
LOG_FILE="__WORKING_DIR__auth.log"
SERVER_ID="__SERVER_ID__"

SERVER_INTERFACE="__SERVER_INTERFACE__"
LOCK_FILE="__WORKING_DIR__acl.lock"

# nftables：表 / 链 / 集合结构见 client_online.sh 顶部说明。
if [ -x /usr/sbin/nft ]; then
    NFT="sudo /usr/sbin/nft"
else
    NFT="sudo nft"
fi

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
list_v6_cidrs() {
    tr -d '\r' \
        | sed -n 's/^[[:space:]]*6#//p' \
        | sed 's/[[:space:]]*$//' \
        | grep -v '^[[:space:]]*$' \
        | sort -u
}

minor_from_v4() {
    local _c _d M
    _c=$(printf '%s' "$1" | cut -d. -f3)
    _d=$(printf '%s' "$1" | cut -d. -f4)
    M=$(( (${_c:-0} << 8) | ${_d:-0} ))
    [ "$M" -le 0 ] && M=1
    [ "$M" -ge 65535 ] && M=65534
    [ "$M" -eq 9999 ] && M=9998
    printf '%s' "$M"
}
minor_from_v6() {
    local _h M
    _h=${1##*:}
    case "$_h" in ''|*[!0-9a-fA-F]*) _h=0 ;; esac
    M=$(( 16#${_h:-0} ))
    [ "$M" -le 0 ] && M=1
    [ "$M" -ge 65535 ] && M=65534
    [ "$M" -eq 9999 ] && M=9998
    printf '%s' "$M"
}

CLIENT_IP4="$ifconfig_pool_remote_ip"
CLIENT_IP6="$ifconfig_pool_remote_ip6"

log_message "INFO" "用户下线 CertificateName $common_name User $username Virtual IP: $CLIENT_IP4 Virtual IP6: $CLIENT_IP6 Client IP: $untrusted_ip:$untrusted_port Bytes Received: $bytes_received Bytes Sent: $bytes_sent Duration: $time_duration seconds"
encoded_username=$(echo -n "$username" | base64 -w 0 | tr -d '\n')

# 执行下线命令
request_body="server_id: $SERVER_ID
username: $encoded_username
real_ip_addr: $untrusted_ip:$untrusted_port
client_cert_name: $common_name
virtual_ip_addr: $CLIENT_IP4
virtual_ip6_addr: $CLIENT_IP6
bytes_received: $bytes_received
bytes_send: $bytes_sent"

curl -s -X POST -d "$request_body" http://$INTERNAL_API/user/offline

# ===== 清理带宽限速 (tc) =====
# classid/prio 与上线脚本一致（有 IPv4 用 IPv4，否则用 IPv6）；无论是否设置过限速都尝试清理。
# 下载限速可能建在主接口 ingress(redirect 到 ifb 或 police)，也可能是 ifb 上的 HTB 类，
# 这里统一把两种形态都清理掉。
TC_BIN=""
for _tc in /usr/sbin/tc /sbin/tc /usr/bin/tc /bin/tc; do
    if [ -x "$_tc" ]; then TC_BIN="sudo $_tc"; break; fi
done
[ -z "$TC_BIN" ] && TC_BIN="sudo tc"

IP_BIN=""
for _ip in /usr/sbin/ip /sbin/ip /usr/bin/ip /bin/ip; do
    if [ -x "$_ip" ]; then IP_BIN="sudo $_ip"; break; fi
done
[ -z "$IP_BIN" ] && IP_BIN="sudo ip"

IFB_DEV="ifb${SERVER_ID}"

TC_DEV="$SERVER_INTERFACE"
if { [ -n "$CLIENT_IP4" ] || [ -n "$CLIENT_IP6" ]; } && ip link show "$TC_DEV" >/dev/null 2>&1; then
    # 与上线共用同一把 flock，串行化清理，避免与并发上/下线进程互相覆盖 qdisc/class/filter
    (
    if ! flock -w 60 9; then
        log_message "WARNING" "用户 User $username VirtualIP: $CLIENT_IP4/$CLIENT_IP6 获取限速锁超时，跳过 tc 清理"
        exit 0
    fi
    if [ -n "$CLIENT_IP4" ]; then
        MINOR=$(minor_from_v4 "$CLIENT_IP4")
    else
        MINOR=$(minor_from_v6 "$CLIENT_IP6")
    fi

    # 上传（主接口 root htb）
    $TC_BIN filter del dev "$TC_DEV" parent 1: protocol ip prio $MINOR u32 2>/dev/null || true
    $TC_BIN filter del dev "$TC_DEV" parent 1: protocol ipv6 prio $MINOR u32 2>/dev/null || true
    $TC_BIN class del dev "$TC_DEV" classid 1:$MINOR 2>/dev/null || true
    # 下载：主接口 ingress 上的 redirect/police 过滤器
    $TC_BIN filter del dev "$TC_DEV" parent ffff: protocol ip prio $MINOR u32 2>/dev/null || true
    $TC_BIN filter del dev "$TC_DEV" parent ffff: protocol ipv6 prio $MINOR u32 2>/dev/null || true
    # 下载：ifb 上的 HTB 类与过滤器（若存在）
    if $IP_BIN link show "$IFB_DEV" >/dev/null 2>&1; then
        $TC_BIN filter del dev "$IFB_DEV" parent 1: protocol ip prio $MINOR u32 2>/dev/null || true
        $TC_BIN filter del dev "$IFB_DEV" parent 1: protocol ipv6 prio $MINOR u32 2>/dev/null || true
        $TC_BIN class del dev "$IFB_DEV" classid 1:$MINOR 2>/dev/null || true
        # ifb 上已无自定义类（只剩默认 9999）时，可删除 ifb 设备回收资源
        REMAIN_CLS=$($TC_BIN class show dev "$IFB_DEV" 2>/dev/null | grep -c 'class htb 1:' || true)
        if [ "${REMAIN_CLS:-0}" -le 1 ]; then
            $IP_BIN link del "$IFB_DEV" 2>/dev/null || true
        fi
    fi
    log_message "INFO" "用户 User $username VirtualIP: $CLIENT_IP4/$CLIENT_IP6 已清理 tc 限速 classid 1:$MINOR"
    ) 9>"$LOCK_FILE"
fi

# 取回该客户端上线时存入的 ACL（/user/acl/del 会返回并删除 AddedServerACLRecord）
request_body="server_id: $SERVER_ID
username: $encoded_username
real_ip_addr: $untrusted_ip:$untrusted_port
virtual_ip_addr: $CLIENT_IP4
virtual_ip6_addr: $CLIENT_IP6"
result=$(curl -s -X POST -d "$request_body" http://$INTERNAL_API/user/acl/del)

log_message "DEBUG" "acl内容 $result"

CIDRS4=$(printf '%s\n' "$result" | list_v4_cidrs)
CIDRS6=$(printf '%s\n' "$result" | list_v6_cidrs)
if [ -z "$CIDRS4" ] && [ -z "$CIDRS6" ]; then
    log_message "WARNING" "用户 User $username VirtualIP: $CLIENT_IP4/$CLIENT_IP6 未能还原 ACL 记录，跳过清理"
    exit 0
fi

if ! $NFT list table inet "$TABLE_NAME" >/dev/null 2>&1; then
    log_message "WARNING" "nftables 表 $TABLE_NAME 不存在，跳过清理"
    exit 0
fi

# 临界区：从源集移除该客户端；源集空了则按“删主链跳转 -> flush子链 -> 删子链 -> 删set”的顺序回收
(
    if ! flock -w 60 9; then
        log_message "WARNING" "用户 User $username VirtualIP: $CLIENT_IP4/$CLIENT_IP6 获取ACL锁超时，跳过nft清理"
        exit 0
    fi

    # ---------- IPv4 ----------
    if [ -n "$CIDRS4" ] && [ -n "$CLIENT_IP4" ]; then
        HASH=$(printf '%s\n' "$CIDRS4" | sha256sum | cut -c1-$HASH_LEN)
        SRC_SET="ov${SERVER_ID}_s_${HASH}"
        DST_SET="ov${SERVER_ID}_d_${HASH}"
        SUB_CHAIN="ov${SERVER_ID}_c_${HASH}"

        if ! $NFT list set inet "$TABLE_NAME" "$SRC_SET" >/dev/null 2>&1; then
            log_message "WARNING" "源集 $SRC_SET 不存在(可能已清理或哈希不匹配) 用户 User $username VirtualIP: $CLIENT_IP4"
        else
            $NFT delete element inet "$TABLE_NAME" "$SRC_SET" "{ $CLIENT_IP4 }" 2>/dev/null || \
                log_message "WARNING" "从 $SRC_SET 移除 $CLIENT_IP4 失败(可能已不存在)"

            # 源集已空时 nft list set 不再输出 elements 段
            REMAIN=1
            $NFT list set inet "$TABLE_NAME" "$SRC_SET" 2>/dev/null | grep -q 'elements' || REMAIN=0

            log_message "INFO" "用户 User $username VirtualIP: $CLIENT_IP4 下线(nft) src=$SRC_SET 组 $HASH 剩余=$REMAIN"

            if [ "$REMAIN" = "0" ]; then
                # 顺序关键：acl 链规则引用源集、子链规则引用目标集，需先删规则再删子链与集合。
                # 通过 handle 精确定位 acl 链中引用该源集的跳转规则后删除。
                HANDLE=$($NFT -a list chain inet "$TABLE_NAME" "$ACL_CHAIN" 2>/dev/null \
                    | awk -v s="@$SRC_SET" 'index($0,s){for(i=1;i<=NF;i++) if($i=="handle"){print $(i+1); exit}}')
                [ -n "$HANDLE" ] && $NFT delete rule inet "$TABLE_NAME" "$ACL_CHAIN" handle "$HANDLE" 2>/dev/null || true
                $NFT flush chain inet "$TABLE_NAME" "$SUB_CHAIN" 2>/dev/null || true
                $NFT delete chain inet "$TABLE_NAME" "$SUB_CHAIN" 2>/dev/null || true
                $NFT delete set inet "$TABLE_NAME" "$SRC_SET" 2>/dev/null || true
                $NFT delete set inet "$TABLE_NAME" "$DST_SET" 2>/dev/null || true
                log_message "INFO" "组 $HASH 已无在线客户端，回收 src=$SRC_SET dst=$DST_SET chain=$SUB_CHAIN"
            fi
        fi
    fi

    # ---------- IPv6 ----------
    if [ -n "$CIDRS6" ] && [ -n "$CLIENT_IP6" ]; then
        HASH6=$(printf '%s\n' "$CIDRS6" | sha256sum | cut -c1-$HASH_LEN)
        SRC6_SET="ov${SERVER_ID}_6s_${HASH6}"
        DST6_SET="ov${SERVER_ID}_6d_${HASH6}"
        SUB6_CHAIN="ov${SERVER_ID}_6c_${HASH6}"

        if ! $NFT list set inet "$TABLE_NAME" "$SRC6_SET" >/dev/null 2>&1; then
            log_message "WARNING" "IPv6 源集 $SRC6_SET 不存在(可能已清理或哈希不匹配) 用户 User $username VirtualIP6: $CLIENT_IP6"
        else
            $NFT delete element inet "$TABLE_NAME" "$SRC6_SET" "{ $CLIENT_IP6 }" 2>/dev/null || \
                log_message "WARNING" "从 $SRC6_SET 移除 $CLIENT_IP6 失败(可能已不存在)"

            REMAIN6=1
            $NFT list set inet "$TABLE_NAME" "$SRC6_SET" 2>/dev/null | grep -q 'elements' || REMAIN6=0

            log_message "INFO" "用户 User $username VirtualIP6: $CLIENT_IP6 下线(nft) src=$SRC6_SET 组 $HASH6 剩余=$REMAIN6"

            if [ "$REMAIN6" = "0" ]; then
                HANDLE6=$($NFT -a list chain inet "$TABLE_NAME" "$ACL_CHAIN" 2>/dev/null \
                    | awk -v s="@$SRC6_SET" 'index($0,s){for(i=1;i<=NF;i++) if($i=="handle"){print $(i+1); exit}}')
                [ -n "$HANDLE6" ] && $NFT delete rule inet "$TABLE_NAME" "$ACL_CHAIN" handle "$HANDLE6" 2>/dev/null || true
                $NFT flush chain inet "$TABLE_NAME" "$SUB6_CHAIN" 2>/dev/null || true
                $NFT delete chain inet "$TABLE_NAME" "$SUB6_CHAIN" 2>/dev/null || true
                $NFT delete set inet "$TABLE_NAME" "$SRC6_SET" 2>/dev/null || true
                $NFT delete set inet "$TABLE_NAME" "$DST6_SET" 2>/dev/null || true
                log_message "INFO" "IPv6 组 $HASH6 已无在线客户端，回收 src=$SRC6_SET dst=$DST6_SET chain=$SUB6_CHAIN"
            fi
        fi
    fi
) 9>"$LOCK_FILE"

exit 0
