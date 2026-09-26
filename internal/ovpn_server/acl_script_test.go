package ovpnserver

import (
	"strings"
	"testing"
)

// 确保内置的 ACL 动态放行/回收脚本存在且包含关键占位符与逻辑。
func TestDefaultACLScripts(t *testing.T) {
	for _, id := range []string{RESOURCE_ID_ACL_ADD_SCRIPT, RESOURCE_ID_ACL_DEL_SCRIPT} {
		content := GetSetDefaultResource(RESOURCE_SET_LINUX_IPTABLES, id)
		if content == "" {
			t.Fatalf("默认资源 %s 为空", id)
		}
		for _, want := range []string{"__WORKING_DIR__", "__SERVER_ID__", "__SERVER_INTERFACE__", "virtual_ip:", "flock"} {
			if !strings.Contains(content, want) {
				t.Errorf("资源 %s 缺少 %q", id, want)
			}
		}
	}
	// 回收脚本必须清空该客户端的连接跟踪(状态表)，否则已建立的连接在登出后仍会放行。
	del := GetSetDefaultResource(RESOURCE_SET_LINUX_IPTABLES, RESOURCE_ID_ACL_DEL_SCRIPT)
	for _, want := range []string{"flush_conntrack", "conntrack"} {
		if !strings.Contains(del, want) {
			t.Errorf("acl_del.sh 缺少 %q", want)
		}
	}
}

// 确保内置脚本包含 IPv6 支持（IPv6 ACL、双栈 tc 限速、IPv6 子链回收）。
func TestDefaultScriptsIPv6Support(t *testing.T) {
	cases := map[string][]string{
		RESOURCE_ID_CLIENT_ONLINE_SCRIPT:  {"ifconfig_pool_remote_ip6", "IP6TABLES", "virtual_ip6", "6#", "match ip6 dst", "family inet6"},
		RESOURCE_ID_CLIENT_OFFLINE_SCRIPT: {"ifconfig_pool_remote_ip6", "IP6TABLES", "virtual_ip6", "6#", "minor_from_v6"},
		RESOURCE_ID_ACL_ADD_SCRIPT:        {"virtual_ip6", "IP6TABLES", "family inet6"},
		RESOURCE_ID_ACL_DEL_SCRIPT:        {"virtual_ip6", "IP6TABLES", "_6s_"},
		RESOURCE_ID_RATE_LIMIT_SCRIPT:     {"ipv6", "minor_from_v6", "match $MATCH"},
		RESOURCE_ID_SERVER_START_SCRIPT:   {"ip6tables", "ov${SERVER_ID}_6c_"},
		RESOURCE_ID_SERVER_EXIT_SCRIPT:    {"ip6tables", "ov${SERVER_ID}_6c_"},
	}
	for id, wants := range cases {
		content := GetSetDefaultResource(RESOURCE_SET_LINUX_IPTABLES, id)
		if content == "" {
			t.Fatalf("默认资源 %s 为空", id)
		}
		for _, want := range wants {
			if !strings.Contains(content, want) {
				t.Errorf("资源 %s 缺少 IPv6 相关内容 %q", id, want)
			}
		}
	}
}

// TestDefaultScriptsDownloadLimitFallback 确保内置脚本的下载限速与普通 Linux 发行版匹配：
// 直接使用 ingress police 按源 IP 限速（普通发行版内核普遍自带 act_police，不做 ifb 探测）。
func TestDefaultScriptsDownloadLimitFallback(t *testing.T) {
	// 上线与达量限速脚本负责设置下载限速，使用 ingress police。
	for _, id := range []string{RESOURCE_ID_CLIENT_ONLINE_SCRIPT, RESOURCE_ID_RATE_LIMIT_SCRIPT} {
		content := GetSetDefaultResource(RESOURCE_SET_LINUX_IPTABLES, id)
		for _, want := range []string{"police rate", "parent ffff"} {
			if !strings.Contains(content, want) {
				t.Errorf("资源 %s 缺少下载限速(police)相关内容 %q", id, want)
			}
		}
	}
	// 下线脚本负责清理主接口 ingress 上的 police 规则。
	offline := GetSetDefaultResource(RESOURCE_SET_LINUX_IPTABLES, RESOURCE_ID_CLIENT_OFFLINE_SCRIPT)
	for _, want := range []string{"parent ffff", "ingress"} {
		if !strings.Contains(offline, want) {
			t.Errorf("client_offline.sh 缺少下载限速清理相关内容 %q", want)
		}
	}
}
