// Package totp 实现 RFC 6238 的 TOTP（基于时间的一次性密码），
// 仅依赖标准库：HMAC-SHA1 + Base32 密钥，6 位数字、30 秒步长。
// 不引入第三方依赖，便于在离线 / OpenWrt 环境构建。
package totp

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/skip2/go-qrcode"
)

const (
	// secretBytes 生成密钥的随机字节数（160 bit，RFC 6238 推荐）。
	secretBytes = 20
	// Period TOTP 步长（秒）。
	Period = 30
	// Digits 验证码位数。
	Digits = 6
	// drift 允许前后漂移的时间窗口数，兼容客户端与服务器时钟偏差。
	drift = 1
)

var b32 = base32.StdEncoding.WithPadding(base32.NoPadding)

// GenerateSecret 生成一个新的随机 Base32 密钥（无填充、大写）。
func GenerateSecret() (string, error) {
	buf := make([]byte, secretBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return b32.EncodeToString(buf), nil
}

func normalizeSecret(secret string) string {
	s := strings.ToUpper(strings.TrimSpace(secret))
	s = strings.ReplaceAll(s, " ", "")
	s = strings.ReplaceAll(s, "-", "")
	return s
}

func decodeSecret(secret string) ([]byte, error) {
	return b32.DecodeString(normalizeSecret(secret))
}

func hotp(key []byte, counter uint64) string {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], counter)
	mac := hmac.New(sha1.New, key)
	mac.Write(buf[:])
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	code := (uint32(sum[offset])&0x7f)<<24 |
		uint32(sum[offset+1])<<16 |
		uint32(sum[offset+2])<<8 |
		uint32(sum[offset+3])
	return fmt.Sprintf("%0*d", Digits, code%1000000)
}

// Generate 计算 secret 在时刻 t 的验证码。
func Generate(secret string, t time.Time) (string, error) {
	key, err := decodeSecret(secret)
	if err != nil {
		return "", err
	}
	return hotp(key, uint64(t.Unix()/Period)), nil
}

// Validate 校验 code 是否为 secret 在 t 附近的有效验证码（允许 ±drift 个窗口）。
func Validate(secret, code string, t time.Time) bool {
	code = strings.TrimSpace(code)
	if len(code) != Digits {
		return false
	}
	for _, r := range code {
		if r < '0' || r > '9' {
			return false
		}
	}
	key, err := decodeSecret(secret)
	if err != nil {
		return false
	}
	base := int64(t.Unix() / Period)
	for delta := int64(-drift); delta <= drift; delta++ {
		counter := base + delta
		if counter < 0 {
			continue
		}
		if hmac.Equal([]byte(hotp(key, uint64(counter))), []byte(code)) {
			return true
		}
	}
	return false
}

// ProvisioningURI 生成供认证器 App 扫描/手动录入的 otpauth:// URI。
func ProvisioningURI(issuer, account, secret string) string {
	label := issuer + ":" + account
	v := url.Values{}
	v.Set("secret", normalizeSecret(secret))
	v.Set("issuer", issuer)
	v.Set("algorithm", "SHA1")
	v.Set("digits", fmt.Sprintf("%d", Digits))
	v.Set("period", fmt.Sprintf("%d", Period))
	return "otpauth://totp/" + url.PathEscape(label) + "?" + v.Encode()
}

// QRDataURI 把任意内容（通常是 otpauth URI）渲染成 PNG 的 data URI，
// 前端可直接放到 <img src="..."> 中显示，无需额外的二维码 JS 库。
func QRDataURI(content string) (string, error) {
	png, err := qrcode.Encode(content, qrcode.Medium, 256)
	if err != nil {
		return "", err
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(png), nil
}
