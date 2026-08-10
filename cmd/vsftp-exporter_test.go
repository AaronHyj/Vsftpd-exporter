package main

import (
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

	lines, pos, err := readLocalFile(path, 0)
	if err != nil {
		t.Fatalf("readLocalFile 失败: %v", err)
	}
	if len(lines) != 1 || lines[0] != "line1" {
		t.Fatalf("行 = %v, 期望 [line1]（末行无换行符暂不消费）", lines)
	}
	if pos != int64(len("line1\n")) {
		t.Fatalf("position = %d, 期望 %d", pos, len("line1\n"))
	}

	// 追加换行符后再次读取
	if err := os.WriteFile(path, []byte(content+"\n"), 0644); err != nil {
		t.Fatalf("更新测试文件失败: %v", err)
	}

	lines2, pos2, err := readLocalFile(path, pos)
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

	_, pos, err := readLocalFile(path, 0)
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

	lines, pos2, err := readLocalFile(path, pos)
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

	lines, pos, err := readLocalFile(path, 0)
	if err != nil {
		t.Fatalf("readLocalFile 失败: %v", err)
	}
	if len(lines) != maxLinesPerRead {
		t.Fatalf("行数 = %d, 期望 %d", len(lines), maxLinesPerRead)
	}
	if pos != int64(maxLinesPerRead*len("line\n")) {
		t.Fatalf("position = %d, 期望 %d", pos, maxLinesPerRead*len("line\n"))
	}

	lines, _, err = readLocalFile(path, pos)
	if err != nil {
		t.Fatalf("readLocalFile 失败: %v", err)
	}
	if len(lines) != 500 {
		t.Fatalf("剩余行数 = %d, 期望 500", len(lines))
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
		code        string
		message     string
	}{
		{
			name:        "530 密码错误响应",
			line:        `Sun Aug  9 16:34:51 2026 [pid 1234] [ftpuser] FTP response: Client "192.168.1.100", "530 Login incorrect."`,
			shouldMatch: true,
			code:        "530",
			message:     "Login incorrect.",
		},
		{
			name:        "421 连接数过多响应",
			line:        `Sun Aug  9 16:34:51 2026 [pid 1234] [] FTP response: Client "192.168.1.100", "421 There are too many connections from your internet address."`,
			shouldMatch: true,
			code:        "421",
			message:     "There are too many connections from your internet address.",
		},
		{
			name:        "550 目录不存在响应",
			line:        `Sun Aug  9 16:34:51 2026 [pid 1234] [ftpuser] FTP response: Client "192.168.1.100", "550 Failed to change directory."`,
			shouldMatch: true,
			code:        "550",
			message:     "Failed to change directory.",
		},
		{
			name:        "成功响应 230 不应视为错误",
			line:        `Sun Aug  9 16:34:51 2026 [pid 1234] [ftpuser] FTP response: Client "192.168.1.100", "230 Login successful."`,
			shouldMatch: true,
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
				if len(matches) < 5 || matches[4] != tt.code+" "+tt.message {
					t.Errorf("提取的响应 = %q, 期望 %q", matches[4], tt.code+" "+tt.message)
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

	if got := testutil.ToFloat64(filesByTypeTotal.WithLabelValues("mp4", "upload")) - before; got != 1 {
		t.Errorf("mp4/upload 增量 = %v, 期望 1", got)
	}
	if got := testutil.ToFloat64(filesByTypeTotal.WithLabelValues("txt", "upload")) - before; got != 1 {
		t.Errorf("txt/upload 增量 = %v, 期望 1", got)
	}
	if got := testutil.ToFloat64(filesByTypeTotal.WithLabelValues("mkv", "download")) - before; got != 1 {
		t.Errorf("mkv/download 增量 = %v, 期望 1", got)
	}
	if got := testutil.ToFloat64(filesByTypeTotal.WithLabelValues("xyz", "upload")) - before; got != 1 {
		t.Errorf("xyz/upload 增量 = %v, 期望 1", got)
	}
}
