package api

import (
	"fmt"
	"log"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"openvpn-pannel/internal/models"
	ovpnserver "openvpn-pannel/internal/ovpn_server"
	"openvpn-pannel/internal/totp"

	"github.com/gin-gonic/gin"
)

// nowFunc 便于测试注入固定时间。
var nowFunc = time.Now

// 客户端自助页面：监听一个额外端口（config.client_page_listen），仅面向已连接的 VPN 客户端。
// 通过 HTTP 来源 IP（VPN 虚拟 IP）识别是哪个客户端/用户；非 VPN 客户端访问返回错误。
// 启用 TOTP 的用户在 VPN 认证通过后不下发任何 ACL，需在此页面输入动态验证码，
// 验证通过后由面板调用 acl_add.sh 放行，登出时调用 acl_del.sh 回收。
// 同时提供无头设备可用的 curl 接口：GET /login/<code> 与 GET /logout。

type clientIdentity struct {
	record *models.ConnectedClientInfoRecord
	server *models.Server
	user   *models.User
}

// SetupClientPageRouter 构建客户端自助页面的独立路由。
func (a *App) SetupClientPageRouter() *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())
	// 该页面按 TCP 来源地址（RemoteAddr）识别 VPN 客户端，不采信可伪造的代理头；
	// 这里同步可信代理配置仅为消除 gin 默认“信任所有代理”的告警。
	if len(a.cfg.TrustedProxies) > 0 {
		if err := r.SetTrustedProxies(a.cfg.TrustedProxies); err != nil {
			log.Printf("配置客户端自助页面可信代理失败: %v", err)
		}
	} else {
		_ = r.SetTrustedProxies(nil)
	}
	r.GET("/", a.ClientPortalIndex)
	r.GET("/index.html", a.ClientPortalIndex)
	r.GET("/api/info", a.ClientPortalInfo)
	r.POST("/api/login", a.ClientPortalLogin)
	r.POST("/api/logout", a.ClientPortalLogout)
	// 兼容无头设备的 curl 调用
	r.GET("/login/:code", a.ClientPortalLoginCurl)
	r.GET("/logout", a.ClientPortalLogoutCurl)
	r.GET("/api/login/:code", a.ClientPortalLoginCurl)
	r.GET("/api/logout", a.ClientPortalLogoutCurl)
	return r
}

// remoteIP 取 TCP 对端地址（不使用可伪造的 X-Forwarded-For）。
func remoteIP(c *gin.Context) string {
	host, _, err := net.SplitHostPort(c.Request.RemoteAddr)
	if err != nil {
		return c.Request.RemoteAddr
	}
	return host
}

// ipInServerCIDR 判断 ip 是否属于服务器声明的网段。兼容 "10.8.0.0/24" 与
// OpenVPN 的 "10.8.0.0 255.255.255.0" 两种写法。
func ipInServerCIDR(ip, cidr string) bool {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	cidr = strings.TrimSpace(cidr)
	if cidr == "" {
		return false
	}
	if strings.Contains(cidr, "/") {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			return false
		}
		return network.Contains(parsed)
	}
	fields := strings.Fields(cidr)
	if len(fields) == 2 {
		networkIP := net.ParseIP(fields[0])
		maskIP := net.ParseIP(fields[1])
		if networkIP == nil || maskIP == nil {
			return false
		}
		mask := net.IPMask(maskIP.To4())
		if mask == nil {
			return false
		}
		return (&net.IPNet{IP: networkIP.Mask(mask), Mask: mask}).Contains(parsed)
	}
	// 单个 IP
	return net.ParseIP(cidr) != nil && net.ParseIP(cidr).Equal(parsed)
}

var errNotVPNClient = fmt.Errorf("非 VPN 客户端，无法访问此页面")

// resolveClient 依据 HTTP 来源 IP 识别在线客户端及其服务器与用户。
func (a *App) resolveClient(c *gin.Context) (*clientIdentity, error) {
	src := remoteIP(c)
	records, err := a.daoManager.ListConnectedClientInfoRecordByVirtualIP(src)
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, errNotVPNClient
	}
	var chosen *models.ConnectedClientInfoRecord
	var chosenServer *models.Server
	for _, rec := range records {
		srv, err := a.daoManager.GetOpenVPNServerByID(rec.ServerID)
		if err != nil {
			continue
		}
		// IPv6 来源按 server_cidr6 判定，IPv4 来源按 server_cidr 判定。
		cidr := srv.ServerCIDR
		if strings.Contains(src, ":") {
			cidr = srv.ServerCIDR6
		}
		if cidr != "" && ipInServerCIDR(src, cidr) {
			chosen, chosenServer = rec, srv
			break
		}
	}
	// 网段无法判定时，若只有唯一记录则直接采用
	if chosen == nil && len(records) == 1 {
		chosen = records[0]
		chosenServer, _ = a.daoManager.GetOpenVPNServerByID(chosen.ServerID)
	}
	if chosen == nil || chosenServer == nil {
		return nil, errNotVPNClient
	}
	user, err := a.daoManager.GetUserByUsername(chosen.Username)
	if err != nil {
		return nil, fmt.Errorf("用户不存在")
	}
	return &clientIdentity{record: chosen, server: chosenServer, user: user}, nil
}

// runUserACLScript 运行 acl_add.sh / acl_del.sh，通过标准输入传入客户端与 ACL 列表。
// virtualIP 为客户端主地址（有 IPv4 用 IPv4，否则 IPv6），virtualIP4/virtualIP6 分别为
// 该客户端实际的 IPv4 / IPv6 虚拟地址，脚本据此分别下发 v4/v6 规则。
func (a *App) runUserACLScript(resourceID string, serverModel *models.Server, virtualIP, virtualIP4, virtualIP6, username string, records []*models.AddedServerACLRecord) error {
	resourceMap := a.PrepareResourceMap([]string{ovpnserver.RESOURCE_ID_MISC_CONFIG, resourceID})
	script, ok := resourceMap[resourceID]
	if !ok {
		script = a.GetActiveResourceContent(resourceID)
	}
	miscConfig, err := ovpnserver.ParseMiscConfig(resourceMap)
	if err != nil {
		return err
	}
	workingDir := fmt.Sprintf("%s/%d/", strings.TrimRight(a.cfg.WorkingDir, "/"), serverModel.ID)
	script = strings.ReplaceAll(script, "__WORKING_DIR__", workingDir)
	script = strings.ReplaceAll(script, "__SERVER_ID__", strconv.FormatUint(uint64(serverModel.ID), 10))
	script = strings.ReplaceAll(script, "__SERVER_INTERFACE__", serverModel.Dev)

	var stdin strings.Builder
	fmt.Fprintf(&stdin, "virtual_ip: %s\n", virtualIP)
	fmt.Fprintf(&stdin, "virtual_ip4: %s\n", virtualIP4)
	fmt.Fprintf(&stdin, "virtual_ip6: %s\n", virtualIP6)
	fmt.Fprintf(&stdin, "username: %s\n", username)
	for _, r := range records {
		fmt.Fprintf(&stdin, "%d#%s\n", r.ACLType, r.ACLValue)
	}

	cmd := exec.Command(miscConfig.ShellPath, "-c", script)
	cmd.Stdin = strings.NewReader(stdin.String())
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w 脚本输出: %s", err, string(out))
	}
	return nil
}

// applyUserACL 为 TOTP 已通过验证的会话下发 ACL，并记录到 AddedServerACLRecord。
func (a *App) applyUserACL(id *clientIdentity) error {
	aclList, err := a.daoManager.GetACLByUser(id.user.ID, id.server.ID)
	if err != nil {
		return err
	}
	records := make([]*models.AddedServerACLRecord, 0, len(aclList))
	for _, acl := range aclList {
		// 仅处理 IPv4(4) 与 IPv6(6) ACL，其它类型暂时忽略。
		if acl.Type != 4 && acl.Type != 6 {
			continue
		}
		records = append(records, &models.AddedServerACLRecord{
			ServerID:       id.server.ID,
			VirtualIPAddr:  id.record.VirtualIPAddr,
			VirtualIP6Addr: id.record.VirtualIP6Addr,
			ACLType:        acl.Type,
			ACLValue:       acl.Value,
		})
	}
	// 清理可能残留的记录，保证幂等
	if err := a.daoManager.DeleteAddedACLByIP(id.record.VirtualIPAddr, id.server.ID); err != nil {
		return err
	}
	virtualIP4 := id.record.VirtualIPAddr
	if strings.Contains(virtualIP4, ":") {
		virtualIP4 = ""
	}
	if err := a.runUserACLScript(ovpnserver.RESOURCE_ID_ACL_ADD_SCRIPT, id.server, id.record.VirtualIPAddr, virtualIP4, id.record.VirtualIP6Addr, id.user.Username, records); err != nil {
		return err
	}
	if len(records) > 0 {
		if err := a.daoManager.SaveAddedACL(records); err != nil {
			return err
		}
	}
	if err := a.daoManager.UpdateConnectedClientInfoRecordMFAVerified(id.server.ID, id.record.VirtualIPAddr, true); err != nil {
		return err
	}
	// 同步内存中的会话状态，使本次请求返回的状态即为“已通过验证”。
	id.record.MFAVerified = true
	return nil
}

// removeUserACL 回收该会话已下发的 ACL（登出）。
func (a *App) removeUserACL(id *clientIdentity) error {
	records, err := a.daoManager.ListAddedACLByIP(id.record.VirtualIPAddr, id.server.ID)
	if err != nil {
		return err
	}
	virtualIP4 := id.record.VirtualIPAddr
	if strings.Contains(virtualIP4, ":") {
		virtualIP4 = ""
	}
	if err := a.runUserACLScript(ovpnserver.RESOURCE_ID_ACL_DEL_SCRIPT, id.server, id.record.VirtualIPAddr, virtualIP4, id.record.VirtualIP6Addr, id.user.Username, records); err != nil {
		return err
	}
	if err := a.daoManager.DeleteAddedACLByIP(id.record.VirtualIPAddr, id.server.ID); err != nil {
		return err
	}
	if err := a.daoManager.UpdateConnectedClientInfoRecordMFAVerified(id.server.ID, id.record.VirtualIPAddr, false); err != nil {
		return err
	}
	id.record.MFAVerified = false
	return nil
}

// clientPortalStatus 组装页面/接口所需的连接信息。
func (a *App) clientPortalStatus(id *clientIdentity) gin.H {
	uploadKB, downloadKB, _ := a.daoManager.ResolveUserRateLimit(id.user.ID, id.server.ID)

	networks := make([]string, 0)
	// 仅当已放行（未启用 MFA，或 MFA 已通过）时展示可访问网络。
	// 这里读取的是“会话已实际下发的 ACL”（added_server_acl_records），而不是按用户/用户组实时计算：
	// 用户/用户组 ACL 的变更需在下一次上线（或 MFA 登出再登录）后才会生效，页面应与实际放行保持一致。
	loggedIn := id.user.MFAType == models.MFA_TYPE_NONE || id.record.MFAVerified
	if loggedIn {
		if records, err := a.daoManager.ListAddedACLByIP(id.record.VirtualIPAddr, id.server.ID); err == nil {
			seen := make(map[string]bool)
			for _, r := range records {
				key := fmt.Sprintf("%d#%s", r.ACLType, r.ACLValue)
				if !seen[key] {
					seen[key] = true
					networks = append(networks, key)
				}
			}
		}
	}

	publicIP := id.record.RealIPAddr
	if host, _, err := net.SplitHostPort(publicIP); err == nil {
		publicIP = host
	}

	return gin.H{
		"result":            "success",
		"vpn":               true,
		"logged_in":         loggedIn,
		"mfa_required":      id.user.MFAType != models.MFA_TYPE_NONE,
		"mfa_type":          id.user.MFAType,
		"mfa_verified":      id.record.MFAVerified,
		"server_name":       id.server.Name,
		"username":          id.user.Username,
		"cert_name":         id.record.ClientCertName,
		"public_ip":         publicIP,
		"virtual_ip":        id.record.VirtualIPAddr,
		"virtual_ip6":       id.record.VirtualIP6Addr,
		"networks":          networks,
		"upload_bytes":      id.record.ByteSent,
		"download_bytes":    id.record.ByteReceived,
		"upload_limit_kb":   uploadKB,
		"download_limit_kb": downloadKB,
	}
}

// ClientPortalIndex 返回自助页面（单文件 HTML）。
// 页面内容取自当前启用资源集的 client-page.html，用户可在「资源管理」中自行修改。
func (a *App) ClientPortalIndex(c *gin.Context) {
	html := a.GetActiveResourceContent(ovpnserver.RESOURCE_ID_CLIENT_PAGE)
	c.Data(200, "text/html; charset=utf-8", []byte(html))
}

// ClientPortalInfo 返回当前客户端的连接信息 / 登录状态。
func (a *App) ClientPortalInfo(c *gin.Context) {
	id, err := a.resolveClient(c)
	if err != nil {
		c.JSON(403, gin.H{"result": "failed", "vpn": false, "error": err.Error()})
		return
	}
	c.JSON(200, a.clientPortalStatus(id))
}

// ClientPortalLogin 校验 TOTP 验证码并放行 ACL。
func (a *App) ClientPortalLogin(c *gin.Context) {
	id, err := a.resolveClient(c)
	if err != nil {
		c.JSON(403, gin.H{"result": "failed", "vpn": false, "error": err.Error()})
		return
	}
	var param struct {
		Code string `json:"code"`
	}
	_ = c.ShouldBindJSON(&param)
	code := strings.TrimSpace(param.Code)
	if code == "" {
		code = c.Query("code")
	}
	if err := a.verifyAndApplyMFA(id, code); err != nil {
		c.JSON(401, gin.H{"result": "failed", "error": err.Error()})
		return
	}
	c.JSON(200, a.clientPortalStatus(id))
}

// ClientPortalLoginCurl 兼容无头设备：GET /login/<code>，返回纯文本。
func (a *App) ClientPortalLoginCurl(c *gin.Context) {
	id, err := a.resolveClient(c)
	if err != nil {
		c.String(403, "result=failed "+err.Error())
		return
	}
	if err := a.verifyAndApplyMFA(id, c.Param("code")); err != nil {
		c.String(401, "result=failed "+err.Error())
		return
	}
	c.String(200, "result=success\nserver=%s\nusername=%s\nvirtual_ip=%s\nvirtual_ip6=%s\n", id.server.Name, id.user.Username, id.record.VirtualIPAddr, id.record.VirtualIP6Addr)
}

// verifyAndApplyMFA 对启用 MFA 的用户校验验证码并放行 ACL；未启用则直接成功。
func (a *App) verifyAndApplyMFA(id *clientIdentity, code string) error {
	switch id.user.MFAType {
	case models.MFA_TYPE_NONE:
		return nil
	case models.MFA_TYPE_TOTP:
		// 防暴力破解：同一用户 2 秒内只允许提交一次验证码。
		if ok, remain := a.loginCooldown.Allow("mfa:" + id.user.Username); !ok {
			return fmt.Errorf("验证码尝试过于频繁，请 %d 秒后再试", retryAfterSeconds(remain))
		}
		if !totp.Validate(id.user.MFAData, code, nowFunc()) {
			return fmt.Errorf("验证码错误或已过期")
		}
	default:
		return fmt.Errorf("暂不支持的多因素认证类型")
	}
	if err := a.applyUserACL(id); err != nil {
		log.Printf("客户端页面放行 ACL 失败 server %d 用户 %s: %v", id.server.ID, id.user.Username, err)
		return fmt.Errorf("放行网络失败: %s", err.Error())
	}
	a.daoManager.CreateEvent(id.server.ID, models.SERVER_EVENT_TYPE_SECOND_AUTH_SUCCESS, id.record.RealIPAddr, fmt.Sprintf("二次认证成功 用户=%s 虚拟IP=%s", id.user.Username, id.record.VirtualIPAddr))
	log.Printf("客户端页面 MFA 验证通过，已放行 ACL server %d 用户 %s 虚拟IP %s", id.server.ID, id.user.Username, id.record.VirtualIPAddr)
	return nil
}

// ClientPortalLogout 回收 ACL。
func (a *App) ClientPortalLogout(c *gin.Context) {
	id, err := a.resolveClient(c)
	if err != nil {
		c.JSON(403, gin.H{"result": "failed", "vpn": false, "error": err.Error()})
		return
	}
	if err := a.logoutClient(id); err != nil {
		c.JSON(500, gin.H{"result": "failed", "error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"result": "success", "error": nil})
}

// ClientPortalLogoutCurl 兼容无头设备：GET /logout，返回纯文本。
func (a *App) ClientPortalLogoutCurl(c *gin.Context) {
	id, err := a.resolveClient(c)
	if err != nil {
		c.String(403, "result=failed "+err.Error())
		return
	}
	if err := a.logoutClient(id); err != nil {
		c.String(500, "result=failed "+err.Error())
		return
	}
	c.String(200, "result=success")
}

// logoutClient 回收 ACL 并标记未验证。仅对启用 MFA 的用户执行。
func (a *App) logoutClient(id *clientIdentity) error {
	if id.user.MFAType == models.MFA_TYPE_NONE {
		return nil
	}
	if err := a.removeUserACL(id); err != nil {
		log.Printf("客户端页面回收 ACL 失败 server %d 用户 %s: %v", id.server.ID, id.user.Username, err)
		return fmt.Errorf("回收网络失败: %s", err.Error())
	}
	a.daoManager.CreateEvent(id.server.ID, models.SERVER_EVENT_TYPE_SECOND_AUTH_LOGOUT, id.record.RealIPAddr, fmt.Sprintf("二次认证下线 用户=%s 虚拟IP=%s", id.user.Username, id.record.VirtualIPAddr))
	log.Printf("客户端页面登出，已回收 ACL server %d 用户 %s 虚拟IP %s", id.server.ID, id.user.Username, id.record.VirtualIPAddr)
	return nil
}
