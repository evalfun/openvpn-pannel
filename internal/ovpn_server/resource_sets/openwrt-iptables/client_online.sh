#!/bin/bash

INTERNAL_API="__INTERNAL_API__"
LOG_FILE="__WORKING_DIR__auth.log"
SERVER_ID="__SERVER_ID__"

SERVER_INTERFACE="__SERVER_INTERFACE__"
CHAIN_NAME="openvpn_acl_$SERVER_ID"
LOCK_FILE="__WORKING_DIR__acl.lock"

IPTABLES="/usr/sbin/iptables"
IP6TABLES="/usr/sbin/ip6tables"
if [ -x /usr/sbin/ipset ]; then
    IPSET="/usr/sbin/ipset"
else
    IPSET="/usr/sbin/ipset"
fi

# 探测 ipset（二进制 + 内核模块）。可用则走 ipset 聚合模式并用 flock 串行化；
# 不可用则回落传统的“每条 ACL 一条 iptables/ip6tables 规则”模式（此模式不使用 flock）。
# 注意：tc 限速与 ipset 无关，始终占用同一把 flock 串行化。
# 注意：ipset --version 在缺少内核权限时也会失败，故用 list -n 做功能性探测。
HAVE_IPSET=0
if $IPSET list -n >/dev/null 2>&1; then
    HAVE_IPSET=1
fi

# IPv6 是否可用（ip6tables 存在）。
HAVE_IP6TABLES=0
if [ -x /usr/sbin/ip6tables ] || command -v ip6tables >/dev/null 2>&1; then
    HAVE_IP6TABLES=1
fi

# ipset 模式下，每个 ACL 组用“子链 + 两个单 match-set 规则”表达 (源 ∈ 客户端集) AND (目的 ∈ CIDR集)：
#   ov<sid>_s_<hash>   源集(hash:ip)  放该组所有在线客户端IP
#   ov<sid>_d_<hash>   目标集(hash:net) 放该组允许的 CIDR
#   ov<sid>_c_<hash>   子链
# IPv6 用同名前缀 + 6 区分：ov<sid>_6s_/6d_/6c_，配合 ip6tables 与 ipset family inet6。
# hash 由排序去重后的 CIDR 集合算出，ACL 相同的客户端共享同一组对象，
# 于是主链规则数 = 不同 ACL 组数，与在线用户数无关。
HASH_LEN=16

log_message() {
    local level="$1"
    local message="$2"
    echo "$(date '+%Y-%m-%d %H:%M:%S') [${level}] ${message}" >> "$LOG_FILE"
}

# 从 "4#<cidr>" / "6#<cidr>" 形式的行里提取、去空白、排序去重后的 CIDR 列表（每行一个）
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

# 由虚拟 IPv4 末两段推导 classid/prio（同一 /24 内唯一），避开保留的 0 与默认类 9999。
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
# 纯 IPv6 客户端：由 IPv6 末 16 位推导独立 classid/prio。
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

log_message "INFO" "用户上线 CertificateName $common_name User $username VirtualIP: $CLIENT_IP4 VirtualIP6: $CLIENT_IP6 ClientIP: $untrusted_ip:$untrusted_port "
encoded_username=$(echo -n "$username" | base64 -w 0 | tr -d '\n')

# 执行上线命令
request_body="server_id: $SERVER_ID
username: $encoded_username
real_ip_addr: $untrusted_ip:$untrusted_port
client_cert_name: $common_name
virtual_ip_addr: $CLIENT_IP4
virtual_ip6_addr: $CLIENT_IP6"

curl -s -X POST -d "$request_body" http://$INTERNAL_API/user/online &> /dev/null

# ===== 带宽限速 (tc) =====
# 上传 = 服务器 -> 客户端：在 tun 出方向用 HTB 按目的 IP 整形；
# 下载 = 客户端 -> 服务器：在 tun 入方向限速。入方向无法直接整形，优先把包 redirect 到
#   ifb 设备（ovpnrl<服务器ID>）再用 HTB 整形（依赖 ifb + act_mirred，OpenWrt 常见）；
#   若 ifb/act_mirred 不可用则回退到 ingress police（依赖 act_police）；
#   两者都不可用时记录 WARNING 并跳过下载限速（不再静默失败）。
# 速率由服务器按用户策略(先看用户，再看活跃用户组)算出，单位 KB/s，0 = 不限速。
# IPv4 与 IPv6 复用同一 classid（由 IPv4 末两段推导）；纯 IPv6 客户端使用由 IPv6 推导的独立 classid。
TC_BIN=""
for _tc in /sbin/tc /usr/sbin/tc /usr/bin/tc /bin/tc; do
    if [ -x "$_tc" ]; then TC_BIN="$_tc"; break; fi
done
[ -z "$TC_BIN" ] && TC_BIN="tc"

IP_BIN=""
for _ip in /sbin/ip /usr/sbin/ip /usr/bin/ip /bin/ip; do
    if [ -x "$_ip" ]; then IP_BIN="$_ip"; break; fi
done
[ -z "$IP_BIN" ] && IP_BIN="ip"

TC_DEV="$SERVER_INTERFACE"

# 下载（入方向）限速方式：ifb 优先，其次 police。
# ifb 设备名带上 server_id 前缀且加长（ovpnrl=openvpn rate limit），避免与系统自带的
# ifb0/ifb1 或其它程序的 ifb 设备重名。
IFB_DEV="ovpnrl${SERVER_ID}"
DOWNLOAD_METHOD="none"

# 探测前必须先创建 ingress qdisc：向 parent ffff: 挂过滤器前若没有 ffff: qdisc，
# 内核会返回 "RTNETLINK answers: Invalid argument"，从而把可用方案误判为不可用。
# 记录本次探测是否由本脚本新建了 ingress，失败时要回滚，避免留下空 qdisc。
INGRESS_CREATED=0
if ! $TC_BIN qdisc show dev "$TC_DEV" 2>/dev/null | grep -q 'ingress'; then
    if $TC_BIN qdisc add dev "$TC_DEV" handle ffff: ingress >/dev/null 2>&1; then
        INGRESS_CREATED=1
    fi
fi
if $TC_BIN qdisc show dev "$TC_DEV" 2>/dev/null | grep -q 'ingress'; then
    if $IP_BIN link show "$IFB_DEV" >/dev/null 2>&1 || $IP_BIN link add "$IFB_DEV" type ifb >/dev/null 2>&1; then
        # ifb 设备可用；再确认 act_mirred 可用（尝试加一条 mirred 规则，失败则回退）
        $IP_BIN link set "$IFB_DEV" up >/dev/null 2>&1 || true
        if $TC_BIN filter add dev "$TC_DEV" parent ffff: protocol ip prio 65535 u32 match ip src 255.255.255.255/32 action mirred egress redirect dev "$IFB_DEV" >/dev/null 2>&1; then
            $TC_BIN filter del dev "$TC_DEV" parent ffff: protocol ip prio 65535 >/dev/null 2>&1 || true
            DOWNLOAD_METHOD="ifb"
        fi
    fi
    if [ "$DOWNLOAD_METHOD" = "none" ]; then
        # 回退 police：探测 act_police 是否可用
        if $TC_BIN filter add dev "$TC_DEV" parent ffff: protocol ip prio 65535 u32 match ip src 255.255.255.255/32 police rate 1mbit burst 10k drop >/dev/null 2>&1; then
            $TC_BIN filter del dev "$TC_DEV" parent ffff: protocol ip prio 65535 >/dev/null 2>&1 || true
            DOWNLOAD_METHOD="police"
        fi
    fi
    # 探测失败且 ingress 是本脚本新建的：删除它，避免遗留一个空的 ingress qdisc
    if [ "$DOWNLOAD_METHOD" = "none" ] && [ "$INGRESS_CREATED" = "1" ]; then
        $TC_BIN qdisc del dev "$TC_DEV" ingress >/dev/null 2>&1 || true
    fi
fi

rate_request="server_id: $SERVER_ID
username: $encoded_username"
rate_result=$(curl -s -X POST -d "$rate_request" http://$INTERNAL_API/user/ratelimit/get)
UPLOAD_KB=$(printf '%s\n' "$rate_result" | sed -n 's/^upload_kb:[[:space:]]*//p' | head -n1)
DOWNLOAD_KB=$(printf '%s\n' "$rate_result" | sed -n 's/^download_kb:[[:space:]]*//p' | head -n1)
case "$UPLOAD_KB" in ''|*[!0-9]*) UPLOAD_KB=0 ;; esac
case "$DOWNLOAD_KB" in ''|*[!0-9]*) DOWNLOAD_KB=0 ;; esac

log_message "INFO" "用户 User $username VirtualIP: $CLIENT_IP4/$CLIENT_IP6 生效限速 上传=${UPLOAD_KB}KB/s 下载=${DOWNLOAD_KB}KB/s (0=不限速)"

if { [ -n "$CLIENT_IP4" ] || [ -n "$CLIENT_IP6" ]; } && { [ "$UPLOAD_KB" -gt 0 ] || [ "$DOWNLOAD_KB" -gt 0 ]; }; then
    if ! /sbin/ip link show "$TC_DEV" >/dev/null 2>&1; then
        log_message "WARNING" "接口 $TC_DEV 不存在，跳过 tc 限速 用户 User $username VirtualIP: $CLIENT_IP4/$CLIENT_IP6"
    else
        # tc 操作放进 flock 临界区串行化，避免与并发的上/下线进程互相覆盖 qdisc/class/filter
        (
        if ! flock 9; then
            log_message "WARNING" "用户 User $username VirtualIP: $CLIENT_IP4/$CLIENT_IP6 获取限速锁超时，跳过 tc 限速"
            exit 0
        fi
        if [ -n "$CLIENT_IP4" ]; then
            MINOR=$(minor_from_v4 "$CLIENT_IP4")
        else
            MINOR=$(minor_from_v6 "$CLIENT_IP6")
        fi

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
            if [ -n "$CLIENT_IP4" ]; then
                # 先按 prio 清除该客户端旧过滤器再加，保证重复上线不产生重复过滤项(prio 唯一即只影响本客户端)
                $TC_BIN filter del dev "$TC_DEV" parent 1: protocol ip prio $MINOR u32 2>/dev/null || true
                $TC_BIN filter add dev "$TC_DEV" parent 1: protocol ip prio $MINOR u32 match ip dst ${CLIENT_IP4}/32 flowid 1:$MINOR
            fi
            if [ -n "$CLIENT_IP6" ]; then
                $TC_BIN filter del dev "$TC_DEV" parent 1: protocol ipv6 prio $MINOR u32 2>/dev/null || true
                $TC_BIN filter add dev "$TC_DEV" parent 1: protocol ipv6 prio $MINOR u32 match ip6 dst ${CLIENT_IP6}/128 flowid 1:$MINOR
            fi
            log_message "INFO" "用户 User $username VirtualIP: $CLIENT_IP4/$CLIENT_IP6 上传限速已设置 ${UPLOAD_KB}KB/s classid 1:$MINOR"
        fi

        if [ "$DOWNLOAD_KB" -gt 0 ]; then
            DOWNLOAD_KBIT=$((DOWNLOAD_KB * 8))
            DOWNLOAD_BURST=$((DOWNLOAD_KBIT * 12))
            [ "$DOWNLOAD_BURST" -lt 3000 ] && DOWNLOAD_BURST=3000
            case "$DOWNLOAD_METHOD" in
            ifb)
                # 入方向 redirect 到 ifb，再在 ifb 上用 HTB 整形
                $IP_BIN link set "$IFB_DEV" up 2>/dev/null || true
                $TC_BIN qdisc add dev "$TC_DEV" handle ffff: ingress 2>/dev/null || true
                if ! $TC_BIN qdisc show dev "$IFB_DEV" 2>/dev/null | grep -q 'htb 1:'; then
                    $TC_BIN qdisc add dev "$IFB_DEV" root handle 1: htb default 9999 2>/dev/null
                    $TC_BIN class add dev "$IFB_DEV" parent 1: classid 1:9999 htb rate 100gbit ceil 100gbit burst 15k cburst 15k quantum 1500 2>/dev/null
                fi
                $TC_BIN class add dev "$IFB_DEV" parent 1: classid 1:$MINOR htb rate ${DOWNLOAD_KBIT}kbit ceil ${DOWNLOAD_KBIT}kbit burst ${DOWNLOAD_BURST} cburst ${DOWNLOAD_BURST} quantum 1500 2>/dev/null \
                    || $TC_BIN class change dev "$IFB_DEV" classid 1:$MINOR htb rate ${DOWNLOAD_KBIT}kbit ceil ${DOWNLOAD_KBIT}kbit burst ${DOWNLOAD_BURST} cburst ${DOWNLOAD_BURST} quantum 1500
                if [ -n "$CLIENT_IP4" ]; then
                    $TC_BIN filter del dev "$TC_DEV" parent ffff: protocol ip prio $MINOR u32 2>/dev/null || true
                    $TC_BIN filter add dev "$TC_DEV" parent ffff: protocol ip prio $MINOR u32 match ip src ${CLIENT_IP4}/32 action mirred egress redirect dev "$IFB_DEV"
                    $TC_BIN filter del dev "$IFB_DEV" parent 1: protocol ip prio $MINOR u32 2>/dev/null || true
                    $TC_BIN filter add dev "$IFB_DEV" parent 1: protocol ip prio $MINOR u32 match ip src ${CLIENT_IP4}/32 flowid 1:$MINOR
                fi
                if [ -n "$CLIENT_IP6" ]; then
                    $TC_BIN filter del dev "$TC_DEV" parent ffff: protocol ipv6 prio $MINOR u32 2>/dev/null || true
                    $TC_BIN filter add dev "$TC_DEV" parent ffff: protocol ipv6 prio $MINOR u32 match ip6 src ${CLIENT_IP6}/128 action mirred egress redirect dev "$IFB_DEV"
                    $TC_BIN filter del dev "$IFB_DEV" parent 1: protocol ipv6 prio $MINOR u32 2>/dev/null || true
                    $TC_BIN filter add dev "$IFB_DEV" parent 1: protocol ipv6 prio $MINOR u32 match ip6 src ${CLIENT_IP6}/128 flowid 1:$MINOR
                fi
                log_message "INFO" "用户 User $username VirtualIP: $CLIENT_IP4/$CLIENT_IP6 下载限速已设置(ifb) ${DOWNLOAD_KB}KB/s classid 1:$MINOR dev=$IFB_DEV"
                ;;
            police)
                $TC_BIN qdisc add dev "$TC_DEV" handle ffff: ingress 2>/dev/null || true
                if [ -n "$CLIENT_IP4" ]; then
                    $TC_BIN filter del dev "$TC_DEV" parent ffff: protocol ip prio $MINOR u32 2>/dev/null || true
                    if ! $TC_BIN filter add dev "$TC_DEV" parent ffff: protocol ip prio $MINOR u32 match ip src ${CLIENT_IP4}/32 police rate ${DOWNLOAD_KBIT}kbit burst ${DOWNLOAD_BURST} drop flowid :1 2>/dev/null; then
                        log_message "WARNING" "用户 User $username 下载限速(police)设置失败 虚拟IP ${CLIENT_IP4}"
                    fi
                fi
                if [ -n "$CLIENT_IP6" ]; then
                    $TC_BIN filter del dev "$TC_DEV" parent ffff: protocol ipv6 prio $MINOR u32 2>/dev/null || true
                    if ! $TC_BIN filter add dev "$TC_DEV" parent ffff: protocol ipv6 prio $MINOR u32 match ip6 src ${CLIENT_IP6}/128 police rate ${DOWNLOAD_KBIT}kbit burst ${DOWNLOAD_BURST} drop flowid :1 2>/dev/null; then
                        log_message "WARNING" "用户 User $username 下载限速(police)设置失败 虚拟IP6 ${CLIENT_IP6}"
                    fi
                fi
                log_message "INFO" "用户 User $username VirtualIP: $CLIENT_IP4/$CLIENT_IP6 下载限速已设置(police) ${DOWNLOAD_KB}KB/s"
                ;;
            *)
                log_message "WARNING" "用户 User $username 无法设置下载限速：ifb/act_mirred 与 police/act_police 均不可用，请安装相应内核模块（OpenWrt: kmod-ifb + kmod-sched-act-mirred 或 kmod-sched-act-police）"
                ;;
            esac
        fi
        ) 9>"$LOCK_FILE"
    fi
fi

# 根据用户组获取防火墙权限（同时把本次上线使用的 ACL 记录进 AddedServerACLRecord，
# 下线时原样取回用于还原同一组哈希，避免会话中途改组 ACL 导致拆不掉）
request_body="server_id: $SERVER_ID
username: $encoded_username
real_ip_addr: $untrusted_ip:$untrusted_port
virtual_ip_addr: $CLIENT_IP4
virtual_ip6_addr: $CLIENT_IP6"
result=$(curl -s -X POST -d "$request_body" http://$INTERNAL_API/user/acl/get)

log_message "DEBUG" "acl内容 $result"

CIDRS4=$(printf '%s\n' "$result" | list_v4_cidrs)
CIDRS6=$(printf '%s\n' "$result" | list_v6_cidrs)
if [ -z "$CIDRS4" ] && [ -z "$CIDRS6" ]; then
    log_message "INFO" "用户 User $username VirtualIP: $CLIENT_IP4/$CLIENT_IP6 无可放行 ACL，跳过"
    exit 0
fi

if [ "$HAVE_IPSET" = "1" ]; then
    (
        if ! flock 9; then
            log_message "WARNING" "用户 User $username VirtualIP: $CLIENT_IP4/$CLIENT_IP6 获取ACL锁超时，跳过放行(默认DROP，失败即拒绝)"
            exit 0
        fi

        # ---------- IPv4 ----------
        if [ -n "$CIDRS4" ] && [ -n "$CLIENT_IP4" ]; then
            HASH=$(printf '%s\n' "$CIDRS4" | sha256sum | cut -c1-$HASH_LEN)
            SRC_SET="ov${SERVER_ID}_s_${HASH}"
            DST_SET="ov${SERVER_ID}_d_${HASH}"
            SUB_CHAIN="ov${SERVER_ID}_c_${HASH}"

            # ipset 的 hash:net 不允许零前缀网段（如 0.0.0.0/0）：退化为零前缀直连放行，只维护源集。
            ALLOW_ALL=0
            if printf '%s\n' "$CIDRS4" | grep -qE '/0$'; then
                ALLOW_ALL=1
            fi

            $IPSET -! create "$SRC_SET" hash:ip hashsize 1024 maxelem 65536 || \
                log_message "WARNING" "创建源集 $SRC_SET 失败"

            if [ "$ALLOW_ALL" = "1" ]; then
                $IPSET add -! "$SRC_SET" "$CLIENT_IP4" 2>/dev/null || \
                    log_message "WARNING" "添加客户端 $CLIENT_IP4 到 $SRC_SET 失败"
                $IPTABLES -w -C "$CHAIN_NAME" -m set --match-set "$SRC_SET" src -j ACCEPT 2>/dev/null \
                    || $IPTABLES -w -A "$CHAIN_NAME" -m set --match-set "$SRC_SET" src -j ACCEPT
                log_message "INFO" "用户 User $username VirtualIP: $CLIENT_IP4 放行完成(ipset-零前缀直连放行) src=$SRC_SET"
            else
                $IPTABLES -w -N "$SUB_CHAIN" 2>/dev/null || true
                $IPSET -! create "$DST_SET" hash:net hashsize 1024 maxelem 4096 || \
                    log_message "WARNING" "创建目标集 $DST_SET 失败"
                for cidr in $CIDRS4; do
                    $IPSET add -! "$DST_SET" "$cidr" 2>/dev/null || \
                        log_message "WARNING" "添加 CIDR $cidr 到 $DST_SET 失败"
                done
                $IPSET add -! "$SRC_SET" "$CLIENT_IP4" 2>/dev/null || \
                    log_message "WARNING" "添加客户端 $CLIENT_IP4 到 $SRC_SET 失败"
                $IPTABLES -w -C "$CHAIN_NAME" -m set --match-set "$SRC_SET" src -j "$SUB_CHAIN" 2>/dev/null \
                    || $IPTABLES -w -A "$CHAIN_NAME" -m set --match-set "$SRC_SET" src -j "$SUB_CHAIN"
                $IPTABLES -w -C "$SUB_CHAIN" -m set --match-set "$DST_SET" dst -j ACCEPT 2>/dev/null \
                    || $IPTABLES -w -A "$SUB_CHAIN" -m set --match-set "$DST_SET" dst -j ACCEPT
                log_message "INFO" "用户 User $username VirtualIP: $CLIENT_IP4 放行完成(ipset) src=$SRC_SET dst=$DST_SET chain=$SUB_CHAIN"
            fi
        fi

        # ---------- IPv6 ----------
        if [ -n "$CIDRS6" ] && [ -n "$CLIENT_IP6" ]; then
            if [ "$HAVE_IP6TABLES" != "1" ]; then
                log_message "WARNING" "用户 User $username 有 IPv6 ACL 但系统无 ip6tables，跳过 IPv6 放行"
            else
                HASH6=$(printf '%s\n' "$CIDRS6" | sha256sum | cut -c1-$HASH_LEN)
                SRC6_SET="ov${SERVER_ID}_6s_${HASH6}"
                DST6_SET="ov${SERVER_ID}_6d_${HASH6}"
                SUB6_CHAIN="ov${SERVER_ID}_6c_${HASH6}"

                ALLOW_ALL6=0
                if printf '%s\n' "$CIDRS6" | grep -qE '/0$'; then
                    ALLOW_ALL6=1
                fi

                $IPSET -! create "$SRC6_SET" hash:ip family inet6 hashsize 1024 maxelem 65536 || \
                    log_message "WARNING" "创建 IPv6 源集 $SRC6_SET 失败"

                if [ "$ALLOW_ALL6" = "1" ]; then
                    $IPSET add -! "$SRC6_SET" "$CLIENT_IP6" 2>/dev/null || \
                        log_message "WARNING" "添加客户端 $CLIENT_IP6 到 $SRC6_SET 失败"
                    $IP6TABLES -w -C "$CHAIN_NAME" -m set --match-set "$SRC6_SET" src -j ACCEPT 2>/dev/null \
                        || $IP6TABLES -w -A "$CHAIN_NAME" -m set --match-set "$SRC6_SET" src -j ACCEPT
                    log_message "INFO" "用户 User $username VirtualIP6: $CLIENT_IP6 放行完成(ipset6-零前缀直连放行) src=$SRC6_SET"
                else
                    $IP6TABLES -w -N "$SUB6_CHAIN" 2>/dev/null || true
                    $IPSET -! create "$DST6_SET" hash:net family inet6 hashsize 1024 maxelem 4096 || \
                        log_message "WARNING" "创建 IPv6 目标集 $DST6_SET 失败"
                    for cidr in $CIDRS6; do
                        $IPSET add -! "$DST6_SET" "$cidr" 2>/dev/null || \
                            log_message "WARNING" "添加 IPv6 CIDR $cidr 到 $DST6_SET 失败"
                    done
                    $IPSET add -! "$SRC6_SET" "$CLIENT_IP6" 2>/dev/null || \
                        log_message "WARNING" "添加客户端 $CLIENT_IP6 到 $SRC6_SET 失败"
                    $IP6TABLES -w -C "$CHAIN_NAME" -m set --match-set "$SRC6_SET" src -j "$SUB6_CHAIN" 2>/dev/null \
                        || $IP6TABLES -w -A "$CHAIN_NAME" -m set --match-set "$SRC6_SET" src -j "$SUB6_CHAIN"
                    $IP6TABLES -w -C "$SUB6_CHAIN" -m set --match-set "$DST6_SET" dst -j ACCEPT 2>/dev/null \
                        || $IP6TABLES -w -A "$SUB6_CHAIN" -m set --match-set "$DST6_SET" dst -j ACCEPT
                    log_message "INFO" "用户 User $username VirtualIP6: $CLIENT_IP6 放行完成(ipset6) src=$SRC6_SET dst=$DST6_SET chain=$SUB6_CHAIN"
                fi
            fi
        fi
    ) 9>"$LOCK_FILE"
else
    # 传统模式：每条 ACL 一条规则，不使用 flock
    if [ -n "$CIDRS4" ] && [ -n "$CLIENT_IP4" ]; then
        for cidr in $CIDRS4; do
            command="$IPTABLES -w -A $CHAIN_NAME -s $CLIENT_IP4 -d $cidr -j ACCEPT"
            log_message "INFO" "用户 User $username VirtualIP: $CLIENT_IP4 设置acl命令(ipset不可用-传统模式): $command"
            $command
        done
    fi
    if [ -n "$CIDRS6" ] && [ -n "$CLIENT_IP6" ]; then
        if [ "$HAVE_IP6TABLES" = "1" ]; then
            for cidr in $CIDRS6; do
                command="$IP6TABLES -w -A $CHAIN_NAME -s $CLIENT_IP6 -d $cidr -j ACCEPT"
                log_message "INFO" "用户 User $username VirtualIP6: $CLIENT_IP6 设置acl命令(ipset不可用-传统模式): $command"
                $command
            done
        else
            log_message "WARNING" "用户 User $username 有 IPv6 ACL 但系统无 ip6tables，跳过 IPv6 放行"
        fi
    fi
fi

exit 0
