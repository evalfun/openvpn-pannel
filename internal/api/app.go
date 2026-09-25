package api

import (
	"fmt"
	"log"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"openvpn-pannel/internal/config"
	"openvpn-pannel/internal/dao"
	ovpnserver "openvpn-pannel/internal/ovpn_server"
)

type App struct {
	data            uint32
	router          *gin.Engine
	cfg             *config.Config
	daoManager      *dao.DaoManager
	ovpnProcessList map[uint]*ovpnserver.OpenVPNServerInstance
	// ovpnProcessLock 为每个服务器实例保存一把读写锁，保护该实例自身的可变状态与操作：
	// 进程启停(cmd/pid)、配置文件/ccd 写入、管理 socket 命令(status/kill)、日志读写与轮换。
	//
	// 这里必须用 *sync.RWMutex：map 元素不可取地址，值类型的锁无法原地 Lock()，
	// 复制出来加锁只会锁到副本（且 go vet 会报 copylocks）。
	ovpnProcessLock map[uint]*sync.RWMutex
	// ovpnProcessLockMu 只保护 ovpnProcessLock 这个 map 的读写。它独立于全局锁 a.lock，
	// 这样即便调用方已持有 a.lock，也能安全地获取实例锁，避免自死锁。
	ovpnProcessLockMu sync.Mutex
	// lock 全局读写锁，只保护 ovpnProcessList 这个 map 的读写。
	lock              sync.RWMutex
	internalAPIRouter *gin.Engine
	buildDate         string
	// loginCooldown 对每个用户的登录密码与 MFA 验证码尝试做最小间隔限制，防暴力破解。
	loginCooldown *cooldownLimiter
}

func (a *App) Run() {
	a.RecoverRunningServers()
	a.AutoStartOpenVPNServer()
	go a.internalAPIRouter.Run(a.cfg.InternalAPIListen)
	// 面向已连接 VPN 客户端的自助页面（额外端口），未配置则不启用。
	if a.cfg.ClientPageListen != "" {
		clientPageRouter := a.SetupClientPageRouter()
		go func() {
			if err := clientPageRouter.Run(a.cfg.ClientPageListen); err != nil {
				log.Printf("客户端自助页面监听失败 %s: %v", a.cfg.ClientPageListen, err)
			}
		}()
	}
	go a.StartConnectedClientInfoUpdater(12 * time.Second)
	go a.StartLogRotationTask(60 * time.Second)
	go a.StartServerKeepAliveTask(60 * time.Second)
	a.router.Run(a.cfg.Listen)
}

// setupTrustedProxies 配置 gin 信任的反向代理来源。
//
// 只有请求来自受信任代理时，gin 才会把 X-Forwarded-For / X-Real-IP 当作客户端真实 IP；
// 否则客户端可随意伪造这些头，导致日志/审计记录到虚假来源 IP。
// config.trusted_proxies 为空时禁用所有代理信任，ClientIP 直接取 TCP 来源地址。
func (a *App) setupTrustedProxies() {
	proxies := a.cfg.TrustedProxies
	if len(proxies) == 0 {
		_ = a.router.SetTrustedProxies(nil)
		_ = a.internalAPIRouter.SetTrustedProxies(nil)
		return
	}
	if err := a.router.SetTrustedProxies(proxies); err != nil {
		log.Printf("配置面板可信代理失败: %v", err)
	}
	if err := a.internalAPIRouter.SetTrustedProxies(proxies); err != nil {
		log.Printf("配置内部 API 可信代理失败: %v", err)
	}
}

// getProcessLock 返回指定服务器实例的读写锁；不存在时创建。
// 仅操作 ovpnProcessLock 自身，不涉及全局锁 a.lock，可在任意上下文安全调用。
// 加锁顺序约定：先获取全局锁 a.lock（若需要），再获取实例锁；持有实例锁时不要再获取 a.lock。
func (a *App) getProcessLock(serverID uint) *sync.RWMutex {
	a.ovpnProcessLockMu.Lock()
	defer a.ovpnProcessLockMu.Unlock()
	if a.ovpnProcessLock == nil {
		a.ovpnProcessLock = make(map[uint]*sync.RWMutex)
	}
	l, ok := a.ovpnProcessLock[serverID]
	if !ok {
		l = &sync.RWMutex{}
		a.ovpnProcessLock[serverID] = l
	}
	return l
}

// removeProcessLock 删除指定服务器实例的锁（服务器被删除时调用）。
func (a *App) removeProcessLock(serverID uint) {
	a.ovpnProcessLockMu.Lock()
	defer a.ovpnProcessLockMu.Unlock()
	delete(a.ovpnProcessLock, serverID)
}

// snapshotServerInstances 在全局读锁下复制一份实例列表，供定时任务遍历，
// 避免遍历期间实例被增删导致并发读写 map。
func (a *App) snapshotServerInstances() []*ovpnserver.OpenVPNServerInstance {
	a.lock.RLock()
	defer a.lock.RUnlock()
	list := make([]*ovpnserver.OpenVPNServerInstance, 0, len(a.ovpnProcessList))
	for _, ins := range a.ovpnProcessList {
		list = append(list, ins)
	}
	return list
}

func (a *App) StartConnectedClientInfoUpdater(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		a.updateConnectedClientInfoRecords()
	}
}

// StartLogRotationTask 周期性检查各服务器日志容量，达到 config.max_log_size_kb 时轮换并清理最早日志。
func (a *App) StartLogRotationTask(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		a.rotateAllServerLogs()
	}
}

// StartServerKeepAliveTask 周期性检查各服务器的 openvpn 进程是否存活，
// 对面板期望运行但已异常退出的实例自动重新拉起，避免失联。
// 存活检测只基于子进程状态（Wait 回收 + signal 0），不连接 management socket，
// 因此不会向 openvpn 产生额外的管理接口日志。
func (a *App) StartServerKeepAliveTask(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		a.checkAndRestartServers()
	}
}

// checkAndRestartServers 检查并拉起所有面板期望运行但进程已退出的服务器。
func (a *App) checkAndRestartServers() {
	a.lock.Lock()
	defer a.lock.Unlock()

	for serverID, serverInstance := range a.ovpnProcessList {
		// 只保活“面板期望其运行”的实例；主动停止或不曾启动的实例不处理。
		if !serverInstance.KeepAlive() {
			continue
		}
		pl := a.getProcessLock(serverID)
		pl.RLock()
		alive := serverInstance.Running()
		pl.RUnlock()
		if alive {
			continue
		}
		serverModel, err := a.daoManager.GetOpenVPNServerByID(serverID)
		if err != nil {
			log.Printf("保活检查: 服务器 %d 已不存在，清理进程列表条目", serverID)
			delete(a.ovpnProcessList, serverID)
			a.removeProcessLock(serverID)
			continue
		}
		log.Printf("保活检查: 服务器 %d (%s) 的 openvpn 进程已退出，正在重新拉起", serverID, serverModel.Name)
		a.startServerLocked(serverModel, "keepalive")
	}
}

// recordServerProcess 在“退出时不停止实例”（stop_instances_on_exit=false）模式下，
// 把实例 PID 记入数据库，供面板重启后重新接管。其它模式下不写记录。
func (a *App) recordServerProcess(serverID uint, pid int) {
	if a.cfg.ShouldStopInstancesOnExit() {
		return
	}
	if err := a.daoManager.SaveServerProcess(serverID, pid); err != nil {
		log.Printf("记录服务器 %d 进程 PID 失败: %v", serverID, err)
	}
}

// RecoverRunningServers 面板启动时，依据数据库中的进程 PID 记录重新接管仍在运行的
// OpenVPN 实例。仅在 config.stop_instances_on_exit=false（退出时不停止实例）时启用；
// 否则说明退出时实例会被停止，残留记录已无意义，直接清理。
//
// 必须在 AutoStartOpenVPNServer 之前调用：被接管的实例在进程列表中已处于“运行中”，
// 自启动逻辑会将其识别为无需重复启动。
func (a *App) RecoverRunningServers() {
	if a.cfg.ShouldStopInstancesOnExit() {
		if err := a.daoManager.DeleteAllServerProcess(); err != nil {
			log.Printf("清理进程 PID 记录失败: %v", err)
		}
		return
	}
	records, err := a.daoManager.ListServerProcessRecords()
	if err != nil {
		log.Printf("列出进程 PID 记录失败: %v", err)
		return
	}
	if len(records) == 0 {
		return
	}
	resourceMap := a.PrepareResourceMap([]string{ovpnserver.RESOURCE_ID_MISC_CONFIG})
	miscConfig, err := ovpnserver.ParseMiscConfig(resourceMap)
	if err != nil {
		log.Printf("接管运行中实例失败: 解析杂项配置失败: %v", err)
		return
	}
	for _, record := range records {
		if !ovpnserver.ProcessAlive(record.PID) {
			// 进程已不在，删除记录；若服务器配置了自启动，随后会被正常拉起。
			_ = a.daoManager.DeleteServerProcess(record.ServerID)
			continue
		}
		serverModel, err := a.daoManager.GetOpenVPNServerByID(record.ServerID)
		if err != nil {
			log.Printf("接管运行中实例失败: 服务器 %d 已不存在，删除记录", record.ServerID)
			_ = a.daoManager.DeleteServerProcess(record.ServerID)
			continue
		}
		serverInstance := ovpnserver.NewOpenVPNServerInstance(
			a.resolvedServerModel(serverModel), nil, nil,
			fmt.Sprintf("%s/%d", a.cfg.WorkingDir, serverModel.ID),
			a.cfg.InternalAPIListen, miscConfig.OpenVPNPath)
		serverInstance.AttachPID(record.PID)
		a.lock.Lock()
		a.ovpnProcessList[serverModel.ID] = serverInstance
		a.lock.Unlock()
		log.Printf("已接管运行中的 OpenVPN 实例 server %d (%s) PID: %d", serverModel.ID, serverModel.Name, record.PID)
	}
}

func (a *App) rotateAllServerLogs() {
	if a.cfg.MaxLogSizeKB <= 0 {
		return
	}
	maxBytes := a.cfg.MaxLogSizeKB * 1024

	serverInstances := a.snapshotServerInstances()

	resourceMap := a.PrepareResourceMap([]string{ovpnserver.RESOURCE_ID_MISC_CONFIG})
	for _, serverInstance := range serverInstances {
		serverID := serverInstance.GetServerModel().ID
		// 日志轮换会截断日志文件，与实例的日志读写互斥。
		pl := a.getProcessLock(serverID)
		pl.Lock()
		err := serverInstance.RotateLogsFromMisc(resourceMap, maxBytes)
		pl.Unlock()
		if err != nil {
			log.Printf("日志轮换任务失败: server %d: %v", serverID, err)
		}
	}
}

func (a *App) updateConnectedClientInfoRecords() {
	// 达量限速：先重置已跨周期的用户流量统计
	if resetCount, err := a.daoManager.ResetExpiredRateLimitCycles(); err != nil {
		log.Printf("重置达量限速周期失败: %v", err)
	} else if resetCount > 0 {
		log.Printf("已重置 %d 个用户的达量限速周期", resetCount)
	}

	// 避免并发修改 ovpnProcessList
	serverInstances := a.snapshotServerInstances()

	for _, serverInstance := range serverInstances {
		serverID := serverInstance.GetServerModel().ID
		// 采集状态读取状态文件，持实例读锁；随后释放，避免与需要写锁的踢人/限速下发嵌套。
		pl := a.getProcessLock(serverID)
		pl.RLock()
		resourceMap := a.PrepareResourceMap([]string{ovpnserver.RESOURCE_ID_MISC_CONFIG})
		clientStatusList, err := serverInstance.GetStatusFromFile(resourceMap)
		pl.RUnlock()
		if err != nil {
			log.Printf("更新客户端流量记录失败: 读取服务器状态文件失败 server %d GetStatus failed: %v", serverID, err)
			continue
		}
		//log.Printf("更新客户端会话流量记录 服务器id %d 客户端数量 %d", serverID, len(clientStatusList))
		for _, clientInfo := range clientStatusList {
			// 记录以“主地址”（有 IPv4 用 IPv4，否则用 IPv6）为键，因此这里也按主地址更新流量。
			virtualAddrs := clientInfo.VirtualIPAddr
			if len(virtualAddrs) == 0 {
				virtualAddrs = clientInfo.VirtualIP6Addr
			}
			for _, virtualIPAddr := range virtualAddrs {
				//log.Println("更新客户端会话流量记录 ", virtualIPAddr)
				err = a.daoManager.UpdateConnectedClientInfoRecordTraffic(serverID, virtualIPAddr, uint64(clientInfo.ByteReceived), uint64(clientInfo.ByteSent))
				if err != nil {
					log.Printf("更新客户端流量记录失败: server %d virtual_ip %s: %v", serverID, virtualIPAddr, err)
				}
			}
		}
		// 达量限速：重新计算生效限速，只对变化（或需要踢下线）的客户端处理
		a.reconcileRateLimits(serverInstance, clientStatusList)
	}
}

// rateLimitChange 描述一个需要重新下发 tc 限速的在线客户端。
type rateLimitChange struct {
	virtualIPAddr string
	uploadKB      uint64
	downloadKB    uint64
}

// reconcileRateLimits 根据达量限速方案与用户/用户组限速重新计算每个在线客户端的生效限速：
//   - 匹配到“禁止连接”规则：立即踢下线；
//   - 生效限速（取最低）与记录不同：调用 ratelimit.sh 只对这些客户端重新设置 tc。
func (a *App) reconcileRateLimits(serverInstance *ovpnserver.OpenVPNServerInstance, clientStatusList []*ovpnserver.ServerStatusClientInfoResponse) {
	serverID := serverInstance.GetServerModel().ID
	records, err := a.daoManager.ListConnectedClientInfoRecordByServerID(serverID)
	if err != nil || len(records) == 0 {
		return
	}
	commonNameByVIP := make(map[string]string)
	for _, info := range clientStatusList {
		for _, virtualIPAddr := range info.VirtualIPAddr {
			commonNameByVIP[virtualIPAddr] = info.CommonName
		}
		for _, virtualIPAddr := range info.VirtualIP6Addr {
			commonNameByVIP[virtualIPAddr] = info.CommonName
		}
	}

	changes := make([]rateLimitChange, 0)
	for _, record := range records {
		user, err := a.daoManager.GetUserByUsername(record.Username)
		if err != nil {
			continue
		}
		uploadKB, downloadKB, allowConnect, err := a.daoManager.ResolveUserRateLimitAndConnect(user.ID, serverID)
		if err != nil {
			log.Printf("达量限速计算失败 server %d 用户 %s: %v", serverID, record.Username, err)
			continue
		}
		if !allowConnect {
			commonName := commonNameByVIP[record.VirtualIPAddr]
			if commonName == "" {
				continue
			}
			resourceMap := a.PrepareResourceMap([]string{ovpnserver.RESOURCE_ID_MISC_CONFIG})
			// 踢下线要访问管理 socket，与实例的其它操作互斥，取写锁。
			pl := a.getProcessLock(serverID)
			pl.Lock()
			_, killErr := serverInstance.CloseClient(commonName, record.RealIPAddr, resourceMap)
			pl.Unlock()
			if killErr != nil {
				log.Printf("达量限速踢下线失败 server %d 用户 %s 证书 %s: %v", serverID, record.Username, commonName, killErr)
			} else {
				log.Printf("达量限速禁止连接，已断开 server %d 用户 %s 证书 %s", serverID, record.Username, commonName)
			}
			continue
		}
		if uploadKB != record.UploadLimitKB || downloadKB != record.DownloadLimitKB {
			changes = append(changes, rateLimitChange{
				virtualIPAddr: record.VirtualIPAddr,
				uploadKB:      uploadKB,
				downloadKB:    downloadKB,
			})
		}
	}
	if len(changes) == 0 {
		return
	}
	// 限速更新脚本会改动该实例网卡的 tc 状态，与其它实例操作互斥，取写锁。
	pl := a.getProcessLock(serverID)
	pl.Lock()
	err = a.runRateLimitUpdateScript(serverInstance, changes)
	pl.Unlock()
	if err != nil {
		log.Printf("达量限速更新脚本执行失败 server %d: %v", serverID, err)
		return
	}
	for _, change := range changes {
		if err := a.daoManager.UpdateConnectedClientInfoRecordLimit(serverID, change.virtualIPAddr, change.uploadKB, change.downloadKB); err != nil {
			log.Printf("记录已下发的限速失败 server %d ip %s: %v", serverID, change.virtualIPAddr, err)
		}
	}
	log.Printf("达量限速已更新 server %d 客户端数 %d", serverID, len(changes))
}

// runRateLimitUpdateScript 运行达量限速更新脚本资源（ratelimit.sh），
// 通过标准输入把变化的客户端传给脚本。
func (a *App) runRateLimitUpdateScript(serverInstance *ovpnserver.OpenVPNServerInstance, changes []rateLimitChange) error {
	resourceMap := a.PrepareResourceMap([]string{ovpnserver.RESOURCE_ID_MISC_CONFIG, ovpnserver.RESOURCE_ID_RATE_LIMIT_SCRIPT})
	script, ok := resourceMap[ovpnserver.RESOURCE_ID_RATE_LIMIT_SCRIPT]
	if !ok {
		script = ovpnserver.GetDefaultResource(ovpnserver.RESOURCE_ID_RATE_LIMIT_SCRIPT)
	}
	miscConfig, err := ovpnserver.ParseMiscConfig(resourceMap)
	if err != nil {
		return err
	}
	serverModel := serverInstance.GetServerModel()
	workingDir := fmt.Sprintf("%s/%d/", strings.TrimRight(a.cfg.WorkingDir, "/"), serverModel.ID)
	script = strings.ReplaceAll(script, "__INTERNAL_API__", a.cfg.InternalAPIListen)
	script = strings.ReplaceAll(script, "__WORKING_DIR__", workingDir)
	script = strings.ReplaceAll(script, "__SERVER_ID__", fmt.Sprintf("%d", serverModel.ID))
	script = strings.ReplaceAll(script, "__SERVER_INTERFACE__", serverModel.Dev)

	var stdin strings.Builder
	for _, change := range changes {
		fmt.Fprintf(&stdin, "%s %d %d\n", change.virtualIPAddr, change.uploadKB, change.downloadKB)
	}

	cmd := exec.Command(miscConfig.ShellPath, "-c", script)
	cmd.Stdin = strings.NewReader(stdin.String())
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w 脚本输出: %s", err, string(out))
	}
	return nil
}

func NewApp(cfg *config.Config, buildDate string) *App {
	app := &App{
		cfg:             cfg,
		ovpnProcessList: make(map[uint]*ovpnserver.OpenVPNServerInstance),
		ovpnProcessLock: make(map[uint]*sync.RWMutex),
		buildDate:       buildDate,
		loginCooldown:   newCooldownLimiter(loginCooldownInterval),
	}
	var err error
	app.daoManager, err = dao.NewDaoManager(cfg)
	if err != nil {
		panic(err)
	}
	app.router = gin.Default()
	app.internalAPIRouter = gin.Default()
	app.setupTrustedProxies()
	app.SetupInternalAPIRoutes()
	app.setupRoutes()
	app.setupStaticFiles()
	return app
}
