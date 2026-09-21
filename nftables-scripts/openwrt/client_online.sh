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
#   ov<sid>_s_<hash>  源集(ipv4_addr)        放该组所有在线客户端IP
#   ov<sid>_d_<hash>  目标集(ipv4_addr,interval) 放该组允许的 CIDR
#   ov<sid>_c_<hash>  子链
#   acl 链: ip saddr @源集 jump 子链
#   子链:   ip daddr @目标集 accept
# hash 由排序去重后的 IPv4 CIDR 集合算出，ACL 相同的客户端共享同一组对象，
# 于是 acl 链规则数 = 不同 ACL 组数，与在线用户数无关。
# 集合是 nftables 原生能力，无需安装 ipset，也不存在 iptables 模式的回落逻辑。
NFT_BIN=""
for _nft in /usr/sbin/nft /sbin/nft /usr/bin/nft /bin/nft; do
    if [ -x "$_nft" ]; then NFT_BIN="$_nft"; break; fi
done
[ -z "$NFT_BIN" ] && NFT_BIN="nft"

TABLE_NAME="openvpn_acl_$SERVER_ID"
ACL_CHAIN="acl"
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

if ! $NFT_BIN list table inet "$TABLE_NAME" >/dev/null 2>&1; then
    log_message "WARNING" "nftables 表 $TABLE_NAME 不存在，跳过放行(请确认 server_start.sh 已执行)"
    exit 0
fi

HASH=$(printf '%s\n' "$CIDRS" | sha256sum | cut -c1-$HASH_LEN)
SRC_SET="ov${SERVER_ID}_s_${HASH}"
DST_SET="ov${SERVER_ID}_d_${HASH}"
SUB_CHAIN="ov${SERVER_ID}_c_${HASH}"

# 临界区：建集/加成员/建子链/加规则，串行化以防并发上线下线把系统状态改乱
(
    if ! flock 9; then
        log_message "WARNING" "用户 User $username VirtualIP: $CLIENT_IP 获取ACL锁超时，跳过放行(默认DROP，失败即拒绝)"
        exit 0
    fi

    # 集合已存在时忽略报错(nft 无 -N 的 -! 等价选项)；目标集用 interval 才能容纳 CIDR
    $NFT_BIN add set inet "$TABLE_NAME" "$SRC_SET" '{ type ipv4_addr ; }' 2>/dev/null || true
    $NFT_BIN add set inet "$TABLE_NAME" "$DST_SET" '{ type ipv4_addr ; flags interval ; }' 2>/dev/null || true

    for cidr in $CIDRS; do
        $NFT_BIN add element inet "$TABLE_NAME" "$DST_SET" "{ $cidr }" 2>/dev/null || true
    done
    # 元素已存在时 nft 会报错，忽略即可(等价 ipset 的 -!)
    $NFT_BIN add element inet "$TABLE_NAME" "$SRC_SET" "{ $CLIENT_IP }" 2>/dev/null || true

    $NFT_BIN add chain inet "$TABLE_NAME" "$SUB_CHAIN" 2>/dev/null || true

    # 同组规则只建一次：按集合名判断 acl 链/子链中是否已存在对应跳转/放行规则
    if ! $NFT_BIN -a list chain inet "$TABLE_NAME" "$ACL_CHAIN" 2>/dev/null | grep -qF "@$SRC_SET"; then
        $NFT_BIN add rule inet "$TABLE_NAME" "$ACL_CHAIN" ip saddr "@$SRC_SET" jump "$SUB_CHAIN" 2>/dev/null || \
            log_message "WARNING" "添加主链规则 $ACL_CHAIN @$SRC_SET 失败"
    fi
    if ! $NFT_BIN -a list chain inet "$TABLE_NAME" "$SUB_CHAIN" 2>/dev/null | grep -qF "@$DST_SET"; then
        $NFT_BIN add rule inet "$TABLE_NAME" "$SUB_CHAIN" ip daddr "@$DST_SET" accept 2>/dev/null || \
            log_message "WARNING" "添加子链规则 $SUB_CHAIN @$DST_SET 失败"
    fi

    log_message "INFO" "用户 User $username VirtualIP: $CLIENT_IP 放行完成(nft) src=$SRC_SET dst=$DST_SET chain=$SUB_CHAIN"
) 9>"$LOCK_FILE"

exit 0
