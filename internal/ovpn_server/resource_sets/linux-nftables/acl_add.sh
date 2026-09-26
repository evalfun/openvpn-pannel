#!/bin/bash
# 由管理面板调用：为指定在线客户端“动态添加”可访问网络（ACL）。nftables 版本（通用 Linux，需 sudo）。
# 使用场景：启用 TOTP 的用户，VPN 认证通过后先不放行 ACL，待其在客户端自助页面
# 输入动态验证码通过后，面板再调用本脚本放行（登出时由 acl_del.sh 回收）。
#
# 标准输入（每行一条）：
#   virtual_ip: 10.8.0.5        # 主地址（有 IPv4 用 IPv4，否则 IPv6）
#   virtual_ip4: 10.8.0.5       # 实际 IPv4 地址（纯 IPv6 客户端为空）
#   virtual_ip6: fc00::2        # 实际 IPv6 地址
#   username: alice
#   4#10.0.0.0/8
#   6#fc00:1::/64
# 面板调用前会替换 __WORKING_DIR__ / __SERVER_ID__ / __SERVER_INTERFACE__。
#
# 本脚本操作的表/链/集合与 nftables 版 client_online.sh 完全一致：
#   inet openvpn_acl_<SERVER_ID> 的 acl 链 + ov<sid>_s_/d_/c_<hash>(IPv4) 与 ov<sid>_6s_/6d_/6c_<hash>(IPv6)

LOG_FILE="__WORKING_DIR__auth.log"
SERVER_ID="__SERVER_ID__"
SERVER_INTERFACE="__SERVER_INTERFACE__"
LOCK_FILE="__WORKING_DIR__acl.lock"

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
    log_message "WARNING" "acl_add(nft) 缺少 virtual_ip，跳过"
    exit 1
fi

CIDRS4=$(printf '%s\n' "$ACL_LINES" | list_v4_cidrs)
CIDRS6=$(printf '%s\n' "$ACL_LINES" | list_v6_cidrs)
if [ -z "$CIDRS4" ] && [ -z "$CIDRS6" ]; then
    log_message "INFO" "用户 User $USERNAME VirtualIP: $CLIENT_IP 无 ACL，无需放行"
    exit 0
fi

if ! $NFT list table inet "$TABLE_NAME" >/dev/null 2>&1; then
    log_message "WARNING" "nftables 表 $TABLE_NAME 不存在，跳过放行(请确认 server_start.sh 已执行)"
    exit 0
fi

log_message "INFO" "用户 User $USERNAME VirtualIP: $CLIENT_IP 动态放行 ACL 开始(nft)"

(
    if ! flock -w 60 9; then
        log_message "WARNING" "用户 User $USERNAME VirtualIP: $CLIENT_IP 获取ACL锁超时，跳过放行(默认DROP)"
        exit 0
    fi

    # ---------- IPv4 ----------
    if [ -n "$CIDRS4" ] && [ -n "$CLIENT_IP4" ]; then
        HASH=$(printf '%s\n' "$CIDRS4" | sha256sum | cut -c1-$HASH_LEN)
        SRC_SET="ov${SERVER_ID}_s_${HASH}"
        DST_SET="ov${SERVER_ID}_d_${HASH}"
        SUB_CHAIN="ov${SERVER_ID}_c_${HASH}"

        $NFT add set inet "$TABLE_NAME" "$SRC_SET" '{ type ipv4_addr ; }' 2>/dev/null || true
        $NFT add set inet "$TABLE_NAME" "$DST_SET" '{ type ipv4_addr ; flags interval ; }' 2>/dev/null || true

        for cidr in $CIDRS4; do
            $NFT add element inet "$TABLE_NAME" "$DST_SET" "{ $cidr }" 2>/dev/null || true
        done
        $NFT add element inet "$TABLE_NAME" "$SRC_SET" "{ $CLIENT_IP4 }" 2>/dev/null || true

        $NFT add chain inet "$TABLE_NAME" "$SUB_CHAIN" 2>/dev/null || true

        if ! $NFT -a list chain inet "$TABLE_NAME" "$ACL_CHAIN" 2>/dev/null | grep -qF "@$SRC_SET"; then
            $NFT add rule inet "$TABLE_NAME" "$ACL_CHAIN" ip saddr "@$SRC_SET" jump "$SUB_CHAIN" 2>/dev/null || \
                log_message "WARNING" "添加主链规则 $ACL_CHAIN @$SRC_SET 失败"
        fi
        if ! $NFT -a list chain inet "$TABLE_NAME" "$SUB_CHAIN" 2>/dev/null | grep -qF "@$DST_SET"; then
            $NFT add rule inet "$TABLE_NAME" "$SUB_CHAIN" ip daddr "@$DST_SET" accept 2>/dev/null || \
                log_message "WARNING" "添加子链规则 $SUB_CHAIN @$DST_SET 失败"
        fi

        log_message "INFO" "用户 User $USERNAME VirtualIP: $CLIENT_IP4 放行完成(nft) src=$SRC_SET dst=$DST_SET chain=$SUB_CHAIN"
    fi

    # ---------- IPv6 ----------
    if [ -n "$CIDRS6" ] && [ -n "$CLIENT_IP6" ]; then
        HASH6=$(printf '%s\n' "$CIDRS6" | sha256sum | cut -c1-$HASH_LEN)
        SRC6_SET="ov${SERVER_ID}_6s_${HASH6}"
        DST6_SET="ov${SERVER_ID}_6d_${HASH6}"
        SUB6_CHAIN="ov${SERVER_ID}_6c_${HASH6}"

        $NFT add set inet "$TABLE_NAME" "$SRC6_SET" '{ type ipv6_addr ; }' 2>/dev/null || true
        $NFT add set inet "$TABLE_NAME" "$DST6_SET" '{ type ipv6_addr ; flags interval ; }' 2>/dev/null || true

        for cidr in $CIDRS6; do
            $NFT add element inet "$TABLE_NAME" "$DST6_SET" "{ $cidr }" 2>/dev/null || true
        done
        $NFT add element inet "$TABLE_NAME" "$SRC6_SET" "{ $CLIENT_IP6 }" 2>/dev/null || true

        $NFT add chain inet "$TABLE_NAME" "$SUB6_CHAIN" 2>/dev/null || true

        if ! $NFT -a list chain inet "$TABLE_NAME" "$ACL_CHAIN" 2>/dev/null | grep -qF "@$SRC6_SET"; then
            $NFT add rule inet "$TABLE_NAME" "$ACL_CHAIN" ip6 saddr "@$SRC6_SET" jump "$SUB6_CHAIN" 2>/dev/null || \
                log_message "WARNING" "添加主链规则 $ACL_CHAIN @$SRC6_SET 失败"
        fi
        if ! $NFT -a list chain inet "$TABLE_NAME" "$SUB6_CHAIN" 2>/dev/null | grep -qF "@$DST6_SET"; then
            $NFT add rule inet "$TABLE_NAME" "$SUB6_CHAIN" ip6 daddr "@$DST6_SET" accept 2>/dev/null || \
                log_message "WARNING" "添加子链规则 $SUB6_CHAIN @$DST6_SET 失败"
        fi

        log_message "INFO" "用户 User $USERNAME VirtualIP6: $CLIENT_IP6 放行完成(nft) src=$SRC6_SET dst=$DST6_SET chain=$SUB6_CHAIN"
    fi
) 9>"$LOCK_FILE"

exit 0
