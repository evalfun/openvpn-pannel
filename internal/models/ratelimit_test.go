package models

import "testing"

func TestMatchRateLimitRule(t *testing.T) {
	// 已按优先级降序排列：P3 阈值最高（禁止连接），P2 次之（限速），P1 最低（不限速）
	rules := []*RateLimitRule{
		{Priority: 3, UploadThresholdBytes: 300, DownloadThresholdBytes: 300, AllowConnect: false},
		{Priority: 2, UploadThresholdBytes: 200, DownloadThresholdBytes: 200, LimitUploadKB: 1000, AllowConnect: true},
		{Priority: 1, UploadThresholdBytes: 100, DownloadThresholdBytes: 100, AllowConnect: true},
	}
	cases := []struct {
		name     string
		up, down uint64
		wantIdx  int // -1 表示未触发任何规则
	}{
		{"none_triggered", 50, 80, -1},
		{"lowest_by_upload", 150, 80, 2},
		{"lowest_by_download", 80, 150, 2},
		{"middle_by_upload", 250, 80, 1},
		{"middle_by_download", 80, 250, 1},
		{"highest_by_upload", 350, 10, 0},
		{"highest_by_download", 10, 350, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := MatchRateLimitRule(rules, tc.up, tc.down)
			if tc.wantIdx < 0 {
				if got != nil {
					t.Fatalf("expected nil, got priority=%d", got.Priority)
				}
				return
			}
			if got == nil {
				t.Fatalf("MatchRateLimitRule returned nil")
			}
			if got.Priority != rules[tc.wantIdx].Priority {
				t.Fatalf("matched priority = %d, want %d", got.Priority, rules[tc.wantIdx].Priority)
			}
		})
	}
	if got := MatchRateLimitRule(nil, 1, 1); got != nil {
		t.Fatalf("empty rules should return nil, got %+v", got)
	}
}
