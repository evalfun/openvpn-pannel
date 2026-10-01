package api

import (
	"encoding/json"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"openvpn-pannel/internal/certutil"
	"openvpn-pannel/internal/models"
)

// chainResponse 是 /certificate/chain 响应的最小解析结构。
type chainResponse struct {
	Result string `json:"result"`
	Data   struct {
		Chain []struct {
			ID         uint   `json:"id"`
			Name       string `json:"name"`
			CommonName string `json:"common_name"`
			Type       uint   `json:"type"`
			IsRoot     bool   `json:"is_root"`
			Verified   bool   `json:"verified"`
			Note       string `json:"note"`
		} `json:"chain"`
		Complete bool   `json:"complete"`
		Note     string `json:"note"`
	} `json:"data"`
}

func callGetCertificateChain(t *testing.T, app *App, id uint) chainResponse {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.GET("/certificate/chain", func(c *gin.Context) { app.GetCertificateChainHandler(c, nil) })
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/certificate/chain?id="+strconv.FormatUint(uint64(id), 10), nil)
	engine.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("chain 状态码 = %d, body=%s", rec.Code, rec.Body.String())
	}
	var body chainResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("解析 chain 响应失败: %v body=%s", err, rec.Body.String())
	}
	return body
}

func saveCertPair(t *testing.T, app *App, name string, certType, parentID uint, pair *certutil.CertPair) uint {
	t.Helper()
	cert := app.certPairToModel(name, certType, parentID, pair, "")
	if err := app.daoManager.CreateCertificate(cert); err != nil {
		t.Fatalf("保存证书 %s: %v", name, err)
	}
	return cert.ID
}

// 信任链：服务器证书 -> 中间 CA -> 根 CA；同时验证未记录 parent_id 时按 Issuer 回退回溯。
func TestCertificateChain(t *testing.T) {
	app, _, _ := newTestApp(t)

	root, err := certutil.GenerateCA(certutil.CertOptions{CommonName: "Root CA", Key: certutil.KeyOptions{KeyType: certutil.KeyTypeEC, ECCurve: "P256"}})
	if err != nil {
		t.Fatalf("generate root: %v", err)
	}
	rootID := saveCertPair(t, app, "root", models.CERT_TYPE_CA, 0, root)

	inter, err := certutil.GenerateCASignedBy(root.CertPEM, root.KeyPEM, certutil.CertOptions{CommonName: "Intermediate CA", Key: certutil.KeyOptions{KeyType: certutil.KeyTypeRSA, RSABits: 2048}})
	if err != nil {
		t.Fatalf("generate intermediate: %v", err)
	}
	interID := saveCertPair(t, app, "inter", models.CERT_TYPE_CA, rootID, inter)

	server, err := certutil.SignCert(inter.CertPEM, inter.KeyPEM, certutil.CertOptions{CommonName: "server", ServerAuth: true, Key: certutil.KeyOptions{KeyType: certutil.KeyTypeRSA, RSABits: 2048}})
	if err != nil {
		t.Fatalf("sign server: %v", err)
	}
	serverID := saveCertPair(t, app, "server", models.CERT_TYPE_SERVER, interID, server)

	resp := callGetCertificateChain(t, app, serverID)
	if resp.Result != "success" || !resp.Data.Complete {
		t.Fatalf("信任链应完整: %+v", resp)
	}
	if len(resp.Data.Chain) != 3 {
		t.Fatalf("信任链长度 = %d, want 3", len(resp.Data.Chain))
	}
	if resp.Data.Chain[0].CommonName != "server" || !resp.Data.Chain[0].Verified {
		t.Fatalf("第 1 级应为已验签的 server: %+v", resp.Data.Chain[0])
	}
	if resp.Data.Chain[1].CommonName != "Intermediate CA" || !resp.Data.Chain[1].Verified {
		t.Fatalf("第 2 级应为已验签的中间 CA: %+v", resp.Data.Chain[1])
	}
	if !resp.Data.Chain[2].IsRoot || resp.Data.Chain[2].CommonName != "Root CA" {
		t.Fatalf("第 3 级应为自签名根 CA: %+v", resp.Data.Chain[2])
	}

	// 未记录 parent_id 的证书：应能按 Issuer 匹配到库中的中间 CA 并继续向上回溯。
	orphan, err := certutil.SignCert(inter.CertPEM, inter.KeyPEM, certutil.CertOptions{CommonName: "orphan-server", ServerAuth: true, Key: certutil.KeyOptions{KeyType: certutil.KeyTypeRSA, RSABits: 2048}})
	if err != nil {
		t.Fatalf("sign orphan: %v", err)
	}
	orphanID := saveCertPair(t, app, "orphan", models.CERT_TYPE_SERVER, 0, orphan)
	resp = callGetCertificateChain(t, app, orphanID)
	if !resp.Data.Complete || len(resp.Data.Chain) != 3 {
		t.Fatalf("按 Issuer 回退应得到完整三层链: %+v", resp)
	}

	// 根 CA 自身的信任链只应有一个节点。
	rootResp := callGetCertificateChain(t, app, rootID)
	if len(rootResp.Data.Chain) != 1 || !rootResp.Data.Chain[0].IsRoot || !rootResp.Data.Complete {
		t.Fatalf("根 CA 信任链异常: %+v", rootResp)
	}
}

func callGenerateCA(t *testing.T, app *App, body string) (int, map[string]interface{}) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.POST("/certificate/ca/generate", func(c *gin.Context) { app.GenerateCAHandler(c, nil) })
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/certificate/ca/generate", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(rec, req)
	var resp map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	return rec.Code, resp
}

// 指定上级 CA 时应生成带 parent_id 的中间 CA。
func TestGenerateCAWithParent(t *testing.T) {
	app, dm, _ := newTestApp(t)

	root, err := certutil.GenerateCA(certutil.CertOptions{CommonName: "Root CA", Key: certutil.KeyOptions{KeyType: certutil.KeyTypeEC, ECCurve: "P256"}})
	if err != nil {
		t.Fatalf("generate root: %v", err)
	}
	rootID := saveCertPair(t, app, "root", models.CERT_TYPE_CA, 0, root)

	body := `{"name":"inter","common_name":"Intermediate CA","parent_ca_id":` + strconv.FormatUint(uint64(rootID), 10) + `}`
	code, resp := callGenerateCA(t, app, body)
	if code != 200 || resp["result"] != "success" {
		t.Fatalf("生成中间 CA 失败: %d %v", code, resp)
	}
	data, _ := resp["data"].(map[string]interface{})
	parentID, _ := data["parent_id"].(float64)
	if uint(parentID) != rootID {
		t.Fatalf("parent_id = %v, want %d", data["parent_id"], rootID)
	}

	// 库中应存在该中间 CA，且确实由根 CA 签发。
	list, err := dm.ListCertificateByType(models.CERT_TYPE_CA)
	if err != nil {
		t.Fatalf("list CA: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("CA 数量 = %d, want 2", len(list))
	}
	var inter *models.Certificate
	for _, cert := range list {
		if cert.ID == uint(parentID) {
			inter = cert
		}
	}
	if inter == nil {
		t.Fatal("未找到新建的中间 CA")
	}
	if err := certutil.ValidateSignedBy(inter.Cert, root.CertPEM); err != nil {
		t.Fatalf("中间 CA 应由根 CA 签发: %v", err)
	}
}
