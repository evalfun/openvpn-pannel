package api

import (
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"

	"openvpn-pannel/internal/config"
	"openvpn-pannel/internal/dao"
	"openvpn-pannel/internal/models"
	ovpnserver "openvpn-pannel/internal/ovpn_server"
)

// TestKeepAliveRestartsCrashedServer 用 kill 模拟 openvpn 进程异常退出，
// 验证保活检查会重新拉起该服务器。
func TestKeepAliveRestartsCrashedServer(t *testing.T) {
	dir := t.TempDir()
	workDir := filepath.Join(dir, "work")
	cfg := &config.Config{
		SQLiteDB:          filepath.Join(dir, "test.db"),
		PasswordSalt:      "testsalt",
		WorkingDir:        workDir,
		InternalAPIListen: "127.0.0.1:0",
	}
	dm, err := dao.NewDaoManager(cfg)
	if err != nil {
		t.Fatalf("new dao manager: %v", err)
	}
	if err := models.MigrateDB(dm.DB); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// 服务器工作目录。用一个“假 openvpn” 包装脚本代替真实进程：忽略传入的配置文件参数，前台睡眠。
	serverWorkDir := filepath.Join(workDir, "1")
	if err := os.MkdirAll(serverWorkDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	fakeOpenvpn := filepath.Join(dir, "fake-openvpn.sh")
	if err := os.WriteFile(fakeOpenvpn, []byte("#!/bin/sh\nexec sleep 600\n"), 0755); err != nil {
		t.Fatalf("write fake openvpn: %v", err)
	}

	// 覆盖资源：openvpn_path 指向假进程；启动/退出脚本空操作，避免改动主机 iptables。
	misc := `{"shell_path":"/bin/sh","openvpn_path":"` + fakeOpenvpn + `","server_config_file_name":"server.conf",` +
		`"ca_file_name":"ca.crt","server_cert_file_name":"server.crt","server_key_file_name":"server.key",` +
		`"dh_file_name":"dh.pem","ta_file_name":"ta.key","ccd_dir":"ccd","ipp_file_name":"ipp.txt",` +
		`"status_file_name":"status.log","client_online_script_name":"_client_online.sh",` +
		`"client_offline_script_name":"_client_offline.sh","client_auth_script_name":"_auth.sh"}`
	for id, content := range map[string]string{
		ovpnserver.RESOURCE_ID_MISC_CONFIG:           misc,
		ovpnserver.RESOURCE_ID_SERVER_START_SCRIPT:   "exit 0",
		ovpnserver.RESOURCE_ID_SERVER_EXIT_SCRIPT:    "exit 0",
		ovpnserver.RESOURCE_ID_CONFIG_TEMPLATE:       "port {{.ServerConfig.Port}}\n",
		ovpnserver.RESOURCE_ID_CLIENT_ONLINE_SCRIPT:  "exit 0",
		ovpnserver.RESOURCE_ID_CLIENT_OFFLINE_SCRIPT: "exit 0",
		ovpnserver.RESOURCE_ID_AUTH_SCRIPT:           "exit 0",
	} {
		if err := dm.WriteResourceBySet(ovpnserver.RESOURCE_SET_LINUX_IPTABLES, id, content); err != nil {
			t.Fatalf("write resource %s: %v", id, err)
		}
	}

	server := &models.Server{
		Name: "ka", Proto: "udp", Port: 1194, Dev: "tun9",
		CA: "CA", Cert: "CERT", Key: "KEY", DH: "DH", TLSAuthKey: "TA",
		ServerCIDR: "10.8.0.0/24", Topology: "subnet", DataCipher: "AES-256-GCM", Keepalive: "10 60",
	}
	if err := dm.CreateOpenVPNServer(server, nil); err != nil {
		t.Fatalf("create server: %v", err)
	}

	app := &App{
		cfg:             cfg,
		daoManager:      dm,
		ovpnProcessList: map[uint]*ovpnserver.OpenVPNServerInstance{},
		ovpnProcessLock: map[uint]*sync.RWMutex{},
	}

	app.lock.Lock()
	app.startServerLocked(server, "test")
	app.lock.Unlock()

	app.lock.RLock()
	ins := app.ovpnProcessList[server.ID]
	app.lock.RUnlock()
	if ins == nil {
		t.Fatal("instance not created")
	}
	if !ins.Running() {
		t.Fatal("server should be running after start")
	}
	oldPID := ins.GetPID()

	// 用 kill 模拟 openvpn 进程异常退出
	if err := syscall.Kill(oldPID, syscall.SIGKILL); err != nil {
		t.Fatalf("kill: %v", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for ins.Running() && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if ins.Running() {
		t.Fatal("crashed process should be reported as not running")
	}

	// 触发保活检查
	app.checkAndRestartServers()

	app.lock.RLock()
	ins2 := app.ovpnProcessList[server.ID]
	app.lock.RUnlock()
	if ins2 == nil {
		t.Fatal("instance missing after keepalive")
	}
	if !ins2.Running() {
		t.Fatal("keepalive should have restarted the process")
	}
	if ins2.GetPID() == oldPID {
		t.Fatalf("expected a new PID after restart, got same %d", oldPID)
	}

	// 看门狗拉起后应记录“服务器已恢复”事件
	resp, err := dm.GetEventList(server.ID, []int{models.SERVER_EVENT_TYPE_SERVER_RECOVERED}, "", 0, 0, 1, 10)
	if err != nil {
		t.Fatalf("get event list: %v", err)
	}
	if resp.Total < 1 {
		t.Fatalf("expected a SERVER_RECOVERED event, got total=%d", resp.Total)
	}

	// 主动停止后不应再被保活拉起。
	if err := ins2.Stop(app.PrepareResourceMap([]string{ovpnserver.RESOURCE_ID_MISC_CONFIG, ovpnserver.RESOURCE_ID_SERVER_EXIT_SCRIPT})); err != nil {
		t.Fatalf("stop: %v", err)
	}
	app.checkAndRestartServers()
	app.lock.RLock()
	ins3 := app.ovpnProcessList[server.ID]
	app.lock.RUnlock()
	if ins3 == nil {
		t.Fatal("instance should remain in list after stop")
	}
	if ins3.Running() {
		t.Fatal("intentionally stopped server must not be restarted by keepalive")
	}
}
