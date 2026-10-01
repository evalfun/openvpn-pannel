package api

import (
	"time"

	"openvpn-pannel/internal/models"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

// getActiveSessionUser 从会话中取出用户，并校验其仍然存在且处于可用状态
// （未被禁用、未过期）。任何一步失败都返回 nil，由调用方统一返回 401。
func (a *App) getActiveSessionUser(c *gin.Context) *models.User {
	session := sessions.Default(c)
	_userID := session.Get("user_id")
	if _userID == nil {
		return nil
	}
	userID, ok := _userID.(uint)
	if !ok {
		return nil
	}
	user, err := a.daoManager.GetUserByID(userID)
	if err != nil {
		return nil
	}
	if err := user.CheckAvailable(time.Now()); err != nil {
		return nil
	}
	return user
}

func (a *App) UserLoginWarper(fun func(c *gin.Context, user *models.User)) func(*gin.Context) {
	return func(c *gin.Context) {
		user := a.getActiveSessionUser(c)
		if user == nil {
			c.JSON(401, gin.H{
				"result": "failed",
				"error":  "unauthorized",
			})
			return
		}
		fun(c, user)
	}
}

func (a *App) AdminLoginWarper(fun func(c *gin.Context, user *models.User)) func(*gin.Context) {
	return func(c *gin.Context) {
		user := a.getActiveSessionUser(c)
		if user == nil {
			c.JSON(401, gin.H{
				"result": "failed",
				"error":  "unauthorized",
			})
			return
		}
		adminGroup, err := a.daoManager.GetGroupByName("admin")
		if err != nil {
			c.JSON(403, gin.H{
				"result": "failed",
				"error":  "forbidden",
			})
			return
		}
		isAdmin, err := a.daoManager.UserInGroup(user.ID, adminGroup.ID)
		if err != nil || !isAdmin {
			c.JSON(403, gin.H{
				"result": "failed",
				"error":  "forbidden",
			})
			return
		}
		fun(c, user)
	}
}

func (a *App) GetBuildDate(c *gin.Context, user *models.User) {
	// 获取编译日期
	c.String(200, a.buildDate)
}
