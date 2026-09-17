#!/bin/bash

INTERNAL_API="__INTERNAL_API__"
LOG_FILE="__WORKING_DIR__auth.log"
SERVER_ID="__SERVER_ID__"

SERVER_INTERFACE="__SERVER_INTERFACE__"
CHAIN_NAME="openvpn_acl_$SERVER_ID"
LOCK_FILE="__WORKING_DIR__acl.lock"

IPTABLES="/usr/sbin/iptables"
if [ -x /usr/sbin/ipset ]; then
    IPSET="/usr/sbin/ipset"
else
    IPSET="ipset"
fi

# 探测 ipset（二进制 + 内核模块）。可用则走 ipset 聚合模式并用 flock 串行化；
# 不可用则回落传统的“每条 ACL 一条 iptables 规则”模式（此模式不使用 flock）。
# 注意：tc 限速与 ipset 无关，始终占用同一把 flock 串行化。
# 注意：ipset --version 在缺少内核权限时也会失败，故用 list -n 做功能性探测。
HAVE_IPSET=0
if $IPSET list -n >/dev/null 2>&1; then
    HAVE_IPSET=1
fi

# ipset 模式下，每个 ACL 组用“子链 + 两个单 match-set 规则”表达 (源 ∈ 客户端集) AND (目的 ∈ CIDR集)：
#   ov<sid>_s_<hash>  源集(hash:ip)  放该组所有在线客户端IP
#   ov<sid>_d_<hash>  目标集(hash:net) 放该组允许的 CIDR
#   ov<sid>_c_<hash>  子链
#   主链:  -m set --match-set 源集 src        -j 子链
#   子链:  -m set --match-set 目标集 dst -j ACCEPT
# 之所以拆两条：iptables-nft 限制一条规则只能出现一次 --match-set。
# hash 由排序去重后的 IPv4 CIDR 集合算出，ACL 相同的客户端共享同一组对象，
# 于是主链规则数 = 不同 ACL 组数，与在线用户数无关。
HASH_LEN=16

log_message() {
    local level="$1"
    local message="$2"
    echo "$(date '+%Y-%m-%d %H:%M:%S') [${level}] ${message}" >> "$LOG_FILE"
}

# 从 "4#<cidr>" 形式的行里提取、去空白、排序去重后的 IPv4 CIDR 列表（每行一个）
list_v4_cidrs() {
    tr -d '\r' \
        | sed -n 's/^[[:space:]]*4#//p' \
        | sed 's/[[:space:]]*$//' \
        | grep -v '^[[:space:]]*$' \
        | sort -u
}

log_message "INFO" "用户上线 CertificateName $common_name User $username VirtualIP: $ifconfig_pool_remote_ip ClientIP: $untrusted_ip:$untrusted_port "
encoded_username=$(echo -n "$username" | base64 -w 0 | tr -d '\n')

# 执行上线命令
request_body="server_id: $SERVER_ID
username: $encoded_username
real_ip_addr: $untrusted_ip:$untrusted_port
client_cert_name: $common_name
virtual_ip_addr: $ifconfig_pool_remote_ip"

curl -s -X POST -d "$request_body" http://$INTERNAL_API/user/online &> /dev/null

# ===== 带宽限速 (tc) =====
# 上传 = 服务器 -> 客户端：在 tun 出方向用 HTB 按目的 IP 整形；
# 下载 = 客户端 -> 服务器：在 tun 入方向用 police 按源 IP 限速。
# 速率由服务器按用户策略(先看用户，再看活跃用户组)算出，单位 KB/s，0 = 不限速。
# OpenWrt：无需 sudo，直接使用 tc；需安装 tc 及内核模块 sch_htb/cls_u32/act_police/sch_ingress。
TC_BIN=""
for _tc in /sbin/tc /usr/sbin/tc /usr/bin/tc /bin/tc; do
    if [ -x "$_tc" ]; then TC_BIN="$_tc"; break; fi
done
[ -z "$TC_BIN" ] && TC_BIN="tc"

rate_request="server_id: $SERVER_ID
username: $encoded_username"
rate_result=$(curl -s -X POST -d "$rate_request" http://$INTERNAL_API/user/ratelimit/get)
UPLOAD_KB=$(printf '%s\n' "$rate_result" | sed -n 's/^upload_kb:[[:space:]]*//p' | head -n1)
DOWNLOAD_KB=$(printf '%s\n' "$rate_result" | sed -n 's/^download_kb:[[:space:]]*//p' | head -n1)
case "$UPLOAD_KB" in ''|*[!0-9]*) UPLOAD_KB=0 ;; esac
case "$DOWNLOAD_KB" in ''|*[!0-9]*) DOWNLOAD_KB=0 ;; esac

CLIENT_IP="$ifconfig_pool_remote_ip"
TC_DEV="$SERVER_INTERFACE"

log_message "INFO" "用户 User $username VirtualIP: $CLIENT_IP 生效限速 上传=${UPLOAD_KB}KB/s 下载=${DOWNLOAD_KB}KB/s (0=不限速)"

if [ -n "$CLIENT_IP" ] && { [ "$UPLOAD_KB" -gt 0 ] || [ "$DOWNLOAD_KB" -gt 0 ]; }; then
    if [ ! -d "/sys/class/net/$TC_DEV" ]; then
        log_message "WARNING" "接口 $TC_DEV 不存在，跳过 tc 限速 用户 User $username VirtualIP: $CLIENT_IP"
    else
        # tc 操作放进 flock 临界区串行化，避免与并发的上/下线进程互相覆盖 qdisc/class/filter
        (
        if ! flock 9; then
            log_message "WARNING" "用户 User $username VirtualIP: $CLIENT_IP 获取限速锁超时，跳过 tc 限速"
            exit 0
        fi
        # 由虚拟 IPv4 末两段推导唯一 classid/prio(同一 /16 内唯一)，避开保留的 0 与默认类 9999
        _c=$(printf '%s' "$CLIENT_IP" | cut -d. -f3)
        _d=$(printf '%s' "$CLIENT_IP" | cut -d. -f4)
        MINOR=$(( (${_c:-0} << 8) | ${_d:-0} ))
        [ "$MINOR" -le 0 ] && MINOR=1
        [ "$MINOR" -ge 65535 ] && MINOR=65534
        [ "$MINOR" -eq 9999 ] && MINOR=9998

        if [ "$UPLOAD_KB" -gt 0 ]; then
            # 根 qdisc 仅在缺失时创建，避免清掉其它在线客户端的类与过滤器
            if ! $TC_BIN qdisc show dev "$TC_DEV" 2>/dev/null | grep -q 'htb 1:'; then
                $TC_BIN qdisc add dev "$TC_DEV" root handle 1: htb default 9999 2>/dev/null
                $TC_BIN class add dev "$TC_DEV" parent 1: classid 1:9999 htb rate 100gbit ceil 100gbit burst 15k cburst 15k quantum 1500 2>/dev/null
            fi
            UPLOAD_KBIT=$((UPLOAD_KB * 8))
            # burst 按速率估算，最小 3000 字节，避免过小被 tc 拒绝
            UPLOAD_BURST=$((UPLOAD_KBIT * 12))
            [ "$UPLOAD_BURST" -lt 3000 ] && UPLOAD_BURST=3000
            $TC_BIN class add dev "$TC_DEV" parent 1: classid 1:$MINOR htb rate ${UPLOAD_KBIT}kbit ceil ${UPLOAD_KBIT}kbit burst ${UPLOAD_BURST} cburst ${UPLOAD_BURST} quantum 1500 2>/dev/null \
                || $TC_BIN class change dev "$TC_DEV" classid 1:$MINOR htb rate ${UPLOAD_KBIT}kbit ceil ${UPLOAD_KBIT}kbit burst ${UPLOAD_BURST} cburst ${UPLOAD_BURST} quantum 1500
            # 先按 prio 清除该客户端旧过滤器再加，保证重复上线不产生重复过滤项(prio 唯一即只影响本客户端)
            $TC_BIN filter del dev "$TC_DEV" parent 1: protocol ip prio $MINOR u32 2>/dev/null || true
            $TC_BIN filter add dev "$TC_DEV" parent 1: protocol ip prio $MINOR u32 match ip dst ${CLIENT_IP}/32 flowid 1:$MINOR
            log_message "INFO" "用户 User $username VirtualIP: $CLIENT_IP 上传限速已设置 ${UPLOAD_KB}KB/s classid 1:$MINOR"
        fi

        if [ "$DOWNLOAD_KB" -gt 0 ]; then
            $TC_BIN qdisc add dev "$TC_DEV" handle ffff: ingress 2>/dev/null || true
            DOWNLOAD_KBIT=$((DOWNLOAD_KB * 8))
            DOWNLOAD_BURST=$((DOWNLOAD_KBIT * 12))
            [ "$DOWNLOAD_BURST" -lt 3000 ] && DOWNLOAD_BURST=3000
            $TC_BIN filter del dev "$TC_DEV" parent ffff: protocol ip prio $MINOR u32 2>/dev/null || true
            $TC_BIN filter add dev "$TC_DEV" parent ffff: protocol ip prio $MINOR u32 match ip src ${CLIENT_IP}/32 police rate ${DOWNLOAD_KBIT}kbit burst ${DOWNLOAD_BURST} drop flowid :1
            log_message "INFO" "用户 User $username VirtualIP: $CLIENT_IP 下载限速已设置 ${DOWNLOAD_KB}KB/s"
        fi
        ) 9>"$LOCK_FILE"
    fi
fi

# 根据用户组获取防火墙权限（同时把本次上线使用的 ACL 记录进 AddedServerACLRecord，
# 下线时原样取回用于还原同一组哈希，避免会话中途改组 ACL 导致拆不掉）
request_body="server_id: $SERVER_ID
username: $encoded_username
real_ip_addr: $untrusted_ip:$untrusted_port
virtual_ip_addr: $ifconfig_pool_remote_ip"
result=$(curl -s -X POST -d "$request_body" http://$INTERNAL_API/user/acl/get)

log_message "DEBUG" "acl内容 $result"

CIDRS=$(printf '%s\n' "$result" | list_v4_cidrs)
if [ -z "$CIDRS" ]; then
    log_message "INFO" "用户 User $username VirtualIP: $ifconfig_pool_remote_ip 无 IPv4 ACL，跳过放行(仅 IPv6 或空)"
    exit 0
fi

CLIENT_IP="$ifconfig_pool_remote_ip"

if [ "$HAVE_IPSET" = "1" ]; then
    HASH=$(printf '%s\n' "$CIDRS" | sha256sum | cut -c1-$HASH_LEN)
    SRC_SET="ov${SERVER_ID}_s_${HASH}"
    DST_SET="ov${SERVER_ID}_d_${HASH}"
    SUB_CHAIN="ov${SERVER_ID}_c_${HASH}"

    # ipset 的 hash:net 不允许零前缀网段（如 0.0.0.0/0，会报 "CIDR parameter ... invalid"）。
    # 遇到这种 ACL 时目标已是“全部”，无需也无法用目标集表达：退化为零前缀直连放行，
    # 只维护源集，主链直接 ACCEPT，不创建目标集与子链。
    ALLOW_ALL=0
    if printf '%s\n' "$CIDRS" | grep -qE '/0$'; then
        ALLOW_ALL=1
    fi

    # 临界区：建子链/建集/加成员/加规则，串行化以防并发上线下线把系统状态改乱
    (
        if ! flock 9; then
            log_message "WARNING" "用户 User $username VirtualIP: $CLIENT_IP 获取ACL锁超时，跳过放行(默认DROP，失败即拒绝)"
            exit 0
        fi

        $IPSET -! create "$SRC_SET" hash:ip hashsize 1024 maxelem 65536 || \
            log_message "WARNING" "创建源集 $SRC_SET 失败"

        if [ "$ALLOW_ALL" = "1" ]; then
            $IPSET add -! "$SRC_SET" "$CLIENT_IP" 2>/dev/null || \
                log_message "WARNING" "添加客户端 $CLIENT_IP 到 $SRC_SET 失败"

            $IPTABLES -w -C "$CHAIN_NAME" -m set --match-set "$SRC_SET" src -j ACCEPT 2>/dev/null \
                || $IPTABLES -w -A "$CHAIN_NAME" -m set --match-set "$SRC_SET" src -j ACCEPT

            log_message "INFO" "用户 User $username VirtualIP: $CLIENT_IP 放行完成(ipset-零前缀直连放行) src=$SRC_SET 主链直接ACCEPT"
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

            log_message "INFO" "用户 User $username VirtualIP: $CLIENT_IP 放行完成(ipset) src=$SRC_SET dst=$DST_SET chain=$SUB_CHAIN"
        fi
    ) 9>"$LOCK_FILE"
else
    # 传统模式：每条 ACL 一条规则，不使用 flock
    for cidr in $CIDRS; do
        command="$IPTABLES -w -A $CHAIN_NAME -s $CLIENT_IP -d $cidr -j ACCEPT"
        log_message "INFO" "用户 User $username VirtualIP: $CLIENT_IP 设置acl命令(ipset不可用-传统模式): $command"
        $command
    done
fi

exit 0
