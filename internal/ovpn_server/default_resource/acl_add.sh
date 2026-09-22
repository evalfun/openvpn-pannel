#!/bin/bash
# 由管理面板调用：为指定在线客户端“动态添加”其可访问网络（ACL）。
# 使用场景：启用了 TOTP 的用户，VPN 认证通过后先不放行任何 ACL，
# 待其在客户端自助页面输入动态验证码通过后，面板再调用本脚本放行。
#
# 标准输入（每行一条）：
#   virtual_ip: 10.0.1.5
#   username: alice
#   4#10.8.0.0/24
#   4#192.168.1.0/24
# 面板调用前会替换 __WORKING_DIR__ / __SERVER_ID__ / __SERVER_INTERFACE__。

LOG_FILE="__WORKING_DIR__auth.log"
SERVER_ID="__SERVER_ID__"
SERVER_INTERFACE="__SERVER_INTERFACE__"
CHAIN_NAME="openvpn_acl_$SERVER_ID"
LOCK_FILE="__WORKING_DIR__acl.lock"

IPTABLES="sudo /usr/sbin/iptables"
if [ -x /usr/sbin/ipset ]; then
    IPSET="sudo /usr/sbin/ipset"
else
    IPSET="sudo ipset"
fi

# 与 client_online.sh 一致：ipset 可用走聚合模式(带 flock)，否则传统逐条 iptables(无 flock)
HAVE_IPSET=0
if $IPSET list -n >/dev/null 2>&1; then
    HAVE_IPSET=1
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

# 解析标准输入
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
    log_message "WARNING" "acl_add 缺少 virtual_ip，跳过"
    exit 1
fi

CIDRS=$(printf '%s\n' "$ACL_LINES" | list_v4_cidrs)
if [ -z "$CIDRS" ]; then
    log_message "INFO" "用户 User $USERNAME VirtualIP: $CLIENT_IP 无 IPv4 ACL，无需放行"
    exit 0
fi

log_message "INFO" "用户 User $USERNAME VirtualIP: $CLIENT_IP 动态放行 ACL 开始"

if [ "$HAVE_IPSET" = "1" ]; then
    HASH=$(printf '%s\n' "$CIDRS" | sha256sum | cut -c1-$HASH_LEN)
    SRC_SET="ov${SERVER_ID}_s_${HASH}"
    DST_SET="ov${SERVER_ID}_d_${HASH}"
    SUB_CHAIN="ov${SERVER_ID}_c_${HASH}"

    ALLOW_ALL=0
    if printf '%s\n' "$CIDRS" | grep -qE '/0$'; then
        ALLOW_ALL=1
    fi

    (
        if ! flock -w 60 9; then
            log_message "WARNING" "用户 User $USERNAME VirtualIP: $CLIENT_IP 获取ACL锁超时，跳过放行(默认DROP)"
            exit 0
        fi

        $IPSET -! create "$SRC_SET" hash:ip hashsize 1024 maxelem 65536 || \
            log_message "WARNING" "创建源集 $SRC_SET 失败"

        if [ "$ALLOW_ALL" = "1" ]; then
            $IPSET add -! "$SRC_SET" "$CLIENT_IP" 2>/dev/null || \
                log_message "WARNING" "添加客户端 $CLIENT_IP 到 $SRC_SET 失败"
            $IPTABLES -w -C "$CHAIN_NAME" -m set --match-set "$SRC_SET" src -j ACCEPT 2>/dev/null \
                || $IPTABLES -w -A "$CHAIN_NAME" -m set --match-set "$SRC_SET" src -j ACCEPT
            log_message "INFO" "用户 User $USERNAME VirtualIP: $CLIENT_IP 放行完成(ipset-零前缀直连放行) src=$SRC_SET"
        else
            $IPTABLES -w -N "$SUB_CHAIN" 2>/dev/null || true
            $IPSET -! create "$DST_SET" hash:net hashsize 1024 maxelem 4096 || \
                log_message "WARNING" "创建目标集 $DST_SET 失败"
            for cidr in $CIDRS; do
                $IPSET add -! "$DST_SET" "$cidr" 2>/dev/null || \
                    log_message "WARNING" "添加 CIDR $cidr 到 $DST_SET 失败"
            done
            $IPSET add -! "$SRC_SET" "$CLIENT_IP" 2>/dev/null || \
                log_message "WARNING" "添加客户端 $CLIENT_IP 到 $SRC_SET 失败"
            $IPTABLES -w -C "$CHAIN_NAME" -m set --match-set "$SRC_SET" src -j "$SUB_CHAIN" 2>/dev/null \
                || $IPTABLES -w -A "$CHAIN_NAME" -m set --match-set "$SRC_SET" src -j "$SUB_CHAIN"
            $IPTABLES -w -C "$SUB_CHAIN" -m set --match-set "$DST_SET" dst -j ACCEPT 2>/dev/null \
                || $IPTABLES -w -A "$SUB_CHAIN" -m set --match-set "$DST_SET" dst -j ACCEPT
            log_message "INFO" "用户 User $USERNAME VirtualIP: $CLIENT_IP 放行完成(ipset) src=$SRC_SET dst=$DST_SET chain=$SUB_CHAIN"
        fi
    ) 9>"$LOCK_FILE"
else
    for cidr in $CIDRS; do
        command="$IPTABLES -w -A $CHAIN_NAME -s $CLIENT_IP -d $cidr -j ACCEPT"
        log_message "INFO" "用户 User $USERNAME VirtualIP: $CLIENT_IP 设置acl命令(ipset不可用-传统模式): $command"
        $command
    done
fi

exit 0
