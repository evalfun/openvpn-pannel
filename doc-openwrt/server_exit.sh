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
    IPSET="/usr/sbin/ipset"
else
    IPSET="ipset"
fi

if [ $MSS -gt 100 ]; then 
    /usr/sbin/iptables -D FORWARD -p tcp -i $SERVER_INTERFACE --tcp-flags SYN,RST SYN -j TCPMSS --set-mss $MSS &> /dev/null
    /usr/sbin/iptables -D FORWARD -p tcp -o $SERVER_INTERFACE --tcp-flags SYN,RST SYN -j TCPMSS --set-mss $MSS &> /dev/null
fi 

/usr/sbin/iptables -D FORWARD -i $SERVER_INTERFACE -j $CHAIN_NAME &> /dev/null
/usr/sbin/iptables -D FORWARD -i $SERVER_INTERFACE -j DROP        &> /dev/null
/usr/sbin/iptables -D FORWARD -o $SERVER_INTERFACE -j ACCEPT      &> /dev/null

/usr/sbin/iptables -F $CHAIN_NAME
/usr/sbin/iptables -X $CHAIN_NAME


/usr/sbin/ip6tables -D FORWARD -i $SERVER_INTERFACE -j $CHAIN_NAME &> /dev/null
/usr/sbin/ip6tables -D FORWARD -i $SERVER_INTERFACE -j DROP        &> /dev/null
/usr/sbin/ip6tables -D FORWARD -o $SERVER_INTERFACE -j ACCEPT      &> /dev/null

/usr/sbin/ip6tables -F $CHAIN_NAME
/usr/sbin/ip6tables -X $CHAIN_NAME

# 回收本服务器遗留：先删子链(其规则引用着目标集)，再销毁 ipset（set 全局存在，不随链删除而消失）
/usr/sbin/iptables -S 2>/dev/null | awk -v p="ov${SERVER_ID}_c_" '$1=="-N" && index($2,p)==1 {print $2}' | while read -r _sub; do
    /usr/sbin/iptables -F "$_sub" 2>/dev/null || true
    /usr/sbin/iptables -X "$_sub" 2>/dev/null || true
done

$IPSET list -n 2>/dev/null | grep -E "^ov${SERVER_ID}_" | while read -r _set; do
    $IPSET destroy "$_set" 2>/dev/null || true
done

exit 0