#!/bin/bash
# 在服务端停止后执行此脚本

INTERNAL_API="__INTERNAL_API__"
LOG_FILE="__WORKING_DIR__auth.log"
SERVER_ID="__SERVER_ID__"

SERVER_INTERFACE="__SERVER_INTERFACE__"
CHAIN_NAME="openvpn_acl_$SERVER_ID"

MSS=0

log_message() {
    local level="$1"
    local message="$2"
    echo "$(date '+%Y-%m-%d %H:%M:%S') [${level}] ${message}" >> "$LOG_FILE"
}

if [ -x /usr/sbin/ipset ]; then
    IPSET="ipset"
else
    IPSET="ipset"
fi

if [ $MSS -gt 100 ]; then 
    iptables -D FORWARD -p tcp -i $SERVER_INTERFACE --tcp-flags SYN,RST SYN -j TCPMSS --set-mss $MSS &> /dev/null
    iptables -D FORWARD -p tcp -o $SERVER_INTERFACE --tcp-flags SYN,RST SYN -j TCPMSS --set-mss $MSS &> /dev/null
fi

iptables -D FORWARD -i $SERVER_INTERFACE -j $CHAIN_NAME &> /dev/null
iptables -D FORWARD -i $SERVER_INTERFACE -j DROP        &> /dev/null
iptables -D FORWARD -o $SERVER_INTERFACE -j ACCEPT      &> /dev/null

iptables -F $CHAIN_NAME
iptables -X $CHAIN_NAME


ip6tables -D FORWARD -i $SERVER_INTERFACE -j $CHAIN_NAME &> /dev/null
ip6tables -D FORWARD -i $SERVER_INTERFACE -j DROP        &> /dev/null
ip6tables -D FORWARD -o $SERVER_INTERFACE -j ACCEPT      &> /dev/null

ip6tables -F $CHAIN_NAME
ip6tables -X $CHAIN_NAME

# 回收本服务器遗留：先删子链(其规则引用着目标集)，再销毁 ipset（set 全局存在，不随链删除而消失）
iptables -S 2>/dev/null | awk -v p="ov${SERVER_ID}_c_" '$1=="-N" && index($2,p)==1 {print $2}' | while read -r _sub; do
    iptables -F "$_sub" 2>/dev/null || true
    iptables -X "$_sub" 2>/dev/null || true
done

ip6tables -S 2>/dev/null | awk -v p="ov${SERVER_ID}_6c_" '$1=="-N" && index($2,p)==1 {print $2}' | while read -r _sub; do
    ip6tables -F "$_sub" 2>/dev/null || true
    ip6tables -X "$_sub" 2>/dev/null || true
done

$IPSET list -n 2>/dev/null | grep -E "^ov${SERVER_ID}_" | while read -r _set; do
    $IPSET destroy "$_set" 2>/dev/null || true
done

# 回收本服务器遗留的下载限速对象：先删主接口 ingress 上的 redirect/police 过滤器，
# 再删 ingress qdisc，最后删除 ifb 设备（ifb 名见 client_online.sh：ovpnrl<服务器ID>）。
# 注：iperf3 方向约定下，下载限速挂在服务器接口的 ingress 上。
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

IFB_DEV="ovpnrl${SERVER_ID}"
if [ -n "$SERVER_INTERFACE" ]; then
    # 清掉 ingress 上的所有过滤器与 ingress qdisc（避免只删 ifb 后主接口残留 ingress）
    $TC_BIN qdisc del dev "$SERVER_INTERFACE" ingress >/dev/null 2>&1 || true
fi
if $IP_BIN link show "$IFB_DEV" >/dev/null 2>&1; then
    $IP_BIN link del "$IFB_DEV" >/dev/null 2>&1 || true
    log_message "INFO" "已回收下载限速设备 $IFB_DEV"
fi

exit 0