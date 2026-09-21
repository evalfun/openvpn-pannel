package api

import (
	"strings"
	"testing"
)

func TestParseBatchUserCSV(t *testing.T) {
	text := "用户名,密码,用户备注,用户组\n" +
		"u1,p1,备注一,vip\n" +
		"u2,p2,,\n" +
		"\n" +
		"u3,p3\n"
	rows, err := parseBatchUserCSV(text)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d: %+v", len(rows), rows)
	}
	if rows[0].Line != 2 || rows[0].Username != "u1" || rows[0].Password != "p1" || rows[0].Description != "备注一" || rows[0].GroupName != "vip" {
		t.Fatalf("row0 mismatch: %+v", rows[0])
	}
	if rows[1].Username != "u2" || rows[1].Description != "" || rows[1].GroupName != "" {
		t.Fatalf("row1 mismatch: %+v", rows[1])
	}
	// 第三行只有用户名/密码，缺少备注与用户组列，应解析为空
	if rows[2].Username != "u3" || rows[2].Description != "" || rows[2].GroupName != "" {
		t.Fatalf("row2 mismatch: %+v", rows[2])
	}
}

func TestParseBatchUserCSVWithBOMAndEnglishHeader(t *testing.T) {
	rows, err := parseBatchUserCSV("\ufeffusername,password,description,group\nbob,pw,hello,g1\n")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(rows) != 1 || rows[0].Username != "bob" || rows[0].Description != "hello" || rows[0].GroupName != "g1" {
		t.Fatalf("unexpected rows: %+v", rows)
	}
}

func TestParseBatchUserCSVMissingRequiredColumn(t *testing.T) {
	_, err := parseBatchUserCSV("用户名,用户组\nu1,vip\n")
	if err == nil || !strings.Contains(err.Error(), "密码") {
		t.Fatalf("expected missing password column error, got %v", err)
	}
}

func TestValidateBatchUserRow(t *testing.T) {
	longName := strings.Repeat("a", 51)
	longDesc := strings.Repeat("d", 501)
	cases := []struct {
		name string
		row  batchUserRow
		want string
	}{
		{"ok", batchUserRow{Username: "u", Password: "p"}, ""},
		{"ok with desc and group", batchUserRow{Username: "u", Password: "p", Description: "备注", GroupName: "g"}, ""},
		{"empty username", batchUserRow{Password: "p"}, "用户名为空"},
		{"empty password", batchUserRow{Username: "u"}, "密码为空"},
		{"long username", batchUserRow{Username: longName, Password: "p"}, "用户名长度不能超过 50 个字符"},
		{"long description", batchUserRow{Username: "u", Password: "p", Description: longDesc}, "用户备注长度不能超过 500 个字符"},
	}
	for _, tc := range cases {
		if got := validateBatchUserRow(tc.row); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}
