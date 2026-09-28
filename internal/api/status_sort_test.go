package api

import "testing"

// TestParseConnectedSince 验证 OpenVPN 状态时间字符串的解析。
func TestParseConnectedSince(t *testing.T) {
	// 越靠后的时间应得到越大的 Unix 秒。
	ordered := []string{
		"Mon Sep 28 09:00:00 2026",
		"Mon Sep 28 10:00:00 2026",
		"Mon Sep 28 11:00:00 2026",
	}
	var prev int64 = -1
	for _, s := range ordered {
		v := parseConnectedSince(s)
		if v == 0 {
			t.Fatalf("应能解析 %q", s)
		}
		if v <= prev {
			t.Fatalf("%q 解析值 %d 未大于前一个 %d", s, v, prev)
		}
		prev = v
	}

	// 空格填充个位数日期也应能解析。
	if parseConnectedSince("Mon Sep  8 10:00:00 2026") == 0 {
		t.Fatalf("空格填充日期应可解析")
	}
	// 纯 Unix 秒兜底。
	if got := parseConnectedSince("1790488608"); got != 1790488608 {
		t.Fatalf("Unix 秒兜底 = %d, 期望 1790488608", got)
	}
	// 空与非法输入返回 0。
	for _, s := range []string{"", "   ", "not a time"} {
		if got := parseConnectedSince(s); got != 0 {
			t.Fatalf("%q 应返回 0, 得到 %d", s, got)
		}
	}
}

// TestSortClientListByConnectedSince 验证按连接时间升序排序，未知时间排在最后。
func TestSortClientListByConnectedSince(t *testing.T) {
	list := []*ServerStatusClientInfoResponse{
		{CommonName: "newest", ConnectedSince: "Mon Sep 28 12:00:00 2026"},
		{CommonName: "unknown", ConnectedSince: ""},
		{CommonName: "oldest", ConnectedSince: "Mon Sep 28 08:00:00 2026"},
		{CommonName: "middle", ConnectedSince: "Mon Sep 28 10:00:00 2026"},
	}
	sortClientListByConnectedSince(list)

	want := []string{"oldest", "middle", "newest", "unknown"}
	for i, c := range list {
		if c.CommonName != want[i] {
			t.Fatalf("第 %d 个 = %q, 期望 %q（完整顺序 %v）", i, c.CommonName, want[i], names(list))
		}
	}
}

func names(list []*ServerStatusClientInfoResponse) []string {
	out := make([]string, 0, len(list))
	for _, c := range list {
		out = append(out, c.CommonName)
	}
	return out
}
