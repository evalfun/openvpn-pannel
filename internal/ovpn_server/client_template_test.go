package ovpnserver

import (
	"strings"
	"testing"

	"openvpn-pannel/internal/models"
)

func TestRenderOpenvpnClientConfig(t *testing.T) {
	templ := GetSetDefaultResource(RESOURCE_SET_LINUX_IPTABLES, RESOURCE_ID_CLIENT_CONFIG)
	if strings.TrimSpace(templ) == "" {
		t.Fatal("client-config default resource is empty")
	}
	param := &OpenvpnClientTemplateParam{
		Server:     &models.Server{Proto: "udp", DataCipher: "AES-256-GCM"},
		CA:         "-----BEGIN CERTIFICATE-----\nCA\n-----END CERTIFICATE-----\n",
		Cert:       "-----BEGIN CERTIFICATE-----\nCERT\n-----END CERTIFICATE-----\n",
		Key:        "-----BEGIN PRIVATE KEY-----\nKEY\n-----END PRIVATE KEY-----\n",
		TLSAuthKey: "-----BEGIN OpenVPN Static key V1-----\nTA\n-----END OpenVPN Static key V1-----\n",
		RemoteHost: "vpn.example.com",
		RemotePort: 1194,
	}
	out, err := RenderOpenvpnClientConfig(templ, param)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	for _, want := range []string{
		"remote vpn.example.com 1194",
		"proto udp",
		"auth-user-pass",
		"data-ciphers AES-256-GCM",
		"<ca>", "</ca>",
		"<cert>", "</cert>",
		"<key>", "</key>",
		"key-direction 1",
		"<tls-auth>", "</tls-auth>",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected output to contain %q, got:\n%s", want, out)
		}
	}

	param.Key = ""
	param.TLSAuthKey = ""
	out, err = RenderOpenvpnClientConfig(templ, param)
	if err != nil {
		t.Fatalf("render without key: %v", err)
	}
	if strings.Contains(out, "<key>") {
		t.Errorf("expected no <key> block when key is empty, got:\n%s", out)
	}
	if strings.Contains(out, "<tls-auth>") {
		t.Errorf("expected no <tls-auth> block when tls key is empty, got:\n%s", out)
	}
}

// TestRenderOpenvpnServerConfigIPv6 验证填写 server_cidr6 时渲染出 server-ipv6，留空时不渲染。
func TestRenderOpenvpnServerConfigIPv6(t *testing.T) {
	templ := GetSetDefaultResource(RESOURCE_SET_LINUX_IPTABLES, RESOURCE_ID_CONFIG_TEMPLATE)
	misc := &MiscConfig{
		ServerConfigFileName: "server.conf", CAFileName: "ca.crt", ServerCertFileName: "server.crt",
		ServerKeyFileName: "server.key", DHFileName: "dh.pem", TAFileName: "ta.key",
		ManagementSocket: "mgmt.sock", StatusFileName: "status.log", ServerLog: "openvpn.log",
		CCDDir: "ccd", IPPFileName: "ipp.txt",
	}

	withV6 := &OpenvpnServerTemplateParam{
		ServerConfig: &models.Server{Port: 1194, Proto: "udp", Dev: "tun0", ServerCIDR: "10.8.0.0 255.255.255.0", ServerCIDR6: "fc00:2048:1024::/64", Topology: "subnet", Keepalive: "10 60"},
		MiscConfig:   misc,
	}
	out, err := RenderOpenvpnServerConfig(templ, withV6)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(out, "server-ipv6 fc00:2048:1024::/64") {
		t.Errorf("expected server-ipv6 line, got:\n%s", out)
	}

	noV6 := &OpenvpnServerTemplateParam{
		ServerConfig: &models.Server{Port: 1194, Proto: "udp", Dev: "tun0", ServerCIDR: "10.8.0.0 255.255.255.0", Topology: "subnet", Keepalive: "10 60"},
		MiscConfig:   misc,
	}
	out2, err := RenderOpenvpnServerConfig(templ, noV6)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if strings.Contains(out2, "server-ipv6") {
		t.Errorf("did not expect server-ipv6 line, got:\n%s", out2)
	}
}

// TestRenderOpenvpnServerConfigRoutes 验证 IPv4 用 push "route ..."、
// IPv6 用 push "route-ipv6 ..."，且 4 套内置资源集的配置模板行为一致。
func TestRenderOpenvpnServerConfigRoutes(t *testing.T) {
	misc := &MiscConfig{
		ServerConfigFileName: "server.conf", CAFileName: "ca.crt", ServerCertFileName: "server.crt",
		ServerKeyFileName: "server.key", DHFileName: "dh.pem", TAFileName: "ta.key",
		ManagementSocket: "mgmt.sock", StatusFileName: "status.log", ServerLog: "openvpn.log",
		CCDDir: "ccd", IPPFileName: "ipp.txt",
	}
	param := &OpenvpnServerTemplateParam{
		ServerConfig: &models.Server{Port: 1194, Proto: "udp", Dev: "tun0", ServerCIDR: "10.8.0.0 255.255.255.0", Topology: "subnet", Keepalive: "10 60"},
		ServerRoute: []*models.ServerRoute{
			{Network: "10.13.2.0 255.255.255.0"},
			{Network: "fc00:1024::/32"},
		},
		MiscConfig: misc,
	}
	for _, s := range ListResourceSets() {
		templ := GetSetDefaultResource(s.ID, RESOURCE_ID_CONFIG_TEMPLATE)
		out, err := RenderOpenvpnServerConfig(templ, param)
		if err != nil {
			t.Fatalf("资源集 %s 渲染失败: %v", s.ID, err)
		}
		if !strings.Contains(out, `push "route 10.13.2.0 255.255.255.0"`) {
			t.Errorf("资源集 %s 缺少 IPv4 推送路由, got:\n%s", s.ID, out)
		}
		if !strings.Contains(out, `push "route-ipv6 fc00:1024::/32"`) {
			t.Errorf("资源集 %s 缺少 IPv6 推送路由（应为 route-ipv6）, got:\n%s", s.ID, out)
		}
		// IPv6 路由不应被渲染成 push "route fc00:1024::/32"。
		if strings.Contains(out, `push "route fc00:1024::/32"`) {
			t.Errorf("资源集 %s 的 IPv6 路由被错误渲染为 route 而非 route-ipv6", s.ID)
		}
	}

	// 无路由时不渲染任何 push "route / push "route-ipv6（保留 push "dhcp-option 等其它 push 不算）。
	emptyParam := &OpenvpnServerTemplateParam{
		ServerConfig: param.ServerConfig,
		ServerRoute:  nil,
		MiscConfig:   misc,
	}
	templ := GetSetDefaultResource(RESOURCE_SET_LINUX_IPTABLES, RESOURCE_ID_CONFIG_TEMPLATE)
	out, err := RenderOpenvpnServerConfig(templ, emptyParam)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if strings.Contains(out, `push "route`) {
		t.Errorf("无路由时不应出现 push \"route/route-ipv6, got:\n%s", out)
	}
}
