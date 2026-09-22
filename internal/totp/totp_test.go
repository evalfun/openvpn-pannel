package totp

import (
	"strings"
	"testing"
	"time"
)

// RFC 6238 Appendix B 的 SHA1 测试向量（secret "12345678901234567890"）。
func TestGenerateRFC6238Vectors(t *testing.T) {
	// 20 字节 ASCII secret 的 Base32 编码
	secret := "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
	cases := []struct {
		unix int64
		want string
	}{
		{59, "94287082"},
		{1111111109, "07081804"},
		{1111111111, "14050471"},
		{1234567890, "89005924"},
		{2000000000, "69279037"},
		{20000000000, "65353130"},
	}
	for _, c := range cases {
		got, err := Generate(secret, time.Unix(c.unix, 0))
		if err != nil {
			t.Fatalf("Generate: %v", err)
		}
		// RFC 向量是 8 位，这里比较后 6 位
		if got != c.want[len(c.want)-6:] {
			t.Errorf("t=%d got=%s want=%s", c.unix, got, c.want)
		}
	}
}

func TestValidateWindowAndFormat(t *testing.T) {
	secret, err := GenerateSecret()
	if err != nil {
		t.Fatalf("GenerateSecret: %v", err)
	}
	now := time.Unix(1700000000, 0)
	code, err := Generate(secret, now)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if len(code) != 6 {
		t.Fatalf("code 长度 = %d, want 6", len(code))
	}
	if !Validate(secret, code, now) {
		t.Errorf("当前窗口校验失败")
	}
	// 前一个窗口
	if !Validate(secret, code, now.Add(Period*time.Second)) {
		t.Errorf("±1 窗口漂移校验失败")
	}
	// 超过漂移范围
	if Validate(secret, code, now.Add(3*Period*time.Second)) {
		t.Errorf("超出漂移范围不应通过")
	}
	// 非法格式
	if Validate(secret, "abcdef", now) || Validate(secret, "12345", now) {
		t.Errorf("非法格式不应通过")
	}
	// 带空格/小写的密钥也能识别
	spaced := strings.ToLower(secret[:4]) + " " + secret[4:]
	if !Validate(spaced, code, now) {
		t.Errorf("带空格/小写密钥校验失败")
	}
}

func TestProvisioningURI(t *testing.T) {
	uri := ProvisioningURI("OpenVPN管理", "alice", "abcd efgh")
	if !strings.HasPrefix(uri, "otpauth://totp/") {
		t.Fatalf("URI 前缀错误: %s", uri)
	}
	for _, want := range []string{"secret=ABCDEFGH", "issuer=", "period=30", "digits=6"} {
		if !strings.Contains(uri, want) {
			t.Errorf("URI %s 缺少 %s", uri, want)
		}
	}
}

func TestQRDataURI(t *testing.T) {
	uri := ProvisioningURI("OpenVPN管理", "alice", "ABCDEFGH")
	dataURI, err := QRDataURI(uri)
	if err != nil {
		t.Fatalf("QRDataURI: %v", err)
	}
	if !strings.HasPrefix(dataURI, "data:image/png;base64,") {
		t.Fatalf("data URI 前缀错误: %.40s", dataURI)
	}
	if len(dataURI) < 200 {
		t.Fatalf("data URI 过短，可能不是有效二维码: %d", len(dataURI))
	}
}
