package api

import (
	"openvpn-pannel/internal/models"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

func (a *App) UserLoginWarper(fun func(c *gin.Context, user *models.User)) func(*gin.Context) {
	return func(c *gin.Context) {
		session := sessions.Default(c)
		_userID := session.Get("user_id")
		if _userID == nil {
			c.JSON(401, gin.H{
				"result": "failed",
				"error":  "unauthorized",
			})
			return
		}
		userID, ok := _userID.(uint)
		if !ok {
			c.JSON(401, gin.H{
				"result": "failed",
				"error":  "unauthorized",
			})
			return
		}
		user, err := a.daoManager.GetUserByID(userID)
		if err != nil {
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
		session := sessions.Default(c)
		_userID := session.Get("user_id")
		if _userID == nil {
			c.JSON(401, gin.H{
				"result": "failed",
				"error":  "unauthorized",
			})
			return
		}
		userID, ok := _userID.(uint)
		if !ok {
			c.JSON(401, gin.H{
				"result": "failed",
				"error":  "unauthorized",
			})
			return
		}
		user, err := a.daoManager.GetUserByID(userID)
		if err != nil {
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
