package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// 内部 API 令牌中间件：未带/错误令牌一律 403，正确令牌放行。
func TestInternalAPITokenMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	app := &App{internalToken: "secret-token"}
	r := gin.New()
	r.Use(app.internalAPITokenMiddleware())
	r.POST("/x", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	do := func(token string) int {
		req := httptest.NewRequest(http.MethodPost, "/x", nil)
		if token != "" {
			req.Header.Set(internalAPITokenHeader, token)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w.Code
	}

	if code := do(""); code != http.StatusForbidden {
		t.Fatalf("缺少令牌应 403，实际 %d", code)
	}
	if code := do("wrong-token"); code != http.StatusForbidden {
		t.Fatalf("错误令牌应 403，实际 %d", code)
	}
	if code := do("secret-token"); code != http.StatusOK {
		t.Fatalf("正确令牌应 200，实际 %d", code)
	}
}

// 未配置令牌（internalToken 为空）时拒绝所有请求，避免误开放内部 API。
func TestInternalAPITokenMiddlewareUnconfigured(t *testing.T) {
	gin.SetMode(gin.TestMode)
	app := &App{}
	r := gin.New()
	r.Use(app.internalAPITokenMiddleware())
	r.POST("/x", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	req := httptest.NewRequest(http.MethodPost, "/x", nil)
	req.Header.Set(internalAPITokenHeader, "anything")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("未配置令牌时应 403，实际 %d", w.Code)
	}
}
