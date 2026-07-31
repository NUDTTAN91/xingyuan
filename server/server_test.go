package server

import "testing"

// TestCalculateSampleInterval 采样间隔计算
func TestCalculateSampleInterval(t *testing.T) {
	tests := []struct {
		name     string
		start    string
		end      string
		expected int
	}{
		{"1小时范围", "2026-07-31 10:00:00", "2026-07-31 11:00:00", 2},          // 3600/1800=2
		{"30分钟范围(不足1800点)", "2026-07-31 10:00:00", "2026-07-31 10:30:00", 1}, // 1800/1800=1
		{"24小时范围", "2026-07-31 00:00:00", "2026-08-01 00:00:00", 48},         // 86400/1800=48
		{"7天范围", "2026-07-24 00:00:00", "2026-07-31 00:00:00", 336},          // 604800/1800=336
		{"时间格式错误", "not-a-time", "2026-07-31 11:00:00", 1},
		{"结束早于开始", "2026-07-31 11:00:00", "2026-07-31 10:00:00", 1},
		{"起止相同", "2026-07-31 10:00:00", "2026-07-31 10:00:00", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calculateSampleInterval(tt.start, tt.end)
			if got != tt.expected {
				t.Errorf("calculateSampleInterval(%q, %q) = %d, 期望 %d", tt.start, tt.end, got, tt.expected)
			}
		})
	}
}

// TestIsValidContainerID 容器ID格式校验
func TestIsValidContainerID(t *testing.T) {
	tests := []struct {
		name  string
		id    string
		valid bool
	}{
		{"12位短ID", "1a2b3c4d5e6f", true},
		{"64位完整ID", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", true},
		{"大写hex", "1A2B3C4D5E6F", true},
		{"空字符串", "", false},
		{"太短", "abc123", false},
		{"超过64位", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0", false},
		{"含非hex字符", "1a2b3c4d5e6g", false},
		{"注入尝试-参数", "--privileged", false},
		{"注入尝试-空格", "abc123def456 rm", false},
		{"注入尝试-分号", "abc123def456;id", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isValidContainerID(tt.id); got != tt.valid {
				t.Errorf("isValidContainerID(%q) = %v, 期望 %v", tt.id, got, tt.valid)
			}
		})
	}
}
