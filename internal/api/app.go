package api

import (
	"log"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"openvpn-pannel/internal/config"
	"openvpn-pannel/internal/dao"
	ovpnserver "openvpn-pannel/internal/ovpn_server"
)

type App struct {
	data              uint32
	router            *gin.Engine
	cfg               *config.Config
	daoManager        *dao.DaoManager
	ovpnProcessList   map[uint]*ovpnserver.OpenVPNServerInstance
	lock              sync.RWMutex
	internalAPIRouter *gin.Engine
	buildDate         string
}

func (a *App) Run() {
	a.AutoStartOpenVPNServer()
	go a.internalAPIRouter.Run(a.cfg.InternalAPIListen)
	go a.StartConnectedClientInfoUpdater(60 * time.Second)
	go a.StartLogRotationTask(60 * time.Second)
	a.router.Run(a.cfg.Listen)
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

func (a *App) rotateAllServerLogs() {
	if a.cfg.MaxLogSizeKB <= 0 {
		return
	}
	maxBytes := a.cfg.MaxLogSizeKB * 1024

	a.lock.RLock()
	serverInstances := make([]*ovpnserver.OpenVPNServerInstance, 0, len(a.ovpnProcessList))
	for _, serverInstance := range a.ovpnProcessList {
		serverInstances = append(serverInstances, serverInstance)
	}
	a.lock.RUnlock()

	resourceMap := a.PrepareResourceMap([]string{ovpnserver.RESOURCE_ID_MISC_CONFIG})
	for _, serverInstance := range serverInstances {
		if err := serverInstance.RotateLogsFromMisc(resourceMap, maxBytes); err != nil {
			log.Printf("日志轮换任务失败: server %d: %v", serverInstance.GetServerModel().ID, err)
		}
	}
}

func (a *App) updateConnectedClientInfoRecords() {
	// 避免并发修改 ovpnProcessList
	a.lock.RLock()
	serverInstances := make([]*ovpnserver.OpenVPNServerInstance, 0, len(a.ovpnProcessList))
	for _, serverInstance := range a.ovpnProcessList {
		serverInstances = append(serverInstances, serverInstance)
	}
	a.lock.RUnlock()

	for _, serverInstance := range serverInstances {
		serverID := serverInstance.GetServerModel().ID
		resourceMap := a.PrepareResourceMap([]string{ovpnserver.RESOURCE_ID_MISC_CONFIG})
		clientStatusList, err := serverInstance.GetStatusFromFile(resourceMap)
		if err != nil {
			log.Printf("更新客户端流量记录失败: 读取服务器状态文件失败 server %d GetStatus failed: %v", serverID, err)
			continue
		}
		//log.Printf("更新客户端会话流量记录 服务器id %d 客户端数量 %d", serverID, len(clientStatusList))
		for _, clientInfo := range clientStatusList {
			for _, virtualIPAddr := range clientInfo.VirtualIPAddr {
				//log.Println("更新客户端会话流量记录 ", virtualIPAddr)
				err = a.daoManager.UpdateConnectedClientInfoRecordTraffic(serverID, virtualIPAddr, uint64(clientInfo.ByteReceived), uint64(clientInfo.ByteSent))
				if err != nil {
					log.Printf("更新客户端流量记录失败: server %d virtual_ip %s: %v", serverID, virtualIPAddr, err)
				}
			}
		}
	}
}

func NewApp(cfg *config.Config, buildDate string) *App {
	app := &App{
		cfg:             cfg,
		ovpnProcessList: make(map[uint]*ovpnserver.OpenVPNServerInstance),
		buildDate:       buildDate,
	}
	var err error
	app.daoManager, err = dao.NewDaoManager(cfg)
	if err != nil {
		panic(err)
	}
	app.router = gin.Default()
	app.internalAPIRouter = gin.Default()
	app.SetupInternalAPIRoutes()
	app.setupRoutes()
	app.setupStaticFiles()
	return app
}
