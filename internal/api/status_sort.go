package api

import (
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

// parseConnectedSince 解析 OpenVPN 状态输出里的 “Connected Since” 时间字符串。
// 管理接口 `status`（v1）与状态文件（openvpn-status.log）都使用 asctime 风格，
// 例如 "Mon Sep 28 10:00:00 2026"（个位数日期为空格填充，如 "Mon Sep  8 ..."）。
// 解析成功返回 Unix 秒；为空或无法解析时返回 0。
func parseConnectedSince(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	// 兼容空格填充/不填充/零填充三种日格式。
	layouts := []string{
		"Mon Jan _2 15:04:05 2006",
		"Mon Jan 2 15:04:05 2006",
		"Mon Jan 02 15:04:05 2006",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t.Unix()
		}
	}
	// 兜底：整个字符串本身就是 Unix 秒（某些场景可能直接给出 time_t）。
	if v, err := strconv.ParseInt(s, 10, 64); err == nil && v > 0 {
		return v
	}
	return 0
}

// sortClientListByConnectedSince 按连接时间升序排序（最早连接的在前）。
// 无法解析出连接时间的条目统一排在最后，且保持其相对顺序（稳定排序）。
func sortClientListByConnectedSince(list []*ServerStatusClientInfoResponse) {
	if len(list) < 2 {
		return
	}
	// 预计算时间，避免比较函数中重复解析；用成对结构排序，避免下标错位。
	type timedClient struct {
		t int64
		c *ServerStatusClientInfoResponse
	}
	items := make([]timedClient, len(list))
	for i, c := range list {
		t := parseConnectedSince(c.ConnectedSince)
		if t == 0 {
			// 未知时间排在最后。
			t = math.MaxInt64
		}
		items[i] = timedClient{t: t, c: c}
	}
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].t < items[j].t
	})
	for i := range items {
		list[i] = items[i].c
	}
}
