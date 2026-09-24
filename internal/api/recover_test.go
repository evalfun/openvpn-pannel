package api

import (
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"openvpn-pannel/internal/config"
	"openvpn-pannel/internal/dao"
	"openvpn-pannel/internal/models"
	ovpnserver "openvpn-pannel/internal/ovpn_server"
)

// TestRecoverRunningServers 验证 stop_instances_on_exit=false 时：
//   - 面板启动会依据数据库 PID 记录接管仍在运行的进程（不重启、不重写配置）；
//   - PID 已失效的记录会被清理；
//   - 停止接管的实例会真正结束进程并删除记录。
func TestRecoverRunningServers(t *testing.T) {
	dir := t.TempDir()
	workDir := filepath.Join(dir, "work")
	no := false
	cfg := &config.Config{
		SQLiteDB:            filepath.Join(dir, "test.db"),
		PasswordSalt:        "testsalt",
		WorkingDir:          workDir,
		InternalAPIListen:   "127.0.0.1:0",
		StopInstancesOnExit: &no,
	}
	dm, err := dao.NewDaoManager(cfg)
	if err != nil {
		t.Fatalf("new dao manager: %v", err)
	}
	if err := models.MigrateDB(dm.DB); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(workDir, "1"), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// 退出脚本空操作，避免改动主机网络。
	for id, content := range map[string]string{
		ovpnserver.RESOURCE_ID_SERVER_EXIT_SCRIPT: "exit 0",
		ovpnserver.RESOURCE_ID_MISC_CONFIG:        `{"shell_path":"/bin/sh"}`,
	} {
		if err := dm.WriteResource(id, content); err != nil {
			t.Fatalf("write resource %s: %v", id, err)
		}
	}

	server := &models.Server{
		Name: "rc", Proto: "udp", Port: 1194, Dev: "tun9",
		CA: "CA", Cert: "CERT", Key: "KEY", DH: "DH", TLSAuthKey: "TA",
		ServerCIDR: "10.8.0.0/24", Topology: "subnet", DataCipher: "AES-256-GCM", Keepalive: "10 60",
	}
	if err := dm.CreateOpenVPNServer(server, nil); err != nil {
		t.Fatalf("create server: %v", err)
	}

	// 模拟一个“面板崩溃后仍在运行”的孤儿进程。
	orphan := exec.Command("sleep", "600")
	if err := orphan.Start(); err != nil {
		t.Fatalf("start orphan: %v", err)
	}
	orphanPID := orphan.Process.Pid
	defer orphan.Process.Kill()

	if err := dm.SaveServerProcess(server.ID, orphanPID); err != nil {
		t.Fatalf("save process: %v", err)
	}
	// 另一条指向不存在进程的记录，应被清理。
	if err := dm.SaveServerProcess(999999, 2147480000); err != nil {
		t.Fatalf("save stale process: %v", err)
	}

	app := &App{
		cfg:             cfg,
		daoManager:      dm,
		ovpnProcessList: map[uint]*ovpnserver.OpenVPNServerInstance{},
		ovpnProcessLock: map[uint]*sync.RWMutex{},
	}
	app.RecoverRunningServers()

	ins := app.ovpnProcessList[server.ID]
	if ins == nil {
		t.Fatal("running instance should have been recovered")
	}
	if !ins.Running() {
		t.Fatal("recovered instance should be reported as running")
	}
	if ins.GetPID() != orphanPID {
		t.Fatalf("recovered PID = %d, want %d", ins.GetPID(), orphanPID)
	}
	if !ins.KeepAlive() {
		t.Fatal("recovered instance should have keepAlive set")
	}

	records, err := dm.ListServerProcessRecords()
	if err != nil {
		t.Fatalf("list process records: %v", err)
	}
	if len(records) != 1 || records[0].ServerID != server.ID {
		t.Fatalf("stale record should be cleaned, got %+v", records)
	}

	// 停止接管的实例：应结束进程并删除记录。
	app.StopAllOpenVPNServer()
	deadline := time.Now().Add(5 * time.Second)
	for ovpnserver.ProcessAlive(orphanPID) && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if ovpnserver.ProcessAlive(orphanPID) {
		t.Fatal("orphan process should have been stopped")
	}
	records, _ = dm.ListServerProcessRecords()
	if len(records) != 0 {
		t.Fatalf("process record should be removed after stop, got %+v", records)
	}
}

// TestRecoverSkippedWhenStopOnExit 验证 stop_instances_on_exit=true（默认）时，
// 启动不会接管任何进程，并清理残留的 PID 记录。
func TestRecoverSkippedWhenStopOnExit(t *testing.T) {
	dir := t.TempDir()
	yes := true
	cfg := &config.Config{
		SQLiteDB:            filepath.Join(dir, "test.db"),
		PasswordSalt:        "testsalt",
		WorkingDir:          filepath.Join(dir, "work"),
		InternalAPIListen:   "127.0.0.1:0",
		StopInstancesOnExit: &yes,
	}
	dm, err := dao.NewDaoManager(cfg)
	if err != nil {
		t.Fatalf("new dao manager: %v", err)
	}
	if err := models.MigrateDB(dm.DB); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	server := &models.Server{Name: "rc2", Proto: "udp", Port: 1194, Dev: "tun9"}
	if err := dm.CreateOpenVPNServer(server, nil); err != nil {
		t.Fatalf("create server: %v", err)
	}
	if err := dm.SaveServerProcess(server.ID, 2147480000); err != nil {
		t.Fatalf("save process: %v", err)
	}

	app := &App{
		cfg:             cfg,
		daoManager:      dm,
		ovpnProcessList: map[uint]*ovpnserver.OpenVPNServerInstance{},
		ovpnProcessLock: map[uint]*sync.RWMutex{},
	}
	app.RecoverRunningServers()
	if len(app.ovpnProcessList) != 0 {
		t.Fatalf("no instance should be recovered, got %d", len(app.ovpnProcessList))
	}
	records, _ := dm.ListServerProcessRecords()
	if len(records) != 0 {
		t.Fatalf("records should be cleared, got %+v", records)
	}
}

// TestPIDPersistedAcrossRestart 端到端验证：退出不停止实例的场景下，
// 面板“崩溃”后重启能接管崩溃前由面板启动的实例，PID 不变。
func TestPIDPersistedAcrossRestart(t *testing.T) {
	dir := t.TempDir()
	workDir := filepath.Join(dir, "work")
	no := false
	cfg := &config.Config{
		SQLiteDB:            filepath.Join(dir, "test.db"),
		PasswordSalt:        "testsalt",
		WorkingDir:          workDir,
		InternalAPIListen:   "127.0.0.1:0",
		StopInstancesOnExit: &no,
	}
	dm, err := dao.NewDaoManager(cfg)
	if err != nil {
		t.Fatalf("new dao manager: %v", err)
	}
	if err := models.MigrateDB(dm.DB); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(workDir, "1"), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	fakeOpenvpn := filepath.Join(dir, "fake-openvpn.sh")
	if err := os.WriteFile(fakeOpenvpn, []byte("#!/bin/sh\nexec sleep 600\n"), 0755); err != nil {
		t.Fatalf("write fake openvpn: %v", err)
	}
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
		if err := dm.WriteResource(id, content); err != nil {
			t.Fatalf("write resource %s: %v", id, err)
		}
	}
	server := &models.Server{
		Name: "rc3", Proto: "udp", Port: 1194, Dev: "tun9",
		CA: "CA", Cert: "CERT", Key: "KEY", DH: "DH", TLSAuthKey: "TA",
		ServerCIDR: "10.8.0.0/24", Topology: "subnet", DataCipher: "AES-256-GCM", Keepalive: "10 60",
	}
	if err := dm.CreateOpenVPNServer(server, nil); err != nil {
		t.Fatalf("create server: %v", err)
	}

	newApp := func() *App {
		return &App{
			cfg:             cfg,
			daoManager:      dm,
			ovpnProcessList: map[uint]*ovpnserver.OpenVPNServerInstance{},
			ovpnProcessLock: map[uint]*sync.RWMutex{},
		}
	}

	// 面板第一次启动并拉起实例。
	app1 := newApp()
	app1.lock.Lock()
	app1.startServerLocked(server, "test")
	app1.lock.Unlock()
	ins1 := app1.ovpnProcessList[server.ID]
	if ins1 == nil || !ins1.Running() {
		t.Fatal("instance should be running after start")
	}
	pid := ins1.GetPID()
	records, _ := dm.ListServerProcessRecords()
	if len(records) != 1 || records[0].PID != pid {
		t.Fatalf("PID should be persisted, got %+v (pid=%d)", records, pid)
	}

	// 模拟面板崩溃：不调用 Stop，直接丢弃 app1，进程成为孤儿。
	// 面板重启：新 App 接管。
	app2 := newApp()
	app2.RecoverRunningServers()
	ins2 := app2.ovpnProcessList[server.ID]
	if ins2 == nil {
		t.Fatal("instance should be recovered after restart")
	}
	if !ins2.Running() {
		t.Fatal("recovered instance should be running")
	}
	if ins2.GetPID() != pid {
		t.Fatalf("recovered PID = %d, want original %d (process must not be restarted)", ins2.GetPID(), pid)
	}

	// 收尾：停止并确认记录清理。
	app2.StopAllOpenVPNServer()
	deadline := time.Now().Add(5 * time.Second)
	for ovpnserver.ProcessAlive(pid) && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if ovpnserver.ProcessAlive(pid) {
		t.Fatal("instance should be stopped")
	}
	records, _ = dm.ListServerProcessRecords()
	if len(records) != 0 {
		t.Fatalf("record should be removed, got %+v", records)
	}
}
