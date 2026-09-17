package dao

import (
	"errors"
	"fmt"
	"openvpn-pannel/internal/models"
)

// 创建服务器
func (um *DaoManager) CreateOpenVPNServer(server *models.Server, serverRouteList []string) error {
	tx := um.DB.Begin()
	err := tx.Create(server).Error
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("创建服务器失败: %s", err.Error())
	}

	if len(serverRouteList) != 0 {
		var serverRouteModelList []*models.ServerRoute
		for _, serverRoute := range serverRouteList {
			serverRouteModelList = append(serverRouteModelList, &models.ServerRoute{
				ServerID: server.ID,
				Network:  serverRoute,
			})
		}
		err = tx.CreateInBatches(serverRouteModelList, 100).Error
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("创建服务器路由失败: %s", err.Error())
		}
	}

	err = tx.Commit().Error
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("创建服务器失败: 提交事务失败: %s", err.Error())
	}
	return nil
}

// 更新服务器
func (um *DaoManager) UpdateOpenVPNServer(server *models.Server, serverRouteList []string) error {
	tx := um.DB.Begin()
	var err error
	err = tx.Save(server).Error
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("更新服务器失败: %s", err.Error())
	}
	err = tx.Where("server_id = ?", server.ID).Delete(&models.ServerRoute{}).Error
	if err != nil {
		tx.Rollback()
		return errors.New("更新服务器失败: 清理路由失败 " + err.Error())
	}

	if len(serverRouteList) != 0 {
		var serverRouteModelList []*models.ServerRoute
		for _, serverRoute := range serverRouteList {
			serverRouteModelList = append(serverRouteModelList, &models.ServerRoute{
				ServerID: server.ID,
				Network:  serverRoute,
			})
		}
		err = tx.CreateInBatches(serverRouteModelList, 100).Error
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("更新服务器路由失败: %s", err.Error())
		}
	}

	err = tx.Commit().Error
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("更新服务器失败: 提交事务失败: %s", err.Error())
	}
	return um.DB.Save(server).Error
}

// UpdateServerExportSetting 只更新客户端导出使用的服务器地址、端口与附加配置。
func (um *DaoManager) UpdateServerExportSetting(serverID uint, host string, port uint32, extraConfig string) error {
	return um.DB.Model(&models.Server{}).Where("id = ?", serverID).
		Updates(map[string]interface{}{
			"export_host":         host,
			"export_port":         port,
			"export_extra_config": extraConfig,
		}).Error
}

// 删除服务器
func (um *DaoManager) DeleteOpenVPNServer(serverID uint) error {
	tx := um.DB.Begin()
	result := tx.Where("server_id = ?", serverID).Delete(&models.ServerRoute{})
	if result.Error != nil {
		tx.Rollback()
		return errors.New("删除服务器路由失败 " + result.Error.Error())
	}

	result = tx.Where("server_id = ?", serverID).Delete(&models.ClientConfig{})
	if result.Error != nil {
		tx.Rollback()
		return errors.New("删除服务器客户端配置失败 " + result.Error.Error())
	}

	result = tx.Where("server_id = ?", serverID).Delete(&models.ServerPermission{})
	if result.Error != nil {
		tx.Rollback()
		return errors.New("删除服务器权限配置失败 " + result.Error.Error())
	}

	result = tx.Where("server_id = ?", serverID).Delete(&models.ServerEvent{})
	if result.Error != nil {
		tx.Rollback()
		return errors.New("清理服务器事件失败 " + result.Error.Error())
	}

	result = tx.Where("server_id = ?", serverID).Delete(&models.AddedServerACLRecord{})
	if result.Error != nil {
		tx.Rollback()
		return errors.New("清理服务器已经添加的ACL失败 " + result.Error.Error())
	}

	result = tx.Where("server_id = ?", serverID).Delete(&models.ConnectedClientInfoRecord{})
	if result.Error != nil {
		tx.Rollback()
		return errors.New("清理服务器已经连接的客户端信息失败 " + result.Error.Error())
	}

	err := tx.Delete(&models.Server{}, serverID).Error
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("删除服务器失败: %s", err.Error())
	}
	err = tx.Commit().Error
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("删除服务器失败: 提交事务失败: %s", err.Error())
	}
	return nil
}

// 列出服务器
func (um *DaoManager) ListOpenVPNServer() ([]*models.Server, error) {
	var serverModelList []*models.Server
	result := um.DB.Find(&serverModelList)
	if result.Error != nil {
		return nil, errors.New("列出服务器失败 " + result.Error.Error())
	}
	return serverModelList, nil
}

func (um *DaoManager) ListOpenVPNServerByAutoStart(autostart bool) ([]*models.Server, error) {
	var serverModelList []*models.Server
	result := um.DB.Where("auto_start = ?", autostart).Find(&serverModelList)
	if result.Error != nil {
		return nil, errors.New("列出服务器失败 " + result.Error.Error())
	}
	return serverModelList, nil
}

// 获取服务器路由
func (um *DaoManager) ListOpenVPNServerRoute(serverID uint) ([]*models.ServerRoute, error) {
	var serverRouteList []*models.ServerRoute
	result := um.DB.Where("server_id = ?", serverID).Find(&serverRouteList)
	if result.Error != nil {
		return nil, errors.New("列出服务器路由失败 " + result.Error.Error())
	}
	return serverRouteList, nil
}

// 获取所有服务器路由
func (um *DaoManager) ListAllOpenVPNServerRoute() ([]*models.ServerRoute, error) {
	var serverRouteList []*models.ServerRoute
	result := um.DB.Find(&serverRouteList)
	if result.Error != nil {
		return nil, errors.New("列出服务器路由失败 " + result.Error.Error())
	}
	return serverRouteList, nil
}

// 删除服务器路由  弃用
func (um *DaoManager) DeleteServerRoute(serverID uint) error {
	result := um.DB.Where("server_id = ?", serverID).Delete(&models.ServerRoute{})
	if result.Error != nil {
		return errors.New("删除服务器路由失败 " + result.Error.Error())
	}
	return nil
}

// 添加服务器路由  弃用
func (um *DaoManager) CreateServerRoute(serverID uint, serverRouteStringList []string) error {
	if len(serverRouteStringList) == 0 {
		return nil
	}
	var serverRouteList []*models.ServerRoute
	for _, serverRouteString := range serverRouteStringList {
		serverRoute := models.ServerRoute{
			ServerID: serverID,
			Network:  serverRouteString,
		}
		serverRouteList = append(serverRouteList, &serverRoute)
	}
	um.DB.CreateInBatches(serverRouteList, 100)
	return nil
}

func (um *DaoManager) GetOpenVPNServerClientConfig(id uint) (*models.ClientConfig, error) {
	clientConfig := models.ClientConfig{
		ID: id,
	}
	err := um.DB.First(&clientConfig).Error
	return &clientConfig, err
}

// 获取服务器客户端配置
func (um *DaoManager) ListOpenVPNServerClientConfig(serverID uint) ([]*models.ClientConfig, error) {
	var clientConfigList []*models.ClientConfig
	result := um.DB.Where("server_id = ?", serverID).Find(&clientConfigList)
	if result.Error != nil {
		return nil, errors.New("列出客户端配置失败 " + result.Error.Error())
	}
	return clientConfigList, nil
}

// 删除客户端配置
func (um *DaoManager) DeleteOpenVPNServerClientConfig(clientConfigID []uint) error {
	return um.DB.Delete(&models.ClientConfig{}, clientConfigID).Error
}

// 新增或更新客户端配置
func (um *DaoManager) AddOpenVPNServerClientConfig(clientConfig *models.ClientConfig) error {
	tx := um.DB.Begin()
	var currentConfig *models.ClientConfig
	err := tx.Where("server_id = ? and client_cert_name = ?", clientConfig.ServerID, clientConfig.ClientCertName).First(&currentConfig).Error
	if err != nil {
		err = um.DB.Create(clientConfig).Error
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("新增客户端配置失败: %s", err.Error())
		}
	} else {
		clientConfig.ID = currentConfig.ID
		err = tx.Save(clientConfig).Error
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("更新客户端配置失败: %s", err.Error())
		}
	}
	err = tx.Commit().Error
	return err
}

// 添加权限
func (um *DaoManager) AddServerPermission(serverPerm *models.ServerPermission) error {
	return um.DB.Create(serverPerm).Error
}

// 删除权限
func (um *DaoManager) DelServerPermission(permID uint) error {
	return um.DB.Delete(&models.ServerPermission{}, permID).Error
}

// 列出权限

type ServerPermissionRecord struct {
	PermissionID uint
	ObjID        uint
	ObjName      string
	Action       uint
}
type ServerPermissionResult struct {
	GroupPermission []ServerPermissionRecord
	UserPermission  []ServerPermissionRecord
}

func (um *DaoManager) ListServerPermission(serverID uint) (*ServerPermissionResult, error) {
	result := &ServerPermissionResult{}
	err := um.DB.Model(&models.ServerPermission{}).Select("server_permissions.id as permission_id,  server_permissions.obj_id, `groups`.name as obj_name, server_permissions.action ").Joins("left  join servers on server_permissions.server_id = servers.id  left  join `groups` on server_permissions.obj_id = `groups`.id ").Where(" server_id= ? and obj_type=2", serverID).Scan(&result.GroupPermission).Error
	if err != nil {
		return nil, fmt.Errorf("列出用户组权限信息失败: %s", err.Error())
	}

	err = um.DB.Model(&models.ServerPermission{}).Select("server_permissions.id as permission_id, server_permissions.obj_id, `users`.username as obj_name, server_permissions.action ").Joins("left  join servers on server_permissions.server_id = servers.id  left  join `users` on server_permissions.obj_id = `users`.id ").Where(" server_id= ? and obj_type=1", serverID).Scan(&result.UserPermission).Error
	if err != nil {
		return nil, fmt.Errorf("列出用户权限信息失败: %s", err.Error())
	}
	return result, nil
}

// 通过id获取服务器
func (um *DaoManager) GetOpenVPNServerByID(serverID uint) (*models.Server, error) {
	var serverModel models.Server
	err := um.DB.First(&serverModel, serverID).Error
	if err != nil {
		return nil, err
	}
	return &serverModel, nil
}

// 获取权限列表
// CheckServerPermission 检查用户是否有访问指定服务器的权限
// 参数:
//
//	serverID: 服务器ID
//	userModel: 用户模型对象
//
// 返回值:
//
//	bool: 是否有权限
//	error: 错误信息，如果有权限则为nil
func (um *DaoManager) CheckServerPermission(serverID uint, userModel *models.User) (bool, error) {
	// 先判断用户是不是有权限
	var serverPermissionList []*models.ServerPermission
	err := um.DB.Where("server_id = ? and obj_type= ? and obj_id = ?", serverID, models.SERVER_PERM_OBJ_TYPE_USER, userModel.ID).Find(&serverPermissionList).Error
	if err != nil {
		return false, err
	}
	resultPermit := false
	for _, serverPerserverPermission := range serverPermissionList {
		if serverPerserverPermission.Action == models.SERVER_PERM_ACTION_PERMIT {
			resultPermit = true
		} else if serverPerserverPermission.Action == models.SERVER_PERM_ACTION_DENY {
			return false, fmt.Errorf("用户 %s 访问服务器 %d 时被拒绝", userModel.Username, serverID)
		}
	}
	// 用户规则的优先级大于用户组规则的优先级
	if resultPermit {
		return true, nil
	}
	// 判断用户组是不是有权限
	//err := um.DB.Find(&serverPermissionList).Error
	um.DB.Where("obj_id in (?) and obj_type = ? and server_id=?", um.DB.Table("user_groups").Where("user_id = ? ", userModel.ID).Select("group_id"), models.SERVER_PERM_OBJ_TYPE_GROUP, serverID).Find(&serverPermissionList)
	// SQL: SELECT * FROM "server_permission" WHERE obj_id in (SELECT group_id FROM user_group where user_id = ?) and obj_type=2;
	resultPermit = false
	for _, serverPerserverPermission := range serverPermissionList {
		if serverPerserverPermission.ObjType == models.SERVER_PERM_OBJ_TYPE_GROUP {
			if serverPerserverPermission.Action == models.SERVER_PERM_ACTION_PERMIT {
				resultPermit = true
			} else if serverPerserverPermission.Action == models.SERVER_PERM_ACTION_DENY {
				groupModel, err2 := um.GetGroupByID(serverPerserverPermission.ObjID)
				if err2 != nil {
					return false, fmt.Errorf("用户 %s 的所属组id %d 访问服务器id %d 时被拒绝", userModel.Username, serverPerserverPermission.ObjID, serverID)
				}
				return false, fmt.Errorf("用户 %s 的所属组 %s 访问服务器id %d 时被拒绝", userModel.Username, groupModel.Name, serverID)
			}
		}
	}
	if resultPermit {
		return true, nil
	} else {
		return false, fmt.Errorf("用户 %s 访问服务器id %d 时由于没有明确的允许规则，被拒绝", userModel.Username, serverID)
	}
}

// 列出用户有权限的服务器
// ListServerByUser 根据用户权限获取服务器列表
const (
	SERVER_LIST_USER_ALLOW  = 1
	SERVER_LIST_USER_DENY   = 2
	SERVER_LIST_GROUP_ALLOW = 3
	SERVER_LIST_GROUP_DENY  = 4
)

func (um *DaoManager) ListServerByUserPermission(userID uint64) (map[int][]uint, error) {
	var response map[int][]uint
	response = make(map[int][]uint)
	var serverMapUserAllow, serverMapUserDeny, serverMapGroupAllow, serverMapGroupDeny map[uint]bool
	serverMapUserAllow = make(map[uint]bool)
	serverMapUserDeny = make(map[uint]bool)
	serverMapGroupAllow = make(map[uint]bool)
	serverMapGroupDeny = make(map[uint]bool)

	// 先列出用户权限 select * from server_permissions where obj_type=“models.SERVER_PERM_OBJ_TYPE_USER” and obj_id=用户id;
	var serverPermissionList []*models.ServerPermission
	err := um.DB.Where("obj_type= ? and obj_id = ?", models.SERVER_PERM_OBJ_TYPE_USER, userID).Find(&serverPermissionList).Error
	if err != nil {
		return nil, err
	}
	// 收集action是allow的结果，并删除action是deny的结果
	for i := 0; i < len(serverPermissionList); i++ {
		if serverPermissionList[i].Action == models.SERVER_PERM_ACTION_PERMIT {
			serverMapUserAllow[serverPermissionList[i].ServerID] = true
		} else if serverPermissionList[i].Action == models.SERVER_PERM_ACTION_DENY {
			serverMapUserDeny[serverPermissionList[i].ServerID] = true
		}
	}
	// 查询用户所属用户组的服务器权限
	// SELECT * FROM server_permissions WHERE obj_id in (SELECT group_id FROM user_groups where user_id=1) and obj_type=2;
	um.DB.Where("obj_id in (?) and obj_type=?", um.DB.Table("user_groups").Where("user_id = ? ", userID).Select("group_id"), models.SERVER_PERM_OBJ_TYPE_GROUP).Find(&serverPermissionList)
	for i := 0; i < len(serverPermissionList); i++ {
		if serverPermissionList[i].Action == models.SERVER_PERM_ACTION_PERMIT {
			serverMapGroupAllow[serverPermissionList[i].ServerID] = true
		} else {
			serverMapGroupDeny[serverPermissionList[i].ServerID] = true
		}
	}

	for k, _ := range serverMapUserAllow {
		response[SERVER_LIST_USER_ALLOW] = append(response[SERVER_LIST_USER_ALLOW], k)
	}
	for k, _ := range serverMapGroupAllow {
		response[SERVER_LIST_GROUP_ALLOW] = append(response[SERVER_LIST_GROUP_ALLOW], k)
	}
	for k, _ := range serverMapUserDeny {
		response[SERVER_LIST_USER_DENY] = append(response[SERVER_LIST_USER_DENY], k)
	}
	for k, _ := range serverMapGroupDeny {
		response[SERVER_LIST_GROUP_DENY] = append(response[SERVER_LIST_GROUP_DENY], k)
	}
	//log.Println(response)
	return response, nil
}

// 保存用户添加的acl
func (um *DaoManager) SaveAddedACL(aclList []*models.AddedServerACLRecord) error {
	return um.DB.Create(aclList).Error
}

func (um *DaoManager) DeleteAddedACLByIP(ipAddr string, serverID uint) error {
	return um.DB.Where("virtual_ip_addr = ? and server_id = ?", ipAddr, serverID).Delete(&models.AddedServerACLRecord{}).Error
}
func (um *DaoManager) DeleteAddedACLByServerID(serverID uint) error {
	return um.DB.Where("server_id = ?", serverID).Delete(&models.AddedServerACLRecord{}).Error
}

func (um *DaoManager) ListAddedACLByIP(ipAddr string, serverID uint) ([]*models.AddedServerACLRecord, error) {
	var aclList []*models.AddedServerACLRecord
	err := um.DB.Where("virtual_ip_addr = ? and server_id = ?", ipAddr, serverID).Find(&aclList).Error
	return aclList, err
}
func (um *DaoManager) ListAddedACLByServerID(serverID uint) ([]*models.AddedServerACLRecord, error) {
	var aclList []*models.AddedServerACLRecord
	err := um.DB.Where("server_id = ?", serverID).Find(&aclList).Error
	return aclList, err
}

// 获取用户可以访问的ACL
func (um *DaoManager) GetACLByUser(userID uint, serverID uint) ([]*models.GroupACL, error) {
	// 检查有没有用户权限
	var serverPermissionList []*models.ServerPermission
	var groupACLList []*models.GroupACL
	err := um.DB.Where("obj_id = ? and obj_type = ? and server_id=?", userID, models.SERVER_PERM_OBJ_TYPE_USER, serverID).Find(&serverPermissionList).Error
	if err == nil {
		userAllow := false
		for _, serverPermission := range serverPermissionList {
			if serverPermission.Action == models.SERVER_PERM_ACTION_DENY {
				// 用户拒绝访问服务器，不返回任何权限
				return nil, nil
			}
			if serverPermission.Action == models.SERVER_PERM_ACTION_PERMIT {
				userAllow = true
			}
		}
		if userAllow {
			// 如果用户允许访问这个服务器，返回他所属组的所有ACL
			groupACLList, err = um.GetAllGroupACLByUser(userID)
			return groupACLList, err
		}
	}

	// 没有指定用户权限，返回服务器权限记录里的所属组的acl，被拒绝的用户组除外
	groupList, err := um.ListGroupsForUser(userID)

	var groupIDList []uint
	for _, group := range groupList {
		groupIDList = append(groupIDList, group.ID)
	}
	//serverPermissionList = nil
	if len(groupIDList) == 0 {
		return nil, nil
	}
	//log.Println("group id list", groupIDList)
	err = um.DB.Where("obj_id in ? and obj_type = ? and server_id=?", groupIDList, models.SERVER_PERM_OBJ_TYPE_GROUP, serverID).Find(&serverPermissionList).Error
	if err != nil {
		return nil, fmt.Errorf("根据用户组查询权限记录失败: %s", err.Error())
	}
	var groupIDMap map[uint]bool
	groupIDMap = make(map[uint]bool)
	for _, serverPermission := range serverPermissionList {
		//log.Println("server perm ", serverPermission)
		if serverPermission.Action == models.SERVER_PERM_ACTION_DENY {
			groupIDMap[serverPermission.ObjID] = false
		}
		if serverPermission.Action == models.SERVER_PERM_ACTION_PERMIT {
			perm, ok := groupIDMap[serverPermission.ObjID]
			if ok {
				if perm == false {
					continue
				}
			}
			groupIDMap[serverPermission.ObjID] = true
		}
	}
	groupACLList = nil
	groupIDList = nil
	for groupID, perm := range groupIDMap {
		if perm == false {
			continue
		}
		groupIDList = append(groupIDList, groupID)
	}
	groupACLList, err = um.ListGroupACLBluk(groupIDList)
	//log.Println("acl list", groupACLList)
	if err != nil {
		return nil, fmt.Errorf("根据用户组ID列表查询ACL记录失败: %s", err.Error())
	}
	return groupACLList, err
}

func (um *DaoManager) CreateConnectedClientInfoRecord(info *models.ConnectedClientInfoRecord) error {
	return um.DB.Create(info).Error
}

func (um *DaoManager) DeleteConnectedClientInfoRecord(virtualIPAddr string, serverID uint) error {

	result := um.DB.Where("virtual_ip_addr = ? and server_id = ?", virtualIPAddr, serverID).Delete(&models.ConnectedClientInfoRecord{})
	return result.Error
}

func (um *DaoManager) DeleteConnectedClientInfoRecordByServerID(serverID uint) error {

	result := um.DB.Where("server_id = ?", serverID).Delete(&models.ConnectedClientInfoRecord{})
	return result.Error
}

func (um *DaoManager) ListConnectedClientInfoRecordByServerID(serverID uint) ([]*models.ConnectedClientInfoRecord, error) {
	var infoList []*models.ConnectedClientInfoRecord
	err := um.DB.Where("server_id = ?", serverID).Find(&infoList).Error
	return infoList, err
}

func (um *DaoManager) ListConnectedClientInfoRecord() ([]*models.ConnectedClientInfoRecord, error) {
	var infoList []*models.ConnectedClientInfoRecord
	err := um.DB.Find(&infoList).Error
	return infoList, err
}

func (um *DaoManager) UpdateConnectedClientInfoRecordTraffic(serverID uint, virtualIPAddr string, byteReceived uint64, byteSent uint64) error {
	result := um.DB.Model(&models.ConnectedClientInfoRecord{}).
		Where("server_id = ? and virtual_ip_addr = ?", serverID, virtualIPAddr).
		Updates(map[string]interface{}{
			"byte_received": byteReceived,
			"byte_sent":     byteSent,
		})
	return result.Error
}
