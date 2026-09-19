package certutil

import (
	"crypto/x509/pkix"
	"os"
	"testing"
)

func TestGenerateAndSign(t *testing.T) {
	ca, err := GenerateCA(CertOptions{CommonName: "Test CA", Days: 365, Key: KeyOptions{KeyType: KeyTypeEC, ECCurve: "P256"}})
	if err != nil {
		t.Fatalf("generate CA: %v", err)
	}
	srv, err := SignCert(ca.CertPEM, ca.KeyPEM, CertOptions{
		CommonName: "server", Days: 365, ServerAuth: true, ClientAuth: true,
		Key: KeyOptions{KeyType: KeyTypeRSA, RSABits: 2048},
	})
	if err != nil {
		t.Fatalf("sign server cert: %v", err)
	}
	if err := ValidateSignedBy(srv.CertPEM, ca.CertPEM); err != nil {
		t.Fatalf("validate signed by: %v", err)
	}
	if err := KeyMatchesCert(srv.CertPEM, srv.KeyPEM); err != nil {
		t.Fatalf("key matches cert: %v", err)
	}
	if srv.KeyType != KeyTypeRSA {
		t.Fatalf("expected rsa key, got %s", srv.KeyType)
	}
}

func TestFingerprints(t *testing.T) {
	ca, err := GenerateCA(CertOptions{CommonName: "Fingerprint CA", Days: 365, Key: KeyOptions{KeyType: KeyTypeRSA, RSABits: 2048}})
	if err != nil {
		t.Fatalf("generate CA: %v", err)
	}
	certFP, err := FingerprintSHA256(ca.CertPEM)
	if err != nil {
		t.Fatalf("cert fingerprint: %v", err)
	}
	if len(certFP) != 64 {
		t.Fatalf("expected 64 hex chars for cert fingerprint, got %d", len(certFP))
	}
	pubFP, err := PublicKeySHA256(ca.CertPEM)
	if err != nil {
		t.Fatalf("public key fingerprint: %v", err)
	}
	if len(pubFP) != 64 {
		t.Fatalf("expected 64 hex chars for public key fingerprint, got %d", len(pubFP))
	}
	if certFP == pubFP {
		t.Fatalf("certificate and public key fingerprints should differ")
	}
}

func TestValidateSignedByRejectsForeignCA(t *testing.T) {
	ca1, _ := GenerateCA(CertOptions{CommonName: "CA1", Key: KeyOptions{KeyType: KeyTypeRSA, RSABits: 2048}})
	ca2, _ := GenerateCA(CertOptions{CommonName: "CA2", Key: KeyOptions{KeyType: KeyTypeRSA, RSABits: 2048}})
	leaf, err := SignCert(ca1.CertPEM, ca1.KeyPEM, CertOptions{CommonName: "leaf", ClientAuth: true, Key: KeyOptions{KeyType: KeyTypeRSA, RSABits: 2048}})
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if err := ValidateSignedBy(leaf.CertPEM, ca2.CertPEM); err == nil {
		t.Fatal("expected validation against foreign CA to fail")
	}
}

func TestGenerateDHParams(t *testing.T) {
	for _, bits := range []int{2048, 3072} {
		pem, err := GenerateDHParams(bits)
		if err != nil {
			t.Fatalf("dh %d: %v", bits, err)
		}
		os.WriteFile(t.TempDir()+"/dh.pem", []byte(pem), 0644)
	}
	if _, err := GenerateDHParams(1234); err == nil {
		t.Fatal("expected error for unsupported bits")
	}
}

func TestCertSubjectFields(t *testing.T) {
	pair, err := GenerateCA(CertOptions{
		CommonName:         "ovpn-server-20021",
		Org:                "ExampleOrg",
		OrganizationalUnit: "IT Dept",
		Country:            "CN",
		Province:           "JiangSu",
		Locality:           "NanJing",
		EmailAddress:       "0000@1111.222com",
		Key:                KeyOptions{KeyType: KeyTypeRSA, RSABits: 2048},
	})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	cert, err := ParseCertificate(pair.CertPEM)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cert.Subject.CommonName != "ovpn-server-20021" {
		t.Fatalf("CN mismatch: %q", cert.Subject.CommonName)
	}
	if len(cert.Subject.Country) == 0 || cert.Subject.Country[0] != "CN" {
		t.Fatalf("C mismatch: %v", cert.Subject.Country)
	}
	if len(cert.Subject.Province) == 0 || cert.Subject.Province[0] != "JiangSu" {
		t.Fatalf("ST mismatch: %v", cert.Subject.Province)
	}
	if len(cert.Subject.Locality) == 0 || cert.Subject.Locality[0] != "NanJing" {
		t.Fatalf("L mismatch: %v", cert.Subject.Locality)
	}
	if len(cert.Subject.OrganizationalUnit) == 0 || cert.Subject.OrganizationalUnit[0] != "IT Dept" {
		t.Fatalf("OU mismatch: %v", cert.Subject.OrganizationalUnit)
	}
	foundEmail := false
	allAttrs := append(append([]pkix.AttributeTypeAndValue{}, cert.Subject.Names...), cert.Subject.ExtraNames...)
	for _, attr := range allAttrs {
		if attr.Type.Equal(oidEmailAddress) {
			if s, ok := attr.Value.(string); ok && s == "0000@1111.222com" {
				foundEmail = true
			}
		}
	}
	if !foundEmail {
		t.Fatalf("emailAddress not found in subject: %v", allAttrs)
	}
	if len(cert.EmailAddresses) == 0 || cert.EmailAddresses[0] != "0000@1111.222com" {
		t.Fatalf("emailAddress SAN missing: %v", cert.EmailAddresses)
	}

	want := "C=CN, ST=JiangSu, L=NanJing, O=ExampleOrg, OU=IT Dept, CN=ovpn-server-20021, emailAddress=0000@1111.222com"
	if pair.Subject != want {
		t.Fatalf("formatted subject mismatch:\n got: %s\nwant: %s", pair.Subject, want)
	}
}

func TestLegacyV1CACertificate(t *testing.T) {
	caCertPEM, err := os.ReadFile("testdata/legacy_ca.crt")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	caKeyPEM, err := os.ReadFile("testdata/legacy_ca.key")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	caCert, err := ParseCertificate(string(caCertPEM))
	if err != nil {
		t.Fatalf("parse legacy CA: %v", err)
	}
	if caCert.Version != 1 {
		t.Fatalf("fixture is not a v1 certificate (version=%d)", caCert.Version)
	}
	if caCert.IsCA {
		t.Fatal("v1 certificate without BasicConstraints should report IsCA=false")
	}
	if !IsCACertificate(caCert) {
		t.Fatal("IsCACertificate should accept a legacy v1 self-signed CA")
	}

	// v1 CA 仍然可以签发与校验子证书
	leaf, err := SignCert(string(caCertPEM), string(caKeyPEM), CertOptions{
		CommonName: "legacy-leaf", ClientAuth: true,
		Key: KeyOptions{KeyType: KeyTypeRSA, RSABits: 2048},
	})
	if err != nil {
		t.Fatalf("sign with legacy CA: %v", err)
	}
	if err := ValidateSignedBy(leaf.CertPEM, string(caCertPEM)); err != nil {
		t.Fatalf("validate leaf against legacy CA: %v", err)
	}
	if err := KeyMatchesCert(leaf.CertPEM, leaf.KeyPEM); err != nil {
		t.Fatalf("leaf key mismatch: %v", err)
	}
}

func TestIsCACertificateAcceptsV3CA(t *testing.T) {
	leaf, err := GenerateCA(CertOptions{CommonName: "leaf-like", Key: KeyOptions{KeyType: KeyTypeRSA, RSABits: 2048}})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	// 这里生成的是 CA，确认方法对标准 v3 CA 返回 true
	cert, err := ParseCertificate(leaf.CertPEM)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !IsCACertificate(cert) {
		t.Fatal("IsCACertificate should accept a standard v3 CA")
	}
}
