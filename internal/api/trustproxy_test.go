package api

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"openvpn-pannel/internal/config"
)

// TestSetupTrustedProxiesBehavior 验证：未配置可信代理时忽略 X-Forwarded-For；
// 配置后仅受信任来源的 XFF 才作为客户端 IP。
func TestSetupTrustedProxiesBehavior(t *testing.T) {
	gin.SetMode(gin.TestMode)

	clientIP := func(app *App) string {
		var got string
		app.router.GET("/ip", func(c *gin.Context) { got = c.ClientIP() })
		req := httptest.NewRequest("GET", "/ip", nil)
		req.RemoteAddr = "192.0.2.10:12345"
		req.Header.Set("X-Forwarded-For", "1.2.3.4")
		app.router.ServeHTTP(httptest.NewRecorder(), req)
		return got
	}

	// 默认：不信任任何代理，取 TCP 来源地址。
	cfg := &config.Config{}
	app := &App{cfg: cfg, router: gin.New(), internalAPIRouter: gin.New()}
	app.setupTrustedProxies()
	if got := clientIP(app); got != "192.0.2.10" {
		t.Fatalf("untrusted XFF should be ignored, ClientIP=%s", got)
	}

	// 配置可信代理后，来自该网段的请求采用 XFF。
	cfg.TrustedProxies = []string{"192.0.2.0/24"}
	app2 := &App{cfg: cfg, router: gin.New(), internalAPIRouter: gin.New()}
	app2.setupTrustedProxies()
	if got := clientIP(app2); got != "1.2.3.4" {
		t.Fatalf("trusted XFF should be honored, ClientIP=%s", got)
	}
}
