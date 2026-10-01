package api

import (
	"encoding/json"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"openvpn-pannel/internal/models"
)

// callUserDisabledHandler 以 JSON 请求体调用禁用/启用处理函数，返回状态码与解析后的响应。
func callUserDisabledHandler(t *testing.T, app *App, body string, disabled bool) (int, map[string]interface{}) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest("POST", "/user/disable", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	if disabled {
		app.DisableUserHandler(c, nil)
	} else {
		app.EnableUserHandler(c, nil)
	}
	var resp map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	return rec.Code, resp
}

func TestUserDisabledHandlers(t *testing.T) {
	app, dm, _ := newTestApp(t)
	if err := dm.CreateUser("u1", "pw", "", models.RATE_LIMIT_TYPE_NONE, 0, 0, 0); err != nil {
		t.Fatalf("create u1: %v", err)
	}
	u1, err := dm.GetUserByUsername("u1")
	if err != nil {
		t.Fatalf("get u1: %v", err)
	}

	// 禁用（批量字段 id_list）
	userID := strconv.FormatUint(uint64(u1.ID), 10)
	code, resp := callUserDisabledHandler(t, app, `{"id_list":[`+userID+`]}`, true)
	if code != 200 || resp["result"] != "success" {
		t.Fatalf("disable 响应 = %d %v", code, resp)
	}
	if got, _ := dm.GetUserByUsername("u1"); !got.Disabled {
		t.Fatalf("u1 应被禁用")
	}

	// 启用（单个字段 id）
	code, resp = callUserDisabledHandler(t, app, `{"id":`+userID+`}`, false)
	if code != 200 || resp["result"] != "success" {
		t.Fatalf("enable 响应 = %d %v", code, resp)
	}
	if got, _ := dm.GetUserByUsername("u1"); got.Disabled {
		t.Fatalf("u1 应恢复启用")
	}

	// 空请求体应返回 400
	code, _ = callUserDisabledHandler(t, app, `{}`, true)
	if code != 400 {
		t.Fatalf("空用户列表应返回 400，实际 %d", code)
	}
}
