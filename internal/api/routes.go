package api

import (
	"net/http"
	"openvpn-pannel/internal/assets"
	"path/filepath"
	"strings"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

func getContentType(path string) string {
	ext := filepath.Ext(path)
	switch ext {
	case ".js":
		return "application/javascript"
	case ".css":
		return "text/css"
	case ".html", ".htm":
		return "text/html; charset=utf-8"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".svg":
		return "image/svg+xml"
	case ".json":
		return "application/json"
	default:
		return "application/octet-stream"
	}
}
func (a *App) setupStaticFiles() {
	// 静态文件处理器
	a.router.NoRoute(func(c *gin.Context) {
		path := c.Request.URL.Path
		if strings.Contains(path, "..") {
			c.Status(http.StatusNotFound)
			return
		}

		// 去掉开头的 /
		filePath := strings.TrimPrefix(path, "/")
		if filePath == "" {
			filePath = "index.html"
		}

		// 从 bindata 获取文件内容
		data, err := assets.Asset(filePath)
		if err != nil {
			// fallback 到 index.html（SPA 支持）
			data, err = assets.Asset("index.html")
			if err != nil {
				c.Status(http.StatusNotFound)
				return
			}
			filePath = "index.html"
		}

		// 自动设置 Content-Type
		contentType := getContentType(filePath)
		c.Data(http.StatusOK, contentType, data)
	})
}
func (a *App) setupRoutes() {
	// 会话数据保存在服务端内存，浏览器 Cookie 中只保存随机会话 ID。
	// 相比 cookie store（把加密后的整段会话数据放进 Cookie），这样不会把会话内容暴露给客户端，
	// 也不会因为会话数据增长而撑大请求头。代价是面板进程重启后需要重新登录。
	store := newMemorySessionStore([]byte(a.cfg.SessionSecret))
	store.Options(sessions.Options{
		HttpOnly: true,
		MaxAge:   36000, // 10 小时
		Secure:   false, // ← 关键：设为 false 表示不强制 HTTPS
		Path:     "/",
	})
	a.router.Use(sessions.Sessions("session", store))

	api := a.router.Group("/api")
	{

		api.GET("/builddate", a.AdminLoginWarper(a.GetBuildDate))

		// 用户接口
		api.POST("/user/login", a.UserLoginHandler)
		api.POST("/user/logout", a.UserLogoutHandler)
		api.POST("/user/create/batch", a.AdminLoginWarper(a.BatchCreateUserHandler))
		api.POST("/user/create", a.AdminLoginWarper(a.CreateUserHandler))
		api.GET("/user/info", a.UserLoginWarper(a.GetUserInfoHandler))
		api.POST("/user/info", a.UserLoginWarper(a.UpdateUserInfoHandler))
		api.GET("/user/list", a.AdminLoginWarper(a.ListUserHandler))
		api.POST("/user/delete", a.AdminLoginWarper(a.DeleteUserHandler))
		api.POST("/user/reset_traffic", a.AdminLoginWarper(a.ResetUserTrafficHandler))

		// 用户组接口
		api.POST("/group/create", a.AdminLoginWarper(a.CreateGroupHandler))
		api.GET("/group/list", a.AdminLoginWarper(a.ListGroupsHandler))
		api.POST("/group/delete", a.AdminLoginWarper(a.DeleteGroupHandler))
		api.POST("/group/update", a.AdminLoginWarper(a.UpdateGroupHandler))

		api.GET("/group/user/list", a.AdminLoginWarper(a.ListUsersInGroupHandler))
		api.POST("/group/user/add", a.AdminLoginWarper(a.AddUserToGroupHandler))
		api.POST("/group/user/remove", a.AdminLoginWarper(a.RemoveUserFromGroupHandler))
		api.POST("/group/user/add/batch", a.AdminLoginWarper(a.BatchAddUsersToGroupHandler))

		api.POST("/group/acl/add", a.AdminLoginWarper(a.AddGroupACLHandler))
		api.GET("/group/acl/list", a.AdminLoginWarper(a.ListGroupACLHandler))
		api.POST("/group/acl/delete", a.AdminLoginWarper(a.DeleteGroupACLHandler))

		// 服务器接口
		api.POST("/server/create", a.AdminLoginWarper(a.CreateOpenVPNServerHandler))
		api.POST("/server/update", a.AdminLoginWarper(a.UpdateOpenVPNServerHandler))
		api.POST("/server/delete", a.AdminLoginWarper(a.DeleteOpenVPNServerHandler))
		api.GET("/server/list", a.AdminLoginWarper(a.ListOpenVPNServerHandler))
		api.GET("/server/list/user_perm", a.AdminLoginWarper(a.ListOpenVPNServerByUserPermissionHandler))
		api.GET("/server/info", a.AdminLoginWarper(a.GetOpenVPNServerInfoHandler))

		api.POST("/server/start", a.AdminLoginWarper(a.StartOpenVPNServerInstanceHandler))
		api.POST("/server/stop", a.AdminLoginWarper(a.StopOpenVPNServerInstanceHandler))

		api.GET("/server/log", a.AdminLoginWarper(a.GetOpenVPNServerLogHandler))
		api.POST("/server/log/clear", a.AdminLoginWarper(a.ClearOpenVPNServerLogHandler))

		api.GET("/server/status", a.AdminLoginWarper(a.GetOpenVPNServerStatusHandler))
		api.POST("/server/client/kill", a.AdminLoginWarper(a.CloseOpenVPNServerClientHandler))

		// 服务器客户端配置接口
		api.POST("/server/client_config/add", a.AdminLoginWarper(a.AddOpenVPNServerClientConfigHandler))
		api.POST("/server/client_config/delete", a.AdminLoginWarper(a.DeleteOpenVPNServerClientConfigHandler))
		api.GET("/server/client_config/list", a.AdminLoginWarper(a.ListOpenVPNServerClientConfigHandler))

		// 客户端配置导出
		api.POST("/server/client_export", a.AdminLoginWarper(a.ExportClientConfigHandler))

		// 证书管理接口
		api.GET("/certificate/list", a.AdminLoginWarper(a.ListCertificateHandler))
		api.GET("/certificate/info", a.AdminLoginWarper(a.GetCertificateHandler))
		api.GET("/certificate/download_cert", a.AdminLoginWarper(a.DownloadCertificateHandler))
		api.GET("/certificate/download_key", a.AdminLoginWarper(a.DownloadCertificateKeyHandler))
		api.POST("/certificate/parse", a.AdminLoginWarper(a.ParseCertificateHandler))
		api.POST("/certificate/ca/generate", a.AdminLoginWarper(a.GenerateCAHandler))
		api.POST("/certificate/sign", a.AdminLoginWarper(a.SignCertificateHandler))
		api.POST("/certificate/import", a.AdminLoginWarper(a.ImportCertificateHandler))
		api.POST("/certificate/delete", a.AdminLoginWarper(a.DeleteCertificateHandler))
		api.POST("/certificate/dh/generate", a.AdminLoginWarper(a.GenerateDHHandler))
		api.POST("/certificate/tls_auth/generate", a.AdminLoginWarper(a.GenerateTLSAuthHandler))
		api.GET("/certificate/event/list", a.AdminLoginWarper(a.ListCertificateEventHandler))
		api.POST("/certificate/event/clear", a.AdminLoginWarper(a.ClearCertificateEventHandler))

		// 资源接口
		api.GET("/resource/get", a.AdminLoginWarper(a.GetResourceHandler))
		api.POST("/resource/write", a.AdminLoginWarper(a.WriteResourceHandler))
		api.POST("/resource/delete", a.AdminLoginWarper(a.DeleteResourceHandler))
		api.GET("/resource/list", a.AdminLoginWarper(a.ListResourceHandler))

		// 权限接口

		api.GET("/permission/list", a.AdminLoginWarper(a.ListOpenVPNServerPermissionHandler))
		api.POST("/permission/add", a.AdminLoginWarper(a.AddOpenVPNServerPermissionHandler))
		api.POST("/permission/delete", a.AdminLoginWarper(a.DelOpenVPNServerPermissionHandler))

		// 事件接口
		api.GET("/event/list", a.AdminLoginWarper(a.GetEventList))
		api.POST("/event/clear", a.AdminLoginWarper(a.ClearEvent))

		// 达量限速接口
		api.GET("/ratelimit/plan/list", a.AdminLoginWarper(a.ListRateLimitPlanHandler))
		api.POST("/ratelimit/plan/create", a.AdminLoginWarper(a.CreateRateLimitPlanHandler))
		api.POST("/ratelimit/plan/update", a.AdminLoginWarper(a.UpdateRateLimitPlanHandler))
		api.POST("/ratelimit/plan/delete", a.AdminLoginWarper(a.DeleteRateLimitPlanHandler))
		api.GET("/ratelimit/plan/users", a.AdminLoginWarper(a.ListRateLimitPlanUsersHandler))
		api.POST("/ratelimit/plan/users/add", a.AdminLoginWarper(a.AddUsersToRateLimitPlanHandler))
		api.POST("/ratelimit/plan/users/remove", a.AdminLoginWarper(a.RemoveUsersFromRateLimitPlanHandler))
		api.GET("/ratelimit/online", a.AdminLoginWarper(a.ListOnlineUserHandler))
		api.GET("/ratelimit/user/status", a.AdminLoginWarper(a.ListRateLimitUserStatusHandler))
		api.POST("/ratelimit/user/reset_cycle", a.AdminLoginWarper(a.ResetRateLimitUserCycleHandler))
	}
}
