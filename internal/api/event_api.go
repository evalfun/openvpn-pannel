package api

import (
	"openvpn-pannel/internal/models"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

func (a *App) GetEventList(c *gin.Context, user *models.User) {
	_serverID := c.DefaultQuery("id", "")
	serverID, err := strconv.ParseUint(_serverID, 10, 64)
	if err != nil {
		c.JSON(400, gin.H{
			"result": "failed",
			"error":  "列出事件失败: 缺少参数id",
		})
		return
	}
	eventTypeListStr := c.DefaultQuery("type", "")
	// 用下划线分割 例如 1_2_3
	_eventTypeList := strings.Split(eventTypeListStr, "_")
	var eventTypeList []int
	for _, eventType := range _eventTypeList {
		eventTypeInt, err := strconv.ParseUint(eventType, 10, 64)
		if err != nil {
			continue
		}
		eventTypeList = append(eventTypeList, int(eventTypeInt))
	}
	_startTime := c.DefaultQuery("start", "0")
	startTime, err := strconv.ParseInt(_startTime, 10, 64)
	if err != nil {
		c.JSON(400, gin.H{
			"result": "failed",
			"error":  "列出事件失败: 缺少参数start",
		})
		return
	}
	_endTime := c.DefaultQuery("end", "0")
	endTime, err := strconv.ParseInt(_endTime, 10, 64)
	if err != nil {
		c.JSON(400, gin.H{
			"result": "failed",
			"error":  "列出事件失败: 缺少参数end",
		})
		return
	}
	ipAddr := c.DefaultQuery("ip", "")
	_page := c.DefaultQuery("page", "1")
	page, err := strconv.ParseInt(_page, 10, 64)
	if err != nil {
		c.JSON(400, gin.H{
			"result": "failed",
			"error":  "列出事件失败: 缺少参数page",
		})
		return
	}
	_pageSize := c.DefaultQuery("page_size", "20")
	pageSize, err := strconv.ParseInt(_pageSize, 10, 64)
	if err != nil {
		c.JSON(400, gin.H{
			"result": "failed",
			"error":  "列出事件失败: 缺少参数page_size",
		})
		return
	}
	eventList, err := a.daoManager.GetEventList(uint(serverID), eventTypeList, ipAddr, startTime, endTime, int(page), int(pageSize))
	if err != nil {
		c.JSON(500, gin.H{
			"result": "failed",
			"error":  "列出事件失败: " + err.Error(),
		})
		return
	}
	c.JSON(200, gin.H{
		"result": "success",
		"error":  nil,
		"data":   eventList.EventList,
		"total":  eventList.Total,
	})
}

// 清空事件
func (a *App) ClearEvent(c *gin.Context, user *models.User) {
	type Param struct {
		ID uint `json:"id"`
	}
	var param Param
	err := c.ShouldBindJSON(&param)
	if err != nil {
		c.JSON(400, gin.H{
			"result": "failed",
			"error":  "清空事件失败: 缺少参数id",
		})
		return
	}
	err = a.daoManager.ClearEvent(param.ID)
	if err != nil {
		c.JSON(500, gin.H{
			"result": "failed",
			"error":  "清空事件失败: " + err.Error(),
		})
		return
	}
	c.JSON(200, gin.H{
		"result": "success",
		"error":  nil,
	})
}
