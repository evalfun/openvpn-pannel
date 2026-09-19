#!/bin/bash
# 达量限速运行时更新脚本（资源名 ratelimit.sh）
# 由面板的状态采集器按服务器调用：当某个在线客户端匹配到新的限速规则、
# 且生效限速（达量限速方案与用户/用户组限速取最低）发生变化时，
# 面板把变化的客户端通过标准输入传进来，本脚本重新设置或移除对应的 tc 限速。
#
# 标准输入每行（空白分隔）：<虚拟IPv4> <上传KB/s> <下载KB/s>
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
for _tc in /usr/sbin/tc /sbin/tc /usr/bin/tc /bin/tc; do
    if [ -x "$_tc" ]; then TC_BIN="sudo $_tc"; break; fi
done
[ -z "$TC_BIN" ] && TC_BIN="sudo tc"

TC_DEV="$SERVER_INTERFACE"
if [ -z "$TC_DEV" ]; then
    log_message "WARNING" "服务器 $SERVER_ID 未配置网卡，跳过达量限速更新"
    exit 0
fi
if ! ip link show "$TC_DEV" >/dev/null 2>&1; then
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
if ! flock -w 60 9; then
    log_message "WARNING" "服务器 $SERVER_ID 获取限速锁超时，跳过达量限速更新"
    exit 0
fi

printf '%b' "$CHANGES" | while read -r client_ip upload_kb download_kb; do
    [ -z "$client_ip" ] && continue
    _c=$(printf '%s' "$client_ip" | cut -d. -f3)
    _d=$(printf '%s' "$client_ip" | cut -d. -f4)
    MINOR=$(( (${_c:-0} << 8) | ${_d:-0} ))
    [ "$MINOR" -le 0 ] && MINOR=1
    [ "$MINOR" -ge 65535 ] && MINOR=65534
    [ "$MINOR" -eq 9999 ] && MINOR=9998

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
        $TC_BIN filter del dev "$TC_DEV" parent 1: protocol ip prio $MINOR u32 2>/dev/null || true
        $TC_BIN filter add dev "$TC_DEV" parent 1: protocol ip prio $MINOR u32 match ip dst ${client_ip}/32 flowid 1:$MINOR
        log_message "INFO" "达量限速更新 用户虚拟IP $client_ip 上传 ${upload_kb}KB/s classid 1:$MINOR"
    else
        $TC_BIN filter del dev "$TC_DEV" parent 1: protocol ip prio $MINOR u32 2>/dev/null || true
        $TC_BIN class del dev "$TC_DEV" classid 1:$MINOR 2>/dev/null || true
        log_message "INFO" "达量限速更新 用户虚拟IP $client_ip 取消上传限速"
    fi

    if [ "$download_kb" -gt 0 ]; then
        $TC_BIN qdisc add dev "$TC_DEV" handle ffff: ingress 2>/dev/null || true
        DOWNLOAD_KBIT=$((download_kb * 8))
        DOWNLOAD_BURST=$((DOWNLOAD_KBIT * 12))
        [ "$DOWNLOAD_BURST" -lt 3000 ] && DOWNLOAD_BURST=3000
        $TC_BIN filter del dev "$TC_DEV" parent ffff: protocol ip prio $MINOR u32 2>/dev/null || true
        $TC_BIN filter add dev "$TC_DEV" parent ffff: protocol ip prio $MINOR u32 match ip src ${client_ip}/32 police rate ${DOWNLOAD_KBIT}kbit burst ${DOWNLOAD_BURST} drop flowid :1
        log_message "INFO" "达量限速更新 用户虚拟IP $client_ip 下载 ${download_kb}KB/s"
    else
        $TC_BIN filter del dev "$TC_DEV" parent ffff: protocol ip prio $MINOR u32 2>/dev/null || true
        log_message "INFO" "达量限速更新 用户虚拟IP $client_ip 取消下载限速"
    fi
done
) 9>"$LOCK_FILE"

exit 0
