package api

import (
	"path/filepath"
	"strings"
	"testing"

	"openvpn-pannel/internal/config"
	"openvpn-pannel/internal/models"
	ovpnserver "openvpn-pannel/internal/ovpn_server"
)

func newResourceTestApp(t *testing.T, allowEdit bool) *App {
	t.Helper()
	cfg := &config.Config{
		SQLiteDB:          filepath.Join(t.TempDir(), "test.db"),
		PasswordSalt:      "testsalt",
		WorkingDir:        t.TempDir(),
		InternalAPIListen: "127.0.0.1:0",
		AllowEditResource: allowEdit,
	}
	app := NewApp(cfg, "test")
	if err := models.MigrateDB(app.daoManager.DB); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return app
}

// 默认（无记录）应使用 linux-iptables，并能从该资源集读取到内容。
func TestResourceResolverDefaults(t *testing.T) {
	app := newResourceTestApp(t, false)
	if got := app.GetActiveResourceSetID(); got != ovpnserver.RESOURCE_SET_LINUX_IPTABLES {
		t.Fatalf("默认资源集 = %q, 期望 %q", got, ovpnserver.RESOURCE_SET_LINUX_IPTABLES)
	}
	page := app.GetActiveResourceContent(ovpnserver.RESOURCE_ID_CLIENT_PAGE)
	if !strings.Contains(page, "<!DOCTYPE html>") {
		t.Fatalf("客户端页面资源内容异常")
	}
}

// 切换资源集后，读取到的同名资源内容应随资源集变化。
func TestResourceResolverSwitchSet(t *testing.T) {
	app := newResourceTestApp(t, true)

	ipt := app.GetResourceContent(ovpnserver.RESOURCE_SET_LINUX_IPTABLES, ovpnserver.RESOURCE_ID_CLIENT_ONLINE_SCRIPT)
	nft := app.GetResourceContent(ovpnserver.RESOURCE_SET_LINUX_NFTABLES, ovpnserver.RESOURCE_ID_CLIENT_ONLINE_SCRIPT)
	if ipt == nft {
		t.Fatalf("不同资源集的 client_online.sh 不应相同")
	}

	if err := app.daoManager.SetActiveResourceSetID(ovpnserver.RESOURCE_SET_LINUX_NFTABLES); err != nil {
		t.Fatalf("切换资源集: %v", err)
	}
	if got := app.GetActiveResourceSetID(); got != ovpnserver.RESOURCE_SET_LINUX_NFTABLES {
		t.Fatalf("切换后活动资源集 = %q", got)
	}
	active := app.GetActiveResourceContent(ovpnserver.RESOURCE_ID_CLIENT_ONLINE_SCRIPT)
	if active != nft {
		t.Fatalf("切换后读取内容未跟随资源集")
	}
}

// 种子化：首次访问后，资源集应写入数据库，且覆盖写入生效。
func TestResourceResolverSeedAndWrite(t *testing.T) {
	app := newResourceTestApp(t, true)
	// 触发种子化
	_ = app.PrepareResourceMap([]string{ovpnserver.RESOURCE_ID_MISC_CONFIG})

	n, err := app.daoManager.CountResourceBySet(ovpnserver.RESOURCE_SET_LINUX_IPTABLES)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != int64(len(ovpnserver.ListResourceIDs())) {
		t.Fatalf("种子化后记录数 = %d, 期望 %d", n, len(ovpnserver.ListResourceIDs()))
	}

	// 覆盖写入后读取到自定义内容
	if err := app.daoManager.WriteResourceBySet(ovpnserver.RESOURCE_SET_LINUX_IPTABLES, ovpnserver.RESOURCE_ID_MISC_CONFIG, `{"shell_path":"/bin/zzz"}`); err != nil {
		t.Fatalf("write: %v", err)
	}
	m := app.PrepareResourceMap([]string{ovpnserver.RESOURCE_ID_MISC_CONFIG})
	if !strings.Contains(m[ovpnserver.RESOURCE_ID_MISC_CONFIG], "/bin/zzz") {
		t.Fatalf("覆盖写入未生效: %s", m[ovpnserver.RESOURCE_ID_MISC_CONFIG])
	}

	// 删除后回落内置默认
	if err := app.daoManager.DeleteResourceBySet(ovpnserver.RESOURCE_SET_LINUX_IPTABLES, ovpnserver.RESOURCE_ID_MISC_CONFIG); err != nil {
		t.Fatalf("delete: %v", err)
	}
	m = app.PrepareResourceMap([]string{ovpnserver.RESOURCE_ID_MISC_CONFIG})
	if strings.Contains(m[ovpnserver.RESOURCE_ID_MISC_CONFIG], "/bin/zzz") {
		t.Fatalf("删除后应回落内置默认")
	}
}
