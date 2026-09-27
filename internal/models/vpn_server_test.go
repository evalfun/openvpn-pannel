package models

import "testing"

// TestServerRouteIsIPv6 验证推送路由的 IPv4/IPv6 判定。
func TestServerRouteIsIPv6(t *testing.T) {
	cases := []struct {
		network string
		want    bool
	}{
		{"10.13.2.0 255.255.255.0", false},
		{"192.168.0.0 255.255.0.0", false},
		{"fc00:1024::/32", true},
		{"fd00::/64", true},
		{"2001:db8::/48", true},
	}
	for _, c := range cases {
		r := &ServerRoute{Network: c.network}
		if got := r.IsIPv6(); got != c.want {
			t.Errorf("ServerRoute{%q}.IsIPv6() = %v, 期望 %v", c.network, got, c.want)
		}
	}
	// nil 安全
	var nilRoute *ServerRoute
	if nilRoute.IsIPv6() {
		t.Errorf("nil ServerRoute 应返回 false")
	}
}
