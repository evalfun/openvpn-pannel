package api

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"openvpn-pannel/internal/certutil"
	"openvpn-pannel/internal/config"
	"openvpn-pannel/internal/dao"
	"openvpn-pannel/internal/models"
	ovpnserver "openvpn-pannel/internal/ovpn_server"
)

func newTestApp(t *testing.T) (*App, *dao.DaoManager, string) {
	t.Helper()
	dir := t.TempDir()
	cfg := &config.Config{
		SQLiteDB:     filepath.Join(dir, "test.db"),
		PasswordSalt: "testsalt",
	}
	dm, err := dao.NewDaoManager(cfg)
	if err != nil {
		t.Fatalf("new dao manager: %v", err)
	}
	if err := models.MigrateDB(dm.DB); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return &App{daoManager: dm}, dm, dir
}

func TestResolveCertReferenceWritesFiles(t *testing.T) {
	app, dm, dir := newTestApp(t)

	pair, err := certutil.GenerateCA(certutil.CertOptions{CommonName: "Ref CA", Key: certutil.KeyOptions{KeyType: certutil.KeyTypeRSA, RSABits: 2048}})
	if err != nil {
		t.Fatalf("generate CA: %v", err)
	}
	cert := &models.Certificate{
		Name: "ref-ca", Type: models.CERT_TYPE_CA,
		Cert: pair.CertPEM, Key: pair.KeyPEM, KeyType: pair.KeyType,
	}
	if err := dm.CreateCertificate(cert); err != nil {
		t.Fatalf("create cert: %v", err)
	}

	refCert := models.CERT_REF_PREFIX + "1/cert"
	refKey := models.CERT_REF_PREFIX + "1/key"
	server := &models.Server{Name: "s", CA: refCert, Cert: refCert, Key: refKey, Proto: "udp", Port: 1194, Dev: "tun0"}

	resolved := app.resolvedServerModel(server)
	if resolved.CA != pair.CertPEM {
		t.Fatalf("CA ref not resolved")
	}
	if resolved.Key != pair.KeyPEM {
		t.Fatalf("key ref not resolved")
	}
	if server.CA != refCert {
		t.Fatalf("original server model must not be mutated")
	}

	workdir := filepath.Join(dir, "workdir") + string(os.PathSeparator)
	ins := ovpnserver.NewOpenVPNServerInstance(resolved, nil, nil, workdir, "", "")
	if err := ins.WriteConfig(map[string]string{}); err != nil {
		t.Fatalf("write config: %v", err)
	}
	misc, err := ovpnserver.ParseMiscConfig(map[string]string{})
	if err != nil {
		t.Fatalf("parse misc: %v", err)
	}
	caData, err := os.ReadFile(filepath.Join(workdir, misc.CAFileName))
	if err != nil {
		t.Fatalf("read ca file: %v", err)
	}
	if strings.TrimSpace(string(caData)) != strings.TrimSpace(pair.CertPEM) {
		t.Fatalf("ca file does not contain resolved certificate")
	}
}

func TestResolveCertReferencePassthroughAndMissing(t *testing.T) {
	app, _, _ := newTestApp(t)
	literal := "-----BEGIN CERTIFICATE-----\nX\n-----END CERTIFICATE-----\n"
	if got := app.resolveCertReference(literal); got != literal {
		t.Fatalf("literal value should pass through unchanged")
	}
	if got := app.resolveCertReference(""); got != "" {
		t.Fatalf("empty value should stay empty")
	}
	if got := app.resolveCertReference(models.CERT_REF_PREFIX + "999/cert"); got != "" {
		t.Fatalf("missing reference should resolve to empty, got %q", got)
	}
}

func TestCertDetailHasKeyJSON(t *testing.T) {
	withKey := &models.Certificate{Name: "ca", Type: models.CERT_TYPE_CA, Cert: "CERT", Key: "KEY"}
	raw, err := json.Marshal(certDetail{Certificate: *withKey, HasKey: withKey.HasKey()})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(raw), `"has_key":true`) {
		t.Fatalf("expected has_key=true in JSON, got %s", raw)
	}
	if !strings.Contains(string(raw), `"key":"KEY"`) {
		t.Fatalf("expected key content in JSON, got %s", raw)
	}

	noKey := &models.Certificate{Name: "ca2", Type: models.CERT_TYPE_CA, Cert: "CERT"}
	raw, err = json.Marshal(certDetail{Certificate: *noKey, HasKey: noKey.HasKey()})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(raw), `"has_key":false`) {
		t.Fatalf("expected has_key=false in JSON, got %s", raw)
	}
}
