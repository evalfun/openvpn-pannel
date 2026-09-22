#!/bin/bash
# 由管理面板调用：为指定在线客户端“动态添加”可访问网络（ACL）。nftables 版本（OpenWrt，无需 sudo）。
# 使用场景：启用 TOTP 的用户，VPN 认证通过后先不放行 ACL，待其在客户端自助页面
# 输入动态验证码通过后，面板再调用本脚本放行（登出时由 acl_del.sh 回收）。
#
# 标准输入（每行一条）：
#   virtual_ip: 10.8.0.5
#   username: alice
#   4#10.0.0.0/8
# 面板调用前会替换 __WORKING_DIR__ / __SERVER_ID__ / __SERVER_INTERFACE__。
#
# 本脚本操作的表/链/集合与 nftables 版 client_online.sh 完全一致：
#   inet openvpn_acl_<SERVER_ID> 的 acl 链 + ov<sid>_s_<hash>/ov<sid>_d_<hash>/ov<sid>_c_<hash>

LOG_FILE="__WORKING_DIR__auth.log"
SERVER_ID="__SERVER_ID__"
SERVER_INTERFACE="__SERVER_INTERFACE__"
LOCK_FILE="__WORKING_DIR__acl.lock"

# OpenWrt：无需 sudo，直接使用 nft 绝对路径。
NFT_BIN=""
for _nft in /usr/sbin/nft /sbin/nft /usr/bin/nft /bin/nft; do
    if [ -x "$_nft" ]; then NFT_BIN="$_nft"; break; fi
done
[ -z "$NFT_BIN" ] && NFT_BIN="nft"
NFT="$NFT_BIN"

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

CLIENT_IP=""
USERNAME=""
ACL_LINES=""
while IFS= read -r _line; do
    _line="${_line%$'\r'}"
    case "$_line" in
        virtual_ip:*)
            CLIENT_IP=$(printf '%s\n' "$_line" | sed -n 's/^virtual_ip:[[:space:]]*//p' | tr -d ' ')
            ;;
        username:*)
            USERNAME=$(printf '%s\n' "$_line" | sed -n 's/^username:[[:space:]]*//p')
            ;;
        4#*)
            ACL_LINES="${ACL_LINES}${_line}"$'\n'
            ;;
    esac
done

if [ -z "$CLIENT_IP" ]; then
    log_message "WARNING" "acl_add(nft) 缺少 virtual_ip，跳过"
    exit 1
fi

CIDRS=$(printf '%s\n' "$ACL_LINES" | list_v4_cidrs)
if [ -z "$CIDRS" ]; then
    log_message "INFO" "用户 User $USERNAME VirtualIP: $CLIENT_IP 无 IPv4 ACL，无需放行"
    exit 0
fi

if ! $NFT list table inet "$TABLE_NAME" >/dev/null 2>&1; then
    log_message "WARNING" "nftables 表 $TABLE_NAME 不存在，跳过放行(请确认 server_start.sh 已执行)"
    exit 0
fi

HASH=$(printf '%s\n' "$CIDRS" | sha256sum | cut -c1-$HASH_LEN)
SRC_SET="ov${SERVER_ID}_s_${HASH}"
DST_SET="ov${SERVER_ID}_d_${HASH}"
SUB_CHAIN="ov${SERVER_ID}_c_${HASH}"

log_message "INFO" "用户 User $USERNAME VirtualIP: $CLIENT_IP 动态放行 ACL 开始(nft)"

(
    if ! flock 9; then
        log_message "WARNING" "用户 User $USERNAME VirtualIP: $CLIENT_IP 获取ACL锁超时，跳过放行(默认DROP)"
        exit 0
    fi

    $NFT add set inet "$TABLE_NAME" "$SRC_SET" '{ type ipv4_addr ; }' 2>/dev/null || true
    $NFT add set inet "$TABLE_NAME" "$DST_SET" '{ type ipv4_addr ; flags interval ; }' 2>/dev/null || true

    for cidr in $CIDRS; do
        $NFT add element inet "$TABLE_NAME" "$DST_SET" "{ $cidr }" 2>/dev/null || true
    done
    $NFT add element inet "$TABLE_NAME" "$SRC_SET" "{ $CLIENT_IP }" 2>/dev/null || true

    $NFT add chain inet "$TABLE_NAME" "$SUB_CHAIN" 2>/dev/null || true

    if ! $NFT -a list chain inet "$TABLE_NAME" "$ACL_CHAIN" 2>/dev/null | grep -qF "@$SRC_SET"; then
        $NFT add rule inet "$TABLE_NAME" "$ACL_CHAIN" ip saddr "@$SRC_SET" jump "$SUB_CHAIN" 2>/dev/null || \
            log_message "WARNING" "添加主链规则 $ACL_CHAIN @$SRC_SET 失败"
    fi
    if ! $NFT -a list chain inet "$TABLE_NAME" "$SUB_CHAIN" 2>/dev/null | grep -qF "@$DST_SET"; then
        $NFT add rule inet "$TABLE_NAME" "$SUB_CHAIN" ip daddr "@$DST_SET" accept 2>/dev/null || \
            log_message "WARNING" "添加子链规则 $SUB_CHAIN @$DST_SET 失败"
    fi

    log_message "INFO" "用户 User $USERNAME VirtualIP: $CLIENT_IP 放行完成(nft) src=$SRC_SET dst=$DST_SET chain=$SUB_CHAIN"
) 9>"$LOCK_FILE"

exit 0
