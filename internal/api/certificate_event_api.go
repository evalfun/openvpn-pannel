package api

import (
	"strconv"
	"strings"

	"openvpn-pannel/internal/models"

	"github.com/gin-gonic/gin"
)

// ListCertificateEventHandler 列出证书操作事件。
// 参数：type（下划线分隔的事件类型）、query（关键字）、start/end（时间范围）、page、page_size。
func (a *App) ListCertificateEventHandler(c *gin.Context, user *models.User) {
	var typeList []int
	for _, part := range strings.Split(c.DefaultQuery("type", ""), "_") {
		if part == "" {
			continue
		}
		if v, err := strconv.Atoi(part); err == nil {
			typeList = append(typeList, v)
		}
	}
	query := strings.TrimSpace(c.DefaultQuery("query", ""))
	startTime, _ := strconv.ParseInt(c.DefaultQuery("start", "0"), 10, 64)
	endTime, _ := strconv.ParseInt(c.DefaultQuery("end", "0"), 10, 64)
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	resp, err := a.daoManager.GetCertificateEventList(typeList, query, startTime, endTime, page, pageSize)
	if err != nil {
		c.JSON(500, gin.H{"result": "failed", "error": "列出证书事件失败: " + err.Error()})
		return
	}
	c.JSON(200, gin.H{
		"result": "success",
		"error":  nil,
		"data":   resp.EventList,
		"total":  resp.Total,
	})
}

// ClearCertificateEventHandler 清空所有证书事件。
func (a *App) ClearCertificateEventHandler(c *gin.Context, user *models.User) {
	if err := a.daoManager.ClearCertificateEvent(); err != nil {
		c.JSON(500, gin.H{"result": "failed", "error": "清空证书事件失败: " + err.Error()})
		return
	}
	c.JSON(200, gin.H{"result": "success", "error": nil})
}
