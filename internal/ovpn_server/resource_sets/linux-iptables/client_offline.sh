#!/bin/bash

INTERNAL_API="__INTERNAL_API__"
LOG_FILE="__WORKING_DIR__auth.log"
SERVER_ID="__SERVER_ID__"

SERVER_INTERFACE="__SERVER_INTERFACE__"
CHAIN_NAME="openvpn_acl_$SERVER_ID"
LOCK_FILE="__WORKING_DIR__acl.lock"

IPTABLES="sudo /usr/sbin/iptables"
IP6TABLES="sudo /usr/sbin/ip6tables"
if [ -x /usr/sbin/ipset ]; then
    IPSET="sudo /usr/sbin/ipset"
else
    IPSET="sudo ipset"
fi

# 与 client_online.sh 一致：ipset 可用则走 ipset 清理(带 flock)，否则传统逐条 iptables -D(无 flock)
# 注意：tc 限速清理与 ipset 无关，始终占用同一把 flock 串行化。
HAVE_IPSET=0
if $IPSET list -n >/dev/null 2>&1; then
    HAVE_IPSET=1
fi

HAVE_IP6TABLES=0
if [ -x /usr/sbin/ip6tables ] || command -v ip6tables >/dev/null 2>&1; then
    HAVE_IP6TABLES=1
fi

HASH_LEN=16

log_message() {
    local level="$1"
    local message="$2"
    echo "$(date '+%Y-%m-%d %H:%M:%S') [${level}] ${message}" >> "$LOG_FILE"
}

# 客户端真实地址：OpenVPN 在 IPv4 端点设置 trusted_ip，在 IPv6 端点改设 trusted_ip6。
# 两者互斥，据此确定真实地址；IPv6 用 [addr]:port 形式避免与端口拼接产生歧义。
REAL_CLIENT_ADDR=""
if [ -n "$trusted_ip" ]; then
    REAL_CLIENT_ADDR="$trusted_ip:$trusted_port"
elif [ -n "$trusted_ip6" ]; then
    REAL_CLIENT_ADDR="[$trusted_ip6]:$trusted_port"
fi

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

log_message "INFO" "用户下线 CertificateName $common_name User $username Virtual IP: $CLIENT_IP4 Virtual IP6: $CLIENT_IP6 Client IP: $REAL_CLIENT_ADDR Bytes Received: $bytes_received Bytes Sent: $bytes_sent Duration: $time_duration seconds"
encoded_username=$(echo -n "$username" | base64 -w 0 | tr -d '\n')

# 执行下线命令
request_body="server_id: $SERVER_ID
username: $encoded_username
real_ip_addr: $REAL_CLIENT_ADDR
client_cert_name: $common_name
virtual_ip_addr: $CLIENT_IP4
virtual_ip6_addr: $CLIENT_IP6
bytes_received: $bytes_received
bytes_send: $bytes_sent"

curl -s -X POST -d "$request_body" http://$INTERNAL_API/user/offline

# ===== 清理带宽限速 (tc) =====
# classid/prio 与上线脚本一致（有 IPv4 用 IPv4，否则用 IPv6）；无论是否设置过限速都尝试清理。
TC_BIN=""
for _tc in /usr/sbin/tc /sbin/tc /usr/bin/tc /bin/tc; do
    if [ -x "$_tc" ]; then TC_BIN="sudo $_tc"; break; fi
done
[ -z "$TC_BIN" ] && TC_BIN="sudo tc"

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
    # 下载（主接口 ingress 上的 police 过滤器）
    $TC_BIN filter del dev "$TC_DEV" parent ffff: protocol ip prio $MINOR u32 2>/dev/null || true
    $TC_BIN filter del dev "$TC_DEV" parent ffff: protocol ipv6 prio $MINOR u32 2>/dev/null || true
    log_message "INFO" "用户 User $username VirtualIP: $CLIENT_IP4/$CLIENT_IP6 已清理 tc 限速 classid 1:$MINOR"
    ) 9>"$LOCK_FILE"
fi

# 取回该客户端上线时存入的 ACL（/user/acl/del 会返回并删除 AddedServerACLRecord）
request_body="server_id: $SERVER_ID
username: $encoded_username
real_ip_addr: $REAL_CLIENT_ADDR
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

if [ "$HAVE_IPSET" = "1" ]; then
    (
        if ! flock -w 60 9; then
            log_message "WARNING" "用户 User $username VirtualIP: $CLIENT_IP4/$CLIENT_IP6 获取ACL锁超时，跳过ipset清理"
            exit 0
        fi

        # ---------- IPv4 ----------
        if [ -n "$CIDRS4" ] && [ -n "$CLIENT_IP4" ]; then
            HASH=$(printf '%s\n' "$CIDRS4" | sha256sum | cut -c1-$HASH_LEN)
            SRC_SET="ov${SERVER_ID}_s_${HASH}"
            DST_SET="ov${SERVER_ID}_d_${HASH}"
            SUB_CHAIN="ov${SERVER_ID}_c_${HASH}"

            if ! $IPSET list "$SRC_SET" >/dev/null 2>&1; then
                log_message "WARNING" "源集 $SRC_SET 不存在(可能已清理或哈希不匹配) 用户 User $username VirtualIP: $CLIENT_IP4"
            else
                $IPSET del "$SRC_SET" "$CLIENT_IP4" 2>/dev/null || \
                    log_message "WARNING" "从 $SRC_SET 移除 $CLIENT_IP4 失败(可能已不存在)"
                REMAIN=$($IPSET list --terse "$SRC_SET" 2>/dev/null | awk '/Number of entries/{print $NF}')
                [ -z "$REMAIN" ] && REMAIN=0
                log_message "INFO" "用户 User $username VirtualIP: $CLIENT_IP4 下线(ipset) src=$SRC_SET 组 $HASH 剩余=$REMAIN"
                if [ "$REMAIN" = "0" ]; then
                    $IPTABLES -w -D "$CHAIN_NAME" -m set --match-set "$SRC_SET" src -j "$SUB_CHAIN" 2>/dev/null || true
                    $IPTABLES -w -D "$CHAIN_NAME" -m set --match-set "$SRC_SET" src -j ACCEPT 2>/dev/null || true
                    $IPTABLES -w -F "$SUB_CHAIN" 2>/dev/null || true
                    $IPTABLES -w -X "$SUB_CHAIN" 2>/dev/null || true
                    $IPSET destroy "$SRC_SET" 2>/dev/null || true
                    $IPSET destroy "$DST_SET" 2>/dev/null || true
                    log_message "INFO" "组 $HASH 已无在线客户端，回收 src=$SRC_SET dst=$DST_SET chain=$SUB_CHAIN"
                fi
            fi
        fi

        # ---------- IPv6 ----------
        if [ -n "$CIDRS6" ] && [ -n "$CLIENT_IP6" ]; then
            if [ "$HAVE_IP6TABLES" != "1" ]; then
                log_message "WARNING" "用户 User $username 有 IPv6 ACL 但系统无 ip6tables，跳过 IPv6 清理"
            else
                HASH6=$(printf '%s\n' "$CIDRS6" | sha256sum | cut -c1-$HASH_LEN)
                SRC6_SET="ov${SERVER_ID}_6s_${HASH6}"
                DST6_SET="ov${SERVER_ID}_6d_${HASH6}"
                SUB6_CHAIN="ov${SERVER_ID}_6c_${HASH6}"

                if ! $IPSET list "$SRC6_SET" >/dev/null 2>&1; then
                    log_message "WARNING" "IPv6 源集 $SRC6_SET 不存在(可能已清理或哈希不匹配) 用户 User $username VirtualIP6: $CLIENT_IP6"
                else
                    $IPSET del "$SRC6_SET" "$CLIENT_IP6" 2>/dev/null || \
                        log_message "WARNING" "从 $SRC6_SET 移除 $CLIENT_IP6 失败(可能已不存在)"
                    REMAIN6=$($IPSET list --terse "$SRC6_SET" 2>/dev/null | awk '/Number of entries/{print $NF}')
                    [ -z "$REMAIN6" ] && REMAIN6=0
                    log_message "INFO" "用户 User $username VirtualIP6: $CLIENT_IP6 下线(ipset6) src=$SRC6_SET 组 $HASH6 剩余=$REMAIN6"
                    if [ "$REMAIN6" = "0" ]; then
                        $IP6TABLES -w -D "$CHAIN_NAME" -m set --match-set "$SRC6_SET" src -j "$SUB6_CHAIN" 2>/dev/null || true
                        $IP6TABLES -w -D "$CHAIN_NAME" -m set --match-set "$SRC6_SET" src -j ACCEPT 2>/dev/null || true
                        $IP6TABLES -w -F "$SUB6_CHAIN" 2>/dev/null || true
                        $IP6TABLES -w -X "$SUB6_CHAIN" 2>/dev/null || true
                        $IPSET destroy "$SRC6_SET" 2>/dev/null || true
                        $IPSET destroy "$DST6_SET" 2>/dev/null || true
                        log_message "INFO" "IPv6 组 $HASH6 已无在线客户端，回收 src=$SRC6_SET dst=$DST6_SET chain=$SUB6_CHAIN"
                    fi
                fi
            fi
        fi
    ) 9>"$LOCK_FILE"
else
    # 传统模式：逐条删除匹配的规则，不使用 flock
    if [ -n "$CIDRS4" ] && [ -n "$CLIENT_IP4" ]; then
        for cidr in $CIDRS4; do
            command="$IPTABLES -w -D $CHAIN_NAME -s $CLIENT_IP4 -d $cidr -j ACCEPT"
            log_message "INFO" "用户 User $username VirtualIP: $CLIENT_IP4 删除acl命令(ipset不可用-传统模式): $command"
            $command
        done
    fi
    if [ -n "$CIDRS6" ] && [ -n "$CLIENT_IP6" ]; then
        if [ "$HAVE_IP6TABLES" = "1" ]; then
            for cidr in $CIDRS6; do
                command="$IP6TABLES -w -D $CHAIN_NAME -s $CLIENT_IP6 -d $cidr -j ACCEPT"
                log_message "INFO" "用户 User $username VirtualIP6: $CLIENT_IP6 删除acl命令(ipset不可用-传统模式): $command"
                $command
            done
        else
            log_message "WARNING" "用户 User $username 有 IPv6 ACL 但系统无 ip6tables，跳过 IPv6 清理"
        fi
    fi
fi

exit 0
