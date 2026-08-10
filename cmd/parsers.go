package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const maxLinesPerRead = 1000

var (
	connectRegex     = regexp.MustCompile(`^(\w+\s+\w+\s+\d{1,2}\s+\d+:\d+:\d+\s+\d+)\s+\[pid\s+(\d+)\]\s+CONNECT:\s+Client\s+"([^"]+)"`)
	loginOKRegex     = regexp.MustCompile(`^(\w+\s+\w+\s+\d{1,2}\s+\d+:\d+:\d+\s+\d+)\s+\[pid\s+(\d+)\]\s+\[([^\]]+)\]\s+OK\s+LOGIN:\s+Client\s+"([^"]+)"`)
	loginFailRegex   = regexp.MustCompile(`^(\w+\s+\w+\s+\d{1,2}\s+\d+:\d+:\d+\s+\d+)\s+\[pid\s+(\d+)\]\s+\[([^\]]*)\]\s+FAIL\s+LOGIN:\s+Client\s+"([^"]+)"`)
	uploadOKRegex    = regexp.MustCompile(`^(\w+\s+\w+\s+\d{1,2}\s+\d+:\d+:\d+\s+\d+)\s+\[pid\s+(\d+)\]\s+\[([^\]]+)\]\s+OK\s+UPLOAD:\s+Client\s+"([^"]+)",\s+"([^"]+)",\s+(\d+)\s+bytes`)
	downloadOKRegex  = regexp.MustCompile(`^(\w+\s+\w+\s+\d{1,2}\s+\d+:\d+:\d+\s+\d+)\s+\[pid\s+(\d+)\]\s+\[([^\]]+)\]\s+OK\s+DOWNLOAD:\s+Client\s+"([^"]+)",\s+"([^"]+)",\s+(\d+)\s+bytes`)
	ftpResponseRegex = regexp.MustCompile(`^(\w+\s+\w+\s+\d{1,2}\s+\d+:\d+:\d+\s+\d+)\s+\[pid\s+(\d+)\].*FTP\s+response:\s+Client\s+"([^"]*)",\s+"([^"]*)"`)
)

type ExporterState struct {
	lastProcessedTime time.Time
	lastPosition      int64

	totalBytesUploaded   int64
	totalBytesDownloaded int64

	vsftpLogPosition int64

	clientLastActivity map[string]time.Time
	clientConnectTimes map[string]time.Time
	activeProcessIDs   map[string]time.Time
	clientLastConnect  map[string]time.Time

	lastUniqueClientUpdate time.Time
	lastProcessUpdate      time.Time
}

func NewExporterState() *ExporterState {
	now := time.Now()
	return &ExporterState{
		lastProcessedTime:  now,
		clientLastActivity: make(map[string]time.Time),
		clientConnectTimes: make(map[string]time.Time),
		activeProcessIDs:   make(map[string]time.Time),
		clientLastConnect:  make(map[string]time.Time),
	}
}

func readRemoteFile(sshMgr *SSHManager, filePath string, startPosition int64) ([]string, int64, error) {
	if sshMgr == nil {
		return readLocalFile(filePath, startPosition)
	}

	slog.Debug("通过SSH读取文件", "path", filePath)

	if !isValidFilePath(filePath) {
		return nil, 0, fmt.Errorf("文件路径包含非法字符: %s", filePath)
	}

	var command string
	if startPosition > 0 {
		command = fmt.Sprintf("s=$(stat -c %%s '%s' 2>/dev/null || echo 0); if [ \"$s\" -lt %d ]; then echo ROTATED; cat '%s' 2>/dev/null | head -n %d; else echo OK; tail -c +%d '%s' 2>/dev/null | head -n %d; fi", filePath, startPosition, filePath, maxLinesPerRead, startPosition+1, filePath, maxLinesPerRead)
	} else {
		command = fmt.Sprintf("echo OK; cat '%s' 2>/dev/null | head -n %d", filePath, maxLinesPerRead)
	}

	rawOutput, err := sshMgr.Execute(command)
	if err != nil {
		return nil, 0, fmt.Errorf("执行SSH命令失败: %w", err)
	}

	header, content, _ := strings.Cut(rawOutput, "\n")
	if header == "ROTATED" {
		slog.Warn("检测到远程日志轮转，从头开始读取", "last_position", startPosition, "path", filePath)
		startPosition = 0
	}

	hasTrailingNewline := strings.HasSuffix(content, "\n")
	lines := strings.Split(content, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	consumedBytes := int64(len(content))
	if !hasTrailingNewline && len(lines) > 0 {
		// 末行没有换行符，说明末行不完整，暂不消费末行
		lastLineLen := int64(len(lines[len(lines)-1]))
		lines = lines[:len(lines)-1]
		consumedBytes -= lastLineLen
	}

	newPosition := startPosition + consumedBytes
	slog.Debug("SSH读取文件成功", "lines", len(lines), "position", newPosition)
	return lines, newPosition, nil
}

func readLocalFile(filePath string, startPosition int64) ([]string, int64, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, 0, fmt.Errorf("打开本地文件失败: %w", err)
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		return nil, 0, fmt.Errorf("获取文件信息失败: %w", err)
	}
	if fileInfo.Size() < startPosition {
		slog.Warn("检测到日志轮转，从头开始读取", "file_size", fileInfo.Size(), "last_position", startPosition, "path", filePath)
		startPosition = 0
	}

	if _, err := file.Seek(startPosition, 0); err != nil {
		return nil, 0, fmt.Errorf("定位文件位置失败: %w", err)
	}

	var lines []string
	bytesRead := int64(0)
	reader := bufio.NewReader(file)
	for {
		line, err := reader.ReadString('\n')
		if len(line) > 0 {
			if err == io.EOF && !strings.HasSuffix(line, "\n") {
				// EOF 且末行没有换行符，说明日志尚在写入中，暂不消费该半行
				break
			}
			lines = append(lines, strings.TrimRight(line, "\r\n"))
			bytesRead += int64(len(line))
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, 0, fmt.Errorf("读取文件失败: %w", err)
		}
		if len(lines) >= maxLinesPerRead {
			break
		}
	}

	newPosition := startPosition + bytesRead
	return lines, newPosition, nil
}

func parseStandardXferlog(line string) (eventTime time.Time, direction string, clientIP string, fileSize int64, filePath string, transferTime int, username string, completed bool) {
	fields := strings.Fields(line)
	n := len(fields)
	if n < 18 {
		return time.Time{}, "", "", 0, "", 0, "", false
	}

	// fields[0..4]: "Wed Oct 15 16:04:42 2025"
	timeStr := fields[0] + " " + fields[1] + " " + fields[2] + " " + fields[3] + " " + fields[4]
	eventTime, _ = parseXferlogTimestamp(timeStr)

	transferTimeStr := fields[5]
	clientIP = fields[6]
	fileSizeStr := fields[7]

	filePath = strings.Join(fields[8:n-9], " ")
	direction = fields[n-7]
	username = fields[n-5]
	completionStatus := fields[n-1]

	if t, err := strconv.Atoi(transferTimeStr); err == nil {
		transferTime = t
	}
	if size, err := strconv.ParseInt(fileSizeStr, 10, 64); err == nil {
		fileSize = size
	}
	completed = (completionStatus == "c")

	return eventTime, direction, clientIP, fileSize, filePath, transferTime, username, completed
}

// parseXferlogTimestamp 解析 xferlog 时间戳格式: "Wed Oct 15 16:04:42 2025"
func parseXferlogTimestamp(timeStr string) (time.Time, error) {
	layouts := []string{
		"Mon Jan _2 15:04:05 2006",
		"Mon Jan 02 15:04:05 2006",
		"Mon Jan 2 15:04:05 2006",
	}
	for _, layout := range layouts {
		if t, err := time.ParseInLocation(layout, timeStr, time.Local); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("无法解析xferlog时间戳: %s", timeStr)
}

func parseFTPLog(logPath string, state *ExporterState, sshMgr *SSHManager) error {
	slog.Debug("开始解析FTP日志文件", "path", logPath, "position", state.lastPosition)

	lines, newPosition, err := readRemoteFile(sshMgr, logPath, state.lastPosition)
	if err != nil {
		return fmt.Errorf("读取日志文件失败: %w", err)
	}

	linesProcessed := 0
	uploadCount := 0
	downloadCount := 0
	incompleteCount := 0

	totalBytesThisRound := int64(0)
	var earliestTime, latestTime time.Time

	for _, line := range lines {
		if linesProcessed >= maxLinesPerRead {
			break
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		linesProcessed++

		eventTime, direction, clientIP, fileSize, _, transferTime, _, completed := parseStandardXferlog(line)
		if direction == "" {
			continue
		}

		// 记录本轮日志的时间范围
		if !eventTime.IsZero() {
			if earliestTime.IsZero() || eventTime.Before(earliestTime) {
				earliestTime = eventTime
			}
			if latestTime.IsZero() || eventTime.After(latestTime) {
				latestTime = eventTime
			}
		}

		if !completed {
			incompleteCount++
			if direction == "i" {
				transferErrorsTotal.WithLabelValues("upload").Inc()
			} else {
				transferErrorsTotal.WithLabelValues("download").Inc()
			}
			continue
		}

		// 按方向更新指标
		var dirLabel string
		if direction == "i" {
			uploadCount++
			ftpUploadTotal.Inc()
			uploadBytesTotal.Add(float64(fileSize))
			state.totalBytesUploaded += fileSize
			dirLabel = "upload"
		} else {
			downloadCount++
			ftpDownloadTotal.Inc()
			downloadBytesTotal.Add(float64(fileSize))
			state.totalBytesDownloaded += fileSize
			dirLabel = "download"
		}

		totalBytesThisRound += fileSize

		if clientIP != "" {
			clientFilesTotal.WithLabelValues(clientIP, dirLabel).Inc()
		}
		if transferTime > 0 {
			transferDurationSeconds.Observe(float64(transferTime))
		}
	}

	state.lastPosition = newPosition

	// 带宽计算：基于日志时间范围
	if !earliestTime.IsZero() && !latestTime.IsZero() {
		logTimeDiff := latestTime.Sub(earliestTime).Seconds()
		if logTimeDiff > 0 {
			bandwidthUsage.Set(float64(totalBytesThisRound) / logTimeDiff)
		}
	} else if totalBytesThisRound == 0 {
		bandwidthUsage.Set(0)
	}

	// 平均传输速度：基于程序运行以来的总量
	totalBytes := state.totalBytesUploaded + state.totalBytesDownloaded
	programRunTime := time.Since(state.lastProcessedTime).Seconds()
	if totalBytes > 0 && programRunTime > 0 {
		averageTransferSpeed.Set(float64(totalBytes) / programRunTime)
	}

	slog.Debug("FTP日志解析完成", "lines", linesProcessed, "uploads", uploadCount, "downloads", downloadCount, "incomplete", incompleteCount)
	return nil
}

// classifyFTPError 根据 FTP 响应码和消息文本将错误归类，用于 vsftp_ftp_errors_total 的 reason 标签。
func classifyFTPError(code, message string) string {
	msg := strings.ToLower(message)
	switch {
	case strings.Contains(msg, "maximum number of clients"),
		strings.Contains(msg, "too many connections"),
		strings.Contains(msg, "too many clients"):
		return "max_connections"
	case code == "530":
		return "auth_failed"
	case code == "552":
		return "quota_exceeded"
	case strings.Contains(msg, "permission denied"),
		strings.Contains(msg, "not allowed"),
		strings.Contains(msg, "denied"):
		return "permission_denied"
	case strings.Contains(msg, "change directory"),
		strings.Contains(msg, "directory not found"),
		strings.Contains(msg, "no such directory"),
		strings.Contains(msg, "cannot find the path"):
		return "dir_not_found"
	case strings.Contains(msg, "no such file"),
		strings.Contains(msg, "file not found"),
		strings.Contains(msg, "cannot find the file"):
		return "file_not_found"
	default:
		return "other"
	}
}

func parseVsftpdLog(config *Config, logPath string, state *ExporterState, sshMgr *SSHManager) error {
	if logPath == "" {
		return nil
	}

	slog.Debug("开始解析vsftpd日志文件", "path", logPath)

	lines, newPosition, err := readRemoteFile(sshMgr, logPath, state.vsftpLogPosition)
	if err != nil {
		return fmt.Errorf("读取vsftpd日志文件失败: %w", err)
	}

	state.vsftpLogPosition = newPosition

	linesProcessed := 0
	connectCount := 0
	loginOKCount := 0
	loginFailCount := 0
	currentTime := time.Now()

	for _, line := range lines {
		if linesProcessed >= maxLinesPerRead {
			break
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		linesProcessed++

		// CONNECT 事件
		if matches := connectRegex.FindStringSubmatch(line); matches != nil {
			eventTime, err := parseVsftpdTimestamp(matches[1])
			if err != nil {
				continue
			}
			processID := matches[2]
			clientIP := matches[3]

			clientConnectionsTotal.WithLabelValues(clientIP).Inc()
			connectCount++

			state.clientLastActivity[clientIP] = eventTime
			state.clientConnectTimes[clientIP] = eventTime
			state.activeProcessIDs[processID] = eventTime

			if lastConnect, exists := state.clientLastConnect[clientIP]; exists {
				if eventTime.Sub(lastConnect).Seconds() <= 30 {
					rapidReconnectionsTotal.Inc()
				}
			}
			state.clientLastConnect[clientIP] = eventTime
			continue
		}

		// OK LOGIN 事件
		if matches := loginOKRegex.FindStringSubmatch(line); matches != nil {
			eventTime, err := parseVsftpdTimestamp(matches[1])
			if err != nil {
				continue
			}
			processID := matches[2]
			username := matches[3]
			clientIP := matches[4]

			userLoginsTotal.WithLabelValues(username).Inc()
			userConnectionsTotal.WithLabelValues(username).Inc()
			ftpLoginTotal.Inc()
			loginOKCount++

			ftpLoginTime.Set(float64(eventTime.Unix()))

			state.clientLastActivity[clientIP] = eventTime
			state.activeProcessIDs[processID] = eventTime

			if connectTime, exists := state.clientConnectTimes[clientIP]; exists {
				delay := eventTime.Sub(connectTime).Seconds()
				if delay >= 0 && delay <= 60 {
					connectionLoginDelaySeconds.Observe(delay)
				}
			}
			continue
		}

		// FAIL LOGIN 事件
		if matches := loginFailRegex.FindStringSubmatch(line); matches != nil {
			eventTime, err := parseVsftpdTimestamp(matches[1])
			if err != nil {
				continue
			}
			clientIP := matches[4]

			// 认证错误次数由下方 FTP response 530 行统计，避免同一事件重复计数
			failedLoginsTotal.Inc()
			loginFailCount++

			state.clientLastActivity[clientIP] = eventTime
			continue
		}

		// OK UPLOAD 事件
		if matches := uploadOKRegex.FindStringSubmatch(line); matches != nil {
			clientIP := matches[4]
			if bytes, err := strconv.ParseInt(matches[6], 10, 64); err == nil {
				state.totalBytesUploaded += bytes
				if config.LogFilePath == "" {
					ftpUploadTotal.Inc()
					uploadBytesTotal.Add(float64(bytes))
					if clientIP != "" {
						clientFilesTotal.WithLabelValues(clientIP, "upload").Inc()
					}
				}
			}
			continue
		}

		// OK DOWNLOAD 事件
		if matches := downloadOKRegex.FindStringSubmatch(line); matches != nil {
			clientIP := matches[4]
			if bytes, err := strconv.ParseInt(matches[6], 10, 64); err == nil {
				state.totalBytesDownloaded += bytes
				if config.LogFilePath == "" {
					ftpDownloadTotal.Inc()
					downloadBytesTotal.Add(float64(bytes))
					if clientIP != "" {
						clientFilesTotal.WithLabelValues(clientIP, "download").Inc()
					}
				}
			}
			continue
		}

		// FTP 协议错误响应（4xx / 5xx）
		if matches := ftpResponseRegex.FindStringSubmatch(line); matches != nil {
			parts := strings.SplitN(matches[4], " ", 2)
			code := parts[0]
			message := ""
			if len(parts) > 1 {
				message = parts[1]
			}
			if !strings.HasPrefix(code, "4") && !strings.HasPrefix(code, "5") {
				continue
			}

			reason := classifyFTPError(code, message)
			ftpErrorsTotal.WithLabelValues(reason).Inc()
			if code == "530" {
				authenticationErrorsTotal.Inc()
			}
			if reason == "max_connections" {
				maxConnectionsReachedTotal.Inc()
			}
			continue
		}
	}

	if currentTime.Sub(state.lastUniqueClientUpdate).Minutes() >= 1 {
		updateUniqueClientsMetric(state, currentTime)
		state.lastUniqueClientUpdate = currentTime
	}

	if currentTime.Sub(state.lastProcessUpdate).Minutes() >= 1 {
		updateActiveProcessesMetric(state, currentTime)
		state.lastProcessUpdate = currentTime
	}

	slog.Debug("vsftpd日志解析完成",
		"lines", linesProcessed,
		"connects", connectCount,
		"logins_ok", loginOKCount,
		"logins_fail", loginFailCount,
	)
	return nil
}

func parseVsftpdTimestamp(timeStr string) (time.Time, error) {
	layouts := []string{
		"Mon Jan _2 15:04:05 2006",
		"Mon Jan 02 15:04:05 2006",
		"Mon Jan 2 15:04:05 2006",
	}

	for _, layout := range layouts {
		if t, err := time.ParseInLocation(layout, timeStr, time.Local); err == nil {
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("无法解析时间戳: %s", timeStr)
}

func updateUniqueClientsMetric(state *ExporterState, currentTime time.Time) {
	activeClients := 0
	cutoffTime := currentTime.Add(-5 * time.Minute)

	for clientIP, lastActivity := range state.clientLastActivity {
		if lastActivity.After(cutoffTime) {
			activeClients++
		} else {
			delete(state.clientLastActivity, clientIP)
			delete(state.clientConnectTimes, clientIP)
			delete(state.clientLastConnect, clientIP)
		}
	}

	uniqueClients.Set(float64(activeClients))
}

func updateActiveProcessesMetric(state *ExporterState, currentTime time.Time) {
	activeProcessCount := 0
	cutoffTime := currentTime.Add(-5 * time.Minute)

	for processID, lastActivity := range state.activeProcessIDs {
		if lastActivity.After(cutoffTime) {
			activeProcessCount++
		} else {
			delete(state.activeProcessIDs, processID)
		}
	}

	activeProcesses.Set(float64(activeProcessCount))
}

func checkConnections(config *Config, sshMgr *SSHManager) error {
	var output string

	if sshMgr != nil {
		var err error
		output, err = sshMgr.Execute("ss -tnH")
		if err != nil {
			slog.Error("SSH远程执行ss命令失败", "error", err)
			ftpConnections.Set(0)
			establishedConnections.Set(0)
			closeWaitConnections.Set(0)
			return fmt.Errorf("SSH远程执行ss命令失败: %w", err)
		}
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		cmd := exec.CommandContext(ctx, "ss", "-tnH")
		outputBytes, err := cmd.Output()
		if err != nil {
			slog.Error("执行ss命令失败", "error", err)
			ftpConnections.Set(0)
			establishedConnections.Set(0)
			closeWaitConnections.Set(0)
			if ctx.Err() == context.DeadlineExceeded {
				return fmt.Errorf("ss命令执行超时")
			}
			return fmt.Errorf("执行ss命令失败: %w", err)
		}
		output = string(outputBytes)
	}

	totalConnections, establishedCount, closeWaitCount := parseSSOutput(output, config.FTPPort)

	ftpConnections.Set(float64(totalConnections))
	establishedConnections.Set(float64(establishedCount))
	closeWaitConnections.Set(float64(closeWaitCount))

	return nil
}

// parseSSOutput parses ss -tnH output and counts connections matching the given FTP port.
func parseSSOutput(output string, ftpPort string) (total, established, closeWait int) {
	portSuffix := ":" + ftpPort
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}
		state := fields[0]
		localAddr := fields[3]
		if !strings.HasSuffix(localAddr, portSuffix) {
			continue
		}
		total++
		switch state {
		case "ESTAB":
			established++
		case "CLOSE-WAIT":
			closeWait++
		}
	}
	return
}
