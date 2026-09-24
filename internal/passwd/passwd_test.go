package passwd

import (
	"strings"
	"testing"
)

func TestHashAndVerifyBcrypt(t *testing.T) {
	hash, err := Hash("correct horse battery staple")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if !IsBcrypt(hash) {
		t.Fatalf("hash should be bcrypt format: %q", hash)
	}
	if !VerifyBcrypt(hash, "correct horse battery staple") {
		t.Fatal("correct password should verify")
	}
	if VerifyBcrypt(hash, "wrong password") {
		t.Fatal("wrong password must not verify")
	}
}

// 超过 bcrypt 72 字节上限的密码也必须能正常哈希与校验（先 SHA-256 定长化）。
func TestHashLongPassword(t *testing.T) {
	long := strings.Repeat("p", 200)
	hash, err := Hash(long)
	if err != nil {
		t.Fatalf("hash long: %v", err)
	}
	if !VerifyBcrypt(hash, long) {
		t.Fatal("long password should verify")
	}
	// 相同前缀但结尾不同的长密码不能误判为匹配。
	if VerifyBcrypt(hash, strings.Repeat("p", 199)+"q") {
		t.Fatal("different long password must not verify")
	}
}

func TestVerifyLegacySHA256(t *testing.T) {
	// sha256("secret") == 2bb80d537b1da3e38bd30361aa855686bde0eacd7162fef6a25fe97bf527a25b
	if !VerifyLegacySHA256("2bb80d537b1da3e38bd30361aa855686bde0eacd7162fef6a25fe97bf527a25b", "secret", "") {
		t.Fatal("known sha256(secret) should verify")
	}
	// sha256("secretsalt") == f84fa2149dbb62ed4e0cf1f550d2949b33a6513d3a7707e08502511c79ccb0ee
	if !VerifyLegacySHA256("f84fa2149dbb62ed4e0cf1f550d2949b33a6513d3a7707e08502511c79ccb0ee", "secret", "salt") {
		t.Fatal("known sha256(secret+salt) should verify")
	}
	if VerifyLegacySHA256("f84fa2149dbb62ed4e0cf1f550d2949b33a6513d3a7707e08502511c79ccb0ee", "secret", "wrongsalt") {
		t.Fatal("wrong salt must not verify")
	}
}

func TestIsBcrypt(t *testing.T) {
	for _, h := range []string{"$2a$10$abcdefghijklmnopqrstuv", "$2b$10$abcdefghijklmnopqrstuv", "$2y$10$abcdefghijklmnopqrstuv"} {
		if !IsBcrypt(h) {
			t.Fatalf("%s should be recognized as bcrypt", h)
		}
	}
	if IsBcrypt("5f4dcc3b5aa765d61d8327deb882cf99") {
		t.Fatal("hex digest must not be bcrypt")
	}
}
