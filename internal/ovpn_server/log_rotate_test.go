package ovpnserver

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeSized(t *testing.T, path string, n int) {
	t.Helper()
	data := strings.Repeat("x", n)
	if err := os.WriteFile(path, []byte(data), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func sizeOf(t *testing.T, path string) int64 {
	t.Helper()
	st, err := os.Stat(path)
	if os.IsNotExist(err) {
		return 0
	}
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	return st.Size()
}

func totalIn(t *testing.T, dir string) int64 {
	t.Helper()
	dents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	var tot int64
	for _, d := range dents {
		if !d.IsDir() {
			tot += sizeOf(t, filepath.Join(dir, d.Name()))
		}
	}
	return tot
}

func TestRotateLogsBelowThresholdNoop(t *testing.T) {
	dir := t.TempDir()
	writeSized(t, filepath.Join(dir, "auth.log"), 100)
	writeSized(t, filepath.Join(dir, "openvpn.log"), 100)
	rot, del, err := RotateLogsInDir(dir, []string{"openvpn.log", "auth.log"}, 1000)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if rot != 0 || del != 0 {
		t.Fatalf("expected noop, got rotated=%d deleted=%d", rot, del)
	}
	if got := sizeOf(t, filepath.Join(dir, "auth.log")); got != 100 {
		t.Fatalf("auth.log should be untouched, got %d", got)
	}
}

func TestRotateLogsDisabledWhenMaxNonPositive(t *testing.T) {
	dir := t.TempDir()
	writeSized(t, filepath.Join(dir, "auth.log"), 9999)
	rot, del, err := RotateLogsInDir(dir, []string{"auth.log"}, 0)
	if err != nil || rot != 0 || del != 0 {
		t.Fatalf("expected disabled noop, got rot=%d del=%d err=%v", rot, del, err)
	}
	if got := sizeOf(t, filepath.Join(dir, "auth.log")); got != 9999 {
		t.Fatalf("file should be untouched when disabled, got %d", got)
	}
}

func TestRotateLogsCopyTruncatesActive(t *testing.T) {
	dir := t.TempDir()
	// 单个活动日志超过阈值，且没有更早归档 -> 归档它并保留(不删唯一归档)
	writeSized(t, filepath.Join(dir, "auth.log"), 1200)
	rot, del, err := RotateLogsInDir(dir, []string{"auth.log"}, 1000) // target=800
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if rot != 1 {
		t.Fatalf("expected 1 rotated, got %d", rot)
	}
	if del != 0 {
		t.Fatalf("expected newest archive kept (deleted=0), got %d", del)
	}
	if got := sizeOf(t, filepath.Join(dir, "auth.log")); got != 0 {
		t.Fatalf("active log should be truncated to 0, got %d", got)
	}
	// 存在一个归档且含全部内容
	var archSize int64 = -1
	_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasPrefix(d.Name(), "auth.log.") {
			archSize = sizeOf(t, p)
		}
		return nil
	})
	if archSize != 1200 {
		t.Fatalf("archive should retain 1200 bytes, got %d", archSize)
	}
}

func TestRotateLogsDeletesOldestKeepsNewest(t *testing.T) {
	dir := t.TempDir()
	active := filepath.Join(dir, "auth.log")
	writeSized(t, active, 200) // 触发：总 1100 >= 1000

	base := time.Now()
	olds := []struct {
		name string
		size int
		age  time.Duration
	}{
		{"auth.log.old1", 300, 30 * time.Minute},
		{"auth.log.old2", 300, 20 * time.Minute},
		{"auth.log.old3", 300, 10 * time.Minute},
	}
	for _, o := range olds {
		p := filepath.Join(dir, o.name)
		writeSized(t, p, o.size)
		mt := base.Add(-o.age)
		if err := os.Chtimes(p, mt, mt); err != nil {
			t.Fatalf("chtimes: %v", err)
		}
	}

	rot, del, err := RotateLogsInDir(dir, []string{"auth.log"}, 1000) // target=800
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if rot != 1 {
		t.Fatalf("expected rotate of active log, got %d", rot)
	}
	if del < 1 {
		t.Fatalf("expected at least one oldest archive deleted, got %d", del)
	}
	// 归档 old1 (最早) 应被删除
	if _, err := os.Stat(filepath.Join(dir, "auth.log.old1")); !os.IsNotExist(err) {
		t.Fatalf("oldest archive auth.log.old1 should be deleted")
	}
	// 轮换后总量应 <= target(800) 或仅剩最新归档；active 被清零
	if got := sizeOf(t, active); got != 0 {
		t.Fatalf("active should be truncated, got %d", got)
	}
	total := totalIn(t, dir)
	if total > 800 {
		t.Fatalf("expected total trimmed to <=800, got %d", total)
	}
}

// 校验实例方法 RotateLogsFromMisc 能通过(内置默认)misc 正确解析出 openvpn.log / auth.log 并轮换。
func TestRotateLogsFromMiscUsesDefaultLogNames(t *testing.T) {
	dir := t.TempDir() + "/"
	writeSized(t, filepath.Join(dir, "auth.log"), 3000) // 超过阈值 -> 应被轮换
	writeSized(t, filepath.Join(dir, "openvpn.log"), 0) // 空 -> 跳过

	ins := &OpenVPNServerInstance{workingDir: dir}
	// 空 resourceMap -> getMiscConfig 回落内置默认(server_log=openvpn.log, script_log=auth.log)
	if err := ins.RotateLogsFromMisc(map[string]string{}, 1000); err != nil {
		t.Fatalf("RotateLogsFromMisc err: %v", err)
	}
	if got := sizeOf(t, filepath.Join(dir, "auth.log")); got != 0 {
		t.Fatalf("auth.log should be truncated via default misc name resolution, got %d", got)
	}
	var archCount int
	_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasPrefix(d.Name(), "auth.log.") {
			archCount++
		}
		return nil
	})
	if archCount == 0 {
		t.Fatalf("expected an auth.log.* archive to be created")
	}
	// 单个超阈值归档作为最新内容会被保留(设计如此)，openvpn.log 保持空未被改动
	if got := sizeOf(t, filepath.Join(dir, "openvpn.log")); got != 0 {
		t.Fatalf("empty openvpn.log should stay untouched, got %d", got)
	}
}
