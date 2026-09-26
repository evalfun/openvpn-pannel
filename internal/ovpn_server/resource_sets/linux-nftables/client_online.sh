#!/bin/bash

INTERNAL_API="__INTERNAL_API__"
LOG_FILE="__WORKING_DIR__auth.log"
SERVER_ID="__SERVER_ID__"

SERVER_INTERFACE="__SERVER_INTERFACE__"
LOCK_FILE="__WORKING_DIR__acl.lock"

# ===== nftables 说明 =====
# 每台服务器使用一张独立的 inet 表 openvpn_acl_<SERVER_ID>，表结构由 server_start.sh 建立：
#   forward  基础链(hook forward)，负责 established 放行、接口跳转/丢弃/放行
#   acl      规则链(被 forward 以 iifname 跳转)，存放各 ACL 组的放行规则
# 每个 ACL 组用“命名集合 + 子链”表达 (源 ∈ 客户端集) AND (目的 ∈ CIDR集)：
#   IPv4: ov<sid>_s_<hash>/ov<sid>_d_<hash>/ov<sid>_c_<hash>，集合类型 ipv4_addr
#   IPv6: ov<sid>_6s_<hash>/ov<sid>_6d_<hash>/ov<sid>_6c_<hash>，集合类型 ipv6_addr
#   子链:   {ip,ip6} daddr @目标集 accept
# hash 由排序去重后的 CIDR 集合算出，ACL 相同的客户端共享同一组对象，
# 于是 acl 链规则数 = 不同 ACL 组数，与在线用户数无关。
# 集合是 nftables 原生能力，无需安装 ipset，也不存在 iptables 模式的回落逻辑。
# inet 表同时处理 IPv4/IPv6，无需像 iptables 那样额外维护 ip6tables。
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
#   ifb<服务器ID> 设备再用 HTB 整形（依赖 ifb + act_mirred，OpenWrt 常见）；
#   若 ifb/act_mirred 不可用则回退到 ingress police（依赖 act_police）；
#   两者都不可用时记录 WARNING 并跳过下载限速（不再静默失败）。
# 速率由服务器按用户策略(先看用户，再看活跃用户组)算出，单位 KB/s，0 = 不限速。
# IPv4 与 IPv6 复用同一 classid（由 IPv4 末两段推导）；纯 IPv6 客户端使用由 IPv6 推导的独立 classid。
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

TC_DEV="$SERVER_INTERFACE"

# 下载（入方向）限速方式：ifb 优先，其次 police。
IFB_DEV="ifb${SERVER_ID}"
DOWNLOAD_METHOD="none"
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

rate_request="server_id: $SERVER_ID
username: $encoded_username"
rate_result=$(curl -s -X POST -d "$rate_request" http://$INTERNAL_API/user/ratelimit/get)
UPLOAD_KB=$(printf '%s\n' "$rate_result" | sed -n 's/^upload_kb:[[:space:]]*//p' | head -n1)
DOWNLOAD_KB=$(printf '%s\n' "$rate_result" | sed -n 's/^download_kb:[[:space:]]*//p' | head -n1)
case "$UPLOAD_KB" in ''|*[!0-9]*) UPLOAD_KB=0 ;; esac
case "$DOWNLOAD_KB" in ''|*[!0-9]*) DOWNLOAD_KB=0 ;; esac

log_message "INFO" "用户 User $username VirtualIP: $CLIENT_IP4/$CLIENT_IP6 生效限速 上传=${UPLOAD_KB}KB/s 下载=${DOWNLOAD_KB}KB/s (0=不限速)"

if { [ -n "$CLIENT_IP4" ] || [ -n "$CLIENT_IP6" ]; } && { [ "$UPLOAD_KB" -gt 0 ] || [ "$DOWNLOAD_KB" -gt 0 ]; }; then
    if ! ip link show "$TC_DEV" >/dev/null 2>&1; then
        log_message "WARNING" "接口 $TC_DEV 不存在，跳过 tc 限速 用户 User $username VirtualIP: $CLIENT_IP4/$CLIENT_IP6"
    else
        # tc 操作放进 flock 临界区串行化，避免与并发的上/下线进程互相覆盖 qdisc/class/filter
        (
        if ! flock -w 60 9; then
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
            # 先按 prio 清除该客户端旧过滤器再加，保证重复上线不产生重复过滤项(prio 唯一即只影响本客户端)
            if [ -n "$CLIENT_IP4" ]; then
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

if ! $NFT list table inet "$TABLE_NAME" >/dev/null 2>&1; then
    log_message "WARNING" "nftables 表 $TABLE_NAME 不存在，跳过放行(请确认 server_start.sh 已执行)"
    exit 0
fi

# 临界区：建集/加成员/建子链/加规则，串行化以防并发上线下线把系统状态改乱
(
    if ! flock -w 60 9; then
        log_message "WARNING" "用户 User $username VirtualIP: $CLIENT_IP4/$CLIENT_IP6 获取ACL锁超时，跳过放行(默认DROP，失败即拒绝)"
        exit 0
    fi

    # ---------- IPv4 ----------
    if [ -n "$CIDRS4" ] && [ -n "$CLIENT_IP4" ]; then
        HASH=$(printf '%s\n' "$CIDRS4" | sha256sum | cut -c1-$HASH_LEN)
        SRC_SET="ov${SERVER_ID}_s_${HASH}"
        DST_SET="ov${SERVER_ID}_d_${HASH}"
        SUB_CHAIN="ov${SERVER_ID}_c_${HASH}"

        # 集合已存在时忽略报错(nft 无 -N 的 -! 等价选项)；目标集用 interval 才能容纳 CIDR
        $NFT add set inet "$TABLE_NAME" "$SRC_SET" '{ type ipv4_addr ; }' 2>/dev/null || true
        $NFT add set inet "$TABLE_NAME" "$DST_SET" '{ type ipv4_addr ; flags interval ; }' 2>/dev/null || true

        for cidr in $CIDRS4; do
            $NFT add element inet "$TABLE_NAME" "$DST_SET" "{ $cidr }" 2>/dev/null || true
        done
        # 元素已存在时 nft 会报错，忽略即可(等价 ipset 的 -!)
        $NFT add element inet "$TABLE_NAME" "$SRC_SET" "{ $CLIENT_IP4 }" 2>/dev/null || true

        $NFT add chain inet "$TABLE_NAME" "$SUB_CHAIN" 2>/dev/null || true

        # 同组规则只建一次：按集合名判断 acl 链/子链中是否已存在对应跳转/放行规则
        if ! $NFT -a list chain inet "$TABLE_NAME" "$ACL_CHAIN" 2>/dev/null | grep -qF "@$SRC_SET"; then
            $NFT add rule inet "$TABLE_NAME" "$ACL_CHAIN" ip saddr "@$SRC_SET" jump "$SUB_CHAIN" 2>/dev/null || \
                log_message "WARNING" "添加主链规则 $ACL_CHAIN @$SRC_SET 失败"
        fi
        if ! $NFT -a list chain inet "$TABLE_NAME" "$SUB_CHAIN" 2>/dev/null | grep -qF "@$DST_SET"; then
            $NFT add rule inet "$TABLE_NAME" "$SUB_CHAIN" ip daddr "@$DST_SET" accept 2>/dev/null || \
                log_message "WARNING" "添加子链规则 $SUB_CHAIN @$DST_SET 失败"
        fi

        log_message "INFO" "用户 User $username VirtualIP: $CLIENT_IP4 放行完成(nft) src=$SRC_SET dst=$DST_SET chain=$SUB_CHAIN"
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

        log_message "INFO" "用户 User $username VirtualIP6: $CLIENT_IP6 放行完成(nft) src=$SRC6_SET dst=$DST6_SET chain=$SUB6_CHAIN"
    fi
) 9>"$LOCK_FILE"

exit 0
