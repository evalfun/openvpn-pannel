// Package certutil 提供基于 Go 标准库的 X.509 证书生成、签发、解析与校验能力，
// 以及 OpenVPN 所需的 DH 参数（RFC 3526 标准 MODP 组）生成。
// 全部使用 Go 实现，不依赖 openssl 命令。
package certutil

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"
)

const (
	KeyTypeRSA = "rsa"
	KeyTypeEC  = "ec"
)

// KeyOptions 私钥生成参数。
type KeyOptions struct {
	KeyType string // rsa / ec
	RSABits int    // 2048 / 3072 / 4096
	ECCurve string // P256 / P384 / P521
}

// CertOptions 证书生成参数。
type CertOptions struct {
	CommonName         string
	Org                string
	OrganizationalUnit string
	Country            string
	Province           string
	Locality           string
	EmailAddress       string
	Days               int
	IsCA               bool
	ServerAuth         bool
	ClientAuth         bool
	Key                KeyOptions
}

// oidEmailAddress 是 PKCS#9 emailAddress 属性的 OID（1.2.840.113549.1.9.1）。
var oidEmailAddress = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 1}

// emailAttribute 构造 emailAddress 属性，按 IA5String(Tag 22) 编码，与 openssl 一致。
func emailAttribute(email string) pkix.AttributeTypeAndValue {
	return pkix.AttributeTypeAndValue{
		Type:  oidEmailAddress,
		Value: asn1.RawValue{Tag: 22, Class: asn1.ClassUniversal, Bytes: []byte(email)},
	}
}

// FormatName 按 openssl 的展示顺序格式化证书 DN，包含 emailAddress。
func FormatName(name pkix.Name) string {
	parts := make([]string, 0, 8)
	add := func(key, value string) {
		if value != "" {
			parts = append(parts, key+"="+value)
		}
	}
	if len(name.Country) > 0 {
		add("C", name.Country[0])
	}
	if len(name.Province) > 0 {
		add("ST", name.Province[0])
	}
	if len(name.Locality) > 0 {
		add("L", name.Locality[0])
	}
	if len(name.Organization) > 0 {
		add("O", name.Organization[0])
	}
	if len(name.OrganizationalUnit) > 0 {
		add("OU", name.OrganizationalUnit[0])
	}
	add("CN", name.CommonName)
	// 解析后的 DN 属性在 Names 中（ExtraNames 仅在构造时使用），两处都查一遍。
	attrs := make([]pkix.AttributeTypeAndValue, 0, len(name.Names)+len(name.ExtraNames))
	attrs = append(attrs, name.Names...)
	attrs = append(attrs, name.ExtraNames...)
	for _, attr := range attrs {
		if attr.Type.Equal(oidEmailAddress) {
			if s, ok := attr.Value.(string); ok {
				add("emailAddress", s)
			}
			break
		}
	}
	return strings.Join(parts, ", ")
}

// CertPair 生成结果。
type CertPair struct {
	CertPEM      string
	KeyPEM       string
	KeyType      string
	SerialNumber string
	Subject      string
	Issuer       string
	NotBefore    int64
	NotAfter     int64
}

func normalizeKeyOptions(opt KeyOptions) (KeyOptions, error) {
	if opt.KeyType == "" {
		opt.KeyType = KeyTypeRSA
	}
	switch opt.KeyType {
	case KeyTypeRSA:
		if opt.RSABits == 0 {
			opt.RSABits = 2048
		}
		switch opt.RSABits {
		case 2048, 3072, 4096:
		default:
			return opt, fmt.Errorf("不支持的 RSA 位数 %d", opt.RSABits)
		}
	case KeyTypeEC:
		if opt.ECCurve == "" {
			opt.ECCurve = "P256"
		}
		switch opt.ECCurve {
		case "P256", "P384", "P521":
		default:
			return opt, fmt.Errorf("不支持的 EC 曲线 %s", opt.ECCurve)
		}
	default:
		return opt, fmt.Errorf("不支持的密钥类型 %s", opt.KeyType)
	}
	return opt, nil
}

func generateKey(opt KeyOptions) (crypto.Signer, error) {
	switch opt.KeyType {
	case KeyTypeEC:
		var curve elliptic.Curve
		switch opt.ECCurve {
		case "P384":
			curve = elliptic.P384()
		case "P521":
			curve = elliptic.P521()
		default:
			curve = elliptic.P256()
		}
		return ecdsa.GenerateKey(curve, rand.Reader)
	default:
		return rsa.GenerateKey(rand.Reader, opt.RSABits)
	}
}

func marshalPrivateKey(key crypto.Signer) (string, error) {
	switch k := key.(type) {
	case *rsa.PrivateKey:
		return string(pem.EncodeToMemory(&pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: x509.MarshalPKCS1PrivateKey(k),
		})), nil
	case *ecdsa.PrivateKey:
		der, err := x509.MarshalECPrivateKey(k)
		if err != nil {
			return "", err
		}
		return string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der})), nil
	default:
		der, err := x509.MarshalPKCS8PrivateKey(key)
		if err != nil {
			return "", err
		}
		return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})), nil
	}
}

func keyTypeName(key crypto.Signer) string {
	if _, ok := key.(*ecdsa.PrivateKey); ok {
		return KeyTypeEC
	}
	return KeyTypeRSA
}

// PrivateKeyType 返回 PEM 私钥的类型（rsa / ec）。
func PrivateKeyType(pemStr string) (string, error) {
	key, err := ParsePrivateKey(pemStr)
	if err != nil {
		return "", err
	}
	return keyTypeName(key), nil
}

func randomSerial() (*big.Int, error) {
	limit := new(big.Int).Lsh(big.NewInt(1), 128)
	serial, err := rand.Int(rand.Reader, limit)
	if err != nil {
		return nil, err
	}
	if serial.Sign() == 0 {
		serial = big.NewInt(1)
	}
	return serial, nil
}

func generateCertificate(opt CertOptions, parent *x509.Certificate, parentKey crypto.Signer) (*CertPair, error) {
	keyOpt, err := normalizeKeyOptions(opt.Key)
	if err != nil {
		return nil, err
	}
	key, err := generateKey(keyOpt)
	if err != nil {
		return nil, err
	}
	serial, err := randomSerial()
	if err != nil {
		return nil, err
	}

	days := opt.Days
	if days <= 0 {
		days = 3650
	}
	notBefore := time.Now().Add(-time.Minute)
	notAfter := notBefore.Add(time.Duration(days) * 24 * time.Hour)

	subject := pkix.Name{CommonName: opt.CommonName}
	if opt.Org != "" {
		subject.Organization = []string{opt.Org}
	}
	if opt.OrganizationalUnit != "" {
		subject.OrganizationalUnit = []string{opt.OrganizationalUnit}
	}
	if opt.Country != "" {
		subject.Country = []string{opt.Country}
	}
	if opt.Province != "" {
		subject.Province = []string{opt.Province}
	}
	if opt.Locality != "" {
		subject.Locality = []string{opt.Locality}
	}
	if opt.EmailAddress != "" {
		subject.ExtraNames = append(subject.ExtraNames, emailAttribute(opt.EmailAddress))
	}

	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               subject,
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		BasicConstraintsValid: true,
		IsCA:                  opt.IsCA,
	}
	if opt.EmailAddress != "" {
		tmpl.EmailAddresses = []string{opt.EmailAddress}
	}
	if opt.IsCA {
		tmpl.KeyUsage = x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature
	} else {
		tmpl.KeyUsage = x509.KeyUsageDigitalSignature
		if keyOpt.KeyType == KeyTypeRSA {
			tmpl.KeyUsage |= x509.KeyUsageKeyEncipherment
		}
		if opt.ServerAuth {
			tmpl.ExtKeyUsage = append(tmpl.ExtKeyUsage, x509.ExtKeyUsageServerAuth)
		}
		if opt.ClientAuth {
			tmpl.ExtKeyUsage = append(tmpl.ExtKeyUsage, x509.ExtKeyUsageClientAuth)
		}
		// 两者都不指定时不写入扩展密钥用法，表示“未指定”用途的证书。
	}

	parentCert := tmpl
	signerKey := key
	if parent != nil {
		parentCert = parent
		signerKey = parentKey
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, parentCert, key.Public(), signerKey)
	if err != nil {
		return nil, err
	}
	keyPEM, err := marshalPrivateKey(key)
	if err != nil {
		return nil, err
	}
	parsed, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, err
	}
	return &CertPair{
		CertPEM:      string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})),
		KeyPEM:       keyPEM,
		KeyType:      keyTypeName(key),
		SerialNumber: parsed.SerialNumber.Text(16),
		Subject:      FormatName(parsed.Subject),
		Issuer:       FormatName(parsed.Issuer),
		NotBefore:    parsed.NotBefore.Unix(),
		NotAfter:     parsed.NotAfter.Unix(),
	}, nil
}

// CertKeyUsage 返回证书扩展密钥用法中是否包含服务器用途与客户端用途。
// 该判定与 OpenVPN --remote-cert-tls client|server 依据的 RFC 3280 规则一致。
func CertKeyUsage(cert *x509.Certificate) (serverAuth, clientAuth bool) {
	if cert == nil {
		return false, false
	}
	for _, eku := range cert.ExtKeyUsage {
		switch eku {
		case x509.ExtKeyUsageServerAuth:
			serverAuth = true
		case x509.ExtKeyUsageClientAuth:
			clientAuth = true
		}
	}
	return serverAuth, clientAuth
}

// ExtKeyUsageNames 返回证书扩展密钥用法的可读名称列表。
func ExtKeyUsageNames(cert *x509.Certificate) []string {
	if cert == nil {
		return nil
	}
	names := make([]string, 0, len(cert.ExtKeyUsage))
	for _, eku := range cert.ExtKeyUsage {
		names = append(names, extKeyUsageName(eku))
	}
	return names
}

func extKeyUsageName(eku x509.ExtKeyUsage) string {
	switch eku {
	case x509.ExtKeyUsageAny:
		return "Any"
	case x509.ExtKeyUsageServerAuth:
		return "TLS Web Server Authentication (serverAuth)"
	case x509.ExtKeyUsageClientAuth:
		return "TLS Web Client Authentication (clientAuth)"
	case x509.ExtKeyUsageCodeSigning:
		return "Code Signing"
	case x509.ExtKeyUsageEmailProtection:
		return "E-mail Protection"
	case x509.ExtKeyUsageIPSECEndSystem:
		return "IPSec End System"
	case x509.ExtKeyUsageIPSECTunnel:
		return "IPSec Tunnel"
	case x509.ExtKeyUsageIPSECUser:
		return "IPSec User"
	case x509.ExtKeyUsageTimeStamping:
		return "Time Stamping"
	case x509.ExtKeyUsageOCSPSigning:
		return "OCSP Signing"
	case x509.ExtKeyUsageMicrosoftServerGatedCrypto:
		return "Microsoft Server Gated Crypto"
	case x509.ExtKeyUsageNetscapeServerGatedCrypto:
		return "Netscape Server Gated Crypto"
	}
	return fmt.Sprintf("Unknown(%d)", int(eku))
}

// KeyUsageNames 返回证书密钥用法的可读名称列表。
func KeyUsageNames(cert *x509.Certificate) []string {
	if cert == nil {
		return nil
	}
	ku := cert.KeyUsage
	var names []string
	if ku&x509.KeyUsageDigitalSignature != 0 {
		names = append(names, "Digital Signature")
	}
	if ku&x509.KeyUsageContentCommitment != 0 {
		names = append(names, "Content Commitment")
	}
	if ku&x509.KeyUsageKeyEncipherment != 0 {
		names = append(names, "Key Encipherment")
	}
	if ku&x509.KeyUsageDataEncipherment != 0 {
		names = append(names, "Data Encipherment")
	}
	if ku&x509.KeyUsageKeyAgreement != 0 {
		names = append(names, "Key Agreement")
	}
	if ku&x509.KeyUsageCertSign != 0 {
		names = append(names, "Certificate Sign")
	}
	if ku&x509.KeyUsageCRLSign != 0 {
		names = append(names, "CRL Sign")
	}
	if ku&x509.KeyUsageEncipherOnly != 0 {
		names = append(names, "Encipher Only")
	}
	if ku&x509.KeyUsageDecipherOnly != 0 {
		names = append(names, "Decipher Only")
	}
	return names
}

// IsCACertificate 判断证书是否可作为 CA 使用。
// 除了标准的 v3 CA（IsCA=true），还兼容未携带 BasicConstraints 扩展的旧版（v1）自签名 CA：
// 这类证书由 openssl x509 -req -signkey 生成，Go 解析后 IsCA 为 false，但仍可用来签发证书。
func IsCACertificate(cert *x509.Certificate) bool {
	if cert == nil {
		return false
	}
	if cert.IsCA {
		return true
	}
	if cert.Version == 1 && cert.Subject.String() == cert.Issuer.String() {
		return cert.CheckSignatureFrom(cert) == nil
	}
	return false
}

// GenerateCA 生成自签名 CA 证书。
func GenerateCA(opt CertOptions) (*CertPair, error) {
	opt.IsCA = true
	return generateCertificate(opt, nil, nil)
}

// SignCert 使用指定 CA 签发服务器/客户端证书。
func SignCert(caCertPEM, caKeyPEM string, opt CertOptions) (*CertPair, error) {
	caCert, err := ParseCertificate(caCertPEM)
	if err != nil {
		return nil, fmt.Errorf("解析 CA 证书失败: %w", err)
	}
	if !IsCACertificate(caCert) {
		return nil, errors.New("指定的证书不是 CA 证书")
	}
	caKey, err := ParsePrivateKey(caKeyPEM)
	if err != nil {
		return nil, fmt.Errorf("解析 CA 私钥失败: %w", err)
	}
	if err := KeyMatchesCert(caCertPEM, caKeyPEM); err != nil {
		return nil, fmt.Errorf("CA 证书与私钥不匹配: %w", err)
	}
	return generateCertificate(opt, caCert, caKey)
}

// ParseCertificate 解析 PEM 证书（取第一个 CERTIFICATE 块）。
func ParseCertificate(pemStr string) (*x509.Certificate, error) {
	block, _ := pem.Decode([]byte(strings.TrimSpace(pemStr)))
	if block == nil {
		return nil, errors.New("证书不是有效的 PEM 格式")
	}
	if block.Type != "CERTIFICATE" {
		return nil, fmt.Errorf("PEM 块类型为 %s，不是证书", block.Type)
	}
	return x509.ParseCertificate(block.Bytes)
}

// FingerprintSHA256 返回证书 DER 编码的 SHA-256 指纹（小写十六进制，不带分隔符）。
func FingerprintSHA256(pemStr string) (string, error) {
	cert, err := ParseCertificate(pemStr)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(cert.Raw)
	return hex.EncodeToString(sum[:]), nil
}

// PublicKeySHA256 返回证书公钥 SPKI(DER) 的 SHA-256 指纹（小写十六进制，不带分隔符）。
func PublicKeySHA256(pemStr string) (string, error) {
	cert, err := ParseCertificate(pemStr)
	if err != nil {
		return "", err
	}
	spki, err := x509.MarshalPKIXPublicKey(cert.PublicKey)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(spki)
	return hex.EncodeToString(sum[:]), nil
}

// ParsePrivateKey 解析 PEM 私钥（支持 PKCS#1 / SEC1 / PKCS#8）。
func ParsePrivateKey(pemStr string) (crypto.Signer, error) {
	block, _ := pem.Decode([]byte(strings.TrimSpace(pemStr)))
	if block == nil {
		return nil, errors.New("私钥不是有效的 PEM 格式")
	}
	if k, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return k, nil
	}
	if k, err := x509.ParseECPrivateKey(block.Bytes); err == nil {
		return k, nil
	}
	if k, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		signer, ok := k.(crypto.Signer)
		if !ok {
			return nil, errors.New("不支持的私钥类型")
		}
		return signer, nil
	}
	return nil, errors.New("无法解析私钥")
}

// KeyMatchesCert 校验私钥与证书是否匹配。
func KeyMatchesCert(certPEM, keyPEM string) error {
	cert, err := ParseCertificate(certPEM)
	if err != nil {
		return err
	}
	key, err := ParsePrivateKey(keyPEM)
	if err != nil {
		return err
	}
	certPub, err := x509.MarshalPKIXPublicKey(cert.PublicKey)
	if err != nil {
		return err
	}
	keyPub, err := x509.MarshalPKIXPublicKey(key.Public())
	if err != nil {
		return err
	}
	if !bytes.Equal(certPub, keyPub) {
		return errors.New("证书与私钥不匹配")
	}
	return nil
}

// ValidateSignedBy 校验 certPEM 是否由 caPEM 签发。
func ValidateSignedBy(certPEM, caPEM string) error {
	cert, err := ParseCertificate(certPEM)
	if err != nil {
		return err
	}
	ca, err := ParseCertificate(caPEM)
	if err != nil {
		return err
	}
	if !IsCACertificate(ca) {
		return errors.New("指定的上级证书不是 CA 证书")
	}
	if err := cert.CheckSignatureFrom(ca); err != nil {
		return errors.New("证书不是由该 CA 签发: " + err.Error())
	}
	return nil
}

type pkcs3DHParameters struct {
	Prime *big.Int
	Base  *big.Int
}

// GenerateDHParams 输出 OpenVPN 可用的 DH PARAMETERS PEM。
// bits 支持 2048 / 3072 / 4096，使用 RFC 3526 标准 MODP 组（生成器为 2）。
func GenerateDHParams(bits int) (string, error) {
	var hexStr string
	switch bits {
	case 2048:
		hexStr = modp2048Hex
	case 3072:
		hexStr = modp3072Hex
	case 4096:
		hexStr = modp4096Hex
	default:
		return "", fmt.Errorf("不支持的 DH 位数 %d（可选 2048/3072/4096）", bits)
	}
	prime := new(big.Int)
	if _, ok := prime.SetString(hexStr, 16); !ok {
		return "", errors.New("内置 MODP 参数无效")
	}
	der, err := asn1.Marshal(pkcs3DHParameters{Prime: prime, Base: big.NewInt(2)})
	if err != nil {
		return "", err
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "DH PARAMETERS", Bytes: der})), nil
}
