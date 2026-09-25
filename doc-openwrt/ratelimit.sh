#!/bin/bash
# 达量限速运行时更新脚本（资源名 ratelimit.sh）
# 由面板的状态采集器按服务器调用：当某个在线客户端匹配到新的限速规则、
# 且生效限速（达量限速方案与用户/用户组限速取最低）发生变化时，
# 面板把变化的客户端通过标准输入传进来，本脚本重新设置或移除对应的 tc 限速。
#
# 标准输入每行（空白分隔）：<虚拟IP> <上传KB/s> <下载KB/s>
# 虚拟 IP 可以是 IPv4 或 IPv6（纯 IPv6 客户端传其 IPv6 地址）。
# 0 表示该方向不限速（会移除该方向已存在的 tc 规则）。
# 上传 = 服务器 -> 客户端；下载 = 客户端 -> 服务器。

INTERNAL_API="__INTERNAL_API__"
LOG_FILE="__WORKING_DIR__auth.log"
SERVER_ID="__SERVER_ID__"
SERVER_INTERFACE="__SERVER_INTERFACE__"
LOCK_FILE="__WORKING_DIR__acl.lock"

log_message() {
    local level="$1"
    local message="$2"
    echo "$(date '+%Y-%m-%d %H:%M:%S') [${level}] ${message}" >> "$LOG_FILE"
}

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

# 下载（入方向）限速方式：与 client_online.sh 保持一致，ifb 优先，其次 police。
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
        $IP_BIN link set "$IFB_DEV" up >/dev/null 2>&1 || true
        if $TC_BIN filter add dev "$TC_DEV" parent ffff: protocol ip prio 65535 u32 match ip src 255.255.255.255/32 action mirred egress redirect dev "$IFB_DEV" >/dev/null 2>&1; then
            $TC_BIN filter del dev "$TC_DEV" parent ffff: protocol ip prio 65535 >/dev/null 2>&1 || true
            DOWNLOAD_METHOD="ifb"
        fi
    fi
    if [ "$DOWNLOAD_METHOD" = "none" ]; then
        if $TC_BIN filter add dev "$TC_DEV" parent ffff: protocol ip prio 65535 u32 match ip src 255.255.255.255/32 police rate 1mbit burst 10k drop >/dev/null 2>&1; then
            $TC_BIN filter del dev "$TC_DEV" parent ffff: protocol ip prio 65535 >/dev/null 2>&1 || true
            DOWNLOAD_METHOD="police"
        fi
    fi
    if [ "$DOWNLOAD_METHOD" = "none" ] && [ "$INGRESS_CREATED" = "1" ]; then
        $TC_BIN qdisc del dev "$TC_DEV" ingress >/dev/null 2>&1 || true
    fi
fi

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

if [ -z "$TC_DEV" ]; then
    log_message "WARNING" "服务器 $SERVER_ID 未配置网卡，跳过达量限速更新"
    exit 0
fi
if ! /sbin/ip link show "$TC_DEV" >/dev/null 2>&1; then
    log_message "WARNING" "接口 $TC_DEV 不存在，跳过达量限速更新"
    exit 0
fi

# 读取全部待更新客户端，避免在 flock 临界区内做阻塞 IO
CHANGES=""
while read -r client_ip upload_kb download_kb; do
    [ -z "$client_ip" ] && continue
    case "$upload_kb" in ''|*[!0-9]*) upload_kb=0 ;; esac
    case "$download_kb" in ''|*[!0-9]*) download_kb=0 ;; esac
    CHANGES="${CHANGES}${client_ip} ${upload_kb} ${download_kb}\n"
done

if [ -z "$CHANGES" ]; then
    exit 0
fi

# 与 client_online.sh / client_offline.sh 共用同一把 flock，串行化 tc 变更
(
if ! flock 9; then
    log_message "WARNING" "服务器 $SERVER_ID 获取限速锁超时，跳过达量限速更新"
    exit 0
fi

printf '%b' "$CHANGES" | while read -r client_ip upload_kb download_kb; do
    [ -z "$client_ip" ] && continue
    # 依据地址族选择协议/匹配方式与 classid 推导（与上/下线脚本一致）
    if [ "${client_ip#*:}" != "$client_ip" ]; then
        PROTO="ipv6"; MATCH="ip6"; PREFIX="/128"; MINOR=$(minor_from_v6 "$client_ip")
    else
        PROTO="ip"; MATCH="ip"; PREFIX="/32"; MINOR=$(minor_from_v4 "$client_ip")
    fi

    if [ "$upload_kb" -gt 0 ]; then
        if ! $TC_BIN qdisc show dev "$TC_DEV" 2>/dev/null | grep -q 'htb 1:'; then
            $TC_BIN qdisc add dev "$TC_DEV" root handle 1: htb default 9999 2>/dev/null
            $TC_BIN class add dev "$TC_DEV" parent 1: classid 1:9999 htb rate 100gbit ceil 100gbit burst 15k cburst 15k quantum 1500 2>/dev/null
        fi
        UPLOAD_KBIT=$((upload_kb * 8))
        UPLOAD_BURST=$((UPLOAD_KBIT * 12))
        [ "$UPLOAD_BURST" -lt 3000 ] && UPLOAD_BURST=3000
        $TC_BIN class add dev "$TC_DEV" parent 1: classid 1:$MINOR htb rate ${UPLOAD_KBIT}kbit ceil ${UPLOAD_KBIT}kbit burst ${UPLOAD_BURST} cburst ${UPLOAD_BURST} quantum 1500 2>/dev/null \
            || $TC_BIN class change dev "$TC_DEV" classid 1:$MINOR htb rate ${UPLOAD_KBIT}kbit ceil ${UPLOAD_KBIT}kbit burst ${UPLOAD_BURST} cburst ${UPLOAD_BURST} quantum 1500
        $TC_BIN filter del dev "$TC_DEV" parent 1: protocol $PROTO prio $MINOR u32 2>/dev/null || true
        $TC_BIN filter add dev "$TC_DEV" parent 1: protocol $PROTO prio $MINOR u32 match $MATCH dst ${client_ip}${PREFIX} flowid 1:$MINOR
        log_message "INFO" "达量限速更新 用户虚拟IP $client_ip 上传 ${upload_kb}KB/s classid 1:$MINOR"
    else
        # 取消上传限速：v4/v6 过滤器都清掉，再删类（同一客户端可能两种协议共用该类）
        $TC_BIN filter del dev "$TC_DEV" parent 1: protocol ip prio $MINOR u32 2>/dev/null || true
        $TC_BIN filter del dev "$TC_DEV" parent 1: protocol ipv6 prio $MINOR u32 2>/dev/null || true
        $TC_BIN class del dev "$TC_DEV" classid 1:$MINOR 2>/dev/null || true
        log_message "INFO" "达量限速更新 用户虚拟IP $client_ip 取消上传限速"
    fi

    if [ "$download_kb" -gt 0 ]; then
        DOWNLOAD_KBIT=$((download_kb * 8))
        DOWNLOAD_BURST=$((DOWNLOAD_KBIT * 12))
        [ "$DOWNLOAD_BURST" -lt 3000 ] && DOWNLOAD_BURST=3000
        case "$DOWNLOAD_METHOD" in
        ifb)
            $IP_BIN link set "$IFB_DEV" up 2>/dev/null || true
            $TC_BIN qdisc add dev "$TC_DEV" handle ffff: ingress 2>/dev/null || true
            if ! $TC_BIN qdisc show dev "$IFB_DEV" 2>/dev/null | grep -q 'htb 1:'; then
                $TC_BIN qdisc add dev "$IFB_DEV" root handle 1: htb default 9999 2>/dev/null
                $TC_BIN class add dev "$IFB_DEV" parent 1: classid 1:9999 htb rate 100gbit ceil 100gbit burst 15k cburst 15k quantum 1500 2>/dev/null
            fi
            $TC_BIN class add dev "$IFB_DEV" parent 1: classid 1:$MINOR htb rate ${DOWNLOAD_KBIT}kbit ceil ${DOWNLOAD_KBIT}kbit burst ${DOWNLOAD_BURST} cburst ${DOWNLOAD_BURST} quantum 1500 2>/dev/null \
                || $TC_BIN class change dev "$IFB_DEV" classid 1:$MINOR htb rate ${DOWNLOAD_KBIT}kbit ceil ${DOWNLOAD_KBIT}kbit burst ${DOWNLOAD_BURST} cburst ${DOWNLOAD_BURST} quantum 1500
            $TC_BIN filter del dev "$TC_DEV" parent ffff: protocol $PROTO prio $MINOR u32 2>/dev/null || true
            $TC_BIN filter add dev "$TC_DEV" parent ffff: protocol $PROTO prio $MINOR u32 match $MATCH src ${client_ip}${PREFIX} action mirred egress redirect dev "$IFB_DEV"
            $TC_BIN filter del dev "$IFB_DEV" parent 1: protocol $PROTO prio $MINOR u32 2>/dev/null || true
            $TC_BIN filter add dev "$IFB_DEV" parent 1: protocol $PROTO prio $MINOR u32 match $MATCH src ${client_ip}${PREFIX} flowid 1:$MINOR
            log_message "INFO" "达量限速更新 用户虚拟IP $client_ip 下载 ${download_kb}KB/s (ifb) classid 1:$MINOR"
            ;;
        police)
            $TC_BIN qdisc add dev "$TC_DEV" handle ffff: ingress 2>/dev/null || true
            $TC_BIN filter del dev "$TC_DEV" parent ffff: protocol $PROTO prio $MINOR u32 2>/dev/null || true
            $TC_BIN filter add dev "$TC_DEV" parent ffff: protocol $PROTO prio $MINOR u32 match $MATCH src ${client_ip}${PREFIX} police rate ${DOWNLOAD_KBIT}kbit burst ${DOWNLOAD_BURST} drop flowid :1
            log_message "INFO" "达量限速更新 用户虚拟IP $client_ip 下载 ${download_kb}KB/s (police)"
            ;;
        *)
            log_message "WARNING" "达量限速更新 用户虚拟IP $client_ip 无法设置下载限速：ifb/act_mirred 与 police/act_police 均不可用"
            ;;
        esac
    else
        # 取消下载限速：主接口 ingress 与 ifb 上的规则/类都清掉
        $TC_BIN filter del dev "$TC_DEV" parent ffff: protocol ip prio $MINOR u32 2>/dev/null || true
        $TC_BIN filter del dev "$TC_DEV" parent ffff: protocol ipv6 prio $MINOR u32 2>/dev/null || true
        if $IP_BIN link show "$IFB_DEV" >/dev/null 2>&1; then
            $TC_BIN filter del dev "$IFB_DEV" parent 1: protocol ip prio $MINOR u32 2>/dev/null || true
            $TC_BIN filter del dev "$IFB_DEV" parent 1: protocol ipv6 prio $MINOR u32 2>/dev/null || true
            $TC_BIN class del dev "$IFB_DEV" classid 1:$MINOR 2>/dev/null || true
            REMAIN_CLS=$($TC_BIN class show dev "$IFB_DEV" 2>/dev/null | grep -c 'class htb 1:' || true)
            if [ "${REMAIN_CLS:-0}" -le 1 ]; then
                $IP_BIN link del "$IFB_DEV" 2>/dev/null || true
            fi
        fi
        log_message "INFO" "达量限速更新 用户虚拟IP $client_ip 取消下载限速"
    fi
done
) 9>"$LOCK_FILE"

exit 0
