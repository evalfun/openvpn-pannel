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

// callSetUsersExpireHandler 以 JSON 请求体调用批量设置有效期处理函数。
func callSetUsersExpireHandler(t *testing.T, app *App, body string) (int, map[string]interface{}) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest("POST", "/user/set_expire", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	app.SetUsersExpireHandler(c, nil)
	var resp map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	return rec.Code, resp
}

func TestSetUsersExpireHandler(t *testing.T) {
	app, dm, _ := newTestApp(t)
	for _, name := range []string{"u1", "u2"} {
		if err := dm.CreateUser(name, "pw", "", models.RATE_LIMIT_TYPE_NONE, 0, 0, 0); err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
	}
	u1, _ := dm.GetUserByUsername("u1")
	u2, _ := dm.GetUserByUsername("u2")
	ids := strconv.FormatUint(uint64(u1.ID), 10) + "," + strconv.FormatUint(uint64(u2.ID), 10)
	expireAt := int64(4102444800) // 2100-01-01

	code, resp := callSetUsersExpireHandler(t, app, `{"id_list":[`+ids+`],"expire_at":`+strconv.FormatInt(expireAt, 10)+`}`)
	if code != 200 || resp["result"] != "success" {
		t.Fatalf("设置有效期响应 = %d %v", code, resp)
	}
	for _, name := range []string{"u1", "u2"} {
		got, _ := dm.GetUserByUsername(name)
		if got.ExpireAt != uint64(expireAt) {
			t.Fatalf("%s.ExpireAt = %d, want %d", name, got.ExpireAt, expireAt)
		}
	}

	// expire_at=0 表示永久
	code, resp = callSetUsersExpireHandler(t, app, `{"id_list":[`+ids+`],"expire_at":0}`)
	if code != 200 || resp["result"] != "success" {
		t.Fatalf("设置永久响应 = %d %v", code, resp)
	}
	if got, _ := dm.GetUserByUsername("u1"); got.ExpireAt != 0 {
		t.Fatalf("u1.ExpireAt = %d, want 0", got.ExpireAt)
	}

	// 空用户列表应返回 400
	code, _ = callSetUsersExpireHandler(t, app, `{"expire_at":123}`)
	if code != 400 {
		t.Fatalf("空用户列表应返回 400，实际 %d", code)
	}
}
