package ovpnserver

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"openvpn-pannel/internal/models"
)

// writeSleepScript 在 dir 下写入一个前台睡眠指定秒数的脚本，作为 openvpn 进程的替身。
func writeSleepScript(t *testing.T, dir, name string, seconds int) {
	t.Helper()
	content := "#!/bin/sh\nsleep " + time.Duration(seconds).String() + "\n"
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0755); err != nil {
		t.Fatalf("write script: %v", err)
	}
}

// waitUntil 轮询 cond 直到为真或超时。
func waitUntil(t *testing.T, timeout time.Duration, desc string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %s", desc)
}

func TestKilledProcessIsNotRunning(t *testing.T) {
	dir := t.TempDir()
	writeSleepScript(t, dir, "run.sh", 30)

	res := map[string]string{
		RESOURCE_ID_MISC_CONFIG:         `{"shell_path":"/bin/sh","server_config_file_name":"run.sh"}`,
		RESOURCE_ID_SERVER_START_SCRIPT: "exit 0",
		RESOURCE_ID_SERVER_EXIT_SCRIPT:  "exit 0",
	}
	ins := NewOpenVPNServerInstance(&models.Server{ID: 1, Name: "keepalive-test"}, nil, nil, dir, "", "/bin/sh")
	if err := ins.Start(res); err != nil {
		t.Fatalf("start: %v", err)
	}

	if !ins.Running() {
		t.Fatal("process should be running after Start")
	}
	if !ins.KeepAlive() {
		t.Fatal("KeepAlive should be true after Start")
	}

	pid := ins.GetPID()
	if err := syscall.Kill(pid, syscall.SIGKILL); err != nil {
		t.Fatalf("kill: %v", err)
	}

	// 进程被杀后即使处于僵尸态，Running 也必须返回 false；否则看门狗无法发现异常退出。
	waitUntil(t, 5*time.Second, "process reported as not running", func() bool { return !ins.Running() })

	// 主动停止：KeepAlive 清零，之后不会被看门狗拉起。
	if err := ins.Stop(res); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if ins.KeepAlive() {
		t.Fatal("KeepAlive should be false after Stop")
	}
}

// TestProcessAlive 验证 ProcessAlive 对存活/不存在/僵尸进程的判定。
func TestProcessAlive(t *testing.T) {
	if ProcessAlive(-1) || ProcessAlive(0) {
		t.Fatal("invalid PID should not be alive")
	}
	// 当前进程自身必然存活。
	if !ProcessAlive(os.Getpid()) {
		t.Fatal("current process should be alive")
	}
	// 不存在的 PID。
	if ProcessAlive(2147480000) {
		t.Fatal("non-existent PID should not be alive")
	}

	// 僵尸进程：kill 后不回收，ProcessAlive 必须返回 false。
	cmd := exec.Command("sleep", "600")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	pid := cmd.Process.Pid
	if err := syscall.Kill(pid, syscall.SIGKILL); err != nil {
		t.Fatalf("kill: %v", err)
	}
	waitUntil(t, 5*time.Second, "process to become zombie", func() bool {
		return processIsZombie(pid)
	})
	if ProcessAlive(pid) {
		t.Fatal("zombie process should not be reported as alive")
	}
	cmd.Wait()
}

func TestStripAddrProto(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"udp4:61.171.212.168:15778", "61.171.212.168:15778"},
		{"udp6:[2001:db8::1]:15778", "[2001:db8::1]:15778"},
		{"tcp4:1.2.3.4:5000", "1.2.3.4:5000"},
		{"61.171.212.168:15778", "61.171.212.168:15778"},
		{"[2001:db8::1]:15778", "[2001:db8::1]:15778"},
	}
	for _, c := range cases {
		if got := stripAddrProto(c.in); got != c.want {
			t.Errorf("stripAddrProto(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestNormalizeRealAddr(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		// status 输出的协议前缀 + 大小写差异，都归一化为同一形式。
		{"udp4:101.82.177.200:20053", "101.82.177.200:20053"},
		{"UDP4:101.82.177.200:20053", "101.82.177.200:20053"},
		{"udp6:[2001:db8::ABCD]:51820", "2001:db8::abcd:51820"},
		// 脚本上报的 IPv6 写法（带方括号、无协议前缀）。
		{"[2001:db8::abcd]:51820", "2001:db8::abcd:51820"},
		{"  203.0.113.9:51820  ", "203.0.113.9:51820"},
	}
	for _, c := range cases {
		if got := normalizeRealAddr(c.in); got != c.want {
			t.Errorf("normalizeRealAddr(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestParseStatus2Output(t *testing.T) {
	// 取自真实 `status 2` 输出（含 IPv4 客户端与 IPv6 客户端）。
	output := "TITLE,OpenVPN 2.7.6 x86_64-openwrt-linux-gnu\r\n" +
		"TIME,2026-09-26 19:40:04,1790422804\r\n" +
		"HEADER,CLIENT_LIST,Common Name,Real Address,Virtual Address,Virtual IPv6 Address,Bytes Received,Bytes Sent,Connected Since,Connected Since (time_t),Username,Client ID,Peer ID,Data Channel Cipher\r\n" +
		"CLIENT_LIST,client-op13,udp4:101.82.177.200:20053,10.12.2.2,fc00:1024:3371::1000,11364,44425,2026-09-26 19:40:01,1790422801,oneplus13,3,0,AES-256-GCM\r\n" +
		"CLIENT_LIST,client-v6,udp6:[2001:db8::abcd]:51820,,fc00:1024:3371::1001,1,2,2026-09-26 19:41:00,1790422860,user6,7,0,AES-256-GCM\r\n" +
		"HEADER,ROUTING_TABLE,Virtual Address,Common Name,Real Address,Last Ref,Last Ref (time_t)\r\n" +
		"ROUTING_TABLE,fc00:1024:3371::1000,client-op13,udp4:101.82.177.200:20053,2026-09-26 19:40:01,1790422801\r\n" +
		"ROUTING_TABLE,10.12.2.2,client-op13,udp4:101.82.177.200:20053,2026-09-26 19:40:03,1790422803\r\n" +
		"GLOBAL_STATS,Max bcast/mcast queue length,0\r\n" +
		"GLOBAL_STATS,dco_enabled,0\r\n" +
		"END\r\n"

	clients := parseStatus2Output(output)
	if len(clients) != 2 {
		t.Fatalf("got %d clients, want 2", len(clients))
	}

	ipv4 := clients[0]
	if ipv4.CommonName != "client-op13" || ipv4.ClientID != "3" || ipv4.Username != "oneplus13" {
		t.Errorf("unexpected ipv4 client: %+v", ipv4)
	}
	if ipv4.RealIPAddr != "udp4:101.82.177.200:20053" {
		t.Errorf("ipv4 real addr = %q", ipv4.RealIPAddr)
	}
	// CLIENT_LIST 的 IPv4/IPv6 虚拟地址都各归其位。
	if len(ipv4.VirtualIPAddr) != 1 || ipv4.VirtualIPAddr[0] != "10.12.2.2" {
		t.Errorf("ipv4 virtual addr = %#v", ipv4.VirtualIPAddr)
	}
	if len(ipv4.VirtualIP6Addr) != 1 || ipv4.VirtualIP6Addr[0] != "fc00:1024:3371::1000" {
		t.Errorf("ipv4 virtual ip6 addr = %#v", ipv4.VirtualIP6Addr)
	}
	if ipv4.ByteReceived != 11364 || ipv4.ByteSent != 44425 {
		t.Errorf("ipv4 bytes = %d/%d", ipv4.ByteReceived, ipv4.ByteSent)
	}
	if ipv4.LastRef != "2026-09-26 19:40:03" {
		t.Errorf("ipv4 last ref = %q", ipv4.LastRef)
	}

	ipv6 := clients[1]
	if ipv6.CommonName != "client-v6" || ipv6.ClientID != "7" {
		t.Errorf("unexpected ipv6 client: %+v", ipv6)
	}
	if ipv6.RealIPAddr != "udp6:[2001:db8::abcd]:51820" {
		t.Errorf("ipv6 real addr = %q", ipv6.RealIPAddr)
	}
	// 空的 IPv4 虚拟地址不应被追加。
	if len(ipv6.VirtualIPAddr) != 0 {
		t.Errorf("ipv6 client should have no IPv4 virtual addr, got %#v", ipv6.VirtualIPAddr)
	}
}

func TestFindClientForKill(t *testing.T) {
	clients := []*ServerStatusClientInfoResponse{
		{CommonName: "client-a", RealIPAddr: "udp4:101.82.177.200:20053", ClientID: "3"},
		{CommonName: "client-b", RealIPAddr: "udp6:[2001:db8::abcd]:51820", ClientID: "7"},
	}

	// 按证书名匹配。
	if got := findClientForKill(clients, "client-b", ""); got == nil || got.ClientID != "7" {
		t.Errorf("match by common name failed: %+v", got)
	}

	// 脚本上报的真实地址（IPv4，无协议前缀）应能匹配 status 输出（带协议前缀）。
	if got := findClientForKill(clients, "", "101.82.177.200:20053"); got == nil || got.ClientID != "3" {
		t.Errorf("match ipv4 by script addr failed: %+v", got)
	}

	// 脚本上报 IPv6 真实地址 "[ipv6]:port" 应能匹配 status 的 "udp6:[ipv6]:port"。
	if got := findClientForKill(clients, "", "[2001:db8::abcd]:51820"); got == nil || got.ClientID != "7" {
		t.Errorf("match ipv6 by script addr failed: %+v", got)
	}

	// 未知地址返回 nil。
	if got := findClientForKill(clients, "", "203.0.113.9:1"); got != nil {
		t.Errorf("unknown addr should return nil, got %+v", got)
	}

	// 未知证书名返回 nil。
	if got := findClientForKill(clients, "nope", ""); got != nil {
		t.Errorf("unknown common name should return nil, got %+v", got)
	}
}
