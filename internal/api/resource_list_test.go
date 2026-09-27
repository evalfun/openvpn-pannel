package api

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	ovpnserver "openvpn-pannel/internal/ovpn_server"
)

// listResources 通过 Gin 测试路由调用 ListResourceHandler，并解析返回的 data 列表。
func listResources(t *testing.T, app *App, setID string) map[string]bool {
	t.Helper()
	gin.SetMode(gin.TestMode)

	// 每次使用独立的路由引擎，避免重复注册同一路径导致 panic。
	engine := gin.New()
	engine.GET("/resource/list", func(c *gin.Context) { app.ListResourceHandler(c, nil) })

	url := "/resource/list"
	if setID != "" {
		url += "?set=" + setID
	}
	var body struct {
		Result string `json:"result"`
		View   string `json:"view_set"`
		Data   []struct {
			ID       string `json:"id"`
			Modified bool   `json:"modified"`
		} `json:"data"`
	}

	req := httptest.NewRequest("GET", url, nil)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("resource/list 状态码 = %d, body=%s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("解析响应失败: %v, body=%s", err, rec.Body.String())
	}
	if body.Result != "success" {
		t.Fatalf("result = %q", body.Result)
	}
	modified := make(map[string]bool)
	for _, r := range body.Data {
		modified[r.ID] = r.Modified
	}
	return modified
}

// TestListResourceModifiedFlag 验证“修改过”标记：
//   - 种子化（把内置默认写入数据库）后，所有资源仍应为未修改；
//   - 改写某个资源内容后，只有该资源变为已修改；
//   - 重置（删除覆盖记录）后，该资源恢复为未修改。
func TestListResourceModifiedFlag(t *testing.T) {
	app := newResourceTestApp(t, true)
	setID := ovpnserver.RESOURCE_SET_LINUX_IPTABLES

	// 触发种子化：全部资源写入数据库，但内容与内置一致，均不应标记为“修改过”。
	_ = app.PrepareResourceMap([]string{ovpnserver.RESOURCE_ID_MISC_CONFIG})

	modified := listResources(t, app, setID)
	if len(modified) != len(ovpnserver.ListResourceIDs()) {
		t.Fatalf("资源数量 = %d, 期望 %d", len(modified), len(ovpnserver.ListResourceIDs()))
	}
	for id, m := range modified {
		if m {
			t.Fatalf("种子化后资源 %s 不应标记为已修改", id)
		}
	}

	// 改写 misc 的内容，只有它应变为已修改。
	if err := app.daoManager.WriteResourceBySet(setID, ovpnserver.RESOURCE_ID_MISC_CONFIG, `{"shell_path":"/bin/zzz"}`); err != nil {
		t.Fatalf("write: %v", err)
	}
	modified = listResources(t, app, setID)
	if !modified[ovpnserver.RESOURCE_ID_MISC_CONFIG] {
		t.Fatalf("改写后 misc 应标记为已修改")
	}
	for id, m := range modified {
		if id != ovpnserver.RESOURCE_ID_MISC_CONFIG && m {
			t.Fatalf("改写 misc 不应影响资源 %s", id)
		}
	}

	// 重置（删除覆盖）后恢复为未修改。
	if err := app.daoManager.DeleteResourceBySet(setID, ovpnserver.RESOURCE_ID_MISC_CONFIG); err != nil {
		t.Fatalf("delete: %v", err)
	}
	modified = listResources(t, app, setID)
	if modified[ovpnserver.RESOURCE_ID_MISC_CONFIG] {
		t.Fatalf("重置后 misc 不应标记为已修改")
	}
}

// TestListResourceModifiedPerSet 验证“修改过”按所选资源集区分：
// 只改 A 集的资源，不应影响 B 集的同名资源。
func TestListResourceModifiedPerSet(t *testing.T) {
	app := newResourceTestApp(t, true)
	setA := ovpnserver.RESOURCE_SET_LINUX_IPTABLES
	setB := ovpnserver.RESOURCE_SET_LINUX_NFTABLES

	// 两个集合都先种子化。
	_ = app.PrepareResourceMap([]string{ovpnserver.RESOURCE_ID_MISC_CONFIG})
	if err := app.daoManager.WriteResourceBySet(setB, ovpnserver.RESOURCE_ID_MISC_CONFIG, `{"shell_path":"/bin/seed"}`); err != nil {
		t.Fatalf("seed B: %v", err)
	}
	// 上面写入的 seed 内容与默认不同，先重置回默认，确保起点干净。
	if err := app.daoManager.DeleteResourceBySet(setB, ovpnserver.RESOURCE_ID_MISC_CONFIG); err != nil {
		t.Fatalf("reset B: %v", err)
	}

	// 只修改 A 集。
	if err := app.daoManager.WriteResourceBySet(setA, ovpnserver.RESOURCE_ID_CLIENT_ONLINE_SCRIPT, "# custom A\n"); err != nil {
		t.Fatalf("write A: %v", err)
	}

	modifiedA := listResources(t, app, setA)
	if !modifiedA[ovpnserver.RESOURCE_ID_CLIENT_ONLINE_SCRIPT] {
		t.Fatalf("A 集 client_online.sh 应标记为已修改")
	}
	modifiedB := listResources(t, app, setB)
	if modifiedB[ovpnserver.RESOURCE_ID_CLIENT_ONLINE_SCRIPT] {
		t.Fatalf("B 集 client_online.sh 不应标记为已修改")
	}
}
