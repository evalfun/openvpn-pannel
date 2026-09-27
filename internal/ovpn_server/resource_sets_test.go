package ovpnserver

import (
	"strings"
	"testing"
)

// TestResourceSetsComplete 确保 4 套内置资源集都齐全：
// 每套都包含全部 13 个资源文件且内容非空，其中 client-page.html 为合法的 HTML。
func TestResourceSetsComplete(t *testing.T) {
	sets := ListResourceSets()
	if len(sets) != 4 {
		t.Fatalf("期望 4 套资源集，实际 %d", len(sets))
	}
	ids := ListResourceIDs()
	if len(ids) != 13 {
		t.Fatalf("期望 13 个资源 ID，实际 %d", len(ids))
	}
	for _, s := range sets {
		if !IsValidResourceSet(s.ID) {
			t.Errorf("资源集 %s 未被 IsValidResourceSet 认可", s.ID)
		}
		for _, item := range ids {
			content := GetSetDefaultResource(s.ID, item.ID)
			if strings.TrimSpace(content) == "" {
				t.Errorf("资源集 %s 的资源 %s 为空", s.ID, item.ID)
			}
		}
		page := GetSetDefaultResource(s.ID, RESOURCE_ID_CLIENT_PAGE)
		if !strings.Contains(page, "<!DOCTYPE html>") || !strings.Contains(page, "api/info") {
			t.Errorf("资源集 %s 的 client-page.html 内容异常", s.ID)
		}
	}
}

// TestResourceSetNftablesDiffers 确保 nftables 资源集确实使用了 nft 脚本（与 iptables 集不同）。
func TestResourceSetNftablesDiffers(t *testing.T) {
	ipt := GetSetDefaultResource(RESOURCE_SET_LINUX_IPTABLES, RESOURCE_ID_CLIENT_ONLINE_SCRIPT)
	nft := GetSetDefaultResource(RESOURCE_SET_LINUX_NFTABLES, RESOURCE_ID_CLIENT_ONLINE_SCRIPT)
	if ipt == nft {
		t.Errorf("ip/nftables 资源集的 client_online.sh 不应相同")
	}
	if !strings.Contains(nft, "nft") {
		t.Errorf("nftables 资源集的 client_online.sh 未使用 nft")
	}
	if !strings.Contains(ipt, "iptables") {
		t.Errorf("iptables 资源集的 client_online.sh 未使用 iptables")
	}
}

// TestResourceSetLinuxDownloadLimit 确保普通 Linux 资源集只用 ingress police，
// 不引入 ifb（降低运维脚本复杂度与风险）。
func TestResourceSetLinuxDownloadLimit(t *testing.T) {
	for _, setID := range []string{RESOURCE_SET_LINUX_IPTABLES, RESOURCE_SET_LINUX_NFTABLES} {
		online := GetSetDefaultResource(setID, RESOURCE_ID_CLIENT_ONLINE_SCRIPT)
		for _, want := range []string{"police rate", "parent ffff"} {
			if !strings.Contains(online, want) {
				t.Errorf("普通 Linux 资源集 %s 的 client_online.sh 缺少 ingress police 相关内容 %q", setID, want)
			}
		}
		offline := GetSetDefaultResource(setID, RESOURCE_ID_CLIENT_OFFLINE_SCRIPT)
		if !strings.Contains(offline, "parent ffff") {
			t.Errorf("普通 Linux 资源集 %s 的 client_offline.sh 未清理 ingress police 过滤器", setID)
		}
		// 普通 Linux 资源集不应包含 ifb 方案。
		for _, resID := range []string{RESOURCE_ID_CLIENT_ONLINE_SCRIPT, RESOURCE_ID_CLIENT_OFFLINE_SCRIPT} {
			content := GetSetDefaultResource(setID, resID)
			if strings.Contains(content, "mirred") || strings.Contains(content, "IFB_DEV") {
				t.Errorf("普通 Linux 资源集 %s 的 %s 不应包含 ifb 方案", setID, resID)
			}
		}
	}
}

// TestResourceSetOpenwrtDownloadLimit 确保 OpenWrt 资源集使用 ifb 优先、police 回退的下载限速方案。
func TestResourceSetOpenwrtDownloadLimit(t *testing.T) {
	for _, setID := range []string{RESOURCE_SET_OPENWRT_IPTABLES, RESOURCE_SET_OPENWRT_NFTABLES} {
		online := GetSetDefaultResource(setID, RESOURCE_ID_CLIENT_ONLINE_SCRIPT)
		for _, want := range []string{"mirred egress redirect", "ovpnrl${SERVER_ID}", "police rate", "INGRESS_CREATED"} {
			if !strings.Contains(online, want) {
				t.Errorf("OpenWrt 资源集 %s 的 client_online.sh 缺少 %q", setID, want)
			}
		}
	}
}

// TestResourceSetHelpDocumentsMemoryEvents 确保每套资源集的帮助信息都说明了 max_memory_events，
// 避免新增配置项后文档漏更新。
func TestResourceSetHelpDocumentsMemoryEvents(t *testing.T) {
	for _, s := range ListResourceSets() {
		help := GetSetDefaultResource(s.ID, RESOURCE_ID_HELP_INFO)
		if !strings.Contains(help, "max_memory_events") {
			t.Errorf("资源集 %s 的 help 未说明 max_memory_events", s.ID)
		}
	}
}
