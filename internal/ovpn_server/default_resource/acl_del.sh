#!/bin/bash
# 由管理面板调用：为指定在线客户端“动态删除”其已放行的可访问网络（ACL）。
# 使用场景：启用 TOTP 的用户在客户端自助页面点击登出时，面板调用本脚本回收放行。
#
# 标准输入（每行一条）：
#   virtual_ip: 10.0.1.5        # 主地址（有 IPv4 用 IPv4，否则 IPv6）
#   virtual_ip4: 10.0.1.5       # 实际 IPv4 地址（纯 IPv6 客户端为空）
#   virtual_ip6: fc00::2        # 实际 IPv6 地址
#   username: alice
#   4#10.8.0.0/24
#   6#fc00:1::/64
# 面板调用前会替换 __WORKING_DIR__ / __SERVER_ID__ / __SERVER_INTERFACE__。

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

# conntrack 用于清理该客户端的连接跟踪(状态表)。ACL 链对 ESTABLISHED,RELATED 一律放行，
# 仅删除 ACL 规则无法断开已建立的连接，必须同时清空其状态表条目。
CONNTRACK=""
for _ct in /usr/sbin/conntrack /sbin/conntrack /usr/bin/conntrack /bin/conntrack; do
    if [ -x "$_ct" ]; then CONNTRACK="sudo $_ct"; break; fi
done
if [ -z "$CONNTRACK" ] && command -v conntrack >/dev/null 2>&1; then
    CONNTRACK="sudo conntrack"
fi

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

# 清空指定客户端 IP 的连接跟踪(状态表)条目（源/目的方向）。best-effort，失败不影响 ACL 回收。
flush_conntrack() {
    local ip="$1"
    [ -z "$ip" ] && return 0
    if [ -z "$CONNTRACK" ]; then
        log_message "WARNING" "conntrack 不可用，跳过清理 $ip 的连接跟踪(状态表)"
        return 0
    fi
    local cnt
    cnt=$($CONNTRACK -D -s "$ip" 2>/dev/null | wc -l)
    $CONNTRACK -D -d "$ip" 2>/dev/null || true
    log_message "INFO" "已清理客户端 $ip 的连接跟踪(状态表) 删除源条目数=${cnt}"
}

CLIENT_IP=""
CLIENT_IP4=""
CLIENT_IP6=""
USERNAME=""
ACL_LINES=""
while IFS= read -r _line; do
    _line="${_line%$'\r'}"
    case "$_line" in
        virtual_ip4:*)
            CLIENT_IP4=$(printf '%s\n' "$_line" | sed -n 's/^virtual_ip4:[[:space:]]*//p' | tr -d ' ')
            ;;
        virtual_ip6:*)
            CLIENT_IP6=$(printf '%s\n' "$_line" | sed -n 's/^virtual_ip6:[[:space:]]*//p' | tr -d ' ')
            ;;
        virtual_ip:*)
            CLIENT_IP=$(printf '%s\n' "$_line" | sed -n 's/^virtual_ip:[[:space:]]*//p' | tr -d ' ')
            ;;
        username:*)
            USERNAME=$(printf '%s\n' "$_line" | sed -n 's/^username:[[:space:]]*//p')
            ;;
        4#*|6#*)
            ACL_LINES="${ACL_LINES}${_line}"$'\n'
            ;;
    esac
done

if [ -z "$CLIENT_IP" ]; then
    if [ -n "$CLIENT_IP4" ]; then CLIENT_IP="$CLIENT_IP4"; else CLIENT_IP="$CLIENT_IP6"; fi
fi
if [ -z "$CLIENT_IP" ]; then
    log_message "WARNING" "acl_del 缺少 virtual_ip，跳过"
    exit 1
fi

# 先回收 ACL，再清空连接跟踪(状态表)：用 EXIT trap 保证在所有回收路径之后执行，
# 避免先清 conntrack 后、ACL 尚未删除期间新建的连接被漏掉。
trap 'flush_conntrack "$CLIENT_IP4"; flush_conntrack "$CLIENT_IP6"' EXIT

CIDRS4=$(printf '%s\n' "$ACL_LINES" | list_v4_cidrs)
CIDRS6=$(printf '%s\n' "$ACL_LINES" | list_v6_cidrs)
if [ -z "$CIDRS4" ] && [ -z "$CIDRS6" ]; then
    log_message "WARNING" "用户 User $USERNAME VirtualIP: $CLIENT_IP 无 ACL 记录，跳过清理"
    exit 0
fi

log_message "INFO" "用户 User $USERNAME VirtualIP: $CLIENT_IP 动态回收 ACL 开始"

if [ "$HAVE_IPSET" = "1" ]; then
    (
        if ! flock -w 60 9; then
            log_message "WARNING" "用户 User $USERNAME VirtualIP: $CLIENT_IP 获取ACL锁超时，跳过ipset清理"
            exit 0
        fi

        # ---------- IPv4 ----------
        if [ -n "$CIDRS4" ] && [ -n "$CLIENT_IP4" ]; then
            HASH=$(printf '%s\n' "$CIDRS4" | sha256sum | cut -c1-$HASH_LEN)
            SRC_SET="ov${SERVER_ID}_s_${HASH}"
            DST_SET="ov${SERVER_ID}_d_${HASH}"
            SUB_CHAIN="ov${SERVER_ID}_c_${HASH}"

            if ! $IPSET list "$SRC_SET" >/dev/null 2>&1; then
                log_message "WARNING" "源集 $SRC_SET 不存在(可能已清理或哈希不匹配) 用户 User $USERNAME VirtualIP: $CLIENT_IP4"
            else
                $IPSET del "$SRC_SET" "$CLIENT_IP4" 2>/dev/null || \
                    log_message "WARNING" "从 $SRC_SET 移除 $CLIENT_IP4 失败(可能已不存在)"
                REMAIN=$($IPSET list --terse "$SRC_SET" 2>/dev/null | awk '/Number of entries/{print $NF}')
                [ -z "$REMAIN" ] && REMAIN=0
                log_message "INFO" "用户 User $USERNAME VirtualIP: $CLIENT_IP4 回收(ipset) src=$SRC_SET 组 $HASH 剩余=$REMAIN"
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
                log_message "WARNING" "用户 User $USERNAME 有 IPv6 ACL 但系统无 ip6tables，跳过 IPv6 清理"
            else
                HASH6=$(printf '%s\n' "$CIDRS6" | sha256sum | cut -c1-$HASH_LEN)
                SRC6_SET="ov${SERVER_ID}_6s_${HASH6}"
                DST6_SET="ov${SERVER_ID}_6d_${HASH6}"
                SUB6_CHAIN="ov${SERVER_ID}_6c_${HASH6}"

                if ! $IPSET list "$SRC6_SET" >/dev/null 2>&1; then
                    log_message "WARNING" "IPv6 源集 $SRC6_SET 不存在(可能已清理或哈希不匹配) 用户 User $USERNAME VirtualIP6: $CLIENT_IP6"
                else
                    $IPSET del "$SRC6_SET" "$CLIENT_IP6" 2>/dev/null || \
                        log_message "WARNING" "从 $SRC6_SET 移除 $CLIENT_IP6 失败(可能已不存在)"
                    REMAIN6=$($IPSET list --terse "$SRC6_SET" 2>/dev/null | awk '/Number of entries/{print $NF}')
                    [ -z "$REMAIN6" ] && REMAIN6=0
                    log_message "INFO" "用户 User $USERNAME VirtualIP6: $CLIENT_IP6 回收(ipset6) src=$SRC6_SET 组 $HASH6 剩余=$REMAIN6"
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
    if [ -n "$CIDRS4" ] && [ -n "$CLIENT_IP4" ]; then
        for cidr in $CIDRS4; do
            command="$IPTABLES -w -D $CHAIN_NAME -s $CLIENT_IP4 -d $cidr -j ACCEPT"
            log_message "INFO" "用户 User $USERNAME VirtualIP: $CLIENT_IP4 删除acl命令(ipset不可用-传统模式): $command"
            $command
        done
    fi
    if [ -n "$CIDRS6" ] && [ -n "$CLIENT_IP6" ]; then
        if [ "$HAVE_IP6TABLES" = "1" ]; then
            for cidr in $CIDRS6; do
                command="$IP6TABLES -w -D $CHAIN_NAME -s $CLIENT_IP6 -d $cidr -j ACCEPT"
                log_message "INFO" "用户 User $USERNAME VirtualIP6: $CLIENT_IP6 删除acl命令(ipset不可用-传统模式): $command"
                $command
            done
        else
            log_message "WARNING" "用户 User $USERNAME 有 IPv6 ACL 但系统无 ip6tables，跳过 IPv6 清理"
        fi
    fi
fi

exit 0
