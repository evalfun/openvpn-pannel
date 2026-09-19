package api

import (
	"encoding/base64"
	"fmt"
	"log"
	"openvpn-pannel/internal/models"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

func ParseInternalAPIData(defaultValue map[string]string, c *gin.Context) (map[string]string, error) {
	// 读取http body
	body, err := c.GetRawData()
	if err != nil {
		return nil, fmt.Errorf("读取http body失败: %s", err.Error())
	}
	if defaultValue == nil {
		defaultValue = make(map[string]string)
	}
	bodyLineList := strings.Split(string(body), "\n")
	for _, line := range bodyLineList {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		keyValue := strings.Split(line, ": ")
		if len(keyValue) != 2 {
			continue
		}
		defaultValue[keyValue[0]] = keyValue[1]
	}
	return defaultValue, nil
}
func (a *App) UserAuthInternalHandler(c *gin.Context) {
	requestData := map[string]string{
		"username":         "",
		"password":         "",
		"server_id":        "",
		"real_ip_addr":     "",
		"client_cert_name": "",
	}
	var err error
	requestData, err = ParseInternalAPIData(requestData, c)
	if err != nil {
		c.String(400, "result="+err.Error())
		return
	}
	serverID, err := strconv.ParseUint(requestData["server_id"], 10, 64)
	if err != nil {
		c.String(400, "result="+"server_id参数错误")
		return
	}
	// 对username和password进行base64解码
	_username, err := base64.StdEncoding.DecodeString(requestData["username"])
	if err != nil {
		c.String(400, "result="+"username参数错误")
		return
	}
	_password, err := base64.StdEncoding.DecodeString(requestData["password"])
	if err != nil {
		c.String(400, "result="+"password参数错误")
		return
	}
	username := string(_username)
	password := string(_password)
	//log.Println("auth", username, password)
	userModel, err := a.daoManager.AuthUser(username, password)
	if err != nil {
		c.String(403, "result="+"用户名或密码错误")
		a.daoManager.CreateEvent(uint(serverID), models.SERVER_EVENT_TYPE_CLIENT_AUTH_FAIL, requestData["real_ip_addr"], fmt.Sprintf("用户名或密码错误 证书=%s 用户名=%s", requestData["client_cert_name"], username))
		return
	}
	// 检查用户是否有权限
	result, err := a.daoManager.CheckServerPermission(uint(serverID), userModel)
	if err != nil {
		c.String(500, "result="+"权限检查失败")
		a.daoManager.CreateEvent(uint(serverID), models.SERVER_EVENT_TYPE_CLIENT_AUTH_FAIL, requestData["real_ip_addr"], fmt.Sprintf("权限检查失败 证书=%s 用户=%s %s", requestData["client_cert_name"], username, err.Error()))
		return
	}
	if result {
		// 达量限速：匹配到“禁止连接”规则时直接认证失败，并提示多久后可继续连接。
		_, _, allowConnect, rlErr := a.daoManager.ResolveUserRateLimitAndConnect(userModel.ID, uint(serverID))
		if rlErr != nil {
			log.Println("达量限速校验失败: ", rlErr.Error())
		}
		if !allowConnect {
			remaining, _ := a.daoManager.GetRateLimitResetRemaining(userModel.ID)
			reason := fmt.Sprintf("已达流量上限，禁止连接，请在 %s 后可继续连接", formatRetryAfter(remaining))
			c.String(403, "result="+reason)
			a.daoManager.CreateEvent(uint(serverID), models.SERVER_EVENT_TYPE_CLIENT_AUTH_FAIL, requestData["real_ip_addr"], fmt.Sprintf("达量限速禁止连接 证书=%s 用户=%s", requestData["client_cert_name"], username))
			return
		}
		c.String(200, "result="+"success")
		a.daoManager.CreateEvent(uint(serverID), models.SERVER_EVENT_TYPE_CLIENT_AUTH_SUCCESS, requestData["real_ip_addr"], fmt.Sprintf("登录成功 证书=%s 用户=%s", requestData["client_cert_name"], username))
	} else {
		c.String(403, "result="+"没有权限")
		a.daoManager.CreateEvent(uint(serverID), models.SERVER_EVENT_TYPE_CLIENT_AUTH_FAIL, requestData["real_ip_addr"], fmt.Sprintf("没有权限 证书=%s 用户=%s", requestData["client_cert_name"], username))
	}
}

func (a *App) DelUserACLInternalHandler(c *gin.Context) {
	requestData := map[string]string{
		"username":        "",
		"server_id":       "",
		"real_ip_addr":    "",
		"virtual_ip_addr": "",
	}
	var err error
	requestData, err = ParseInternalAPIData(requestData, c)
	if err != nil {
		c.String(400, "result="+err.Error())
		return
	}
	serverID, err := strconv.ParseUint(requestData["server_id"], 10, 64)
	if err != nil {
		c.String(400, "result="+"server_id参数错误")
		return
	}
	username, err := base64.StdEncoding.DecodeString(requestData["username"])
	if err != nil {
		c.String(400, "result="+"username参数错误")
		return
	}
	virtualIPAddr := requestData["virtual_ip_addr"]
	realIPAddr := requestData["real_ip_addr"]
	aclList, err := a.daoManager.ListAddedACLByIP(virtualIPAddr, uint(serverID))
	addedACLString := ""
	resultString := ""
	if err == nil {
		for _, acl := range aclList {
			addedACLString = fmt.Sprintf("%s[%d#%s] ", addedACLString, acl.ACLType, acl.ACLValue)
			resultString = fmt.Sprintf("%s%d#%s\n", resultString, acl.ACLType, acl.ACLValue)
		}
	} else {
		log.Println("列出已添加的ACL失败: ", err.Error())
	}
	a.daoManager.CreateEvent(uint(serverID), models.SERVER_EVENT_TYPE_DEL_ACL, realIPAddr, fmt.Sprintf("用户=%s 删除的ACL=%s", username, addedACLString))

	err = a.daoManager.DeleteAddedACLByIP(virtualIPAddr, uint(serverID))
	if err != nil {
		c.String(500, "result="+"删除ACL失败"+err.Error())
		return
	}
	c.String(200, resultString)
}

func (a *App) UserOnlineInternalHandler(c *gin.Context) {
	requestData := map[string]string{
		"username":         "",
		"server_id":        "",
		"real_ip_addr":     "",
		"virtual_ip_addr":  "",
		"client_cert_name": "",
	}
	var err error
	requestData, err = ParseInternalAPIData(requestData, c)
	if err != nil {
		c.String(400, "result="+err.Error())
		return
	}
	serverID, err := strconv.ParseUint(requestData["server_id"], 10, 64)
	if err != nil {
		c.String(400, "result="+"server_id参数错误")
		return
	}
	username, err := base64.StdEncoding.DecodeString(requestData["username"])
	if err != nil {
		c.String(400, "result="+"username参数错误")
		return
	}
	realIPAddr := requestData["real_ip_addr"]
	virtualIPAddr := requestData["virtual_ip_addr"]
	clientCertName := requestData["client_cert_name"]
	a.daoManager.CreateEvent(uint(serverID), models.SERVER_EVENT_TYPE_CLIENT_ONLINE, realIPAddr, fmt.Sprintf("证书=%s 用户=%s 虚拟IP=%s", clientCertName, username, virtualIPAddr))
	// 计算并记录本次会话生效的限速，供运行时“仅对限速变化的客户端重设 tc”判断是否变化。
	var appliedUploadKB, appliedDownloadKB uint64
	if userModel, err := a.daoManager.GetUserByUsername(string(username)); err == nil {
		uploadKB, downloadKB, _, rlErr := a.daoManager.ResolveUserRateLimitAndConnect(userModel.ID, uint(serverID))
		if rlErr != nil {
			log.Println("计算用户生效限速失败: ", rlErr.Error())
		} else {
			appliedUploadKB, appliedDownloadKB = uploadKB, downloadKB
		}
	}
	err = a.daoManager.CreateConnectedClientInfoRecord(&models.ConnectedClientInfoRecord{
		VirtualIPAddr:   virtualIPAddr,
		ServerID:        uint(serverID),
		Username:        string(username),
		UploadLimitKB:   appliedUploadKB,
		DownloadLimitKB: appliedDownloadKB,
	})
	if err != nil {
		log.Println("记录用户在线信息失败" + err.Error())
		return
	}
	c.String(200, "result="+"success")
}

func getReadableFileSize(fileSize uint64) string {
	var size string
	switch {
	case fileSize >= 1024*1024*1024:
		size = fmt.Sprintf("%.2f GB", float64(fileSize)/(1024*1024*1024))
	case fileSize >= 1024*1024:
		size = fmt.Sprintf("%.2f MB", float64(fileSize)/(1024*1024))
	case fileSize >= 1024:
		size = fmt.Sprintf("%.2f KB", float64(fileSize)/1024)
	case fileSize > 0:
		size = fmt.Sprintf("%d B", fileSize)
	}
	return size
}

func (a *App) UserOfflineInternalHandler(c *gin.Context) {
	requestData := map[string]string{
		"username":         "",
		"server_id":        "",
		"real_ip_addr":     "",
		"virtual_ip_addr":  "",
		"client_cert_name": "",
		"bytes_send":       "",
		"bytes_received":   "",
	}
	var err error
	requestData, err = ParseInternalAPIData(requestData, c)
	if err != nil {
		c.String(400, "result="+err.Error())
		return
	}
	serverID, err := strconv.ParseUint(requestData["server_id"], 10, 64)
	if err != nil {
		c.String(400, "result="+"server_id参数错误")
		return
	}
	username, err := base64.StdEncoding.DecodeString(requestData["username"])
	if err != nil {
		c.String(400, "result="+"username参数错误")
		return
	}
	realIPAddr := requestData["real_ip_addr"]
	virtualIPAddr := requestData["virtual_ip_addr"]
	clientCertName := requestData["client_cert_name"]
	bytesSend, err := strconv.ParseUint(requestData["bytes_send"], 10, 64)
	if err != nil {
		c.String(400, "result="+"bytes_send参数错误")
		return
	}
	bytesReceived, err := strconv.ParseUint(requestData["bytes_received"], 10, 64)
	if err != nil {
		c.String(400, "result="+"bytes_received参数错误")
		return
	}
	a.daoManager.CreateEvent(uint(serverID), models.SERVER_EVENT_TYPE_CLIENT_OFFLINE, realIPAddr, fmt.Sprintf("证书=%s 用户=%s IP=%s 虚拟IP=%s 发送=%s 接收=%s 发送字节数=%d 接收字节数=%d",
		clientCertName, username, realIPAddr, virtualIPAddr, getReadableFileSize(bytesSend), getReadableFileSize(bytesReceived), bytesSend, bytesReceived))

	err = a.daoManager.UpdateUserTraffic(string(username), bytesSend, bytesReceived)
	if err != nil {
		log.Println("更新用户流量失败: ", err.Error())
	}
	// 累加到达量限速方案的当前周期统计（未关联方案的会被忽略）。
	if err = a.daoManager.AddUserCycleTraffic(string(username), bytesSend, bytesReceived); err != nil {
		log.Println("更新用户周期流量失败: ", err.Error())
	}

	err = a.daoManager.DeleteConnectedClientInfoRecord(virtualIPAddr, uint(serverID))
	if err != nil {
		log.Println("删除用户在线信息失败" + err.Error())
		return
	}
	c.String(200, "result="+"success")
}

func (a *App) GetUserACLInternalHandler(c *gin.Context) {
	requestData := map[string]string{
		"username":        "",
		"server_id":       "",
		"real_ip_addr":    "",
		"virtual_ip_addr": "",
	}
	var err error
	requestData, err = ParseInternalAPIData(requestData, c)
	if err != nil {
		c.String(400, "result="+err.Error())
		return
	}
	serverID, err := strconv.ParseUint(requestData["server_id"], 10, 64)
	if err != nil {
		c.String(400, "result="+"server_id参数错误")
		return
	}
	username, err := base64.StdEncoding.DecodeString(requestData["username"])
	if err != nil {
		c.String(400, "result="+"username参数错误")
		return
	}
	realIPAddr := requestData["real_ip_addr"]
	virtualIPAddr := requestData["virtual_ip_addr"]

	//log.Println(serverID, username, realIPAddr)
	userModel, err := a.daoManager.GetUserByUsername(string(username))
	if err != nil {
		c.String(404, "result="+"用户不存在")
		return
	}
	var addedACLRecordList []*models.AddedServerACLRecord
	groupACLList, err := a.daoManager.GetACLByUser(userModel.ID, uint(serverID))
	if err != nil {
		c.String(500, "result="+"获取ACL失败 "+err.Error())
		return
	}
	var resultMap map[string]*models.GroupACL
	resultMap = make(map[string]*models.GroupACL)
	for _, groupACL := range groupACLList {
		resultMap[fmt.Sprintf("%d#%s", groupACL.Type, groupACL.Value)] = groupACL
	}
	addedACLString := ""
	resultString := ""
	for k, v := range resultMap {
		addedACLString = fmt.Sprintf("%s[%s] ", addedACLString, k)
		resultString = fmt.Sprintf("%s%s\n", resultString, k)
		addedACLRecordList = append(addedACLRecordList, &models.AddedServerACLRecord{
			ACLType:       v.Type,
			ACLValue:      v.Value,
			VirtualIPAddr: virtualIPAddr,
			ServerID:      uint(serverID),
		})
	}
	a.daoManager.CreateEvent(uint(serverID), models.SERVER_EVENT_TYPE_ADD_ACL, realIPAddr, fmt.Sprintf("用户=%s 添加的ACL=%s", username, addedACLString))
	err = a.daoManager.SaveAddedACL(addedACLRecordList)
	if err != nil {
		log.Println("保存添加的acl失败: ", err.Error())
	}
	c.String(200, resultString)
}

// GetUserRateLimitInternalHandler 供 client_online.sh 在上线时查询该用户在当前服务器上的
// 生效限速，返回纯文本：
//
//	upload_kb: <N>
//	download_kb: <N>
//
// 0 表示不限速。脚本据此用 tc 设置限速。
func (a *App) GetUserRateLimitInternalHandler(c *gin.Context) {
	requestData := map[string]string{
		"username":  "",
		"server_id": "",
	}
	var err error
	requestData, err = ParseInternalAPIData(requestData, c)
	if err != nil {
		c.String(400, "result="+err.Error())
		return
	}
	serverID, err := strconv.ParseUint(requestData["server_id"], 10, 64)
	if err != nil {
		c.String(400, "result="+"server_id参数错误")
		return
	}
	username, err := base64.StdEncoding.DecodeString(requestData["username"])
	if err != nil {
		c.String(400, "result="+"username参数错误")
		return
	}
	userModel, err := a.daoManager.GetUserByUsername(string(username))
	if err != nil {
		c.String(404, "result="+"用户不存在")
		return
	}
	uploadKB, downloadKB, err := a.daoManager.ResolveUserRateLimit(userModel.ID, uint(serverID))
	if err != nil {
		c.String(500, "result="+"计算限速失败 "+err.Error())
		return
	}
	c.String(200, fmt.Sprintf("upload_kb: %d\ndownload_kb: %d\n", uploadKB, downloadKB))
}

func (a *App) SetupInternalAPIRoutes() {
	a.internalAPIRouter.POST("/user/auth", a.UserAuthInternalHandler)
	a.internalAPIRouter.POST("/user/acl/get", a.GetUserACLInternalHandler)
	a.internalAPIRouter.POST("/user/acl/del", a.DelUserACLInternalHandler)
	a.internalAPIRouter.POST("/user/online", a.UserOnlineInternalHandler)
	a.internalAPIRouter.POST("/user/offline", a.UserOfflineInternalHandler)
	a.internalAPIRouter.POST("/user/ratelimit/get", a.GetUserRateLimitInternalHandler)
}
