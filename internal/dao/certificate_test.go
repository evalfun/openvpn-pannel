package dao

import (
	"testing"

	"openvpn-pannel/internal/models"
)

func TestCountChildCertificateByType(t *testing.T) {
	dm := newTestDaoManager(t)
	ca := &models.Certificate{Name: "ca", Type: models.CERT_TYPE_CA, Cert: "C", Key: "K"}
	if err := dm.CreateCertificate(ca); err != nil {
		t.Fatalf("create CA: %v", err)
	}
	for _, name := range []string{"s1", "s2"} {
		if err := dm.CreateCertificate(&models.Certificate{Name: name, Type: models.CERT_TYPE_SERVER, ParentID: ca.ID, Cert: "C"}); err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
	}
	if err := dm.CreateCertificate(&models.Certificate{Name: "c1", Type: models.CERT_TYPE_CLIENT, ParentID: ca.ID, Cert: "C"}); err != nil {
		t.Fatalf("create c1: %v", err)
	}

	serverCount, err := dm.CountChildCertificateByType(ca.ID, models.CERT_TYPE_SERVER)
	if err != nil {
		t.Fatalf("count server: %v", err)
	}
	if serverCount != 2 {
		t.Fatalf("expected 2 server certs under CA, got %d", serverCount)
	}
	clientCount, err := dm.CountChildCertificateByType(ca.ID, models.CERT_TYPE_CLIENT)
	if err != nil {
		t.Fatalf("count client: %v", err)
	}
	if clientCount != 1 {
		t.Fatalf("expected 1 client cert under CA, got %d", clientCount)
	}
}

func TestListCertificateQuery(t *testing.T) {
	dm := newTestDaoManager(t)

	ca := &models.Certificate{Name: "ca", Type: models.CERT_TYPE_CA, Cert: "C", Key: "K"}
	if err := dm.CreateCertificate(ca); err != nil {
		t.Fatalf("create CA: %v", err)
	}
	mk := func(name string, certType uint, parent uint, key string) {
		if err := dm.CreateCertificate(&models.Certificate{
			Name: name, Type: certType, ParentID: parent, Cert: "C", Key: key,
		}); err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
	}
	// CA(ca) 下 4 个服务器证书（其中一个无私钥）+ 1 个客户端证书
	mk("s1", models.CERT_TYPE_SERVER, ca.ID, "K")
	mk("s2", models.CERT_TYPE_SERVER, ca.ID, "K")
	mk("s3", models.CERT_TYPE_SERVER, ca.ID, "K")
	mk("s-nokey", models.CERT_TYPE_SERVER, ca.ID, "")
	mk("c1", models.CERT_TYPE_CLIENT, ca.ID, "K")
	mk("other-server", models.CERT_TYPE_SERVER, 0, "K")

	// type=server 且含私钥，共 4 个（ca 下 3 个 + 无关联 1 个）
	list, total, err := dm.ListCertificateQuery(CertificateListQuery{FilterType: true, CertType: models.CERT_TYPE_SERVER, OnlyWithKey: true})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if total != 4 || len(list) != 4 {
		t.Fatalf("expected 4 server certs with key, got total=%d len=%d", total, len(list))
	}

	// 叠加 parent_id 过滤，只剩 ca 下 3 个
	_, total, err = dm.ListCertificateQuery(CertificateListQuery{
		FilterType: true, CertType: models.CERT_TYPE_SERVER, FilterParent: true, ParentID: ca.ID, OnlyWithKey: true,
	})
	if err != nil {
		t.Fatalf("query parent: %v", err)
	}
	if total != 3 {
		t.Fatalf("expected 3 certs under CA, got %d", total)
	}

	// 分页：每页 2 条
	page1, total, err := dm.ListCertificateQuery(CertificateListQuery{
		FilterType: true, CertType: models.CERT_TYPE_SERVER, OnlyWithKey: true, Page: 1, PageSize: 2,
	})
	if err != nil {
		t.Fatalf("query page1: %v", err)
	}
	if total != 4 || len(page1) != 2 {
		t.Fatalf("unexpected page1: total=%d len=%d", total, len(page1))
	}
	page2, _, err := dm.ListCertificateQuery(CertificateListQuery{
		FilterType: true, CertType: models.CERT_TYPE_SERVER, OnlyWithKey: true, Page: 2, PageSize: 2,
	})
	if err != nil {
		t.Fatalf("query page2: %v", err)
	}
	if len(page2) != 2 || page1[0].ID == page2[0].ID {
		t.Fatalf("pagination did not advance: page1=%v page2=%v", page1[0].ID, page2[0].ID)
	}
}

func TestCountServerReferencingCertificate(t *testing.T) {
	dm := newTestDaoManager(t)

	cert := &models.Certificate{
		Name: "ca", Type: models.CERT_TYPE_CA, Cert: "CERT", Key: "KEY",
	}
	if err := dm.CreateCertificate(cert); err != nil {
		t.Fatalf("CreateCertificate: %v", err)
	}

	// 引用 ca / cert / key 三个字段（key 是 MySQL 保留字，需正确引用列名）
	servers := []*models.Server{
		{Name: "s-ca", CA: "cert-stor:1/cert"},
		{Name: "s-cert", Cert: "cert-stor:1/cert"},
		{Name: "s-key", Key: "cert-stor:1/key"},
		{Name: "s-other", CA: "-----BEGIN CERTIFICATE-----"},
	}
	for _, s := range servers {
		if err := dm.DB.Create(s).Error; err != nil {
			t.Fatalf("create server %s: %v", s.Name, err)
		}
	}

	count, err := dm.CountServerReferencingCertificate(1)
	if err != nil {
		t.Fatalf("CountServerReferencingCertificate: %v", err)
	}
	if count != 3 {
		t.Fatalf("expected 3 referencing servers, got %d", count)
	}

	count, err = dm.CountServerReferencingCertificate(999)
	if err != nil {
		t.Fatalf("CountServerReferencingCertificate(999): %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0 for unused certificate, got %d", count)
	}
}
