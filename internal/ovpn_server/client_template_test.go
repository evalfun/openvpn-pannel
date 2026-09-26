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
