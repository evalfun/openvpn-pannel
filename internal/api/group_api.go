package api

import (
	"net"
	"openvpn-pannel/internal/models"
	"strconv"

	"github.com/gin-gonic/gin"
)

// 创建用户组
func (a *App) CreateGroupHandler(c *gin.Context, user *models.User) {

	type Param struct {
		Name            string `json:"name" binding:"required,min=1,max=50"`
		Description     string `json:"description" binding:"max=32767"`
		UploadLimitKB   uint64 `json:"upload_limit_kb"`
		DownloadLimitKB uint64 `json:"download_limit_kb"`
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
	if param.Name == "" {
		c.JSON(400, gin.H{
			"result": "failed",
			"error":  "name不能为空",
		})
		return
	}
	err = a.daoManager.CreateGroup(param.Name, param.Description, param.UploadLimitKB, param.DownloadLimitKB)
	if err != nil {
		c.JSON(400, gin.H{
			"result": "failed",
			"error":  err.Error(),
		})
		return
	}

	c.JSON(200, gin.H{
		"result": "success",
		"error":  nil,
	})
}

// 列出用户组
func (a *App) ListGroupsHandler(c *gin.Context, user *models.User) {
	_page := c.DefaultQuery("page", "1")
	_pageSize := c.DefaultQuery("page_size", "20")
	query := c.DefaultQuery("query", "")
	var page, pageSize int64
	var err error
	var err1 error
	page, err = strconv.ParseInt(_page, 10, 64)
	pageSize, err1 = strconv.ParseInt(_pageSize, 10, 64)
	if err != nil || err1 != nil {
		c.JSON(400, gin.H{
			"result": "failed",
			"error":  "page和page_size参数必须是整数",
		})
		return
	}
	groupList, totalCount, err := a.daoManager.ListGroups(query, int(page), int(pageSize))
	if err != nil {
		c.JSON(400, gin.H{"result": "failed",
			"error": "列出用户组失败 " + err.Error(),
		})
		return
	}
	c.JSON(200, gin.H{
		"count":  totalCount,
		"data":   groupList,
		"result": "success",
		"error":  nil,
	})
}

// 列出用户组内的用户
func (a *App) ListUsersInGroupHandler(c *gin.Context, user *models.User) {
	groupName := c.DefaultQuery("group", "")
	if groupName == "" {
		c.JSON(400, gin.H{"result": "failed",
			"error": "需要填写group参数",
		})
		return
	}
	group, err := a.daoManager.GetGroupByName(groupName)
	if err != nil {
		c.JSON(400, gin.H{"result": "failed",
			"error": "用户组不存在",
		})
		return
	}
	// page_size 缺省或 <=0 时返回全部（兼容旧调用），否则分页返回。
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "0"))
	query := c.DefaultQuery("query", "")
	if pageSize <= 0 {
		userList, err := a.daoManager.ListUsersInGroup(group.ID)
		if err != nil {
			c.JSON(400, gin.H{"result": "failed",
				"error": "列出用户组失败 " + err.Error(),
			})
			return
		}
		c.JSON(200, gin.H{
			"data":  userList,
			"count": len(userList),
		})
		return
	}
	userList, count, err := a.daoManager.ListUsersInGroupPaged(group.ID, query, page, pageSize)
	if err != nil {
		c.JSON(400, gin.H{"result": "failed",
			"error": "列出用户组失败 " + err.Error(),
		})
		return
	}
	c.JSON(200, gin.H{
		"data":  userList,
		"count": count,
	})
}

// 删除用户组
func (a *App) DeleteGroupHandler(c *gin.Context, user *models.User) {

	type Param struct {
		Name string `json:"name"  binding:"required,max=100"`
	}
	var param Param
	err := c.ShouldBindJSON(&param)
	if err != nil {
		c.JSON(400, gin.H{"result": "failed",
			"error": err.Error(),
		})
		return
	}
	group, err := a.daoManager.GetGroupByName(param.Name)
	if err != nil {
		c.JSON(400, gin.H{"result": "failed",
			"error": "用户组不存在",
		})
		return
	}
	err = a.daoManager.DeleteGroup(group.ID)
	if err != nil {
		c.JSON(400, gin.H{"result": "failed",
			"error": err.Error(),
		})
		return
	}
	c.JSON(200, gin.H{
		"result": "success",
		"error":  nil,
	})
}

// 批量添加用户到用户组
func (a *App) BatchAddUsersToGroupHandler(c *gin.Context, user *models.User) {
	type Param struct {
		UserNameList []string `json:"users"  binding:"required,min=1,dive,max=100" `
		GroupName    string   `json:"group"  binding:"required,min=1,max=100"`
	}
	var param Param
	err := c.ShouldBindJSON(&param)
	if err != nil {
		c.JSON(400, gin.H{"result": "failed",
			"error": err.Error(),
		})
		return
	}
	err = a.daoManager.BatchAddUsersToGroup(param.UserNameList, param.GroupName)
	if err != nil {
		c.JSON(400, gin.H{"result": "failed",
			"error": "批量添加用户到用户组失败 " + err.Error(),
		})
		return
	}
	c.JSON(200, gin.H{
		"result": "success",
		"error":  nil,
	})
}

// 添加用户到用户组
func (a *App) AddUserToGroupHandler(c *gin.Context, u *models.User) {

	type Param struct {
		UserName  string `json:"user"  binding:"required,min=1,max=100"`
		GroupName string `json:"group" binding:"required,min=1,max=100"`
	}
	var param Param
	err := c.ShouldBindJSON(&param)
	if err != nil {
		c.JSON(400, gin.H{"result": "failed",
			"error": err.Error(),
		})
		return
	}
	user, err := a.daoManager.GetUserByUsername(param.UserName)
	if err != nil {
		c.JSON(400, gin.H{"result": "failed",
			"error": "用户不存在",
		})
		return
	}
	group, err := a.daoManager.GetGroupByName(param.GroupName)
	if err != nil {
		c.JSON(400, gin.H{"result": "failed",
			"error": "用户组不存在",
		})
		return
	}
	err = a.daoManager.AddUserToGroup(user.ID, group.ID)
	if err != nil {
		c.JSON(400, gin.H{"result": "failed",
			"error": "添加用户到用户组失败 " + err.Error(),
		})
		return
	}
	c.JSON(200, gin.H{
		"result": "success",
		"error":  nil,
	})
}

// 添加ACL
func (a *App) AddGroupACLHandler(c *gin.Context, u *models.User) {

	type Param struct {
		GroupName string `json:"group"  binding:"required,min=1,max=100"`
		ACLType   uint   `json:"type"  binding:"required"`
		ACLValue  string `json:"value"  binding:"required,min=1,max=1000"`
	}
	var param Param
	err := c.ShouldBindJSON(&param)
	if err != nil {
		c.JSON(400, gin.H{"result": "failed",
			"error": err.Error(),
		})
		return
	}
	group, err := a.daoManager.GetGroupByName(param.GroupName)
	if err != nil {
		c.JSON(400, gin.H{"result": "failed",
			"error": "用户组不存在",
		})
		return
	}
	// ACL类型只能是4(IPv4)或6(IPv6)
	if param.ACLType != 4 && param.ACLType != 6 {
		c.JSON(400, gin.H{"result": "failed",
			"error": "ACL类型只能是4(IPv4)或6(IPv6)",
		})
		return
	}
	// 校验CIDR格式并自动清除主机位
	ip, ipnet, err := net.ParseCIDR(param.ACLValue)
	if err != nil {
		c.JSON(400, gin.H{"result": "failed",
			"error": "ACL值必须是合法的CIDR格式，例如 10.0.0.0/24 或 fc00::/64",
		})
		return
	}
	isIPv4 := ip.To4() != nil
	if param.ACLType == 4 && !isIPv4 {
		c.JSON(400, gin.H{"result": "failed",
			"error": "ACL类型4要求IPv4地址，例如 10.0.0.0/24",
		})
		return
	}
	if param.ACLType == 6 && isIPv4 {
		c.JSON(400, gin.H{"result": "failed",
			"error": "ACL类型6要求IPv6地址，例如 fc00::/64",
		})
		return
	}
	// ipnet.String() 返回网络地址，主机位已被清零
	param.ACLValue = ipnet.String()
	err = a.daoManager.AddGroupACL(group.ID, param.ACLType, param.ACLValue)
	if err != nil {
		c.JSON(400, gin.H{"result": "failed",
			"error": "添加用户组ACL失败 " + err.Error(),
		})
		return
	}
	c.JSON(200, gin.H{
		"result": "success",
		"error":  nil,
	})

}

// 删除ACL
func (a *App) DeleteGroupACLHandler(c *gin.Context, u *models.User) {

	type Param struct {
		ACLID uint `json:"acl_id"`
	}
	var param Param
	err := c.ShouldBindJSON(&param)
	if err != nil {
		c.JSON(400, gin.H{"result": "failed",
			"error": err.Error(),
		})
		return
	}
	err = a.daoManager.DeleteGroupACL(param.ACLID)
	if err != nil {
		c.JSON(400, gin.H{"result": "failed",
			"error": "删除用户组ACL失败 " + err.Error(),
		})
		return
	}
	c.JSON(200, gin.H{
		"result": "success",
		"error":  nil,
	})
}

// 列出用户组的ACL
func (a *App) ListGroupACLHandler(c *gin.Context, u *models.User) {
	groupName := c.DefaultQuery("group", "")
	if groupName == "" {
		c.JSON(400, gin.H{"result": "failed",
			"error": "需要group参数",
		})
		return
	}
	group, err := a.daoManager.GetGroupByName(groupName)
	if err != nil {
		c.JSON(400, gin.H{"result": "failed",
			"error": "用户组不存在",
		})
		return
	}
	groupACLList, err := a.daoManager.ListGroupACL(group.ID)
	if err != nil {
		c.JSON(400, gin.H{"result": "failed",
			"error": "列出用户组ACL失败 " + err.Error(),
		})
		return
	}
	c.JSON(200, gin.H{"result": "success",
		"error": nil,
		"data":  groupACLList,
	})

}

// 删除用户组中的用户
func (a *App) RemoveUserFromGroupHandler(c *gin.Context, u *models.User) {

	type Param struct {
		UserName  string `json:"user"  binding:"required,min=1,max=100"`
		GroupName string `json:"group"  binding:"required,min=1,max=100"`
	}
	var param Param
	err := c.ShouldBindJSON(&param)
	if err != nil {
		c.JSON(400, gin.H{"result": "failed",
			"error": err.Error(),
		})
		return
	}
	user, err := a.daoManager.GetUserByUsername(param.UserName)
	if err != nil {
		c.JSON(400, gin.H{"result": "failed",
			"error": "用户不存在",
		})
		return
	}
	group, err := a.daoManager.GetGroupByName(param.GroupName)
	if err != nil {
		c.JSON(400, gin.H{"result": "failed",
			"error": "用户组不存在",
		})
		return
	}
	a.daoManager.RemoveUserFromGroup(user.ID, group.ID)
	c.JSON(200, gin.H{
		"result": "success",
		"error":  nil,
	})
}

// 更新用户组
func (a *App) UpdateGroupHandler(c *gin.Context, u *models.User) {
	type Param struct {
		ID              uint   `json:"id" `
		Desc            string `json:"desc"  binding:"max=32767"`
		UploadLimitKB   uint64 `json:"upload_limit_kb"`
		DownloadLimitKB uint64 `json:"download_limit_kb"`
	}
	var param Param
	err := c.ShouldBindJSON(&param)
	if err != nil {
		c.JSON(400, gin.H{"result": "failed",
			"error": err.Error(),
		})
		return
	}
	err = a.daoManager.UpdateGroup(param.ID, param.Desc, param.UploadLimitKB, param.DownloadLimitKB)
	if err != nil {
		c.JSON(400, gin.H{"result": "failed",
			"error": "更新用户组失败 " + err.Error(),
		})
		return
	}
	c.JSON(200, gin.H{
		"result": "success",
		"error":  nil,
	})

}
