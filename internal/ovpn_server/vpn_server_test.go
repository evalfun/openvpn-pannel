package ovpnserver

import (
	"os"
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
