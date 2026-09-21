package api

import (
	"encoding/csv"
	"fmt"
	"strings"

	"openvpn-pannel/internal/models"

	"github.com/gin-gonic/gin"
)

// batchUserRow 是从 CSV 解析出的单行用户数据。
type batchUserRow struct {
	Line        int
	Username    string
	Password    string
	Description string
	GroupName   string
}

// batchCreateUserResult 是批量创建的单行结果，逐行返回成功/失败与原因。
type batchCreateUserResult struct {
	Line        int    `json:"line"`
	Username    string `json:"username"`
	Description string `json:"description"`
	GroupName   string `json:"group_name"`
	Success     bool   `json:"success"`
	Error       string `json:"error"`
}

// parseBatchUserCSV 解析批量添加用户的 CSV。
// 首行为表头，需包含「用户名」「密码」，可选「用户备注」「用户组」（同时兼容英文表头）。
// 用户备注写入用户描述；用户组为空表示不加入任何用户组。仅表头缺失等结构性错误会返回 error。
func parseBatchUserCSV(text string) ([]batchUserRow, error) {
	text = strings.TrimPrefix(text, "\ufeff") // 去掉 UTF-8 BOM
	reader := csv.NewReader(strings.NewReader(text))
	reader.FieldsPerRecord = -1
	reader.TrimLeadingSpace = true
	reader.LazyQuotes = true

	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("CSV 解析失败: %s", err.Error())
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("CSV 内容为空")
	}

	headerIndex := make(map[string]int, len(records[0]))
	for i, h := range records[0] {
		h = strings.TrimSpace(strings.TrimPrefix(h, "\ufeff"))
		headerIndex[strings.ToLower(h)] = i
	}
	findColumn := func(aliases ...string) (int, bool) {
		for _, alias := range aliases {
			if idx, ok := headerIndex[strings.ToLower(alias)]; ok {
				return idx, true
			}
		}
		return -1, false
	}

	usernameIdx, ok := findColumn("用户名", "username", "用户")
	if !ok {
		return nil, fmt.Errorf("CSV 缺少「用户名」列")
	}
	passwordIdx, ok := findColumn("密码", "password")
	if !ok {
		return nil, fmt.Errorf("CSV 缺少「密码」列")
	}
	descriptionIdx, hasDescription := findColumn("用户备注", "备注", "description", "desc")
	groupIdx, hasGroup := findColumn("用户组", "用户组名称", "组名", "group", "group_name")

	rows := make([]batchUserRow, 0, len(records)-1)
	for i, record := range records[1:] {
		valueAt := func(idx int) string {
			if idx < 0 || idx >= len(record) {
				return ""
			}
			return strings.TrimSpace(record[idx])
		}
		row := batchUserRow{
			Line:     i + 2, // 含表头，故数据从第 2 行开始
			Username: valueAt(usernameIdx),
			Password: valueAt(passwordIdx),
		}
		if hasDescription {
			row.Description = valueAt(descriptionIdx)
		}
		if hasGroup {
			row.GroupName = valueAt(groupIdx)
		}
		// 跳过完全空白的行
		if row.Username == "" && row.Password == "" && row.Description == "" && row.GroupName == "" {
			continue
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// validateBatchUserRow 校验单行数据，返回空字符串表示通过。
func validateBatchUserRow(row batchUserRow) string {
	switch {
	case row.Username == "":
		return "用户名为空"
	case len([]rune(row.Username)) > 50:
		return "用户名长度不能超过 50 个字符"
	case row.Password == "":
		return "密码为空"
	case len([]rune(row.Password)) > 100:
		return "密码长度不能超过 100 个字符"
	case len([]rune(row.Description)) > 500:
		return "用户备注长度不能超过 500 个字符"
	case len([]rune(row.GroupName)) > 50:
		return "用户组名称长度不能超过 50 个字符"
	}
	return ""
}

// BatchCreateUserHandler 通过 CSV 批量创建用户。
// 逐行创建，返回每行成功/失败与失败原因；用户组为空时不加入任何用户组。
func (a *App) BatchCreateUserHandler(c *gin.Context, user *models.User) {
	type Param struct {
		CSV string `json:"csv" binding:"required,max=1048576"`
	}
	var param Param
	if err := c.ShouldBindJSON(&param); err != nil {
		c.JSON(400, gin.H{"result": "failed", "error": err.Error()})
		return
	}

	rows, err := parseBatchUserCSV(param.CSV)
	if err != nil {
		c.JSON(400, gin.H{"result": "failed", "error": err.Error()})
		return
	}
	if len(rows) == 0 {
		c.JSON(400, gin.H{"result": "failed", "error": "CSV 中没有有效用户数据"})
		return
	}

	results := make([]batchCreateUserResult, 0, len(rows))
	successCount := 0
	for _, row := range rows {
		item := batchCreateUserResult{Line: row.Line, Username: row.Username, Description: row.Description, GroupName: row.GroupName}
		if reason := validateBatchUserRow(row); reason != "" {
			item.Error = reason
		} else if err := a.daoManager.CreateUserWithGroup(row.Username, row.Password, row.Description, row.GroupName); err != nil {
			item.Error = err.Error()
		} else {
			item.Success = true
			successCount++
		}
		results = append(results, item)
	}

	c.JSON(200, gin.H{
		"result":        "success",
		"error":         nil,
		"data":          results,
		"success_count": successCount,
		"failed_count":  len(rows) - successCount,
	})
}
