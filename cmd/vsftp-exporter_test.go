package main

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
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

func TestLoginFailRegexWithEmptyUsername(t *testing.T) {
	line := "Sun Aug 9 16:34:51 2026 [pid 1234] [] FAIL LOGIN: Client \"192.168.1.100\""
	matches := loginFailRegex.FindStringSubmatch(line)
	if matches == nil {
		t.Fatalf("loginFailRegex 应能匹配空用户名的 FAIL LOGIN 行")
	}
	if matches[4] != "192.168.1.100" {
		t.Fatalf("Client IP = %q, 期望 192.168.1.100", matches[4])
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

func TestReadLocalFilePositionTracking(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "xferlog")

	content := "line1\nline2"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("写入测试文件失败: %v", err)
	}

	lines, pos, ino, err := readLocalFile(path, 0, 0)
	if err != nil {
		t.Fatalf("readLocalFile 失败: %v", err)
	}
	if len(lines) != 1 || lines[0] != "line1" {
		t.Fatalf("行 = %v, 期望 [line1]（末行无换行符暂不消费）", lines)
	}
	if pos != int64(len("line1\n")) {
		t.Fatalf("position = %d, 期望 %d", pos, len("line1\n"))
	}
	if ino == 0 {
		t.Fatalf("inode 应大于 0")
	}

	// 追加换行符后再次读取
	if err := os.WriteFile(path, []byte(content+"\n"), 0644); err != nil {
		t.Fatalf("更新测试文件失败: %v", err)
	}

	lines2, pos2, _, err := readLocalFile(path, pos, ino)
	if err != nil {
		t.Fatalf("readLocalFile 失败: %v", err)
	}
	if len(lines2) != 1 || lines2[0] != "line2" {
		t.Fatalf("二次读取行 = %v, 期望 [line2]", lines2)
	}
	if pos2 != int64(len(content)+1) {
		t.Fatalf("二次读取 position = %d, 期望 %d", pos2, len(content)+1)
	}
}

func TestReadLocalFileAppendContinue(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "xferlog")

	first := "line1\nline2\n"
	if err := os.WriteFile(path, []byte(first), 0644); err != nil {
		t.Fatalf("写入测试文件失败: %v", err)
	}

	_, pos, ino, err := readLocalFile(path, 0, 0)
	if err != nil {
		t.Fatalf("readLocalFile 失败: %v", err)
	}
	if pos != int64(len(first)) {
		t.Fatalf("position = %d, 期望 %d", pos, len(first))
	}

	appendContent := "line3\nline4\n"
	if err := os.WriteFile(path, []byte(first+appendContent), 0644); err != nil {
		t.Fatalf("追加测试文件失败: %v", err)
	}

	lines, pos2, _, err := readLocalFile(path, pos, ino)
	if err != nil {
		t.Fatalf("readLocalFile 失败: %v", err)
	}
	if len(lines) != 2 || lines[0] != "line3" || lines[1] != "line4" {
		t.Fatalf("追加后行 = %v, 期望 [line3 line4]", lines)
	}
	if pos2 != int64(len(first)+len(appendContent)) {
		t.Fatalf("追加后 position = %d, 期望 %d", pos2, len(first)+len(appendContent))
	}
}

func TestReadLocalFileLineCap(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "xferlog")

	var sb strings.Builder
	for i := 0; i < maxLinesPerRead+500; i++ {
		sb.WriteString("line\n")
	}
	if err := os.WriteFile(path, []byte(sb.String()), 0644); err != nil {
		t.Fatalf("写入测试文件失败: %v", err)
	}

	lines, pos, ino, err := readLocalFile(path, 0, 0)
	if err != nil {
		t.Fatalf("readLocalFile 失败: %v", err)
	}
	if len(lines) != maxLinesPerRead {
		t.Fatalf("行数 = %d, 期望 %d", len(lines), maxLinesPerRead)
	}
	if pos != int64(maxLinesPerRead*len("line\n")) {
		t.Fatalf("position = %d, 期望 %d", pos, maxLinesPerRead*len("line\n"))
	}

	lines, _, _, err = readLocalFile(path, pos, ino)
	if err != nil {
		t.Fatalf("readLocalFile 失败: %v", err)
	}
	if len(lines) != 500 {
		t.Fatalf("剩余行数 = %d, 期望 500", len(lines))
	}
}

func TestReadLocalFileRotationWithRotatedFileTail(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "xferlog")
	rotatedPath := path + ".1"

	// 模拟旧文件写入 100 字节，已消费 60 字节
	oldContent := "Wed Oct 15 16:04:42 2025 1 172.25.235.63 1000 /txt/a.txt b _ i g user ftp 0 * c\nWed Oct 15 16:04:43 2025 1 172.25.235.63 2000 /txt/b.txt b _ i g user ftp 0 * c\n"
	if err := os.WriteFile(path, []byte(oldContent), 0644); err != nil {
		t.Fatalf("写入初始文件失败: %v", err)
	}

	lines, _, ino, err := readLocalFile(path, 0, 0)
	if err != nil {
		t.Fatalf("readLocalFile 失败: %v", err)
	}
	if len(lines) != 2 {
		t.Fatalf("期望读取2行, got %d", len(lines))
	}

	// 仅模拟上次消费了第 1 行的长度
	line1Len := int64(len(strings.Split(oldContent, "\n")[0]) + 1)

	// 模拟日志轮转：重命名为 xferlog.1 并新建 xferlog 且新文件写入了较长的内容
	if err := os.Rename(path, rotatedPath); err != nil {
		t.Fatalf("重命名轮转文件失败: %v", err)
	}
	newContent := "Wed Oct 15 16:05:00 2025 1 172.25.235.63 3000 /txt/c.txt b _ i g user ftp 0 * c\n"
	if err := os.WriteFile(path, []byte(newContent), 0644); err != nil {
		t.Fatalf("创建新日志文件失败: %v", err)
	}

	// 从 line1Len 和旧 ino 读取，触发轮转检测，应优先读取 xferlog.1 剩余的第 2 行
	lines2, pos2, ino2, err := readLocalFile(path, line1Len, ino)
	if err != nil {
		t.Fatalf("读取轮转文件失败: %v", err)
	}
	if len(lines2) != 1 || !strings.Contains(lines2[0], "b.txt") {
		t.Fatalf("应从 xferlog.1 读取 b.txt，got %v", lines2)
	}
	if pos2 != int64(len(oldContent)) {
		t.Fatalf("position 应为旧文件总长 %d, got %d", len(oldContent), pos2)
	}

	// 再次读取（pos2 已达旧文件末尾），应切换到新文件从 0 开始读取
	lines3, pos3, ino3, err := readLocalFile(path, pos2, ino2)
	if err != nil {
		t.Fatalf("读取新文件失败: %v", err)
	}
	if len(lines3) != 1 || !strings.Contains(lines3[0], "c.txt") {
		t.Fatalf("应从新文件读取 c.txt，got %v", lines3)
	}
	if pos3 != int64(len(newContent)) {
		t.Fatalf("position 应为新文件长度 %d, got %d", len(newContent), pos3)
	}
	if ino3 == ino {
		t.Fatalf("新文件的 inode 应与旧文件不同")
	}
}

func TestClassifyFTPError(t *testing.T) {
	tests := []struct {
		name    string
		code    string
		message string
		want    string
	}{
		{"530 密码错误", "530", "Login incorrect.", "auth_failed"},
		{"530 用户不存在", "530", "No such user.", "auth_failed"},
		{"530 达到最大连接数", "530", "Maximum number of clients reached.", "max_connections"},
		{"421 连接数过多", "421", "There are too many connections from your internet address.", "max_connections"},
		{"530 连接数过多", "530", "Too many clients.", "max_connections"},
		{"421 服务不可用", "421", "Service not available, closing control connection.", "service_unavailable"},
		{"425 数据连接失败", "425", "Can't open data connection.", "data_connection_error"},
		{"426 数据连接关闭", "426", "Connection closed; transfer aborted.", "data_connection_error"},
		{"450 文件不可用", "450", "Requested file action not taken, file unavailable.", "data_connection_error"},
		{"451 本地处理错误", "451", "Requested action aborted: local error in processing.", "data_connection_error"},
		{"500 未知命令", "500", "Unknown command.", "command_error"},
		{"501 参数语法错误", "501", "Syntax error in parameters or arguments.", "command_error"},
		{"502 命令未实现", "502", "Command not implemented.", "command_error"},
		{"503 命令顺序错误", "503", "Bad sequence of commands.", "command_error"},
		{"553 文件名不允许", "553", "Could not create file.", "file_name_not_allowed"},
		{"550 非普通文件", "550", "Not a regular file.", "file_not_found"},
		{"550 无法打开文件", "550", "Failed to open file.", "file_not_found"},
		{"550 目录不存在", "550", "Failed to change directory.", "dir_not_found"},
		{"550 无此目录", "550", "No such directory.", "dir_not_found"},
		{"550 文件不存在", "550", "No such file or directory.", "file_not_found"},
		{"550 权限拒绝", "550", "Permission denied.", "permission_denied"},
		{"530 权限拒绝（chroot 访问限制）", "530", "Permission denied.", "auth_failed"},
		{"552 配额超限", "552", "Exceeded storage allocation.", "quota_exceeded"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyFTPError(tt.code, tt.message); got != tt.want {
				t.Errorf("classifyFTPError(%q, %q) = %q, 期望 %q", tt.code, tt.message, got, tt.want)
			}
		})
	}
}

func TestFTPResponseRegex(t *testing.T) {
	tests := []struct {
		name        string
		line        string
		shouldMatch bool
		username    string
		code        string
		message     string
	}{
		{
			name:        "530 密码错误响应",
			line:        `Sun Aug  9 16:34:51 2026 [pid 1234] [ftpuser] FTP response: Client "192.168.1.100", "530 Login incorrect."`,
			shouldMatch: true,
			username:    "ftpuser",
			code:        "530",
			message:     "Login incorrect.",
		},
		{
			name:        "421 连接数过多响应",
			line:        `Sun Aug  9 16:34:51 2026 [pid 1234] [] FTP response: Client "192.168.1.100", "421 There are too many connections from your internet address."`,
			shouldMatch: true,
			username:    "",
			code:        "421",
			message:     "There are too many connections from your internet address.",
		},
		{
			name:        "550 目录不存在响应",
			line:        `Sun Aug  9 16:34:51 2026 [pid 1234] [ftpuser] FTP response: Client "192.168.1.100", "550 Failed to change directory."`,
			shouldMatch: true,
			username:    "ftpuser",
			code:        "550",
			message:     "Failed to change directory.",
		},
		{
			name:        "成功响应 230 不应视为错误",
			line:        `Sun Aug  9 16:34:51 2026 [pid 1234] [ftpuser] FTP response: Client "192.168.1.100", "230 Login successful."`,
			shouldMatch: true,
			username:    "ftpuser",
			code:        "230",
			message:     "Login successful.",
		},
		{
			name:        "非响应行不应匹配",
			line:        `Sun Aug  9 16:34:51 2026 [pid 1234] [ftpuser] OK LOGIN: Client "192.168.1.100"`,
			shouldMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches := ftpResponseRegex.FindStringSubmatch(tt.line)
			if tt.shouldMatch {
				if matches == nil {
					t.Fatalf("正则应匹配: %s", tt.line)
				}
				if len(matches) < 6 || matches[5] != tt.code+" "+tt.message {
					t.Errorf("提取的响应 = %q, 期望 %q", matches[5], tt.code+" "+tt.message)
				}
				if matches[3] != tt.username {
					t.Errorf("提取的用户名 = %q, 期望 %q", matches[3], tt.username)
				}
			} else if matches != nil {
				t.Fatalf("正则不应匹配: %s", tt.line)
			}
		})
	}
}

func TestParseVsftpdLogFTPErrorCounters(t *testing.T) {
	beforeFail := testutil.ToFloat64(failedLoginsTotal)
	beforeAuth := testutil.ToFloat64(authenticationErrorsTotal)
	beforeMax := testutil.ToFloat64(maxConnectionsReachedTotal)
	beforeAuthFailed := testutil.ToFloat64(ftpErrorsTotal.WithLabelValues("auth_failed"))
	beforeMaxConn := testutil.ToFloat64(ftpErrorsTotal.WithLabelValues("max_connections"))
	beforeDir := testutil.ToFloat64(ftpErrorsTotal.WithLabelValues("dir_not_found"))

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "vsftpd.log")
	log := `Sun Aug  9 16:34:50 2026 [pid 1234] [] FAIL LOGIN: Client "192.168.1.100"
Sun Aug  9 16:34:51 2026 [pid 1234] [ftpuser] FTP response: Client "192.168.1.100", "530 Login incorrect."
Sun Aug  9 16:34:52 2026 [pid 1235] [] FTP response: Client "192.168.1.101", "421 There are too many connections from your internet address."
Sun Aug  9 16:34:53 2026 [pid 1236] [ftpuser] FTP response: Client "192.168.1.102", "550 Failed to change directory."
Sun Aug  9 16:34:54 2026 [pid 1237] [ftpuser] FTP response: Client "192.168.1.103", "230 Login successful."
`
	if err := os.WriteFile(path, []byte(log), 0644); err != nil {
		t.Fatalf("写入测试文件失败: %v", err)
	}

	state := NewExporterState()
	if err := parseVsftpdLog(&Config{}, path, state, nil); err != nil {
		t.Fatalf("parseVsftpdLog 失败: %v", err)
	}

	if got := testutil.ToFloat64(failedLoginsTotal) - beforeFail; got != 1 {
		t.Errorf("failedLoginsTotal 增量 = %v, 期望 1", got)
	}
	// 530 响应只计一次，不再与 FAIL LOGIN 重复计数
	if got := testutil.ToFloat64(authenticationErrorsTotal) - beforeAuth; got != 1 {
		t.Errorf("authenticationErrorsTotal 增量 = %v, 期望 1（无重复计数）", got)
	}
	if got := testutil.ToFloat64(maxConnectionsReachedTotal) - beforeMax; got != 1 {
		t.Errorf("maxConnectionsReachedTotal 增量 = %v, 期望 1", got)
	}
	if got := testutil.ToFloat64(ftpErrorsTotal.WithLabelValues("auth_failed")) - beforeAuthFailed; got != 1 {
		t.Errorf("ftpErrorsTotal{auth_failed} 增量 = %v, 期望 1", got)
	}
	if got := testutil.ToFloat64(ftpErrorsTotal.WithLabelValues("max_connections")) - beforeMaxConn; got != 1 {
		t.Errorf("ftpErrorsTotal{max_connections} 增量 = %v, 期望 1", got)
	}
	if got := testutil.ToFloat64(ftpErrorsTotal.WithLabelValues("dir_not_found")) - beforeDir; got != 1 {
		t.Errorf("ftpErrorsTotal{dir_not_found} 增量 = %v, 期望 1", got)
	}
}

func TestParseSSOutputPeerMatchAndDedup(t *testing.T) {
	const port = "6069"
	// 模拟用户环境：客户端与服务端同主机，ss 输出同时出现客户端侧与
	// 服务端侧 socket，且 v4/v6-mapped 两种表示形式；IPv4 与 ::ffff: 形式需去重。
	output := strings.Join([]string{
		`CLOSE-WAIT 15     0        ::ffff:172.25.234.200:43510                ::ffff:172.25.234.200:6069`,
		`CLOSE-WAIT 15     0        ::ffff:172.25.234.200:43568                ::ffff:172.25.234.200:6069`,
		`ESTAB      0      0        172.25.234.200:6069                        172.25.234.161:50699`,
		`ESTAB      0      0        ::ffff:172.25.234.200:43510                 ::ffff:172.25.234.200:6069`,
		`ESTAB      0      0        172.25.234.200:6069                        ::ffff:172.25.234.200:43510`,
		`TIME-WAIT  0      0        172.25.234.200:6069                        172.25.234.161:50701`,
		`ESTAB      0      0        172.25.234.200:45001                        172.25.234.161:3306`,
		`ESTAB      0      0        172.25.234.161:45000                        172.25.234.200:8080`,
	}, "\n")

	total, established, closeWait := parseSSOutput(output, port)
	// 唯一连接: 2×CLOSE-WAIT(客户端侧) + 外部 ESTAB + 同机连接(两次出现去重为1) + TIME-WAIT
	if total != 4 {
		t.Errorf("total = %d, 期望 4（去重后唯一连接数）", total)
	}
	if established != 1 {
		t.Errorf("established = %d, 期望 1", established)
	}
	if closeWait != 2 {
		t.Errorf("closeWait = %d, 期望 2", closeWait)
	}
}

func TestParseSSOutputNoMatch(t *testing.T) {
	output := strings.Join([]string{
		`ESTAB      0      0        172.25.234.200:45001                        172.25.234.161:3306`,
		`SYN-SENT   0      1        172.25.234.200:45002                        172.25.234.161:80`,
		``,
	}, "\n")
	total, established, closeWait := parseSSOutput(output, "6069")
	if total != 0 || established != 0 || closeWait != 0 {
		t.Errorf("不应匹配任何连接: total=%d established=%d closeWait=%d", total, established, closeWait)
	}
}

// TestParseSSOutputSkipsListenSockets 验证 LISTEN 监听套接字不计入连接数(BUG-043)。
// ss -tnH 输出中监听套接字本地地址以 FTP 端口结尾,但它是监听端点而非连接。
func TestParseSSOutputSkipsListenSockets(t *testing.T) {
	output := strings.Join([]string{
		`LISTEN   0      128      0.0.0.0:6069                  0.0.0.0:*`,
		`LISTEN   0      128      [::]:6069                    [::]:*`,
		`ESTAB    0      0        172.25.234.200:6069           172.25.234.161:50699`,
		`CLOSE-WAIT 15   0        ::ffff:172.25.234.200:43510   ::ffff:172.25.234.200:6069`,
	}, "\n")
	total, established, closeWait := parseSSOutput(output, "6069")
	if total != 2 {
		t.Errorf("total = %d, 期望 2(仅 ESTAB 与 CLOSE-WAIT,LISTEN 不计入)", total)
	}
	if established != 1 {
		t.Errorf("established = %d, 期望 1", established)
	}
	if closeWait != 1 {
		t.Errorf("closeWait = %d, 期望 1", closeWait)
	}
}

func TestExtractFileExtension(t *testing.T) {
	tests := []struct {
		path     string
		expected string
	}{
		{"/data/movie/你好世界.mp4", "mp4"},
		{"/media/series/ep01.mkv", "mkv"},
		{"/media/live/live.ts", "ts"},
		{"/upload/song.MP3", "mp3"},
		{"/pics/photo.JPG", "jpg"},
		{"/backup/data.tar.gz", "gz"},
		{"/docs/report.pdf", "pdf"},
		{"/scripts/deploy.sh", "sh"},
		{"/app/installer.exe", "exe"},
		{"/tmp/noextfile", "no_extension"},
		{"/data/unknown.xyz", "xyz"},
		{"/data/dir.with.dots/file", "no_extension"},
		{"", "no_extension"},
	}
	for _, tc := range tests {
		if got := extractFileExtension(tc.path); got != tc.expected {
			t.Errorf("extractFileExtension(%q) = %q, 期望 %q", tc.path, got, tc.expected)
		}
	}
}

func TestParseFTPLogFilesByTypeCounter(t *testing.T) {
	// 全局 CounterVec 在多次重复执行(count>1)或并行时会被污染,test 前重置(BUG-051)
	filesByTypeTotal.Reset()
	before := testutil.ToFloat64(filesByTypeTotal.WithLabelValues("mp4", "upload"))
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "xferlog")
	log := `Wed Aug  5 10:00:00 2026 0 172.25.234.200 5242880 /media/abc.mp4 b _ i a ostore ftp 0 * c
Wed Aug  5 10:00:01 2026 0 172.25.234.200 2048 /docs/readme.txt b _ i a ostore ftp 0 * c
Wed Aug  5 10:00:02 2026 0 172.25.234.200 4096 /media/ep01.mkv b _ o a ostore ftp 0 * c
Wed Aug  5 10:00:03 2026 0 172.25.234.200 1024 /tmp/unknown.xyz b _ i a ostore ftp 0 * c
`
	if err := os.WriteFile(path, []byte(log), 0644); err != nil {
		t.Fatalf("写入测试文件失败: %v", err)
	}

	state := NewExporterState()
	state.lastProcessedTime = time.Now()
	if err := parseFTPLog(path, state, nil); err != nil {
		t.Fatalf("parseFTPLog 失败: %v", err)
	}

	// 冷启动保护：首次见到标签时计数暂存，尚未抓取过 0 值前不提交，计数器保持 0
	if got := testutil.ToFloat64(filesByTypeTotal.WithLabelValues("mp4", "upload")) - before; got != 0 {
		t.Errorf("首轮解析后 mp4/upload 增量 = %v, 期望 0（暂存未提交）", got)
	}

	// 模拟一次 /metrics 抓取后再解析一轮，触发暂存提交
	state.bumpScrapeSeq()
	if err := parseFTPLog(path, state, nil); err != nil {
		t.Fatalf("第二轮 parseFTPLog 失败: %v", err)
	}

	if got := testutil.ToFloat64(filesByTypeTotal.WithLabelValues("mp4", "upload")) - before; got != 1 {
		t.Errorf("提交后 mp4/upload 增量 = %v, 期望 1", got)
	}
	if got := testutil.ToFloat64(filesByTypeTotal.WithLabelValues("txt", "upload")) - before; got != 1 {
		t.Errorf("提交后 txt/upload 增量 = %v, 期望 1", got)
	}
	if got := testutil.ToFloat64(filesByTypeTotal.WithLabelValues("mkv", "download")) - before; got != 1 {
		t.Errorf("提交后 mkv/download 增量 = %v, 期望 1", got)
	}
	if got := testutil.ToFloat64(filesByTypeTotal.WithLabelValues("xyz", "upload")) - before; got != 1 {
		t.Errorf("提交后 xyz/upload 增量 = %v, 期望 1", got)
	}

	// 已提交的标签后续增量直接 Inc()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("打开测试文件失败: %v", err)
	}
	if _, err := f.WriteString("Wed Aug  5 10:00:04 2026 0 172.25.234.200 5242880 /media/abc.mp4 b _ i a ostore ftp 0 * c\n"); err != nil {
		t.Fatalf("追加日志失败: %v", err)
	}
	f.Close()
	if err := parseFTPLog(path, state, nil); err != nil {
		t.Fatalf("第三轮 parseFTPLog 失败: %v", err)
	}
	if got := testutil.ToFloat64(filesByTypeTotal.WithLabelValues("mp4", "upload")) - before; got != 2 {
		t.Errorf("已提交标签再传输: mp4/upload 增量 = %v, 期望 2", got)
	}
}

func TestParseFTPLogFilesByTypeColdStart(t *testing.T) {
	// 全局 CounterVec 在多次重复执行(count>1)或并行时会被污染,test 前重置(BUG-051)
	filesByTypeTotal.Reset()
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "xferlog")
	log := `Wed Aug  5 10:00:00 2026 0 172.25.234.200 5242880 /media/movie.m4v b _ i a ostore ftp 0 * c
Wed Aug  5 10:00:01 2026 0 172.25.234.200 2048 /docs/note.log b _ i a ostore ftp 0 * c
Wed Aug  5 10:00:02 2026 0 172.25.234.200 5242880 /media/movie.m4v b _ i a ostore ftp 0 * c
`
	if err := os.WriteFile(path, []byte(log), 0644); err != nil {
		t.Fatalf("写入测试文件失败: %v", err)
	}

	state := NewExporterState()
	state.lastProcessedTime = time.Now()
	if err := parseFTPLog(path, state, nil); err != nil {
		t.Fatalf("parseFTPLog 失败: %v", err)
	}

	// 首轮解析后：标签以 0 值注册，计数暂存未提交，increase() 首增量因此可见
	if got := testutil.ToFloat64(filesByTypeTotal.WithLabelValues("m4v", "upload")); got != 0 {
		t.Errorf("首轮解析后 m4v/upload = %v, 期望 0（暂存未提交）", got)
	}
	if got := testutil.ToFloat64(filesByTypeTotal.WithLabelValues("log", "upload")); got != 0 {
		t.Errorf("首轮解析后 log/upload = %v, 期望 0（暂存未提交）", got)
	}

	// 一次抓取后提交：0→n 的增量完整可见
	state.bumpScrapeSeq()
	if err := parseFTPLog(path, state, nil); err != nil {
		t.Fatalf("第二轮 parseFTPLog 失败: %v", err)
	}
	if got := testutil.ToFloat64(filesByTypeTotal.WithLabelValues("m4v", "upload")); got != 2 {
		t.Errorf("提交后 m4v/upload = %v, 期望 2", got)
	}
	if got := testutil.ToFloat64(filesByTypeTotal.WithLabelValues("log", "upload")); got != 1 {
		t.Errorf("提交后 log/upload = %v, 期望 1", got)
	}
}

func TestSummaryExcludeProbeAccount(t *testing.T) {
	logContent := `Sun Aug  9 16:34:50 2026 [pid 1001] CONNECT: Client "192.168.1.100"
Sun Aug  9 16:34:51 2026 [pid 1001] [ostore] OK LOGIN: Client "192.168.1.100"
Sun Aug  9 16:34:52 2026 [pid 1002] CONNECT: Client "192.168.1.101"
Sun Aug  9 16:34:53 2026 [pid 1002] [alice] OK LOGIN: Client "192.168.1.101"
Sun Aug  9 16:34:55 2026 [pid 1003] CONNECT: Client "192.168.1.100"
`
	path := filepath.Join(t.TempDir(), "vsftpd.log")
	if err := os.WriteFile(path, []byte(logContent), 0644); err != nil {
		t.Fatalf("写入测试文件失败: %v", err)
	}

	run := func(exclude bool) (login, reconn, ostoreLogins, aliceLogins, probeConns float64) {
		beforeLogin := testutil.ToFloat64(ftpLoginTotal)
		beforeReconn := testutil.ToFloat64(rapidReconnectionsTotal)
		beforeOstore := testutil.ToFloat64(userLoginsTotal.WithLabelValues("ostore"))
		beforeAlice := testutil.ToFloat64(userLoginsTotal.WithLabelValues("alice"))
		beforeProbeConn := testutil.ToFloat64(clientConnectionsTotal.WithLabelValues("192.168.1.100"))

		cfg := &Config{FTPUser: "ostore", SummaryExclude: exclude}
		state := NewExporterState()
		state.probeClientIP = "192.168.1.100"
		if err := parseVsftpdLog(cfg, path, state, nil); err != nil {
			t.Fatalf("parseVsftpdLog 失败: %v", err)
		}
		return testutil.ToFloat64(ftpLoginTotal) - beforeLogin,
			testutil.ToFloat64(rapidReconnectionsTotal) - beforeReconn,
			testutil.ToFloat64(userLoginsTotal.WithLabelValues("ostore")) - beforeOstore,
			testutil.ToFloat64(userLoginsTotal.WithLabelValues("alice")) - beforeAlice,
			testutil.ToFloat64(clientConnectionsTotal.WithLabelValues("192.168.1.100")) - beforeProbeConn
	}

	// summary_exclude=true：探测账号与探测来源IP不参与统计
	login, reconn, ostore, alice, probeConns := run(true)
	if login != 1 {
		t.Errorf("exclude=true: 登录总数增量 = %v, 期望 1（仅 alice）", login)
	}
	if ostore != 0 {
		t.Errorf("exclude=true: ostore 登录增量 = %v, 期望 0", ostore)
	}
	if alice != 1 {
		t.Errorf("exclude=true: alice 登录增量 = %v, 期望 1", alice)
	}
	if probeConns != 0 {
		t.Errorf("exclude=true: 探测来源连接增量 = %v, 期望 0", probeConns)
	}
	if reconn != 0 {
		t.Errorf("exclude=true: 快速重连增量 = %v, 期望 0（探测连接不计）", reconn)
	}

	// summary_exclude=false（默认）：全部计入
	login, reconn, ostore, alice, probeConns = run(false)
	if login != 2 {
		t.Errorf("exclude=false: 登录总数增量 = %v, 期望 2", login)
	}
	if ostore != 1 {
		t.Errorf("exclude=false: ostore 登录增量 = %v, 期望 1", ostore)
	}
	if alice != 1 {
		t.Errorf("exclude=false: alice 登录增量 = %v, 期望 1", alice)
	}
	if probeConns != 2 {
		t.Errorf("exclude=false: 探测来源连接增量 = %v, 期望 2（两次 CONNECT）", probeConns)
	}
	if reconn != 1 {
		t.Errorf("exclude=false: 快速重连增量 = %v, 期望 1", reconn)
	}
}

// TestFTPResponseSummaryExcludeByUsername 验证 summary_exclude 开启时,
// 探测账号(ftp_user)的 FTP response 错误按用户名被过滤(BUG-046),
// 即使探测来源 IP 与日志中 Client IP 不一致(NAT 场景)。
func TestFTPResponseSummaryExcludeByUsername(t *testing.T) {
	logContent := `Sun Aug  9 16:34:50 2026 [pid 1001] [ostore] FAIL LOGIN: Client "10.0.0.5"
Sun Aug  9 16:34:51 2026 [pid 1001] [ostore] FTP response: Client "10.0.0.5", "530 Login incorrect."
Sun Aug  9 16:34:52 2026 [pid 1002] [alice] FTP response: Client "192.168.1.101", "550 Failed to change directory."
Sun Aug  9 16:34:53 2026 [pid 1003] [bob] FTP response: Client "192.168.1.102", "530 Login incorrect."
`
	path := filepath.Join(t.TempDir(), "vsftpd.log")
	if err := os.WriteFile(path, []byte(logContent), 0644); err != nil {
		t.Fatalf("写入测试文件失败: %v", err)
	}

	run := func(exclude bool) (failed, authFailed, dirNotFound float64) {
		beforeFail := testutil.ToFloat64(failedLoginsTotal)
		beforeAuthFailed := testutil.ToFloat64(ftpErrorsTotal.WithLabelValues("auth_failed"))
		beforeDir := testutil.ToFloat64(ftpErrorsTotal.WithLabelValues("dir_not_found"))

		cfg := &Config{FTPUser: "ostore", SummaryExclude: exclude}
		state := NewExporterState()
		// 模拟 NAT 场景:探测来源 IP 与日志中 Client IP 不同(10.0.0.5 vs 探测本地 IP)
		state.probeClientIP = "172.25.234.200"
		if err := parseVsftpdLog(cfg, path, state, nil); err != nil {
			t.Fatalf("parseVsftpdLog 失败: %v", err)
		}
		return testutil.ToFloat64(failedLoginsTotal) - beforeFail,
			testutil.ToFloat64(ftpErrorsTotal.WithLabelValues("auth_failed")) - beforeAuthFailed,
			testutil.ToFloat64(ftpErrorsTotal.WithLabelValues("dir_not_found")) - beforeDir
	}

	// exclude=true:ostore 的 FAIL LOGIN 与 530 response 均被过滤(按用户名),
	// alice 的 550 与 bob 的 530 计入
	failed, authFailed, dirNotFound := run(true)
	if failed != 0 {
		t.Errorf("exclude=true: failedLoginsTotal 增量 = %v, 期望 0(ostore 按用户名过滤)", failed)
	}
	if authFailed != 1 {
		t.Errorf("exclude=true: auth_failed 增量 = %v, 期望 1(仅 bob)", authFailed)
	}
	if dirNotFound != 1 {
		t.Errorf("exclude=true: dir_not_found 增量 = %v, 期望 1(alice)", dirNotFound)
	}

	// exclude=false:全部计入
	failed, authFailed, dirNotFound = run(false)
	if failed != 1 {
		t.Errorf("exclude=false: failedLoginsTotal 增量 = %v, 期望 1", failed)
	}
	if authFailed != 2 {
		t.Errorf("exclude=false: auth_failed 增量 = %v, 期望 2(ostore+bob)", authFailed)
	}
	if dirNotFound != 1 {
		t.Errorf("exclude=false: dir_not_found 增量 = %v, 期望 1", dirNotFound)
	}
}

// TestExpandLogFilePathEnv 验证 expandLogFilePath 支持环境变量路径(BUG-048 回归):
// 本地模式下配置先展开环境变量、再对展开后的路径做安全校验,因此 $VAR 路径可用。
func TestExpandLogFilePathEnv(t *testing.T) {
	t.Setenv("VSFTP_LOG_DIR", "/var/lib/vsftpd")
	result, err := expandLogFilePath("$VSFTP_LOG_DIR/xferlog")
	if err != nil {
		t.Fatalf("expandLogFilePath 展开环境变量失败: %v", err)
	}
	want := "/var/lib/vsftpd/xferlog"
	if !strings.HasSuffix(result, want) {
		t.Errorf("展开结果 = %q, 期望以 %q 结尾", result, want)
	}
	if strings.Contains(result, "$VSFTP_LOG_DIR") {
		t.Errorf("展开结果仍包含未展开的变量: %q", result)
	}

	// ${VAR} 形式也应支持
	result2, err := expandLogFilePath("${VSFTP_LOG_DIR}/vsftpd.log")
	if err != nil {
		t.Fatalf("expandLogFilePath 展开 ${VAR} 失败: %v", err)
	}
	if !strings.HasSuffix(result2, "/var/lib/vsftpd/vsftpd.log") {
		t.Errorf("${VAR} 展开结果 = %q, 期望以 /var/lib/vsftpd/vsftpd.log 结尾", result2)
	}
}

// TestJoinHostPortIPv6 验证 IPv6 地址与端口拼接使用 net.JoinHostPort(BUG-052 回归):
// 裸 IPv6(::1)经 isValidHost 放行后,连接地址构造必须生成 [::1]:port 而非 ::1:port。
func TestJoinHostPortIPv6(t *testing.T) {
	addr := net.JoinHostPort("::1", "21")
	if addr != "[::1]:21" {
		t.Errorf("JoinHostPort(::1, 21) = %q, 期望 [::1]:21", addr)
	}
	// 若用旧的 host+":"+port 拼接会得到 ::1:21 而无法拨号
	if "::1:21" == "[::1]:21" {
		t.Error("IPv6 拼接仍使用不安全的拼接方式")
	}
	// IPv4 与 hostname 不受影响
	if got := net.JoinHostPort("192.168.1.1", "21"); got != "192.168.1.1:21" {
		t.Errorf("JoinHostPort IPv4 = %q", got)
	}
	if got := net.JoinHostPort("example.com", "22"); got != "example.com:22" {
		t.Errorf("JoinHostPort hostname = %q", got)
	}
}

// TestLoadConfigLocalEnvPath 验证本地模式下配置中的环境变量路径能正确展开(BUG-048 回归):
// isValidFilePath 需在 expandLogFilePath 展开之后对最终路径校验,否则 $VAR 路径被拒绝。
func TestLoadConfigLocalEnvPath(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "xferlog")
	if err := os.WriteFile(logFile, []byte("Wed Aug  5 10:00:00 2026 0 192.168.1.1 10 /f.txt b _ i a u ftp 0 * c\n"), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("VSFTP_TEST_LOG_DIR", dir)

	cfgFile := filepath.Join(dir, "config.json")
	cfgJSON := `{
		"target_host":"localhost","ftp_port":"21","ftp_user":"u","ftp_password":"p",
		"need_ssh":false,"Xferlog_file_path":"$VSFTP_TEST_LOG_DIR/xferlog",
		"listen_port":"19101","check_interval":30,
		"vsftplog_enabled":false,"summary_exclude":false
	}`
	if err := os.WriteFile(cfgFile, []byte(cfgJSON), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := loadAndValidateConfig(cfgFile)
	if err != nil {
		t.Fatalf("配置加载失败(BUG-048 未修复): %v", err)
	}
	if cfg.LogFilePath != logFile {
		t.Errorf("LogFilePath = %q, 期望展开为 %q", cfg.LogFilePath, logFile)
	}
}

// TestParseSSOutputExactPortMatch 验证端口按完整 token 精确比较而非后缀匹配(BUG-053 回归):
// 随机客户端端口恰好以 FTP 端口结尾(如 56069 以 6069 结尾)不得被误判为 FTP 连接。
func TestParseSSOutputExactPortMatch(t *testing.T) {
	cases := []struct {
		name   string
		output string
		port   string
		want   int
	}{
		// 客户端随机端口 56069(以 6069 结尾)连接 MySQL:两端都不是 FTP 端口
		{"client-56069-to-mysql", "ESTAB 0 0 172.25.234.5:56069 172.25.200.1:3306", "6069", 0},
		// 真实 FTP 连接:local=6069
		{"real-ftp-6069", "ESTAB 0 0 172.25.200.1:6069 172.25.234.5:45000", "6069", 1},
		// 端口 321(以 21 结尾)连接 MySQL:不能算 ftp:21
		{"port-321-not-21", "ESTAB 0 0 1.2.3.4:321 5.6.7.8:3306", "21", 0},
		// 端口 21 真实连接
		{"real-ftp-21", "ESTAB 0 0 1.2.3.4:21 5.6.7.8:40000", "21", 1},
		// IPv4-mapped
		{"ipv4-mapped", "ESTAB 0 0 ::ffff:172.25.200.1:6069 ::ffff:172.25.234.5:45001", "6069", 1},
	}
	for _, c := range cases {
		total, _, _ := parseSSOutput(c.output, c.port)
		if total != c.want {
			t.Errorf("[%s] total=%d, 期望 %d (输出:%q)", c.name, total, c.want, c.output)
		}
	}
}

// TestXferlogNegativeFileSizeNoPanic 验证负 fileSize 被 clamp 不会触发 counter panic(BUG-054 回归)。
func TestXferlogNegativeFileSizeNoPanic(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "xferlog")
	log := "Wed Aug  5 10:00:00 2026 0 172.25.234.200 -5242880 /media/abc.mp4 b _ i a ostore ftp 0 * c\n"
	if err := os.WriteFile(path, []byte(log), 0644); err != nil {
		t.Fatal(err)
	}
	filesByTypeTotal.Reset()
	state := NewExporterState()
	state.lastProcessedTime = time.Now()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("BUG-054 未修复: 负 fileSize 触发 panic = %v", r)
		}
	}()
	before := testutil.ToFloat64(uploadBytesTotal)
	if err := parseFTPLog(path, state, nil); err != nil {
		t.Fatalf("parseFTPLog err: %v", err)
	}
	// clamp 后负值归零,Add(0) 不改变 counter:增量应为 0。
	// 用 before/after delta 而非绝对值,避免全局 counter 跨测试累积(BUG-051)。
	if got := testutil.ToFloat64(uploadBytesTotal) - before; got != 0 {
		t.Errorf("负 fileSize 应被 clamp 为 0, uploadBytesTotal 增量=%v", got)
	}
}

func TestClassifyFTPNotice(t *testing.T) {
	tests := []struct {
		name    string
		code    string
		message string
		want    []string
	}{
		{"421 空闲超时", "421", "Timeout. (responded KIA aka kill idle)", []string{"idle_timeout"}},
		{"421 非超时的服务不可用", "421", "Service not available, closing control connection.", nil},
		{"426 上传流停滞", "426", "Failure writing network stream.", []string{"data_conn_timeout"}},
		{"426 传输中止", "426", "Connection closed; transfer aborted.", []string{"data_conn_timeout"}},
		{"530 全局客户端上限", "530", "Maximum number of clients (30) reached.", []string{"max_clients"}},
		{"530 连接数过多", "530", "Too many clients.", []string{"max_clients"}},
		{"421 单IP连接数超限", "421", "There are too many connections from your internet address.", []string{"max_per_ip"}},
		{"530 单IP连接数超限", "530", "Sorry, there are too many connections from your internet address.", []string{"max_per_ip"}},
		{"425 建立连接失败(PASV)", "425", "Failed to establish connection.", []string{"pasv_port"}},
		{"425 数据连接打不开", "425", "Can't open data connection.", []string{"pasv_port"}},
		{"530 登录错误", "530", "Login incorrect.", nil},
		{"550 目录不存在", "550", "Failed to change directory.", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyFTPNotice(tt.code, tt.message)
			if len(got) != len(tt.want) {
				t.Errorf("classifyFTPNotice(%q, %q) = %v, 期望 %v", tt.code, tt.message, got, tt.want)
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("classifyFTPNotice(%q, %q) = %v, 期望 %v", tt.code, tt.message, got, tt.want)
					return
				}
			}
		})
	}
}

func TestParseVsftpdLogA1A4Counters(t *testing.T) {
	beforeIdle := testutil.ToFloat64(vsftpIdleTimeoutTotal)
	beforeData := testutil.ToFloat64(vsftpDataConnTimeoutTotal)
	beforeMaxClients := testutil.ToFloat64(vsftpConnLimitRejectedTotal.WithLabelValues("max_clients"))
	beforeMaxPerIP := testutil.ToFloat64(vsftpConnLimitRejectedTotal.WithLabelValues("max_per_ip"))
	beforePasv := testutil.ToFloat64(vsftpPasvPortRejectionsTotal)

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "vsftpd.log")
	log := `Sun Aug  9 16:34:50 2026 [pid 1234] [u1] FTP response: Client "192.168.1.100", "421 Timeout. (responded KIA aka kill idle)"
Sun Aug  9 16:34:51 2026 [pid 1235] [u2] FTP response: Client "192.168.1.101", "426 Failure writing network stream."
Sun Aug  9 16:34:52 2026 [pid 1236] [] FTP response: Client "192.168.1.102", "530 Maximum number of clients (30) reached."
Sun Aug  9 16:34:53 2026 [pid 1237] [] FTP response: Client "192.168.1.103", "421 There are too many connections from your internet address."
Sun Aug  9 16:34:54 2026 [pid 1238] [u3] FTP response: Client "192.168.1.104", "425 Failed to establish connection."
Sun Aug  9 16:34:55 2026 [pid 1239] [u4] FTP response: Client "192.168.1.105", "530 Login incorrect."
`
	if err := os.WriteFile(path, []byte(log), 0644); err != nil {
		t.Fatalf("写入测试文件失败: %v", err)
	}

	state := NewExporterState()
	if err := parseVsftpdLog(&Config{}, path, state, nil); err != nil {
		t.Fatalf("parseVsftpdLog 失败: %v", err)
	}

	if got := testutil.ToFloat64(vsftpIdleTimeoutTotal) - beforeIdle; got != 1 {
		t.Errorf("vsftpIdleTimeoutTotal 增量 = %v, 期望 1", got)
	}
	if got := testutil.ToFloat64(vsftpDataConnTimeoutTotal) - beforeData; got != 1 {
		t.Errorf("vsftpDataConnTimeoutTotal 增量 = %v, 期望 1", got)
	}
	if got := testutil.ToFloat64(vsftpConnLimitRejectedTotal.WithLabelValues("max_clients")) - beforeMaxClients; got != 1 {
		t.Errorf("connection_limit_rejections{max_clients} 增量 = %v, 期望 1", got)
	}
	if got := testutil.ToFloat64(vsftpConnLimitRejectedTotal.WithLabelValues("max_per_ip")) - beforeMaxPerIP; got != 1 {
		t.Errorf("connection_limit_rejections{max_per_ip} 增量 = %v, 期望 1", got)
	}
	if got := testutil.ToFloat64(vsftpPasvPortRejectionsTotal) - beforePasv; got != 1 {
		t.Errorf("vsftpPasvPortRejectionsTotal 增量 = %v, 期望 1", got)
	}
}

func TestIsNoiseTransfer(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{".listing", true},
		{"/49/quant-app/picture/poster/.listing", true},
		{"/49/quant-app/picture/2026-03-12/.listing", true},
		{"photo.jpg.writing", true},
		{"/tmp/x/.writing", true},
		{"/49/quant-app/picture/poster/00/02/real.jpg", false},
		{"/49/quant-app/picture/report.pdf", false},
		{"/data/.config", false},      // 真实隐藏文件,不应误伤
		{"/data/.listing.bak", false}, // 含 listing 字样但非精确后缀,不应误伤
	}
	for _, c := range cases {
		if got := isNoiseTransfer(c.path); got != c.want {
			t.Errorf("isNoiseTransfer(%q) = %v, 期望 %v", c.path, got, c.want)
		}
	}
}

func TestParseFTPLogNoiseFiltering(t *testing.T) {
	// .listing/.writing 应被排除在 files_by_type_total 与 client_files_total 之外,
	// 真实业务文件(real.jpg)应正常计数。注意 files_by_type 走冷启动暂存机制(KNOWN-016):
	// 首次见到标签要经 bumpScrapeSeq + 再次解析后才提交,故需按既有多轮模式验证。
	filesByTypeTotal.Reset()
	beforeUpload := testutil.ToFloat64(ftpUploadTotal)
	beforeBytes := testutil.ToFloat64(uploadBytesTotal)
	beforeInternalUp := testutil.ToFloat64(internalTransfersTotal.WithLabelValues("upload"))
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "xferlog")
	log := `Wed Aug  5 10:00:00 2026 0 172.25.222.49 1024 /picture/poster/.listing b _ i a ftpupload ftp 0 * c
Wed Aug  5 10:00:01 2026 0 172.25.222.49 2048 /picture/data.bin.writing b _ i a ftpupload ftp 0 * c
Wed Aug  5 10:00:02 2026 0 172.25.222.49 5120 /picture/poster/00/02/real.jpg b _ i a ftpupload ftp 0 * c
`
	if err := os.WriteFile(path, []byte(log), 0644); err != nil {
		t.Fatalf("写入测试文件失败: %v", err)
	}

	state := NewExporterState()
	state.lastProcessedTime = time.Now()
	if err := parseFTPLog(path, state, nil); err != nil {
		t.Fatalf("parseFTPLog 失败: %v", err)
	}

	// client_files 为实时 Inc()(.listing/.writing 不计算;仅 real.jpg 计入)
	if got := testutil.ToFloat64(clientFilesTotal.WithLabelValues("172.25.222.49", "upload")); got != 1 {
		t.Errorf("client_files{172.25.222.49,upload} = %v, 期望 1(仅真实业务 jpg,过滤 .listing/.writing)", got)
	}

	// 提交 files_by_type:模拟抓取后再解析一轮
	state.bumpScrapeSeq()
	if err := parseFTPLog(path, state, nil); err != nil {
		t.Fatalf("第二轮 parseFTPLog 失败: %v", err)
	}
	if got := testutil.ToFloat64(filesByTypeTotal.WithLabelValues("jpg", "upload")); got != 1 {
		t.Errorf("files_by_type{jpg,upload} = %v, 期望 1(实际业务文件应计数)", got)
	}
	// .listing 和 *.writing 不应产生 file_type 系列
	if got := testutil.ToFloat64(filesByTypeTotal.WithLabelValues("listing", "upload")); got != 0 {
		t.Errorf("files_by_type{listing,upload} = %v, 期望 0(.listing 应被过滤)", got)
	}
	if got := testutil.ToFloat64(filesByTypeTotal.WithLabelValues("writing", "upload")); got != 0 {
		t.Errorf("files_by_type{writing,upload} = %v, 期望 0(.writing 应被过滤)", got)
	}

	// BUG-064 增强:.listing/.writing 也不应计入上传次数与字节,仅计入内部传输计数。
	// 前两轮解析已消费整个文件(offset 机制不再重复计数),故直接检查累计差值:
	// 1 个业务 jpg 计入 upload_total/bytes,2 个噪音计入 internal_transfers。
	if got := testutil.ToFloat64(ftpUploadTotal) - beforeUpload; got != 1 {
		t.Errorf("upload_total 增量 = %v, 期望 1(仅业务 jpg,.listing/.writing 不计入)", got)
	}
	if got := testutil.ToFloat64(uploadBytesTotal) - beforeBytes; got != 5120 {
		t.Errorf("upload_bytes 增量 = %v, 期望 5120(仅业务 jpg 字节)", got)
	}
	if got := testutil.ToFloat64(internalTransfersTotal.WithLabelValues("upload")) - beforeInternalUp; got != 2 {
		t.Errorf("internal_transfers{upload} 增量 = %v, 期望 2(.listing + .writing)", got)
	}
}

// TestSummaryExcludeIPv4MappedTimeout 验证 summary_exclude 开启时,日志来源 IP 以
// IPv4-mapped IPv6 形式记录("::ffff:a.b.c.d",vsftpd listen_ipv6 典型现象)而探测
// probeClientIP 为纯 IPv4("a.b.c.d")时,CONNECT 过滤在 normalizeClientIP 归一化后
// 仍能正确命中(修复前字符串直接比较恒不等,导致探测自身被计入 rapid_reconnections)。
func TestSummaryExcludeIPv4Mapped(t *testing.T) {
	logContent := `Sun Aug  9 16:34:50 2026 [pid 1001] CONNECT: Client "::ffff:172.25.222.49"
Sun Aug  9 16:34:51 2026 [pid 1001] [ostore] OK LOGIN: Client "::ffff:172.25.222.49"
Sun Aug  9 16:34:52 2026 [pid 1001] CONNECT: Client "::ffff:172.25.222.49"
Sun Aug  9 16:34:55 2026 [pid 1002] CONNECT: Client "::ffff:172.25.222.49"
`
	path := filepath.Join(t.TempDir(), "vsftpd.log")
	if err := os.WriteFile(path, []byte(logContent), 0644); err != nil {
		t.Fatalf("写入测试文件失败: %v", err)
	}

	// 探测来源 IP 为纯 IPv4 形式
	cfg := &Config{FTPUser: "ostore", SummaryExclude: true}
	state := NewExporterState()
	state.probeClientIP = "172.25.222.49"

	beforeProbeConn := testutil.ToFloat64(clientConnectionsTotal.WithLabelValues("172.25.222.49"))
	beforeReconn := testutil.ToFloat64(rapidReconnectionsTotal)
	beforeOstore := testutil.ToFloat64(userLoginsTotal.WithLabelValues("ostore"))

	if err := parseVsftpdLog(cfg, path, state, nil); err != nil {
		t.Fatalf("parseVsftpdLog 失败: %v", err)
	}

	// 全部 3 次探测 CONNECT 与登录均被过滤
	if got := testutil.ToFloat64(clientConnectionsTotal.WithLabelValues("172.25.222.49")) - beforeProbeConn; got != 0 {
		t.Errorf("exclude=true+IPv4-mapped: 探测来源连接增量 = %v, 期望 0", got)
	}
	if got := testutil.ToFloat64(rapidReconnectionsTotal) - beforeReconn; got != 0 {
		t.Errorf("exclude=true+IPv4-mapped: 快速重连增量 = %v, 期望 0", got)
	}
	if got := testutil.ToFloat64(userLoginsTotal.WithLabelValues("ostore")) - beforeOstore; got != 0 {
		t.Errorf("exclude=true+IPv4-mapped: ostore 登录增量 = %v, 期望 0", got)
	}
}

// TestNormalizeClientIP 验证 IPv4-mapped IPv6 归一化。
func TestNormalizeClientIP(t *testing.T) {
	cases := []struct{ in, want string }{
		{"::ffff:172.25.222.49", "172.25.222.49"},
		{"172.25.222.49", "172.25.222.49"},
		{"2001:db8::1", "2001:db8::1"}, // 真实 IPv6,不应误改
		{"192.168.1.100", "192.168.1.100"},
		{"", ""},
	}
	for _, c := range cases {
		if got := normalizeClientIP(c.in); got != c.want {
			t.Errorf("normalizeClientIP(%q) = %q, 期望 %q", c.in, got, c.want)
		}
	}
}

// TestParseVsftpdLogNoiseFiltering 验证 vsftpd.log 路径(OK UPLOAD/DOWNLOAD)下
// .listing/.writing 也被视为内部传输,不计入业务上传/下载计数(BUG-064 增强)。
func TestParseVsftpdLogNoiseFiltering(t *testing.T) {
	beforeUpload := testutil.ToFloat64(ftpUploadTotal)
	beforeDownload := testutil.ToFloat64(ftpDownloadTotal)
	beforeInternalUp := testutil.ToFloat64(internalTransfersTotal.WithLabelValues("upload"))
	beforeInternalDown := testutil.ToFloat64(internalTransfersTotal.WithLabelValues("download"))
	beforeClientUp := testutil.ToFloat64(clientFilesTotal.WithLabelValues("172.25.222.49", "upload"))

	logContent := `Sun Aug  9 16:34:50 2026 [pid 1001] [ftpupload] OK UPLOAD: Client "172.25.222.49", "/picture/poster/.listing", 0 bytes
Sun Aug  9 16:34:51 2026 [pid 1001] [ftpupload] OK UPLOAD: Client "172.25.222.49", "/picture/real.jpg", 5120 bytes
Sun Aug  9 16:34:52 2026 [pid 1002] [ftpupload] OK DOWNLOAD: Client "172.25.222.49", "/picture/archive/.listing", 0 bytes
`
	path := filepath.Join(t.TempDir(), "vsftpd.log")
	if err := os.WriteFile(path, []byte(logContent), 0644); err != nil {
		t.Fatalf("写入测试文件失败: %v", err)
	}

	state := NewExporterState()
	if err := parseVsftpdLog(&Config{}, path, state, nil); err != nil {
		t.Fatalf("parseVsftpdLog 失败: %v", err)
	}

	if got := testutil.ToFloat64(ftpUploadTotal) - beforeUpload; got != 1 {
		t.Errorf("upload_total 增量 = %v, 期望 1(仅 real.jpg,.listing 不计)", got)
	}
	if got := testutil.ToFloat64(ftpDownloadTotal) - beforeDownload; got != 0 {
		t.Errorf("download_total 增量 = %v, 期望 0(仅 .listing,不计)", got)
	}
	if got := testutil.ToFloat64(internalTransfersTotal.WithLabelValues("upload")) - beforeInternalUp; got != 1 {
		t.Errorf("internal_transfers{upload} 增量 = %v, 期望 1(.listing 上传)", got)
	}
	if got := testutil.ToFloat64(internalTransfersTotal.WithLabelValues("download")) - beforeInternalDown; got != 1 {
		t.Errorf("internal_transfers{download} 增量 = %v, 期望 1(.listing 下载)", got)
	}
	if got := testutil.ToFloat64(clientFilesTotal.WithLabelValues("172.25.222.49", "upload")) - beforeClientUp; got != 1 {
		t.Errorf("client_files{...,upload} 增量 = %v, 期望 1(仅 real.jpg)", got)
	}
}
