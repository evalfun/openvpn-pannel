package api

import (
	"openvpn-pannel/internal/models"
	ovpnserver "openvpn-pannel/internal/ovpn_server"

	"github.com/gin-gonic/gin"
)

// 获取配置
func (a *App) GetResourceHandler(c *gin.Context, user *models.User) {
	resourceID := c.DefaultQuery("id", "")
	setID := c.DefaultQuery("set", "")
	if setID == "" {
		setID = a.GetActiveResourceSetID()
	} else if !ovpnserver.IsValidResourceSet(setID) {
		c.JSON(400, gin.H{
			"result": "failed",
			"error":  "获取资源失败: 资源集不存在",
		})
		return
	}
	c.String(200, a.GetResourceContent(setID, resourceID))
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
	setID := param.SetID
	if setID == "" {
		setID = a.GetActiveResourceSetID()
	}
	if !ovpnserver.IsValidResourceSet(setID) {
		c.JSON(400, gin.H{
			"result": "failed",
			"error":  "写入资源失败: 资源集不存在",
		})
		return
	}
	// 确保该资源集已完成种子化，避免出现“只写了一条、其余缺失”的半成品。
	if err := a.EnsureResourceSetSeeded(setID); err != nil {
		c.JSON(500, gin.H{
			"result": "failed",
			"error":  err.Error(),
		})
		return
	}
	err = a.daoManager.WriteResourceBySet(setID, param.ID, param.Content)
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
		SetID      string `json:"set_id"`
	}
	err := c.ShouldBindJSON(&param)
	if err != nil {
		c.JSON(400, gin.H{
			"result": "failed",
			"error":  err.Error(),
		})
		return
	}
	setID := param.SetID
	if setID == "" {
		setID = a.GetActiveResourceSetID()
	}
	if !ovpnserver.IsValidResourceSet(setID) {
		c.JSON(400, gin.H{
			"result": "failed",
			"error":  "重置资源失败: 资源集不存在",
		})
		return
	}
	err = a.daoManager.DeleteResourceBySet(setID, param.ResourceID)
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

// SetActiveResourceSetHandler 切换当前启用的资源集。
func (a *App) SetActiveResourceSetHandler(c *gin.Context, user *models.User) {
	// 切换资源集会影响所有服务器使用的脚本，等价于资源变更，同样需要 allow_edit_resource。
	if !a.cfg.AllowEditResource {
		c.JSON(403, gin.H{
			"result": "failed",
			"error":  "资源编辑已被禁用：如需切换资源集，请在 config.json 中设置 \"allow_edit_resource\": true",
		})
		return
	}
	var param struct {
		SetID string `json:"set_id" binding:"required,max=50"`
	}
	if err := c.ShouldBindJSON(&param); err != nil {
		c.JSON(400, gin.H{
			"result": "failed",
			"error":  err.Error(),
		})
		return
	}
	if !ovpnserver.IsValidResourceSet(param.SetID) {
		c.JSON(400, gin.H{
			"result": "failed",
			"error":  "切换资源集失败: 资源集不存在",
		})
		return
	}
	// 切换前先种子化目标资源集，使其成为可编辑的工作区。
	if err := a.EnsureResourceSetSeeded(param.SetID); err != nil {
		c.JSON(500, gin.H{
			"result": "failed",
			"error":  err.Error(),
		})
		return
	}
	if err := a.daoManager.SetActiveResourceSetID(param.SetID); err != nil {
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

// 列出配置
func (a *App) ListResourceHandler(c *gin.Context, user *models.User) {
	type ResourceResponse struct {
		ID          string `json:"id"`
		Description string `json:"description"`
	}
	type SetResponse struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Description string `json:"description"`
		Active      bool   `json:"active"`
	}
	activeSetID := a.GetActiveResourceSetID()

	setList := []SetResponse{}
	for _, s := range ovpnserver.ListResourceSets() {
		setList = append(setList, SetResponse{
			ID:          s.ID,
			Name:        s.Name,
			Description: s.Description,
			Active:      s.ID == activeSetID,
		})
	}

	resourceList := []ResourceResponse{}
	for _, item := range ovpnserver.ListResourceIDs() {
		resourceList = append(resourceList, ResourceResponse{
			ID:          item.ID,
			Description: item.Description,
		})
	}

	c.JSON(200, gin.H{
		"result": "success",
		"error":  nil,
		// allow_edit 由 config.json 的 allow_edit_resource 决定，前端据此禁用编辑/重置按钮。
		"allow_edit": a.cfg.AllowEditResource,
		"active_set": activeSetID,
		"sets":       setList,
		"data":       resourceList,
	})
}
