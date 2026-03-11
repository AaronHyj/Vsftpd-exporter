package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestIsValidHost(t *testing.T) {
	tests := []struct {
		name     string
		host     string
		expected bool
	}{
		{"有效的IPv4地址", "192.168.1.1", true},
		{"有效的IPv6地址", "2001:0db8:85a3:0000:0000:8a2e:0370:7334", true},
		{"有效的域名", "example.com", true},
		{"有效的子域名", "ftp.example.com", true},
		{"无效的空字符串", "", false},
		{"无效的域名（过长）", string(make([]byte, 300)), false},
		{"无效的特殊字符", "example@com", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidHost(tt.host)
			if result != tt.expected {
				t.Errorf("isValidHost(%q) = %v, 期望 %v", tt.host, result, tt.expected)
			}
		})
	}
}

func TestIsValidUsername(t *testing.T) {
	tests := []struct {
		name     string
		username string
		expected bool
	}{
		{"有效的用户名（字母）", "testuser", true},
		{"有效的用户名（字母数字）", "user123", true},
		{"有效的用户名（下划线）", "test_user", true},
		{"有效的用户名（连字符）", "test-user", true},
		{"无效的空字符串", "", false},
		{"无效的特殊字符", "user@test", false},
		{"无效的空格", "test user", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidUsername(tt.username)
			if result != tt.expected {
				t.Errorf("isValidUsername(%q) = %v, 期望 %v", tt.username, result, tt.expected)
			}
		})
	}
}

func TestParseStandardXferlog(t *testing.T) {
	tests := []struct {
		name              string
		line              string
		expectedDirection string
		expectedClientIP  string
		expectedFileSize  int64
		expectedCompleted bool
	}{
		{
			name:              "上传完成",
			line:              "Wed Oct 15 16:04:42 2025 1 172.25.235.63 19236361 /txt/yd_platform.txt b _ i g dstore ftp 0 * c",
			expectedDirection: "i",
			expectedClientIP:  "172.25.235.63",
			expectedFileSize:  19236361,
			expectedCompleted: true,
		},
		{
			name:              "下载完成",
			line:              "Wed Oct 15 16:04:42 2025 2 192.168.1.100 1024 /data/file.txt b _ o g testuser ftp 0 * c",
			expectedDirection: "o",
			expectedClientIP:  "192.168.1.100",
			expectedFileSize:  1024,
			expectedCompleted: true,
		},
		{
			name:              "传输未完成",
			line:              "Wed Oct 15 16:04:42 2025 1 172.25.235.63 19236361 /txt/yd_platform.txt b _ i g dstore ftp 0 * i",
			expectedDirection: "i",
			expectedClientIP:  "172.25.235.63",
			expectedFileSize:  19236361,
			expectedCompleted: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, direction, clientIP, fileSize, _, _, _, completed := parseStandardXferlog(tt.line)

			if direction != tt.expectedDirection {
				t.Errorf("方向 = %q, 期望 %q", direction, tt.expectedDirection)
			}
			if clientIP != tt.expectedClientIP {
				t.Errorf("客户端IP = %q, 期望 %q", clientIP, tt.expectedClientIP)
			}
			if fileSize != tt.expectedFileSize {
				t.Errorf("文件大小 = %d, 期望 %d", fileSize, tt.expectedFileSize)
			}
			if completed != tt.expectedCompleted {
				t.Errorf("完成状态 = %v, 期望 %v", completed, tt.expectedCompleted)
			}
		})
	}
}

func TestParseVsftpdTimestamp(t *testing.T) {
	tests := []struct {
		name      string
		timeStr   string
		shouldErr bool
	}{
		{"有效的单数字日期", "Wed Oct  6 10:58:33 2025", false},
		{"有效的双数字日期", "Wed Oct 16 10:58:33 2025", false},
		{"有效的标准格式", "Mon Jan 2 15:04:05 2006", false},
		{"无效的格式", "2025-10-15 16:04:42", true},
		{"空字符串", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseVsftpdTimestamp(tt.timeStr)
			if (err != nil) != tt.shouldErr {
				t.Errorf("parseVsftpdTimestamp(%q) 错误 = %v, 期望错误 = %v", tt.timeStr, err, tt.shouldErr)
			}
		})
	}
}

func TestExpandLogFilePath(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		shouldErr bool
	}{
		{"绝对路径", "/var/log/xferlog", false},
		{"相对路径", "./log/test.log", false},
		{"空路径", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := expandLogFilePath(tt.path)
			if (err != nil) != tt.shouldErr {
				t.Errorf("expandLogFilePath(%q) 错误 = %v, 期望错误 = %v", tt.path, err, tt.shouldErr)
			}
			if !tt.shouldErr && result == "" {
				t.Errorf("expandLogFilePath(%q) 返回空字符串", tt.path)
			}
		})
	}
}

func TestCheckLogFileAccess(t *testing.T) {
	// 创建临时测试文件
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.log")

	// 创建测试文件
	if err := os.WriteFile(testFile, []byte("test content"), 0644); err != nil {
		t.Fatalf("创建测试文件失败: %v", err)
	}

	tests := []struct {
		name      string
		path      string
		shouldErr bool
	}{
		{"存在的文件", testFile, false},
		{"不存在的文件", "/nonexistent/path/file.log", true},
		{"空路径", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkLogFileAccess(tt.path)
			if (err != nil) != tt.shouldErr {
				t.Errorf("checkLogFileAccess(%q) 错误 = %v, 期望错误 = %v", tt.path, err, tt.shouldErr)
			}
		})
	}
}

func TestExtractTimestamp(t *testing.T) {
	tests := []struct {
		name string
		line string
	}{
		{"标准格式", "2025-10-15 16:04:42 [INFO] Test message"},
		{"syslog格式", "Wed Oct 15 16:04:42 2025 [INFO] Test message"},
		{"无时间戳", "This is a log line without timestamp"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			timestamp := extractTimestamp(tt.line)
			if timestamp <= 0 {
				t.Errorf("extractTimestamp(%q) 返回无效时间戳: %d", tt.line, timestamp)
			}
			// 检查时间戳是否在合理范围内（不应该是1970年或未来很远）
			now := time.Now().Unix()
			if timestamp < 946684800 || timestamp > now+86400 { // 2000-01-01 到 明天
				t.Errorf("extractTimestamp(%q) 返回不合理的时间戳: %d", tt.line, timestamp)
			}
		})
	}
}

func BenchmarkParseStandardXferlog(b *testing.B) {
	line := "Wed Oct 15 16:04:42 2025 1 172.25.235.63 19236361 /txt/yd_platform.txt b _ i g dstore ftp 0 * c"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		parseStandardXferlog(line)
	}
}

func BenchmarkIsValidHost(b *testing.B) {
	host := "ftp.example.com"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		isValidHost(host)
	}
}
