package api

import (
	"openvpn-pannel/internal/models"
	ovpnserver "openvpn-pannel/internal/ovpn_server"

	"github.com/gin-gonic/gin"
)

// 获取配置
func (a *App) GetResourceHandler(c *gin.Context, user *models.User) {

	resourceID := c.DefaultQuery("id", "")
	resourceModel, err := a.daoManager.GetResourceByID(resourceID)
	if err == nil {
		c.String(200, resourceModel.Content)
		return
	}
	c.String(200, ovpnserver.GetDefaultResource(resourceID))
}

// 写入配置
func (a *App) WriteResourceHandler(c *gin.Context, user *models.User) {
	// 资源(脚本/配置模板)可被直接执行，改写它们等价于任意代码执行。
	// 必须显式在 config.json 中开启 allow_edit_resource 才允许写入。
	if !a.cfg.AllowEditResource {
		c.JSON(403, gin.H{
			"result": "failed",
			"error":  "资源编辑已被禁用：如需修改或重置脚本资源，请在 config.json 中设置 \"allow_edit_resource\": true",
		})
		return
	}

	var param models.AppResourceRecord
	err := c.ShouldBindJSON(&param)
	if err != nil {
		c.JSON(400, gin.H{
			"result": "failed",
			"error":  err.Error(),
		})
		return
	}
	err = a.daoManager.WriteResource(param.ID, param.Content)
	if err != nil {
		c.JSON(500, gin.H{
			"result": "failed",
			"error":  err.Error(),
		})
	} else {
		c.JSON(200, gin.H{
			"result": "success",
			"error":  nil,
		})
	}

}

// 删除配置
func (a *App) DeleteResourceHandler(c *gin.Context, user *models.User) {
	// 重置资源(删除数据库覆盖后回落内置默认脚本)同样属于脚本资源变更，需显式开启。
	if !a.cfg.AllowEditResource {
		c.JSON(403, gin.H{
			"result": "failed",
			"error":  "资源编辑已被禁用：如需修改或重置脚本资源，请在 config.json 中设置 \"allow_edit_resource\": true",
		})
		return
	}

	var param struct {
		ResourceID string `json:"id"`
	}
	err := c.ShouldBindJSON(&param)
	if err != nil {
		c.JSON(400, gin.H{
			"result": "failed",
			"error":  err.Error(),
		})
		return
	}
	err = a.daoManager.DeleteResource(param.ResourceID)
	if err != nil {
		c.JSON(500, gin.H{
			"result": "failed",
			"error":  err.Error(),
		})
	} else {
		c.JSON(200, gin.H{
			"result": "success",
			"error":  nil,
		})
	}
}

// 列出配置
func (a *App) ListResourceHandler(c *gin.Context, user *models.User) {
	type Response struct {
		ID          string `json:"id"`
		Description string `json:"description"`
	}
	c.JSON(200, gin.H{
		"result": "success",
		"error":  nil,
		// allow_edit 由 config.json 的 allow_edit_resource 决定，前端据此禁用编辑/重置按钮。
		"allow_edit": a.cfg.AllowEditResource,
		"data": []Response{
			{
				ID:          ovpnserver.RESOURCE_ID_AUTH_SCRIPT,
				Description: "客户端认证脚本内容",
			},
			{
				ID:          ovpnserver.RESOURCE_ID_CLIENT_OFFLINE_SCRIPT,
				Description: "客户端下线脚本内容",
			},
			{
				ID:          ovpnserver.RESOURCE_ID_CLIENT_ONLINE_SCRIPT,
				Description: "客户端上线脚本内容",
			},
			{
				ID:          ovpnserver.RESOURCE_ID_RATE_LIMIT_SCRIPT,
				Description: "达量限速运行时更新脚本内容",
			},
			{
				ID:          ovpnserver.RESOURCE_ID_ACL_ADD_SCRIPT,
				Description: "TOTP 验证通过后动态放行用户 ACL 的脚本内容",
			},
			{
				ID:          ovpnserver.RESOURCE_ID_ACL_DEL_SCRIPT,
				Description: "登出后动态回收用户 ACL 的脚本内容",
			},
			{
				ID:          ovpnserver.RESOURCE_ID_CONFIG_TEMPLATE,
				Description: "服务器配置模板内容",
			},
			{
				ID:          ovpnserver.RESOURCE_ID_CLIENT_CONFIG,
				Description: "客户端配置文件模板内容",
			},
			{
				ID:          ovpnserver.RESOURCE_ID_MISC_CONFIG,
				Description: "其他配置内容",
			},
			{
				ID:          ovpnserver.RESOURCE_ID_SERVER_EXIT_SCRIPT,
				Description: "服务端实例停止脚本内容",
			},
			{
				ID:          ovpnserver.RESOURCE_ID_SERVER_START_SCRIPT,
				Description: "服务端实例启动脚本内容",
			},
			{
				ID:          ovpnserver.RESOURCE_ID_HELP_INFO,
				Description: "帮助信息文档内容",
			},
		},
	})

}
