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
    IPSET="ipset"
else
    IPSET="ipset"
fi

# ACL 放行优先用 ipset；探测其功能性可用性（ipset --version 在无内核权限时也会失败，故用 list -n）。
# 不可用时，client_online/offline 会自动回落到传统逐条 iptables 模式，这里仅记录提示。
if ! $IPSET list -n >/dev/null 2>&1; then
    log_message "WARN" "ipset 不可用，ACL 将回落到传统逐条 iptables 模式（建议安装 ipset 及内核模块 ip_set/hash_ip/hash_net）"
fi

if [ $MSS -gt 100 ]; then 
    iptables -D FORWARD -p tcp -i $SERVER_INTERFACE --tcp-flags SYN,RST SYN -j TCPMSS --set-mss 1300 &> /dev/null
    iptables -D FORWARD -p tcp -o $SERVER_INTERFACE --tcp-flags SYN,RST SYN -j TCPMSS --set-mss 1300 &> /dev/null
fi

iptables -D FORWARD -i $SERVER_INTERFACE -j $CHAIN_NAME &> /dev/null
iptables -D FORWARD -i $SERVER_INTERFACE -j DROP        &> /dev/null
iptables -D FORWARD -o $SERVER_INTERFACE -j ACCEPT      &> /dev/null

iptables -N $CHAIN_NAME &> /dev/null
iptables -F $CHAIN_NAME 

# 回收本服务器上次遗留：先删子链(其规则引用着目标集)，再销毁 ipset（链已 flush，set 不再被引用）
iptables -S 2>/dev/null | awk -v p="ov${SERVER_ID}_c_" '$1=="-N" && index($2,p)==1 {print $2}' | while read -r _sub; do
    iptables -F "$_sub" 2>/dev/null || true
    iptables -X "$_sub" 2>/dev/null || true
done

$IPSET list -n 2>/dev/null | grep -E "^ov${SERVER_ID}_" | while read -r _set; do
    $IPSET destroy "$_set" 2>/dev/null || true
done

# 回收上次运行遗留的下载限速对象（进程异常退出时 ifb 设备与主接口 ingress 可能残留）。
# 先删主接口 ingress 上残留的过滤器与 ingress qdisc，再删除 ifb 设备；
# 客户端上线时 client_online.sh 会按需重新创建。ifb 名与 client_online.sh 保持一致。
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
    $TC_BIN qdisc del dev "$SERVER_INTERFACE" ingress >/dev/null 2>&1 || true
fi
if $IP_BIN link show "$IFB_DEV" >/dev/null 2>&1; then
    $IP_BIN link del "$IFB_DEV" >/dev/null 2>&1 || true
    log_message "INFO" "已清理上次遗留的下载限速设备 $IFB_DEV"
fi

if [ $MSS -gt 100 ]; then 
    iptables -A FORWARD -p tcp -i $SERVER_INTERFACE --tcp-flags SYN,RST SYN -j TCPMSS --set-mss $MSS &> /dev/null
    iptables -A FORWARD -p tcp -o $SERVER_INTERFACE --tcp-flags SYN,RST SYN -j TCPMSS --set-mss $MSS &> /dev/null
fi 

iptables -A $CHAIN_NAME -m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT

iptables -A FORWARD -i $SERVER_INTERFACE -j $CHAIN_NAME
iptables -A FORWARD -i $SERVER_INTERFACE -j DROP
iptables -A FORWARD -o $SERVER_INTERFACE -j ACCEPT

ip6tables -D FORWARD -i $SERVER_INTERFACE -j $CHAIN_NAME &> /dev/null
ip6tables -D FORWARD -i $SERVER_INTERFACE -j DROP        &> /dev/null
ip6tables -D FORWARD -o $SERVER_INTERFACE -j ACCEPT      &> /dev/null

ip6tables -N $CHAIN_NAME &> /dev/null
ip6tables -F $CHAIN_NAME 

# 回收本服务器上次遗留的 IPv6 子链（ip6tables 独立于 iptables）
ip6tables -S 2>/dev/null | awk -v p="ov${SERVER_ID}_6c_" '$1=="-N" && index($2,p)==1 {print $2}' | while read -r _sub; do
    ip6tables -F "$_sub" 2>/dev/null || true
    ip6tables -X "$_sub" 2>/dev/null || true
done

ip6tables -A $CHAIN_NAME -m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT

ip6tables -A FORWARD -i $SERVER_INTERFACE -j $CHAIN_NAME
ip6tables -A FORWARD -i $SERVER_INTERFACE -j DROP
ip6tables -A FORWARD -o $SERVER_INTERFACE -j ACCEPT

exit 0 