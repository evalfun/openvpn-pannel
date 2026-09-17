package ovpnserver

import (
	"strings"
	"testing"

	"openvpn-pannel/internal/models"
)

func TestRenderOpenvpnClientConfig(t *testing.T) {
	templ := GetDefaultResource(RESOURCE_ID_CLIENT_CONFIG)
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
