#!/bin/bash
# 在服务端启动后执行此脚本

INTERNAL_API="__INTERNAL_API__"
LOG_FILE="__WORKING_DIR__auth.log"
SERVER_ID="__SERVER_ID__"

SERVER_INTERFACE="__SERVER_INTERFACE__"
CHAIN_NAME="openvpn_acl_$SERVER_ID"

# 设置mss。mss>100时启用 推荐在客户端使用mssfix参数调整mss 
MSS=0

log_message() {
    local level="$1"
    local message="$2"
    echo "$(date '+%Y-%m-%d %H:%M:%S') [${level}] ${message}" >> "$LOG_FILE"
}

if [ -x /usr/sbin/ipset ]; then
    IPSET="sudo /usr/sbin/ipset"
else
    IPSET="sudo ipset"
fi

# ACL 放行优先用 ipset；探测其功能性可用性（ipset --version 在无内核权限时也会失败，故用 list -n）。
# 不可用时，client_online/offline 会自动回落到传统逐条 iptables 模式，这里仅记录提示。
if ! $IPSET list -n >/dev/null 2>&1; then
    log_message "WARN" "ipset 不可用，ACL 将回落到传统逐条 iptables 模式（建议安装 ipset 及内核模块 ip_set/hash_ip/hash_net）"
fi

if [ $MSS -gt 100 ]; then 
    sudo /usr/sbin/iptables -D FORWARD -p tcp -i $SERVER_INTERFACE --tcp-flags SYN,RST SYN -j TCPMSS --set-mss 1300 &> /dev/null
    sudo /usr/sbin/iptables -D FORWARD -p tcp -o $SERVER_INTERFACE --tcp-flags SYN,RST SYN -j TCPMSS --set-mss 1300 &> /dev/null
fi

sudo /usr/sbin/iptables -D FORWARD -i $SERVER_INTERFACE -j $CHAIN_NAME &> /dev/null
sudo /usr/sbin/iptables -D FORWARD -i $SERVER_INTERFACE -j DROP        &> /dev/null
sudo /usr/sbin/iptables -D FORWARD -o $SERVER_INTERFACE -j ACCEPT      &> /dev/null

sudo /usr/sbin/iptables -N $CHAIN_NAME &> /dev/null
sudo /usr/sbin/iptables -F $CHAIN_NAME 

# 回收本服务器上次遗留：先删子链(其规则引用着目标集)，再销毁 ipset（链已 flush，set 不再被引用）
sudo /usr/sbin/iptables -S 2>/dev/null | awk -v p="ov${SERVER_ID}_c_" '$1=="-N" && index($2,p)==1 {print $2}' | while read -r _sub; do
    sudo /usr/sbin/iptables -F "$_sub" 2>/dev/null || true
    sudo /usr/sbin/iptables -X "$_sub" 2>/dev/null || true
done

$IPSET list -n 2>/dev/null | grep -E "^ov${SERVER_ID}_" | while read -r _set; do
    $IPSET destroy "$_set" 2>/dev/null || true
done

if [ $MSS -gt 100 ]; then 
    sudo /usr/sbin/iptables -A FORWARD -p tcp -i $SERVER_INTERFACE --tcp-flags SYN,RST SYN -j TCPMSS --set-mss $MSS &> /dev/null
    sudo /usr/sbin/iptables -A FORWARD -p tcp -o $SERVER_INTERFACE --tcp-flags SYN,RST SYN -j TCPMSS --set-mss $MSS &> /dev/null
fi 

sudo /usr/sbin/iptables -A $CHAIN_NAME -m state --state ESTABLISHED,RELATED -j ACCEPT

sudo /usr/sbin/iptables -A FORWARD -i $SERVER_INTERFACE -j $CHAIN_NAME
sudo /usr/sbin/iptables -A FORWARD -i $SERVER_INTERFACE -j DROP
sudo /usr/sbin/iptables -A FORWARD -o $SERVER_INTERFACE -j ACCEPT

sudo /usr/sbin/ip6tables -D FORWARD -i $SERVER_INTERFACE -j $CHAIN_NAME &> /dev/null
sudo /usr/sbin/ip6tables -D FORWARD -i $SERVER_INTERFACE -j DROP        &> /dev/null
sudo /usr/sbin/ip6tables -D FORWARD -o $SERVER_INTERFACE -j ACCEPT      &> /dev/null

sudo /usr/sbin/ip6tables -N $CHAIN_NAME &> /dev/null
sudo /usr/sbin/ip6tables -F $CHAIN_NAME 

sudo /usr/sbin/ip6tables -A $CHAIN_NAME -m state --state ESTABLISHED,RELATED -j ACCEPT

sudo /usr/sbin/ip6tables -A FORWARD -i $SERVER_INTERFACE -j $CHAIN_NAME
sudo /usr/sbin/ip6tables -A FORWARD -i $SERVER_INTERFACE -j DROP
sudo /usr/sbin/ip6tables -A FORWARD -o $SERVER_INTERFACE -j ACCEPT

exit 0 