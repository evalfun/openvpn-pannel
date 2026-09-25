package api

import "testing"

func TestIPInServerCIDR(t *testing.T) {
	cases := []struct {
		ip   string
		cidr string
		want bool
	}{
		{"10.8.0.2", "10.8.0.0/24", true},
		{"10.8.1.2", "10.8.0.0/24", false},
		{"10.8.0.2", "10.8.0.0 255.255.255.0", true},
		{"10.8.1.2", "10.8.0.0 255.255.255.0", false},
		{"10.8.0.2", "10.8.0.2", true},
		{"10.8.0.3", "10.8.0.2", false},
		{"192.168.1.1", "0.0.0.0/0", true},
		{"10.8.0.2", "", false},
		{"not-an-ip", "10.8.0.0/24", false},
		{"10.8.0.2", "bad-cidr", false},
		{"fc00:2048:1024::2", "fc00:2048:1024::/64", true},
		{"fc00:2048:1025::2", "fc00:2048:1024::/64", false},
		{"fc00:2048:1024::2", "::/0", true},
		{"10.8.0.2", "fc00:2048:1024::/64", false},
	}
	for _, c := range cases {
		if got := ipInServerCIDR(c.ip, c.cidr); got != c.want {
			t.Errorf("ipInServerCIDR(%q, %q) = %v, want %v", c.ip, c.cidr, got, c.want)
		}
	}
}
