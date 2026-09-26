#!/bin/bash
# 在客户端认证时执行此脚本
# 通过环境变量接收用户名和密码 (script-security 4)

INTERNAL_API="__INTERNAL_API__"
LOG_FILE="__WORKING_DIR__auth.log"
SERVER_ID="__SERVER_ID__"

log_message() {
    local level="$1"
    local message="$2"
    echo "$(date '+%Y-%m-%d %H:%M:%S') [${level}] ${message}" >> "$LOG_FILE"
}

# 检查必要的环境变量
if [ -z "$username" ] || [ -z "$password" ]; then
    log_message "ERROR" "$username $password from $untrusted_ip $untrusted_port Missing username or password environment variable."
    log_message "$(set)"
    exit 1 # 验证失败
fi

if [ -z $auth_failed_reason_file ]; then 
    auth_failed_reason_file="auth_failed_reason_file"
fi

# 对username和password进行base64编码
encoded_username=$(echo -n "$username" | base64 -w 0 | tr -d '\n')
encoded_password=$(echo -n "$password" | base64 -w 0 | tr -d '\n')

# 执行应用程序进行认证
http_body="server_id: $SERVER_ID
username: $encoded_username
password: $encoded_password
real_ip_addr: $untrusted_ip:$untrusted_port
client_cert_name: $common_name"

resp_body=$(curl -s -X POST -d "$http_body" http://$INTERNAL_API/user/auth -w "\\nstatus=%{http_code}")
status=""
result=""
while IFS= read -r line
do
    echo "$line" | grep "^status=" >/dev/null
    if [ $? -eq 0 ]; then
        status=$(echo "$line" | cut -d "=" -f 2)
        continue
    fi
    echo "$line" | grep "^result=" >/dev/null
    if [ $? -eq 0 ]; then
        result=$(echo "$line" | cut -d "=" -f 2)
        continue
    fi
done <<< "$resp_body"

if [ "$status" == "200" ]; then
log_message "INFO" "User '$username' from $untrusted_ip $untrusted_port authenticated successfully."
	exit 0 # 验证成功
else 
    log_message "WARNING" "Authentication failed for user '$username' ( $result ) from $untrusted_ip $untrusted_port."
	echo $result >> $auth_failed_reason_file 
    exit 1 # 验证失败
fi

# --- 脚本结束 ---
# 正常情况下不会执行到这里
log_message "ERROR" "Unexpected error in authentication script for user '$username' from $untrusted_ip $untrusted_port."
exit 1