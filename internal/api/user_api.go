package api

import (
	"openvpn-pannel/internal/models"
	"strconv"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

type UserListItem struct {
	ID                       uint   `json:"id"`
	Username                 string `json:"username"`
	Description              string `json:"description"`
	UploadTraffic            uint64 `json:"upload_traffic"`
	DownloadTraffic          uint64 `json:"download_traffic"`
	ConnectedUploadTraffic   uint64 `json:"connected_upload_traffic"`
	ConnectedDownloadTraffic uint64 `json:"connected_download_traffic"`
	RateLimitType            uint   `json:"rate_limit_type"`
	UploadLimitKB            uint64 `json:"upload_limit_kb"`
	DownloadLimitKB          uint64 `json:"download_limit_kb"`
}

// 校验用户限速策略参数
func validateRateLimit(rateLimitType uint) bool {
	return rateLimitType <= uint(models.RATE_LIMIT_TYPE_FIXED)
}

// 用户登录接口
func (a *App) UserLoginHandler(c *gin.Context) {
	type Param struct {
		Username string `json:"username" binding:"required,max=50"`
		Password string `json:"password" binding:"required,max=100"`
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
	user, err := a.daoManager.AuthUser(param.Username, param.Password)
	if err != nil {
		c.JSON(401, gin.H{
			"result": "failed",
			"error":  "authentication failed",
		})
		return
	}
	session := sessions.Default(c)
	session.Set("user_id", user.ID)
	session.Save()
	c.JSON(200, gin.H{
		"username":    user.Username,
		"description": user.Description,
		"result":      "success",
		"error":       nil,
	})
}

// 用户登出接口
func (a *App) UserLogoutHandler(c *gin.Context) {
	session := sessions.Default(c)
	session.Clear()
	session.Save()
	c.JSON(200, gin.H{
		"result": "success",
		"error":  nil,
	})
}

// 创建用户接口
func (a *App) CreateUserHandler(c *gin.Context, user *models.User) {
	// 只有admin组的用户可以创建用户
	session := sessions.Default(c)
	userID := session.Get("user_id")
	if userID == nil {
		c.JSON(401, gin.H{
			"result": "failed",
			"error":  "unauthorized",
		})
		return
	}
	group, err := a.daoManager.GetGroupByName("admin")
	if err != nil {
		c.JSON(500, gin.H{
			"result": "failed",
			"error":  err.Error(),
		})
		return
	}
	isInGroup, err := a.daoManager.UserInGroup(userID.(uint), group.ID)
	if err != nil || !isInGroup {
		c.JSON(403, gin.H{
			"result": "failed",
			"error":  "forbidden",
		})
		return
	}
	type Param struct {
		Username        string `json:"username" binding:"required,min=1,max=50"`
		Password        string `json:"password" binding:"required,min=1,max=100"`
		Description     string `json:"description"`
		RateLimitType   *uint  `json:"rate_limit_type"`
		UploadLimitKB   uint64 `json:"upload_limit_kb"`
		DownloadLimitKB uint64 `json:"download_limit_kb"`
	}
	var param Param
	err = c.ShouldBindJSON(&param)
	if err != nil {
		c.JSON(400, gin.H{
			"result": "failed",
			"error":  err.Error(),
		})
		return
	}
	// 未显式传 rate_limit_type 时使用默认策略：依据活跃用户组的最低速率
	rateLimitType := uint(models.RATE_LIMIT_TYPE_ACTIVE_GROUP_MIN)
	if param.RateLimitType != nil {
		rateLimitType = *param.RateLimitType
	}
	if !validateRateLimit(rateLimitType) {
		c.JSON(400, gin.H{
			"result": "failed",
			"error":  "rate_limit_type 取值非法，只能是 0(不限速)/1(活跃组最低)/2(活跃组最高)/3(固定限速)",
		})
		return
	}
	err = a.daoManager.CreateUser(param.Username, param.Password, param.Description,
		rateLimitType, param.UploadLimitKB, param.DownloadLimitKB)
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
	})
}

// 修改用户信息
func (a *App) UpdateUserInfoHandler(c *gin.Context, user *models.User) {
	type Param struct {
		Description     string `json:"description" binding:"max=16384"`
		Password        string `json:"password"`
		Username        string `json:"username" binding:"required,max=50"`
		RateLimitType   uint   `json:"rate_limit_type"`
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
	if !validateRateLimit(param.RateLimitType) {
		c.JSON(400, gin.H{
			"result": "failed",
			"error":  "rate_limit_type 取值非法，只能是 0(不限速)/1(活跃组最低)/2(活跃组最高)/3(固定限速)",
		})
		return
	}
	targetUserID := user.ID
	isAdmin := false
	// 默认修改自己的信息
	// 如果是admin组用户，可以修改其他用户信息
	group, err := a.daoManager.GetGroupByName("admin")
	if err == nil {
		isInGroup, err := a.daoManager.UserInGroup(user.ID, group.ID)
		if err == nil && isInGroup {
			isAdmin = true
			if param.Username != "" {
				// admin用户且指定了用户名，修改指定用户信息
				targetUser, err := a.daoManager.GetUserByUsername(param.Username)
				if err == nil {
					targetUserID = targetUser.ID
				}
			}
		} else if !isInGroup && param.Username != "" {
			// 非admin用户不能修改其他用户信息
			c.JSON(403, gin.H{
				"result": "failed",
				"error":  "forbidden",
			})
			return
		}
	}
	// 非管理员修改自己的资料时，不允许改动限速策略，沿用原值
	if !isAdmin {
		currentUser, err := a.daoManager.GetUserByID(targetUserID)
		if err != nil {
			c.JSON(500, gin.H{
				"result": "failed",
				"error":  err.Error(),
			})
			return
		}
		param.RateLimitType = currentUser.RateLimitType
		param.UploadLimitKB = currentUser.UploadLimitKB
		param.DownloadLimitKB = currentUser.DownloadLimitKB
	}
	err = a.daoManager.UpdateUserInfo(targetUserID, param.Description, param.Password,
		param.RateLimitType, param.UploadLimitKB, param.DownloadLimitKB)
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
	})
}

// 获取用户信息接口
func (a *App) GetUserInfoHandler(c *gin.Context, user *models.User) {
	_username := c.DefaultQuery("username", "")
	var user_queryed *models.User
	if _username != "" {
		var err error
		user_queryed, err = a.daoManager.GetUserByUsername(_username)
		if err != nil {
			c.JSON(400, gin.H{
				"result": "failed",
				"error":  "用户不存在",
			})
			return
		}
	} else {
		user_queryed = user
	}
	// 查询用户所在用户组
	groupList, err := a.daoManager.ListGroupsForUser(user_queryed.ID)
	if err != nil {
		c.JSON(500, gin.H{
			"result": "failed",
			"error":  err.Error(),
		})
		return
	}
	// 如果用户不在admin组里且user与user_queryed不同，则返回403
	if user.ID != user_queryed.ID {
		group, err := a.daoManager.GetGroupByName("admin")
		if err != nil {
			c.JSON(500, gin.H{
				"result": "failed",
				"error":  err.Error(),
			})
			return
		}
		isInGroup, err := a.daoManager.UserInGroup(user.ID, group.ID)
		if !isInGroup {
			c.JSON(403, gin.H{
				"result": "failed",
				"error":  "forbidden",
			})
			return
		}
	}
	c.JSON(200, gin.H{
		"username":          user.Username,
		"description":       user.Description,
		"error":             nil,
		"groups":            groupList,
		"rate_limit_type":   user_queryed.RateLimitType,
		"upload_limit_kb":   user_queryed.UploadLimitKB,
		"download_limit_kb": user_queryed.DownloadLimitKB,
		"result":            "success",
	})
}

// 列出用户接口
func (a *App) ListUserHandler(c *gin.Context, user *models.User) {
	_page := c.DefaultQuery("page", "1")
	_pageSize := c.DefaultQuery("page_size", "20")
	_excludeGroupID := c.DefaultQuery("exclude_group_id", "0")
	_excludePlanID := c.DefaultQuery("exclude_plan_id", "0")
	queryName := c.DefaultQuery("query", "")
	var page, pageSize, excludeGroupID, excludePlanID int64
	var err error
	var err1, err2, err3 error
	page, err = strconv.ParseInt(_page, 10, 64)
	pageSize, err1 = strconv.ParseInt(_pageSize, 10, 64)
	excludeGroupID, err2 = strconv.ParseInt(_excludeGroupID, 10, 64)
	excludePlanID, err3 = strconv.ParseInt(_excludePlanID, 10, 64)
	if err != nil || err1 != nil || err2 != nil || err3 != nil {
		c.JSON(400, gin.H{
			"result": "failed",
			"error":  "page和page_size参数必须是整数",
		})
		return
	}
	// 查询用户所在用户组
	userList, err := a.daoManager.ListUsers(int(page), int(pageSize), queryName, uint(excludeGroupID), uint(excludePlanID))
	if err != nil {
		c.JSON(500, gin.H{
			"result": "failed",
			"error":  err.Error(),
		})
		return
	}
	userCount, err := a.daoManager.GetUserCount(queryName, uint(excludeGroupID), uint(excludePlanID))
	if err != nil {
		c.JSON(500, gin.H{
			"result": "failed",
			"error":  err.Error(),
		})
		return
	}

	connectedRecords, err := a.daoManager.ListConnectedClientInfoRecord()
	connectedTrafficMap := make(map[string]struct {
		UploadTraffic   uint64
		DownloadTraffic uint64
	})
	if err == nil {
		for _, record := range connectedRecords {
			if record.Username == "" {
				continue
			}
			data := connectedTrafficMap[record.Username]
			data.UploadTraffic += record.ByteSent
			data.DownloadTraffic += record.ByteReceived
			connectedTrafficMap[record.Username] = data
		}
	}

	responseList := make([]*UserListItem, 0, len(userList))
	for _, u := range userList {
		connectedData := connectedTrafficMap[u.Username]
		responseList = append(responseList, &UserListItem{
			ID:                       u.ID,
			Username:                 u.Username,
			Description:              u.Description,
			UploadTraffic:            u.UploadTraffic,
			DownloadTraffic:          u.DownloadTraffic,
			ConnectedUploadTraffic:   connectedData.UploadTraffic,
			ConnectedDownloadTraffic: connectedData.DownloadTraffic,
			RateLimitType:            u.RateLimitType,
			UploadLimitKB:            u.UploadLimitKB,
			DownloadLimitKB:          u.DownloadLimitKB,
		})
	}

	c.JSON(200, gin.H{
		"error":  nil,
		"data":   responseList,
		"count":  userCount,
		"result": "success",
	})
}

func (a *App) DeleteUserHandler(c *gin.Context, user *models.User) {
	type Param struct {
		UserIDList []uint `json:"id_list"`
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
	err = a.daoManager.BlukDeleteUser(param.UserIDList)
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
	})
}

func (a *App) ResetUserTrafficHandler(c *gin.Context, user *models.User) {
	type Param struct {
		UserID uint `json:"id" binding:"required"`
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
	err = a.daoManager.ResetUserTraffic(param.UserID)
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
	})
}

// 禁用用户接口

// 启用用户接口
