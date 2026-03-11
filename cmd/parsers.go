package main

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	connectRegex     = regexp.MustCompile(`^(\w+\s+\w+\s+\d{1,2}\s+\d+:\d+:\d+\s+\d+)\s+\[pid\s+(\d+)\]\s+CONNECT:\s+Client\s+"([^"]+)"`)
	loginOKRegex     = regexp.MustCompile(`^(\w+\s+\w+\s+\d{1,2}\s+\d+:\d+:\d+\s+\d+)\s+\[pid\s+(\d+)\]\s+\[([^\]]+)\]\s+OK\s+LOGIN:\s+Client\s+"([^"]+)"`)
	loginFailRegex   = regexp.MustCompile(`^(\w+\s+\w+\s+\d{1,2}\s+\d+:\d+:\d+\s+\d+)\s+\[pid\s+(\d+)\]\s+\[([^\]]+)\]\s+FAIL\s+LOGIN:\s+Client\s+"([^"]+)"`)
	uploadOKRegex    = regexp.MustCompile(`^(\w+\s+\w+\s+\d{1,2}\s+\d+:\d+:\d+\s+\d+)\s+\[pid\s+(\d+)\]\s+\[([^\]]+)\]\s+OK\s+UPLOAD:\s+Client\s+"([^"]+)",\s+"([^"]+)",\s+(\d+)\s+bytes`)
	downloadOKRegex  = regexp.MustCompile(`^(\w+\s+\w+\s+\d{1,2}\s+\d+:\d+:\d+\s+\d+)\s+\[pid\s+(\d+)\]\s+\[([^\]]+)\]\s+OK\s+DOWNLOAD:\s+Client\s+"([^"]+)",\s+"([^"]+)",\s+(\d+)\s+bytes`)
	ftpResponseRegex = regexp.MustCompile(`^(\w+\s+\w+\s+\d{1,2}\s+\d+:\d+:\d+\s+\d+)\s+\[pid\s+(\d+)\].*FTP\s+response:.*"530\s+`)

	timestampRegexYMD     = regexp.MustCompile(`(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2})`)
	timestampRegexSyslog  = regexp.MustCompile(`(\w{3} \w{3}\s+\d{1,2} \d{2}:\d{2}:\d{2} \d{4})`)
	timestampRegexSyslog2 = regexp.MustCompile(`(\w{3} \w{3} \d{2} \d{2}:\d{2}:\d{2} \d{4})`)
	timestampRegexDMY     = regexp.MustCompile(`(\d{2}/\d{2}/\d{4} \d{2}:\d{2}:\d{2})`)
	timestampRegexYSlash  = regexp.MustCompile(`(\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2})`)
)

type ExporterState struct {
	lastProcessedTime time.Time
	lastPosition      int64

	totalBytesUploaded   int64
	totalBytesDownloaded int64
	lastBytesTransferred int64

	vsftpLogPosition int64

	clientLastActivity map[string]time.Time
	clientConnectTimes map[string]time.Time
	userClientMapping  map[string]string
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
		userClientMapping:  make(map[string]string),
		activeProcessIDs:   make(map[string]time.Time),
		clientLastConnect:  make(map[string]time.Time),
	}
}

func readRemoteFile(sshMgr *SSHManager, filePath string, startPosition int64) ([]string, int64, error) {
	if sshMgr == nil {
		return readLocalFile(filePath, startPosition)
	}

	slog.Info("通过SSH读取文件", "path", filePath)

	if !isValidFilePath(filePath) {
		return nil, 0, fmt.Errorf("文件路径包含非法字符: %s", filePath)
	}

	var command string
	if startPosition > 0 {
		command = fmt.Sprintf("dd if='%s' bs=1 skip=%d 2>/dev/null", filePath, startPosition)
	} else {
		command = fmt.Sprintf("cat '%s'", filePath)
	}

	output, err := sshMgr.Execute(command)
	if err != nil {
		return nil, 0, fmt.Errorf("执行SSH命令失败: %w", err)
	}

	lines := strings.Split(output, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	newPosition := startPosition + int64(len(output))
	slog.Info("SSH读取文件成功", "lines", len(lines), "position", newPosition)
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
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		lines = append(lines, line)
		bytesRead += int64(len(scanner.Bytes())) + 1
	}

	if err := scanner.Err(); err != nil {
		return nil, 0, fmt.Errorf("读取文件失败: %w", err)
	}

	newPosition := startPosition + bytesRead
	return lines, newPosition, nil
}

func parseStandardXferlog(line string) (eventTime time.Time, direction string, clientIP string, fileSize int64, filePath string, transferTime int, username string, completed bool) {
	fields := strings.Fields(line)
	if len(fields) < 18 {
		return time.Time{}, "", "", 0, "", 0, "", false
	}

	// fields[0..4]: "Wed Oct 15 16:04:42 2025"
	timeStr := fields[0] + " " + fields[1] + " " + fields[2] + " " + fields[3] + " " + fields[4]
	eventTime, _ = parseXferlogTimestamp(timeStr)

	transferTimeStr := fields[5]
	clientIP = fields[6]
	fileSizeStr := fields[7]
	filePath = fields[8]
	direction = fields[11]
	username = fields[13]
	completionStatus := fields[17]

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

func parseFTPLog(config *Config, logPath string, state *ExporterState, sshMgr *SSHManager) error {
	slog.Info("开始解析FTP日志文件", "path", logPath, "position", state.lastPosition)

	lines, newPosition, err := readRemoteFile(sshMgr, logPath, state.lastPosition)
	if err != nil {
		return fmt.Errorf("读取日志文件失败: %w", err)
	}

	linesProcessed := 0
	uploadCount := 0
	downloadCount := 0
	incompleteCount := 0
	const maxLinesPerRead = 1000

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

		eventTime, direction, clientIP, fileSize, _, transferTime, username, completed := parseStandardXferlog(line)
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
		if username != "" {
			userConnectionsTotal.WithLabelValues(username).Inc()
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
	}

	// 平均传输速度：基于程序运行以来的总量
	totalBytes := state.totalBytesUploaded + state.totalBytesDownloaded
	programRunTime := time.Since(state.lastProcessedTime).Seconds()
	if totalBytes > 0 && programRunTime > 0 {
		averageTransferSpeed.Set(float64(totalBytes) / programRunTime)
	}

	state.lastBytesTransferred += totalBytesThisRound

	slog.Info("FTP日志解析完成", "lines", linesProcessed, "uploads", uploadCount, "downloads", downloadCount, "incomplete", incompleteCount)
	return nil
}

func parseVsftpdLog(config *Config, logPath string, state *ExporterState, sshMgr *SSHManager) error {
	if logPath == "" {
		return nil
	}

	slog.Info("开始解析vsftpd日志文件", "path", logPath)

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
			state.userClientMapping[username] = clientIP
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

			failedLoginsTotal.Inc()
			authenticationErrorsTotal.Inc()
			loginFailCount++

			state.clientLastActivity[clientIP] = eventTime
			continue
		}

		// OK UPLOAD 事件
		if matches := uploadOKRegex.FindStringSubmatch(line); matches != nil {
			clientIP := matches[4]
			if bytes, err := strconv.ParseInt(matches[6], 10, 64); err == nil {
				ftpUploadTotal.Inc()
				uploadBytesTotal.Add(float64(bytes))
				state.totalBytesUploaded += bytes
				if clientIP != "" {
					clientFilesTotal.WithLabelValues(clientIP, "upload").Inc()
				}
			}
			continue
		}

		// OK DOWNLOAD 事件
		if matches := downloadOKRegex.FindStringSubmatch(line); matches != nil {
			clientIP := matches[4]
			if bytes, err := strconv.ParseInt(matches[6], 10, 64); err == nil {
				ftpDownloadTotal.Inc()
				downloadBytesTotal.Add(float64(bytes))
				state.totalBytesDownloaded += bytes
				if clientIP != "" {
					clientFilesTotal.WithLabelValues(clientIP, "download").Inc()
				}
			}
			continue
		}

		// 530 认证错误响应
		if ftpResponseRegex.MatchString(line) {
			authenticationErrorsTotal.Inc()
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

	slog.Info("vsftpd日志解析完成",
		"lines", linesProcessed,
		"connects", connectCount,
		"logins_ok", loginOKCount,
		"logins_fail", loginFailCount,
	)
	return nil
}

func extractTimestamp(line string) int64 {
	timeFormats := []struct {
		regex  *regexp.Regexp
		layout string
	}{
		{timestampRegexYMD, "2006-01-02 15:04:05"},
		{timestampRegexSyslog, "Mon Jan _2 15:04:05 2006"},
		{timestampRegexSyslog2, "Mon Jan 02 15:04:05 2006"},
		{timestampRegexDMY, "02/01/2006 15:04:05"},
		{timestampRegexYSlash, "2006/01/02 15:04:05"},
	}

	for _, format := range timeFormats {
		if match := format.regex.FindString(line); match != "" {
			if t, err := time.ParseInLocation(format.layout, match, time.Local); err == nil {
				return t.Unix()
			}
		}
	}

	return time.Now().Unix()
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

func checkConnections(config *Config, state *ExporterState, sshMgr *SSHManager) error {
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
