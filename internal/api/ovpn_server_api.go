package api

import (
	"encoding/json"
	"fmt"
	"log"
	"openvpn-pannel/internal/dao"
	"openvpn-pannel/internal/models"
	ovpnserver "openvpn-pannel/internal/ovpn_server"
	"os"
	"path"
	"strconv"

	"github.com/gin-gonic/gin"
)

func (a *App) PrepareResourceMap(ResourceIDList []string) map[string]string {
	var resourceMap map[string]string
	resourceMap = make(map[string]string)
	resourceModelList, err := a.daoManager.GetResourceByIDList(ResourceIDList)
	if err != nil {
		return resourceMap
	}
	for _, resourceModel := range resourceModelList {
		resourceMap[resourceModel.ID] = resourceModel.Content
	}
	return resourceMap
}

// 创建openvpn服务器接口
func (a *App) CreateOpenVPNServerHandler(c *gin.Context, user *models.User) {
	type Param struct {
		Name              string   `json:"name" binding:"required,min=1,max=100" `
		Local             string   `json:"local" `
		Port              uint32   `json:"port" binding:"required"`
		Proto             string   `json:"proto" binding:"required,min=1,max=100"`
		Dev               string   `json:"dev" binding:"required,min=1,max=100"`
		CA                string   `json:"ca" binding:"required,min=1,max=16384"`
		Cert              string   `json:"cert" binding:"required,min=1,max=16384"`
		Key               string   `json:"key" binding:"required,min=1,max=16384"`
		DH                string   `json:"dh" binding:"required,min=1,max=16384"`
		DataCipher        string   `json:"data_cipher" binding:"required,min=1,max=512"`
		Topology          string   `json:"topology" binding:"required,min=1,max=100"`
		ServerCIDR        string   `json:"server_cidr" binding:"required,min=1,max=100"`
		DuplicateCN       bool     `json:"duplicate_cn"`
		Keepalive         string   `json:"keepalive" binding:"required,min=1,max=100"`
		TLSAuthKey        string   `json:"tls_auth" binding:"required,min=1,max=16384"`
		OtherConfig       string   `json:"other_config" binding:"max=16384"`
		AutoStart         bool     `json:"auto_start"`
		ExportHost        string   `json:"export_host" binding:"max=255"`
		ExportPort        uint32   `json:"export_port"`
		ExportExtraConfig string   `json:"export_extra_config" binding:"max=16384"`
		ServerRoute       []string `json:"server_route" binding:"dive,max=328"`
	}

	var param Param
	err := c.ShouldBindJSON(&param)
	if err != nil {
		c.JSON(400, gin.H{
			"result": "failed",
			"error":  err.Error(),
		})
		return
	}

	if err := a.validateCertReferences(param.CA, param.Cert, param.Key); err != nil {
		c.JSON(400, gin.H{
			"result": "failed",
			"error":  err.Error(),
		})
		return
	}

	serverModel := &models.Server{
		Name:              param.Name,
		Local:             param.Local,
		Port:              param.Port,
		Proto:             param.Proto,
		Dev:               param.Dev,
		CA:                param.CA,
		Cert:              param.Cert,
		Key:               param.Key,
		DH:                param.DH,
		DataCipher:        param.DataCipher,
		Topology:          param.Topology,
		ServerCIDR:        param.ServerCIDR,
		DuplicateCN:       param.DuplicateCN,
		Keepalive:         param.Keepalive,
		TLSAuthKey:        param.TLSAuthKey,
		OtherConfig:       param.OtherConfig,
		AutoStart:         param.AutoStart,
		ExportHost:        param.ExportHost,
		ExportPort:        param.ExportPort,
		ExportExtraConfig: param.ExportExtraConfig,
	}
	err = a.daoManager.CreateOpenVPNServer(serverModel, param.ServerRoute)
	if err != nil {
		c.JSON(500, gin.H{
			"result": "failed",
			"error":  err.Error(),
		})
		return
	}
	for _, id := range certRefIDs(param.CA, param.Cert, param.Key) {
		if cert, err := a.daoManager.GetCertificateByID(id); err == nil {
			a.logCertificateEvent(c, user, models.CERT_EVENT_TYPE_SERVER_REFERENCE, "服务器引用证书", cert, serverModel)
		}
	}
	c.JSON(200, gin.H{
		"result": "success",
		"error":  nil,
	})
}

// 获取服务器信息
func (a *App) GetOpenVPNServerInfoHandler(c *gin.Context, user *models.User) {
	type Response struct {
		ID                uint     `gorm:"primarykey" json:"id" binding:"required"`
		Name              string   `json:"name" binding:"required"`
		Local             string   `json:"local" `
		Port              uint32   `json:"port" binding:"required"`
		Proto             string   `json:"proto" binding:"required"`
		Dev               string   `json:"dev" binding:"required"`
		CA                string   `json:"ca" binding:"required"`
		Cert              string   `json:"cert" binding:"required"`
		Key               string   `json:"key" binding:"required"`
		DH                string   `json:"dh" binding:"required"`
		DataCipher        string   `json:"data_cipher" binding:"required"`
		Topology          string   `json:"topology" binding:"required"`
		ServerCIDR        string   `json:"server_cidr" binding:"required"`
		DuplicateCN       bool     `json:"duplicate_cn" binding:"required"`
		Keepalive         string   `json:"keepalive" binding:"required"`
		TLSAuthKey        string   `json:"tls_auth" binding:"required"`
		OtherConfig       string   `json:"other_config" `
		AutoStart         bool     `json:"auto_start" binding:"required"`
		ExportHost        string   `json:"export_host"`
		ExportPort        uint32   `json:"export_port"`
		ExportExtraConfig string   `json:"export_extra_config"`
		ServerRoute       []string `json:"server_route" binding:"required,dive,max=328"`
	}
	_serverID := c.DefaultQuery("id", "")
	var serverID uint64
	serverID, err := strconv.ParseUint(_serverID, 10, 64)
	if err != nil {
		c.JSON(400, gin.H{
			"result": "failed",
			"error":  "获取服务器信息失败: 参数id必须是int类型",
		})
		return
	}
	serverModel, err := a.daoManager.GetOpenVPNServerByID(uint(serverID))
	if err != nil {
		c.JSON(500, gin.H{
			"result": "failed",
			"error":  "获取服务器信息失败: " + err.Error(),
		})
		return
	}
	response := Response{
		ID:                serverModel.ID,
		Name:              serverModel.Name,
		Local:             serverModel.Local,
		Port:              serverModel.Port,
		Proto:             serverModel.Proto,
		Dev:               serverModel.Dev,
		CA:                serverModel.CA,
		Cert:              serverModel.Cert,
		Key:               serverModel.Key,
		DH:                serverModel.DH,
		DataCipher:        serverModel.DataCipher,
		Topology:          serverModel.Topology,
		ServerCIDR:        serverModel.ServerCIDR,
		DuplicateCN:       serverModel.DuplicateCN,
		Keepalive:         serverModel.Keepalive,
		TLSAuthKey:        serverModel.TLSAuthKey,
		OtherConfig:       serverModel.OtherConfig,
		AutoStart:         serverModel.AutoStart,
		ExportHost:        serverModel.ExportHost,
		ExportPort:        serverModel.ExportPort,
		ExportExtraConfig: serverModel.ExportExtraConfig,
		ServerRoute:       make([]string, 0),
	}
	serverRouteModelList, err := a.daoManager.ListOpenVPNServerRoute(response.ID)
	if err != nil {
		c.JSON(500, gin.H{
			"result": "failed",
			"error":  "获取服务器路由信息失败: " + err.Error(),
		})
		return
	}
	for _, serverRoute := range serverRouteModelList {
		response.ServerRoute = append(response.ServerRoute, serverRoute.Network)
	}
	c.JSON(200, gin.H{
		"result": "success",
		"error":  nil,
		"data":   response,
	})
}

// 修改openvpn服务器接口
func (a *App) UpdateOpenVPNServerHandler(c *gin.Context, user *models.User) {
	type Param struct {
		ID                uint     `gorm:"primarykey" json:"id"`
		Name              string   `json:"name" binding:"required,min=1,max=16384"`
		Local             string   `json:"local" binding:"max=100"`
		Port              uint32   `json:"port" binding:"required"`
		Proto             string   `json:"proto" binding:"required,min=1,max=100"`
		Dev               string   `json:"dev" binding:"required,min=1,max=100"`
		CA                string   `json:"ca" binding:"required,min=1,max=16384"`
		Cert              string   `json:"cert" binding:"required,min=1,max=16384"`
		Key               string   `json:"key" binding:"required,min=1,max=16384"`
		DH                string   `json:"dh" binding:"required,min=1,max=16384"`
		DataCipher        string   `json:"data_cipher" binding:"required,min=1,max=512"`
		Topology          string   `json:"topology" binding:"required,min=1,max=16384"`
		ServerCIDR        string   `json:"server_cidr" binding:"required,min=1,max=16384"`
		DuplicateCN       bool     `json:"duplicate_cn"`
		Keepalive         string   `json:"keepalive" binding:"required,min=1,max=100"`
		TLSAuthKey        string   `json:"tls_auth" binding:"required,min=1,max=16384"`
		OtherConfig       string   `json:"other_config" binding:"max=16384"`
		AutoStart         bool     `json:"auto_start" `
		ExportHost        string   `json:"export_host" binding:"max=255"`
		ExportPort        uint32   `json:"export_port"`
		ExportExtraConfig string   `json:"export_extra_config" binding:"max=16384"`
		ServerRoute       []string `json:"server_route" binding:"dive,max=328"`
	}

	var param Param
	err := c.ShouldBindJSON(&param)
	if err != nil {
		c.JSON(400, gin.H{
			"result": "failed",
			"error":  err.Error(),
		})
		return
	}

	if err := a.validateCertReferences(param.CA, param.Cert, param.Key); err != nil {
		c.JSON(400, gin.H{
			"result": "failed",
			"error":  err.Error(),
		})
		return
	}

	// 更新前记录已引用的证书，仅对新增的引用记录事件
	oldRefIDs := make(map[uint]bool)
	if oldServer, err := a.daoManager.GetOpenVPNServerByID(param.ID); err == nil {
		for _, id := range certRefIDs(oldServer.CA, oldServer.Cert, oldServer.Key) {
			oldRefIDs[id] = true
		}
	}

	serverModel := &models.Server{
		ID:                param.ID,
		Name:              param.Name,
		Local:             param.Local,
		Port:              param.Port,
		Proto:             param.Proto,
		Dev:               param.Dev,
		CA:                param.CA,
		Cert:              param.Cert,
		Key:               param.Key,
		DH:                param.DH,
		DataCipher:        param.DataCipher,
		Topology:          param.Topology,
		ServerCIDR:        param.ServerCIDR,
		DuplicateCN:       param.DuplicateCN,
		Keepalive:         param.Keepalive,
		TLSAuthKey:        param.TLSAuthKey,
		OtherConfig:       param.OtherConfig,
		AutoStart:         param.AutoStart,
		ExportHost:        param.ExportHost,
		ExportPort:        param.ExportPort,
		ExportExtraConfig: param.ExportExtraConfig,
	}
	err = a.daoManager.UpdateOpenVPNServer(serverModel, param.ServerRoute)
	if err != nil {
		c.JSON(500, gin.H{
			"result": "failed",
			"error":  err.Error(),
		})
		return
	}
	for _, id := range certRefIDs(param.CA, param.Cert, param.Key) {
		if oldRefIDs[id] {
			continue
		}
		if cert, err := a.daoManager.GetCertificateByID(id); err == nil {
			a.logCertificateEvent(c, user, models.CERT_EVENT_TYPE_SERVER_REFERENCE, "服务器引用证书", cert, serverModel)
		}
	}
	c.JSON(200, gin.H{
		"result": "success",
		"error":  nil,
	})
}

// 删除openvpn服务器接口
func (a *App) DeleteOpenVPNServerHandler(c *gin.Context, user *models.User) {
	type Param struct {
		ID uint `gorm:"primarykey" json:"id" binding:"required"`
	}
	a.lock.Lock()
	defer a.lock.Unlock()
	var param Param
	err := c.ShouldBindJSON(&param)
	if err != nil {
		c.JSON(400, gin.H{
			"result": "failed",
			"error":  err.Error(),
		})
		return
	}
	// 检查是否在运行
	instance, ok := a.ovpnProcessList[param.ID]
	pl := a.getProcessLock(param.ID)
	pl.RLock()
	running := ok && instance.Running()
	pl.RUnlock()
	if running {
		c.JSON(400, gin.H{
			"result": "failed",
			"error":  "服务器正在运行中,请先停止",
		})
		return
	}
	err = a.daoManager.DeleteOpenVPNServer(param.ID)
	if err != nil {
		c.JSON(500, gin.H{
			"result": "failed",
			"error":  "删除服务器失败: " + err.Error(),
		})
		return
	}
	// 从进程列表移除，避免残留已删除服务器的实例与锁
	delete(a.ovpnProcessList, param.ID)
	a.removeProcessLock(param.ID)
	a.daoManager.DeleteServerProcess(param.ID)
	workdir := path.Join(a.cfg.WorkingDir, fmt.Sprintf("%d", param.ID))
	os.RemoveAll(workdir)
	c.JSON(200, gin.H{
		"result": "success",
		"error":  nil,
	})
}

// 列出已有的服务器
func (a *App) ListOpenVPNServerHandler(c *gin.Context, user *models.User) {
	type Response struct {
		ID         uint   `gorm:"primarykey" json:"id" binding:"required"`
		Name       string `json:"name" binding:"required"`
		Local      string `json:"local" `
		Port       uint32 `json:"port" binding:"required"`
		Proto      string `json:"proto" binding:"required"`
		Dev        string `json:"dev" binding:"required"`
		ServerCIDR string `json:"server_cidr" binding:"required"`
		AutoStart  bool   `json:"auto_start" binding:"required"`
		Running    bool   `json:"running" binding:"required"`
	}
	resultList, err := a.daoManager.ListOpenVPNServer()
	if err != nil {
		c.JSON(200, gin.H{
			"result": "failed",
			"error":  err.Error(),
			"data":   nil,
		})
		return
	}
	a.lock.RLock()
	defer a.lock.RUnlock()
	var responseList []Response
	responseList = make([]Response, 0)
	for _, serverModel := range resultList {
		response := Response{
			ID:         serverModel.ID,
			Name:       serverModel.Name,
			Local:      serverModel.Local,
			Port:       serverModel.Port,
			Proto:      serverModel.Proto,
			Dev:        serverModel.Dev,
			ServerCIDR: serverModel.ServerCIDR,
			AutoStart:  serverModel.AutoStart,
			Running:    false,
		}
		serverInstance, ok := a.ovpnProcessList[response.ID]
		if ok {
			pl := a.getProcessLock(response.ID)
			pl.RLock()
			running := serverInstance.Running()
			pl.RUnlock()
			if running {
				response.Running = true
			}
		}
		responseList = append(responseList, response)
	}
	c.JSON(200, gin.H{
		"result": "success",
		"error":  nil,
		"data":   responseList,
	})
}

// 列出客户端配置
func (a *App) ListOpenVPNServerClientConfigHandler(c *gin.Context, user *models.User) {
	type Response struct {
		ID             uint   `gorm:"primarykey" `
		ClientCertName string `json:"client_cert_name" `
		Config         string `json:"config" `
	}
	_serverID := c.DefaultQuery("id", "")
	var serverID uint64
	serverID, err := strconv.ParseUint(_serverID, 10, 64)
	if err != nil {
		c.JSON(400, gin.H{
			"result": "failed",
			"error":  "列出客户端配置失败: 缺少参数id",
		})
		return
	}
	resultList, err := a.daoManager.ListOpenVPNServerClientConfig(uint(serverID))
	var responseList []Response
	responseList = make([]Response, 0)
	for _, clientConfig := range resultList {
		response := Response{
			ID:             clientConfig.ID,
			ClientCertName: clientConfig.ClientCertName,
			Config:         clientConfig.Config,
		}
		responseList = append(responseList, response)
	}
	if err != nil {
		c.JSON(500, gin.H{
			"result": "failed",
			"error":  "列出客户端配置失败 " + err.Error(),
		})
		return
	}
	c.JSON(200, gin.H{
		"result": "success",
		"error":  nil,
		"data":   responseList,
	})
}

// 删除客户端配置
func (a *App) DeleteOpenVPNServerClientConfigHandler(c *gin.Context, user *models.User) {
	type Param struct {
		IDList []uint `gorm:"primarykey" json:"id_list" binding:"required"`
	}
	var param Param
	err := c.ShouldBindJSON(&param)
	if err != nil {
		c.JSON(400, gin.H{
			"result": "failed",
			"error":  err.Error(),
		})
		return
	}
	var serverID uint
	updateCCD := false
	if len(param.IDList) != 0 {
		clientConfig, err := a.daoManager.GetOpenVPNServerClientConfig(param.IDList[0])
		if err == nil {
			serverID = clientConfig.ServerID
			updateCCD = true
		}
	}

	err = a.daoManager.DeleteOpenVPNServerClientConfig(param.IDList)
	if err != nil {
		c.JSON(500, gin.H{
			"result": "failed",
			"error":  "删除客户端配置失败: " + err.Error(),
		})
		return
	}

	if updateCCD {
		a.lock.RLock()
		defer a.lock.RUnlock()
		serverInstance, ok := a.ovpnProcessList[serverID]
		if ok {
			// UpdateClientConfig 会改写实例的 ccd 目录，取实例写锁。
			pl := a.getProcessLock(serverID)
			pl.Lock()
			defer pl.Unlock()
			if serverInstance.Running() {
				clientConfigList, err := a.daoManager.ListOpenVPNServerClientConfig(serverID)
				if err == nil {
					resourceMap := a.PrepareResourceMap([]string{ovpnserver.RESOURCE_ID_MISC_CONFIG})
					err = serverInstance.UpdateClientConfig(clientConfigList, resourceMap)
				}
			}
		}
	}
	c.JSON(200, gin.H{
		"result": "success",
		"error":  nil,
	})
}

// 新增/修改客户端配置
func (a *App) AddOpenVPNServerClientConfigHandler(c *gin.Context, user *models.User) {
	type Param struct {
		ClientCertName string `json:"client_cert_name" binding:"required,max=100"`
		Config         string `json:"config" binding:"required,max=16384"`
		ServerID       uint   `json:"server_id" binding:"required"`
	}
	var param Param
	err := c.ShouldBindJSON(&param)
	if err != nil {
		c.JSON(400, gin.H{
			"result": "failed",
			"error":  err.Error(),
		})
		return
	}
	clientConfig := &models.ClientConfig{
		ClientCertName: param.ClientCertName,
		Config:         param.Config,
		ServerID:       param.ServerID,
	}
	err = a.daoManager.AddOpenVPNServerClientConfig(clientConfig)
	if err != nil {
		c.JSON(500, gin.H{
			"result": "failed",
			"error":  "添加客户端配置失败: " + err.Error(),
		})
		return
	}
	a.lock.RLock()
	defer a.lock.RUnlock()
	serverInstance, ok := a.ovpnProcessList[clientConfig.ServerID]
	if ok {
		// UpdateClientConfig 会改写实例的 ccd 目录，取实例写锁。
		pl := a.getProcessLock(clientConfig.ServerID)
		pl.Lock()
		defer pl.Unlock()
		if serverInstance.Running() {
			clientConfigList, err := a.daoManager.ListOpenVPNServerClientConfig(clientConfig.ServerID)
			if err == nil {
				resourceMap := a.PrepareResourceMap([]string{ovpnserver.RESOURCE_ID_MISC_CONFIG})
				err = serverInstance.UpdateClientConfig(clientConfigList, resourceMap)
			}
		}
	}
	c.JSON(200, gin.H{
		"result": "success",
		"error":  nil,
	})
}

// openvpn服务器加权限
func (a *App) AddOpenVPNServerPermissionHandler(c *gin.Context, user *models.User) {
	var param models.ServerPermission
	err := c.ShouldBindJSON(&param)
	if err != nil {
		c.JSON(400, gin.H{
			"result": "failed",
			"error":  err.Error(),
		})
		return
	}
	a.daoManager.AddServerPermission(&param)
	c.JSON(200, gin.H{
		"result": "success",
		"error":  nil,
		"data":   param,
	})
}

// openvpn服务器列出权限
func (a *App) ListOpenVPNServerPermissionHandler(c *gin.Context, user *models.User) {
	_serverID := c.DefaultQuery("id", "")
	var serverID uint64
	serverID, err := strconv.ParseUint(_serverID, 10, 64)
	if err != nil {
		c.JSON(400, gin.H{
			"result": "failed",
			"error":  "列出权限信息失败: 参数id必须是int类型",
		})
		return
	}
	serverPermissionList, err := a.daoManager.ListServerPermission(uint(serverID))
	if err != nil {
		c.JSON(500, gin.H{
			"result": "failed",
			"error":  err.Error(),
		})
		return
	}
	c.JSON(200, gin.H{
		"result": "success",
		"error":  nil,
		"data":   serverPermissionList,
	})

}

// openvpn服务器删除权限
func (a *App) DelOpenVPNServerPermissionHandler(c *gin.Context, user *models.User) {
	type Param struct {
		IDList []uint `gorm:"primarykey" json:"id_list" binding:"required"`
	}
	var param Param
	err := c.ShouldBindJSON(&param)
	if err != nil {
		c.JSON(400, gin.H{
			"result": "failed",
			"error":  err.Error(),
		})
		return
	}
	resultMap := make(map[uint]string)
	for _, permID := range param.IDList {
		err := a.daoManager.DelServerPermission(permID)
		if err != nil {
			resultMap[permID] = "删除失败: " + err.Error()
		} else {
			resultMap[permID] = "success"
		}
	}
	c.JSON(200, gin.H{
		"result": "success",
		"error":  nil,
		"data":   resultMap,
	})
}

// 启动服务器
func (a *App) StartOpenVPNServerInstanceHandler(c *gin.Context, user *models.User) {
	type Param struct {
		ID uint `gorm:"primarykey" json:"id" binding:"required"`
	}
	var param Param
	err := c.ShouldBindJSON(&param)
	if err != nil {
		c.JSON(400, gin.H{
			"result": "failed",
			"error":  err.Error(),
		})
		return
	}
	a.lock.Lock()
	defer a.lock.Unlock()

	serverModel, err := a.daoManager.GetOpenVPNServerByID(param.ID)
	if err != nil {
		c.JSON(404, gin.H{
			"result": "failed",
			"error":  "服务器实例不存在",
		})
		return
	}
	requestIP := c.ClientIP()
	serverInstance, ok := a.ovpnProcessList[serverModel.ID]
	pl := a.getProcessLock(serverModel.ID)
	pl.Lock()
	defer pl.Unlock()
	if !ok || !serverInstance.Running() {
		// 当服务器没有在进程列表中存在，或者服务器进程没有运行时，进行初始化配置文件操作
		serverRouteList, err := a.daoManager.ListOpenVPNServerRoute(serverModel.ID)
		if err != nil {
			c.JSON(500, gin.H{
				"result": "failed",
				"error":  "服务器启动失败: 列出服务端路由失败 " + err.Error(),
			})
			a.daoManager.CreateEvent(serverModel.ID, models.SERVER_EVENT_TYPE_SERVER_START_FAIL, requestIP, "服务器启动失败: 列出服务端路由失败 "+err.Error())
			return
		}
		clientConfigList, err := a.daoManager.ListOpenVPNServerClientConfig(serverModel.ID)
		if err != nil {
			c.JSON(500, gin.H{
				"result": "failed",
				"error":  "服务器启动失败: 列出客户端配置失败" + err.Error(),
			})
			a.daoManager.CreateEvent(serverModel.ID, models.SERVER_EVENT_TYPE_SERVER_START_FAIL, requestIP, "服务器启动失败: 列出客户端配置失败 "+err.Error())
			return
		}
		resourceMap := a.PrepareResourceMap([]string{ovpnserver.RESOURCE_ID_CONFIG_TEMPLATE, ovpnserver.RESOURCE_ID_CLIENT_OFFLINE_SCRIPT, ovpnserver.RESOURCE_ID_CLIENT_ONLINE_SCRIPT, ovpnserver.RESOURCE_ID_AUTH_SCRIPT, ovpnserver.RESOURCE_ID_SERVER_START_SCRIPT, ovpnserver.RESOURCE_ID_MISC_CONFIG})
		var miscConfigStr string
		miscConfig, ok := resourceMap[ovpnserver.RESOURCE_ID_MISC_CONFIG]
		if ok {
			miscConfigStr = miscConfig
		} else {
			miscConfigStr = ovpnserver.GetDefaultResource(ovpnserver.RESOURCE_ID_MISC_CONFIG)
		}
		var miscConfigModel ovpnserver.MiscConfig
		err = json.Unmarshal([]byte(miscConfigStr), &miscConfigModel)
		if err != nil {
			c.JSON(500, gin.H{
				"result": "failed",
				"error":  "服务器启动失败: 解析杂项配置失败 " + err.Error(),
			})
			a.daoManager.CreateEvent(serverModel.ID, models.SERVER_EVENT_TYPE_SERVER_START_FAIL, requestIP, "服务器启动失败: 解析杂项配置失败 "+err.Error())
			return
		}
		serverInstance = ovpnserver.NewOpenVPNServerInstance(a.resolvedServerModel(serverModel), serverRouteList, clientConfigList, fmt.Sprintf("%s/%d", a.cfg.WorkingDir, serverModel.ID), a.cfg.InternalAPIListen, miscConfigModel.OpenVPNPath)
		a.ovpnProcessList[serverModel.ID] = serverInstance

		err = serverInstance.WriteConfig(resourceMap)
		if err != nil {
			c.JSON(500, gin.H{
				"result": "failed",
				"error":  "服务器启动失败: 配置文件写入失败" + err.Error(),
			})
			a.daoManager.CreateEvent(serverModel.ID, models.SERVER_EVENT_TYPE_SERVER_START_FAIL, requestIP, "服务器启动失败: 配置文件写入失败 "+err.Error())
			return
		}
		err = serverInstance.Start(resourceMap)
		if err != nil {
			c.JSON(500, gin.H{
				"result": "failed",
				"error":  "服务器启动失败: " + err.Error(),
			})
			a.daoManager.CreateEvent(serverModel.ID, models.SERVER_EVENT_TYPE_SERVER_START_FAIL, requestIP, "服务器启动失败: "+err.Error())
			return
		}
	} else {
		c.JSON(400, gin.H{
			"result": "failed",
			"error":  "服务器进程正在运行中，请停止再启动",
		})
		return
	}
	a.daoManager.CreateEvent(serverModel.ID, models.SERVER_EVENT_TYPE_SERVER_START_SUCCESS, requestIP, "服务器启动成功 操作用户="+user.Username)
	a.recordServerProcess(serverModel.ID, serverInstance.GetPID())
	a.daoManager.DeleteAddedACLByServerID(serverModel.ID)
	a.daoManager.DeleteConnectedClientInfoRecordByServerID(serverModel.ID)
	c.JSON(200, gin.H{
		"result": "success",
		"error":  nil,
	})
}

// 停止服务器
func (a *App) StopOpenVPNServerInstanceHandler(c *gin.Context, user *models.User) {
	type Param struct {
		ID uint `gorm:"primarykey" json:"id" binding:"required"`
	}
	var param Param
	err := c.ShouldBindJSON(&param)
	if err != nil {
		c.JSON(400, gin.H{
			"result": "failed",
			"error":  err.Error(),
		})
		return
	}
	a.lock.Lock()
	defer a.lock.Unlock()

	serverInstance, ok := a.ovpnProcessList[param.ID]
	if !ok {
		c.JSON(400, gin.H{
			"result": "failed",
			"error":  "服务器进程没有启动，不需要停止",
		})
		return
	}
	pl := a.getProcessLock(param.ID)
	pl.Lock()
	defer pl.Unlock()
	if !serverInstance.Running() {
		c.JSON(400, gin.H{
			"result": "failed",
			"error":  "服务器进程没有启动，不需要停止",
		})
		return
	}
	requestIP := c.ClientIP()
	resourceMap := a.PrepareResourceMap([]string{ovpnserver.RESOURCE_ID_SERVER_EXIT_SCRIPT, ovpnserver.RESOURCE_ID_MISC_CONFIG})
	err = serverInstance.Stop(resourceMap)
	if err != nil {
		c.JSON(500, gin.H{
			"result": "failed",
			"error":  "服务器进程停止失败: " + err.Error(),
		})
		a.daoManager.CreateEvent(param.ID, models.SERVER_EVENT_TYPE_SERVER_EXIT_FAIL, requestIP, "服务器停止失败: "+err.Error())
		return
	} else {
		c.JSON(200, gin.H{
			"result": "success",
			"error":  nil,
		})
		a.daoManager.DeleteAddedACLByServerID(param.ID)
		a.daoManager.DeleteServerProcess(param.ID)
		a.daoManager.CreateEvent(param.ID, models.SERVER_EVENT_TYPE_SERVER_EXIT_SUCCESS, requestIP, "服务器停止成功 操作用户="+user.Username)
		return
	}
}

// 列出用户有权限的服务器
func (a *App) ListOpenVPNServerByUserPermissionHandler(c *gin.Context, user *models.User) {
	_userID := c.DefaultQuery("user_id", "")
	var userID uint64
	userID, err := strconv.ParseUint(_userID, 10, 64)
	if err != nil {
		c.JSON(400, gin.H{
			"result": "failed",
			"error":  "列出权限信息失败: 参数user_id必须是int类型",
		})
		return
	}
	havePermServerList, err := a.daoManager.ListServerByUserPermission(userID)
	if err != nil {
		c.JSON(500, gin.H{
			"result": "failed",
			"error":  "列出权限信息失败: " + err.Error(),
		})
		return
	}
	type Response struct {
		ID         uint   `gorm:"primarykey" json:"id" binding:"required"`
		Name       string `json:"name" binding:"required"`
		Local      string `json:"local" `
		Port       uint32 `json:"port" binding:"required"`
		Proto      string `json:"proto" binding:"required"`
		Dev        string `json:"dev" binding:"required"`
		ServerCIDR string `json:"server_cidr" binding:"required"`
		AutoStart  bool   `json:"auto_start" binding:"required"`
		Running    bool   `json:"running" binding:"required"`
	}
	serverModelMap := make(map[uint]*models.Server)
	var responseList map[int][]Response
	responseList = make(map[int][]Response)
	for _, i := range []int{dao.SERVER_LIST_GROUP_ALLOW, dao.SERVER_LIST_GROUP_DENY, dao.SERVER_LIST_USER_ALLOW, dao.SERVER_LIST_USER_DENY} {
		//log.Println("action", i, havePermServerList[i])
		for _, serverID := range havePermServerList[i] {
			//log.Printf("user %s server %d action %d", _userID, serverID, i)
			var serverModel *models.Server
			serverModel, ok := serverModelMap[serverID]
			if !ok {
				serverModel, err = a.daoManager.GetOpenVPNServerByID(serverID)
			}
			if err != nil {
				responseList[i] = append(responseList[i], Response{
					ID:         serverID,
					Name:       "",
					Local:      "",
					Port:       0,
					Proto:      "",
					Dev:        "",
					ServerCIDR: "",
					AutoStart:  false,
				})
			} else {
				serverModelMap[serverID] = serverModel
				responseList[i] = append(responseList[i], Response{
					ID:         serverModel.ID,
					Name:       serverModel.Name,
					Local:      serverModel.Local,
					Port:       serverModel.Port,
					Proto:      serverModel.Proto,
					Dev:        serverModel.Dev,
					ServerCIDR: serverModel.ServerCIDR,
					AutoStart:  serverModel.AutoStart,
				})
			}
		}
	}
	serverACLListMap := make(map[uint][]string)
	for _, serverModel := range serverModelMap {
		serverACLList, err := a.daoManager.GetACLByUser(uint(userID), serverModel.ID)
		if err == nil {
			for _, serverACL := range serverACLList {
				aclString := fmt.Sprintf("%d#%s", serverACL.Type, serverACL.Value)
				serverACLListMap[serverModel.ID] = append(serverACLListMap[serverModel.ID], aclString)
			}
		}
	}
	c.JSON(200, gin.H{
		"result": "success",
		"error":  nil,
		"data":   responseList,
		"acl":    serverACLListMap,
	})
}

// 获取服务器日志
func (a *App) GetOpenVPNServerLogHandler(c *gin.Context, user *models.User) {
	_startLine := c.DefaultQuery("start_line", "")
	_endLine := c.DefaultQuery("end_line", "")
	var startLine, endLine int
	var err error
	startLine, err = strconv.Atoi(_startLine)
	if err != nil {
		c.JSON(400, gin.H{
			"result": "failed",
			"error":  "获取日志失败: 参数start_line必须是int类型",
		})
		return
	}
	endLine, err = strconv.Atoi(_endLine)
	if err != nil {
		c.JSON(400, gin.H{
			"result": "failed",
			"error":  "获取日志失败: 参数end_line必须是int类型",
		})
		return
	}
	logType := c.DefaultQuery("type", "")
	if logType == "" {
		c.JSON(400, gin.H{
			"result": "failed",
			"error":  "获取日志失败: 参数type不能为空",
		})
	}
	if logType != "server" && logType != "script" {
		c.JSON(400, gin.H{
			"result": "failed",
			"error":  "获取日志失败: 参数type只能为server或script",
		})
		return
	}
	var logTypeInt int
	switch logType {
	case "server":
		logTypeInt = ovpnserver.SERVER_LOG_TYPE_SERVER
	case "script":
		logTypeInt = ovpnserver.SERVER_LOG_TYPE_SCRIPT
	}
	resourceMap := a.PrepareResourceMap([]string{ovpnserver.RESOURCE_ID_MISC_CONFIG})
	_serverID := c.DefaultQuery("id", "")
	var serverID uint64
	serverID, err = strconv.ParseUint(_serverID, 10, 64)
	if err != nil {
		c.JSON(400, gin.H{
			"result": "failed",
			"error":  "获取日志失败: 参数server_id必须是int类型",
		})
		return
	}
	a.lock.RLock()
	defer a.lock.RUnlock()
	serverInstance, ok := a.ovpnProcessList[uint(serverID)]
	if !ok {
		var serverModel *models.Server
		serverModel, err = a.daoManager.GetOpenVPNServerByID(uint(serverID))
		if err != nil {
			c.JSON(400, gin.H{
				"result": "failed",
				"error":  "获取日志失败: 服务器不存在",
			})
			return
		}
		serverInstance = ovpnserver.NewOpenVPNServerInstance(serverModel, nil, nil, fmt.Sprintf("%s/%d", a.cfg.WorkingDir, serverID), a.cfg.InternalAPIListen, "")
	}
	// 读取日志文件与日志轮换(copytruncate)互斥，取实例读锁。
	pl := a.getProcessLock(uint(serverID))
	pl.RLock()
	var serverLogResponse *ovpnserver.ServerLogResponse

	serverLogResponse, err = serverInstance.GetLog(logTypeInt, startLine, endLine, resourceMap)
	pl.RUnlock()
	if err != nil {
		c.JSON(500, gin.H{
			"result": "failed",
			"error":  "获取日志失败: " + err.Error(),
		})
		return
	}
	c.JSON(200, gin.H{
		"result": "success",
		"error":  nil,
		"data":   serverLogResponse,
	})
}

// 清理服务器日志
func (a *App) ClearOpenVPNServerLogHandler(c *gin.Context, user *models.User) {
	type RequestParam struct {
		ID      uint   `json:"id" binding:"required"`
		LogType string `json:"log_type" binding:"required"`
	}
	var param RequestParam
	err := c.ShouldBindJSON(&param)
	if err != nil {
		c.JSON(400, gin.H{
			"result": "failed",
			"error":  "清理日志失败: 参数错误",
		})
		return
	}
	var logTypeInt int
	switch param.LogType {
	case "server":
		logTypeInt = ovpnserver.SERVER_LOG_TYPE_SERVER
	case "script":
		logTypeInt = ovpnserver.SERVER_LOG_TYPE_SCRIPT
	}
	resourceMap := a.PrepareResourceMap([]string{ovpnserver.RESOURCE_ID_MISC_CONFIG})
	a.lock.Lock()
	defer a.lock.Unlock()
	serverInstance, ok := a.ovpnProcessList[param.ID]
	if !ok {
		var serverModel *models.Server
		serverModel, err = a.daoManager.GetOpenVPNServerByID(param.ID)
		if err != nil {
			c.JSON(400, gin.H{
				"result": "failed",
				"error":  "清理日志失败: 服务器不存在",
			})
			return
		}
		serverInstance = ovpnserver.NewOpenVPNServerInstance(serverModel, nil, nil, fmt.Sprintf("%s/%d", a.cfg.WorkingDir, param.ID), a.cfg.InternalAPIListen, "")
		a.ovpnProcessList[param.ID] = serverInstance
	}
	// 清空日志文件与日志轮换(copytruncate)互斥，取实例写锁。
	pl := a.getProcessLock(param.ID)
	pl.Lock()
	defer pl.Unlock()
	serverInstance.ClearLog(logTypeInt, resourceMap)
	c.JSON(200, gin.H{
		"result": "success",
		"error":  nil,
	})
}

// 获取服务器状态
type ServerStatusClientInfoResponse struct {
	CommonName     string   `json:"common_name"`
	RealIPAddr     string   `json:"real_ip_addr"`
	VirtualIPAddr  []string `json:"virtual_ip_addr"`
	ByteReceived   int      `json:"last_byte_received"`
	ByteSent       int      `json:"last_byte_sent"`
	ConnectedSince string   `json:"connected_since"`
	LastRef        string   `json:"last_ref"`
	Username       string   `json:"username"`
	ACLList        []string `json:"acl_list"`
}
type ServerStatusResponse struct {
	Version           string `json:"version"`
	ManagementVersion string `json:"management_version"`
	ClientList        []*ServerStatusClientInfoResponse
}

func (a *App) GetOpenVPNServerStatusHandler(c *gin.Context, user *models.User) {
	_serverID := c.DefaultQuery("id", "")
	serverID, err := strconv.ParseUint(_serverID, 10, 64)
	if err != nil {
		c.JSON(400, gin.H{
			"result": "failed",
			"error":  "获取状态失败: 获取参数id失败",
		})
		return
	}
	a.lock.RLock()
	defer a.lock.RUnlock()
	serverInstance, ok := a.ovpnProcessList[uint(serverID)]
	if !ok {
		c.JSON(400, gin.H{
			"result": "failed",
			"error":  "获取状态失败: 服务器未运行",
		})
		return
	}
	resourceMap := a.PrepareResourceMap([]string{ovpnserver.RESOURCE_ID_MISC_CONFIG})
	// 访问管理 socket 获取状态，持实例写锁串行化：管理接口同时只应有一个连接，
	// 与踢人(CloseClient)等其它 socket 操作互斥，避免连接冲突。
	pl := a.getProcessLock(uint(serverID))
	pl.Lock()
	status, err := serverInstance.GetStatus(resourceMap)
	pl.Unlock()
	if err != nil {
		c.JSON(500, gin.H{
			"result": "failed",
			"error":  "获取状态失败: " + err.Error(),
		})
		return
	}
	response := &ServerStatusResponse{
		Version:           status.Version,
		ManagementVersion: status.ManagementVersion,
		ClientList:        []*ServerStatusClientInfoResponse{},
	}
	connectedClientInfoRecordList, err := a.daoManager.ListConnectedClientInfoRecordByServerID(uint(serverID))
	// map[ip][username]
	ip2usernameMap := make(map[string]string)
	if err == nil {
		for _, info := range connectedClientInfoRecordList {
			ip2usernameMap[info.VirtualIPAddr] = info.Username
		}
	}
	aclList, err := a.daoManager.ListAddedACLByServerID(uint(serverID))
	// map[] map[192.168.1.1][]string{"4#10.0.0.0/24", "4#172.31.106.0/24"}
	// map[虚拟ip][]ACL字符串列表
	ip2aclMap := make(map[string][]string)
	if err == nil {
		for _, acl := range aclList {
			aclString := fmt.Sprintf("%d#%s", acl.ACLType, acl.ACLValue)
			ip2aclMap[acl.VirtualIPAddr] = append(ip2aclMap[acl.VirtualIPAddr], aclString)
		}
	}
	for _, clientInfo := range status.ClientList {
		username := ""
		// 给客户端信息补上用户名
		for _, virtualIPAddr := range clientInfo.VirtualIPAddr {
			_username, ok := ip2usernameMap[virtualIPAddr]
			if ok {
				username = _username
				break
			}
		}
		aclStringList := []string{}
		// 给客户端信息补上ACL列表
		// 一般来说  clientInfo.VirtualIPAddr 只有一个虚拟IP
		// 如果有多个，就是客户端网段路由 客户端网段路由不会获取到acl的
		// ipv6先不实现 除非有人充钱给我实现 或者他自己去实现
		for _, virtualIPAddr := range clientInfo.VirtualIPAddr {
			acl, ok := ip2aclMap[virtualIPAddr]
			if ok {
				aclStringList = append(aclStringList, acl...)
			}
		}
		response.ClientList = append(response.ClientList, &ServerStatusClientInfoResponse{
			CommonName:     clientInfo.CommonName,
			RealIPAddr:     clientInfo.RealIPAddr,
			VirtualIPAddr:  clientInfo.VirtualIPAddr,
			ByteReceived:   clientInfo.ByteReceived,
			ByteSent:       clientInfo.ByteSent,
			ConnectedSince: clientInfo.ConnectedSince,
			LastRef:        clientInfo.LastRef,
			Username:       username,
			ACLList:        aclStringList,
		})
	}
	if err != nil {
		c.JSON(500, gin.H{
			"result": "failed",
			"error":  "获取状态失败: " + err.Error(),
		})
		return
	}
	c.JSON(200, gin.H{
		"result": "success",
		"error":  nil,
		"data":   response,
	})
}

// 关闭客户端连接
func (a *App) CloseOpenVPNServerClientHandler(c *gin.Context, user *models.User) {
	type RequestParam struct {
		ID         uint   `json:"id" binding:"required"`
		ReadIPAddr string `json:"read_ip_addr" binding:"required,max=100"`
	}
	var param RequestParam
	err := c.ShouldBindJSON(&param)
	if err != nil {
		c.JSON(400, gin.H{
			"result": "failed",
			"error":  "关闭客户端连接失败: 参数错误",
		})
		return
	}
	resourceMap := a.PrepareResourceMap([]string{ovpnserver.RESOURCE_ID_MISC_CONFIG})
	a.lock.RLock()
	defer a.lock.RUnlock()
	serverInstance, ok := a.ovpnProcessList[param.ID]
	if !ok {
		c.JSON(400, gin.H{
			"result": "failed",
			"error":  "关闭客户端连接失败: 服务器未运行",
		})
		return
	}
	// 踢客户端要访问管理 socket，取实例写锁，与其它 socket 操作互斥。
	pl := a.getProcessLock(param.ID)
	pl.Lock()
	message, err := serverInstance.CloseClient(param.ReadIPAddr, resourceMap)
	pl.Unlock()
	if err != nil {
		c.JSON(500, gin.H{
			"result": "failed",
			"error":  err.Error(),
		})
		return
	}
	c.JSON(200, gin.H{
		"result": "success",
		"error":  nil,
		"data":   message,
	})
}

// 停止所有服务器，用于在按下Ctrl C时执行
func (a *App) StopAllOpenVPNServer() {
	a.lock.Lock()
	defer a.lock.Unlock()
	resourceMap := a.PrepareResourceMap([]string{ovpnserver.RESOURCE_ID_MISC_CONFIG, ovpnserver.RESOURCE_ID_SERVER_EXIT_SCRIPT})
	for serverID, serverInstance := range a.ovpnProcessList {
		// 启停进程会改写实例的 cmd/pid，取实例写锁。
		pl := a.getProcessLock(serverID)
		pl.Lock()
		if serverInstance.Running() {
			err := serverInstance.Stop(resourceMap)
			if err != nil {
				a.daoManager.CreateEvent(serverInstance.GetServerModel().ID, models.SERVER_EVENT_TYPE_SERVER_EXIT_FAIL, "", "服务器停止失败: "+err.Error())

			} else {
				a.daoManager.DeleteAddedACLByServerID(serverInstance.GetServerModel().ID)
				a.daoManager.DeleteConnectedClientInfoRecordByServerID(serverInstance.GetServerModel().ID)
				a.daoManager.DeleteServerProcess(serverInstance.GetServerModel().ID)
				a.daoManager.CreateEvent(serverInstance.GetServerModel().ID, models.SERVER_EVENT_TYPE_SERVER_EXIT_SUCCESS, "internal", "服务器停止成功")

			}
		}
		pl.Unlock()
	}
}

// 自动启动所有标记为自启动的服务器
func (a *App) AutoStartOpenVPNServer() {
	a.lock.Lock()
	defer a.lock.Unlock()
	serverModelList, err := a.daoManager.ListOpenVPNServerByAutoStart(true)
	if err != nil {
		log.Println("列出自动启动服务器列表失败: " + err.Error())
		return
	}
	for _, serverModel := range serverModelList {
		a.autoStartServerLocked(serverModel)
	}
}

// autoStartServerLocked 启动单个自启动服务器，调用方需持有全局写锁 a.lock。
func (a *App) autoStartServerLocked(serverModel *models.Server) {
	a.startServerLocked(serverModel, "internal")
}

// startServerLocked 在持有全局写锁 a.lock 的前提下，(重新)创建并启动服务器实例。
// 内部按“全局锁 -> 实例锁”的顺序获取实例锁；使用 return 而非 continue，保证锁必然释放。
// source 用于服务器事件的来源标记（如 internal / keepalive）。
func (a *App) startServerLocked(serverModel *models.Server, source string) {
	serverInstance, ok := a.ovpnProcessList[serverModel.ID]
	// 启停进程会改写实例的 cmd/pid，取实例写锁。
	pl := a.getProcessLock(serverModel.ID)
	pl.Lock()
	defer pl.Unlock()

	if ok && serverInstance.Running() {
		return
	}
	// 当服务器没有在进程列表中存在，或者服务器进程没有运行时，进行初始化配置文件操作
	serverRouteList, err := a.daoManager.ListOpenVPNServerRoute(serverModel.ID)
	if err != nil {
		log.Println("服务器启动失败: 列出服务端路由失败: " + err.Error())
		a.daoManager.CreateEvent(serverModel.ID, models.SERVER_EVENT_TYPE_SERVER_START_FAIL, source, "服务器启动失败: 列出服务端路由失败 "+err.Error())
		return
	}
	clientConfigList, err := a.daoManager.ListOpenVPNServerClientConfig(serverModel.ID)
	if err != nil {
		log.Println("服务器启动失败: 列出客户端配置失败: " + err.Error())
		a.daoManager.CreateEvent(serverModel.ID, models.SERVER_EVENT_TYPE_SERVER_START_FAIL, source, "服务器启动失败: 列出客户端配置失败 "+err.Error())
		return
	}
	resourceMap := a.PrepareResourceMap([]string{ovpnserver.RESOURCE_ID_CONFIG_TEMPLATE, ovpnserver.RESOURCE_ID_CLIENT_OFFLINE_SCRIPT, ovpnserver.RESOURCE_ID_CLIENT_ONLINE_SCRIPT, ovpnserver.RESOURCE_ID_AUTH_SCRIPT, ovpnserver.RESOURCE_ID_SERVER_START_SCRIPT, ovpnserver.RESOURCE_ID_MISC_CONFIG})
	var miscConfigStr string
	miscConfig, ok := resourceMap[ovpnserver.RESOURCE_ID_MISC_CONFIG]
	if ok {
		miscConfigStr = miscConfig
	} else {
		miscConfigStr = ovpnserver.GetDefaultResource(ovpnserver.RESOURCE_ID_MISC_CONFIG)
	}
	var miscConfigModel ovpnserver.MiscConfig
	err = json.Unmarshal([]byte(miscConfigStr), &miscConfigModel)
	if err != nil {
		log.Println("服务器启动失败: 解析杂项配置失败: " + err.Error())
		a.daoManager.CreateEvent(serverModel.ID, models.SERVER_EVENT_TYPE_SERVER_START_FAIL, source, "服务器启动失败: 解析杂项配置失败 "+err.Error())
		return
	}
	serverInstance = ovpnserver.NewOpenVPNServerInstance(a.resolvedServerModel(serverModel), serverRouteList, clientConfigList, fmt.Sprintf("%s/%d", a.cfg.WorkingDir, serverModel.ID), a.cfg.InternalAPIListen, miscConfigModel.OpenVPNPath)
	a.ovpnProcessList[serverModel.ID] = serverInstance

	err = serverInstance.WriteConfig(resourceMap)
	if err != nil {
		log.Println("服务器启动失败: 配置文件写入失败: " + err.Error())
		a.daoManager.CreateEvent(serverModel.ID, models.SERVER_EVENT_TYPE_SERVER_START_FAIL, source, "服务器启动失败: 配置文件写入失败 "+err.Error())
		return
	}
	err = serverInstance.Start(resourceMap)
	if err != nil {
		log.Println("服务器启动失败: " + err.Error())
		a.daoManager.CreateEvent(serverModel.ID, models.SERVER_EVENT_TYPE_SERVER_START_FAIL, source, "服务器启动失败: "+err.Error())
		return
	}
	a.daoManager.CreateEvent(serverModel.ID, models.SERVER_EVENT_TYPE_SERVER_START_SUCCESS, source, "服务器启动成功")
	// 退出时不停止实例的场景：把 PID 记入数据库，供面板重启后重新接管。
	a.recordServerProcess(serverModel.ID, serverInstance.GetPID())
	// 看门狗（keepalive）拉起：说明服务器此前异常退出，额外记录一条“已恢复”事件。
	if source == "keepalive" {
		a.daoManager.CreateEvent(serverModel.ID, models.SERVER_EVENT_TYPE_SERVER_RECOVERED, source, "服务器异常退出后已被看门狗自动拉起，服务器已恢复")
	}
	a.daoManager.DeleteAddedACLByServerID(serverModel.ID)
	a.daoManager.DeleteConnectedClientInfoRecordByServerID(serverModel.ID)
}
