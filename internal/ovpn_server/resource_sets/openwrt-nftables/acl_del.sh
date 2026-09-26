#!/bin/bash
# 由管理面板调用：为指定在线客户端“动态回收”可访问网络（ACL）。nftables 版本（OpenWrt，无需 sudo）。
# 使用场景：启用 TOTP 的用户在客户端自助页面登出时，面板调用本脚本回收放行。
#
# 标准输入（每行一条）：
#   virtual_ip: 10.8.0.5        # 主地址（有 IPv4 用 IPv4，否则 IPv6）
#   virtual_ip4: 10.8.0.5       # 实际 IPv4 地址（纯 IPv6 客户端为空）
#   virtual_ip6: fc00::2        # 实际 IPv6 地址
#   username: alice
#   4#10.0.0.0/8
#   6#fc00:1::/64
# 面板调用前会替换 __WORKING_DIR__ / __SERVER_ID__ / __SERVER_INTERFACE__。

LOG_FILE="__WORKING_DIR__auth.log"
SERVER_ID="__SERVER_ID__"
SERVER_INTERFACE="__SERVER_INTERFACE__"
LOCK_FILE="__WORKING_DIR__acl.lock"

# OpenWrt：无需 sudo，使用 nft 绝对路径。
NFT_BIN=""
for _nft in /usr/sbin/nft /sbin/nft /usr/bin/nft /bin/nft; do
    if [ -x "$_nft" ]; then NFT_BIN="$_nft"; break; fi
done
[ -z "$NFT_BIN" ] && NFT_BIN="nft"
NFT="$NFT_BIN"

# conntrack 用于清理该客户端的连接跟踪(状态表)。forward 链对 ESTABLISHED,RELATED 一律放行，
# 仅删除 ACL 规则无法断开已建立的连接，必须同时清空其状态表条目。
CONNTRACK=""
for _ct in /usr/sbin/conntrack /sbin/conntrack /usr/bin/conntrack /bin/conntrack; do
    if [ -x "$_ct" ]; then CONNTRACK="$_ct"; break; fi
done
if [ -z "$CONNTRACK" ] && command -v conntrack >/dev/null 2>&1; then
    CONNTRACK="conntrack"
fi

TABLE_NAME="openvpn_acl_$SERVER_ID"
ACL_CHAIN="acl"
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
    log_message "WARNING" "acl_del(nft) 缺少 virtual_ip，跳过"
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

if ! $NFT list table inet "$TABLE_NAME" >/dev/null 2>&1; then
    log_message "WARNING" "nftables 表 $TABLE_NAME 不存在，跳过清理"
    exit 0
fi

log_message "INFO" "用户 User $USERNAME VirtualIP: $CLIENT_IP 动态回收 ACL 开始(nft)"

(
    if ! flock 9; then
        log_message "WARNING" "用户 User $USERNAME VirtualIP: $CLIENT_IP 获取ACL锁超时，跳过nft清理"
        exit 0
    fi

    # ---------- IPv4 ----------
    if [ -n "$CIDRS4" ] && [ -n "$CLIENT_IP4" ]; then
        HASH=$(printf '%s\n' "$CIDRS4" | sha256sum | cut -c1-$HASH_LEN)
        SRC_SET="ov${SERVER_ID}_s_${HASH}"
        DST_SET="ov${SERVER_ID}_d_${HASH}"
        SUB_CHAIN="ov${SERVER_ID}_c_${HASH}"

        if ! $NFT list set inet "$TABLE_NAME" "$SRC_SET" >/dev/null 2>&1; then
            log_message "WARNING" "源集 $SRC_SET 不存在(可能已清理或哈希不匹配) 用户 User $USERNAME VirtualIP: $CLIENT_IP4"
        else
            $NFT delete element inet "$TABLE_NAME" "$SRC_SET" "{ $CLIENT_IP4 }" 2>/dev/null || \
                log_message "WARNING" "从 $SRC_SET 移除 $CLIENT_IP4 失败(可能已不存在)"

            REMAIN=1
            $NFT list set inet "$TABLE_NAME" "$SRC_SET" 2>/dev/null | grep -q 'elements' || REMAIN=0

            log_message "INFO" "用户 User $USERNAME VirtualIP: $CLIENT_IP4 回收(nft) src=$SRC_SET 组 $HASH 剩余=$REMAIN"

            if [ "$REMAIN" = "0" ]; then
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
            log_message "WARNING" "IPv6 源集 $SRC6_SET 不存在(可能已清理或哈希不匹配) 用户 User $USERNAME VirtualIP6: $CLIENT_IP6"
        else
            $NFT delete element inet "$TABLE_NAME" "$SRC6_SET" "{ $CLIENT_IP6 }" 2>/dev/null || \
                log_message "WARNING" "从 $SRC6_SET 移除 $CLIENT_IP6 失败(可能已不存在)"

            REMAIN6=1
            $NFT list set inet "$TABLE_NAME" "$SRC6_SET" 2>/dev/null | grep -q 'elements' || REMAIN6=0

            log_message "INFO" "用户 User $USERNAME VirtualIP6: $CLIENT_IP6 回收(nft) src=$SRC6_SET 组 $HASH6 剩余=$REMAIN6"

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
