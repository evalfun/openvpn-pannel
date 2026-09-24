// Package passwd 提供面板用户密码的哈希与校验。
//
// 新密码使用 bcrypt 存储；为兼容历史数据，仍可校验旧版的
// sha256(password + 全局盐) 十六进制摘要，调用方在校验成功后应把哈希升级为 bcrypt。
package passwd

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

// bcryptCost 为 bcrypt 的计算强度（默认 10）。
const bcryptCost = bcrypt.DefaultCost

// IsBcrypt 判断哈希是否为 bcrypt 格式（$2a$/$2b$/$2y$ 前缀）。
func IsBcrypt(hash string) bool {
	return strings.HasPrefix(hash, "$2a$") ||
		strings.HasPrefix(hash, "$2b$") ||
		strings.HasPrefix(hash, "$2y$")
}

// bcryptInput 把任意长度的密码转换为固定 44 字节的 bcrypt 输入：先 SHA-256 再 base64。
// bcrypt 只使用输入的前 72 字节，先做定长摘要可支持超过 72 字节的密码而不被静默截断。
func bcryptInput(password string) []byte {
	sum := sha256.Sum256([]byte(password))
	return []byte(base64.StdEncoding.EncodeToString(sum[:]))
}

// Hash 生成密码的 bcrypt 哈希。
func Hash(password string) (string, error) {
	h, err := bcrypt.GenerateFromPassword(bcryptInput(password), bcryptCost)
	if err != nil {
		return "", err
	}
	return string(h), nil
}

// VerifyBcrypt 校验密码与 bcrypt 哈希是否匹配。
func VerifyBcrypt(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), bcryptInput(password)) == nil
}

// VerifyLegacySHA256 校验旧版哈希：hex(sha256(password + salt))。使用常量时间比较。
func VerifyLegacySHA256(hash, password, salt string) bool {
	sum := sha256.Sum256([]byte(password + salt))
	expected := hex.EncodeToString(sum[:])
	return subtle.ConstantTimeCompare([]byte(hash), []byte(expected)) == 1
}
