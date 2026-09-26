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

// TestResourceSetOpenwrtDownloadLimit 确保 OpenWrt 资源集使用 ifb 优先的下载限速方案。
func TestResourceSetOpenwrtDownloadLimit(t *testing.T) {
	online := GetSetDefaultResource(RESOURCE_SET_OPENWRT_IPTABLES, RESOURCE_ID_CLIENT_ONLINE_SCRIPT)
	for _, want := range []string{"mirred egress redirect", "ovpnrl${SERVER_ID}", "police rate", "INGRESS_CREATED"} {
		if !strings.Contains(online, want) {
			t.Errorf("OpenWrt 资源集 client_online.sh 缺少 %q", want)
		}
	}
	// 普通 Linux 资源集不启用 ifb，纯 police。
	linuxOnline := GetSetDefaultResource(RESOURCE_SET_LINUX_IPTABLES, RESOURCE_ID_CLIENT_ONLINE_SCRIPT)
	if strings.Contains(linuxOnline, "mirred") {
		t.Errorf("普通 Linux 资源集不应包含 ifb/mirred 方案")
	}
}
