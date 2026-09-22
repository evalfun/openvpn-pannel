#!/bin/bash
# 由管理面板调用：为指定在线客户端“动态删除”其已放行的可访问网络（ACL）。
# 使用场景：启用 TOTP 的用户在客户端自助页面点击登出时，面板调用本脚本回收放行。
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
    log_message "WARNING" "acl_del 缺少 virtual_ip，跳过"
    exit 1
fi

# 先回收 ACL，再清空连接跟踪(状态表)：用 EXIT trap 保证在所有回收路径之后执行，
# 避免先清 conntrack 后、ACL 尚未删除期间新建的连接被漏掉。
trap 'flush_conntrack "$CLIENT_IP"' EXIT

CIDRS=$(printf '%s\n' "$ACL_LINES" | list_v4_cidrs)
if [ -z "$CIDRS" ]; then
    log_message "WARNING" "用户 User $USERNAME VirtualIP: $CLIENT_IP 无 IPv4 ACL 记录，跳过清理"
    exit 0
fi

log_message "INFO" "用户 User $USERNAME VirtualIP: $CLIENT_IP 动态回收 ACL 开始"

if [ "$HAVE_IPSET" = "1" ]; then
    HASH=$(printf '%s\n' "$CIDRS" | sha256sum | cut -c1-$HASH_LEN)
    SRC_SET="ov${SERVER_ID}_s_${HASH}"
    DST_SET="ov${SERVER_ID}_d_${HASH}"
    SUB_CHAIN="ov${SERVER_ID}_c_${HASH}"

    (
        if ! flock -w 60 9; then
            log_message "WARNING" "用户 User $USERNAME VirtualIP: $CLIENT_IP 获取ACL锁超时，跳过ipset清理"
            exit 0
        fi

        if ! $IPSET list "$SRC_SET" >/dev/null 2>&1; then
            log_message "WARNING" "源集 $SRC_SET 不存在(可能已清理或哈希不匹配) 用户 User $USERNAME VirtualIP: $CLIENT_IP"
            exit 0
        fi

        $IPSET del "$SRC_SET" "$CLIENT_IP" 2>/dev/null || \
            log_message "WARNING" "从 $SRC_SET 移除 $CLIENT_IP 失败(可能已不存在)"

        REMAIN=$($IPSET list --terse "$SRC_SET" 2>/dev/null | awk '/Number of entries/{print $NF}')
        [ -z "$REMAIN" ] && REMAIN=0

        log_message "INFO" "用户 User $USERNAME VirtualIP: $CLIENT_IP 回收(ipset) src=$SRC_SET 组 $HASH 剩余=$REMAIN"

        if [ "$REMAIN" = "0" ]; then
            $IPTABLES -w -D "$CHAIN_NAME" -m set --match-set "$SRC_SET" src -j "$SUB_CHAIN" 2>/dev/null || true
            $IPTABLES -w -D "$CHAIN_NAME" -m set --match-set "$SRC_SET" src -j ACCEPT 2>/dev/null || true
            $IPTABLES -w -F "$SUB_CHAIN" 2>/dev/null || true
            $IPTABLES -w -X "$SUB_CHAIN" 2>/dev/null || true
            $IPSET destroy "$SRC_SET" 2>/dev/null || true
            $IPSET destroy "$DST_SET" 2>/dev/null || true
            log_message "INFO" "组 $HASH 已无在线客户端，回收 src=$SRC_SET dst=$DST_SET chain=$SUB_CHAIN"
        fi
    ) 9>"$LOCK_FILE"
else
    for cidr in $CIDRS; do
        command="$IPTABLES -w -D $CHAIN_NAME -s $CLIENT_IP -d $cidr -j ACCEPT"
        log_message "INFO" "用户 User $USERNAME VirtualIP: $CLIENT_IP 删除acl命令(ipset不可用-传统模式): $command"
        $command
    done
fi

exit 0
