package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const maxLinesPerRead = 1000

var (
	connectRegex     = regexp.MustCompile(`^(\w+\s+\w+\s+\d{1,2}\s+\d+:\d+:\d+\s+\d+)\s+\[pid\s+(\d+)\]\s+CONNECT:\s+Client\s+"([^"]+)"`)
	loginOKRegex     = regexp.MustCompile(`^(\w+\s+\w+\s+\d{1,2}\s+\d+:\d+:\d+\s+\d+)\s+\[pid\s+(\d+)\]\s+\[([^\]]+)\]\s+OK\s+LOGIN:\s+Client\s+"([^"]+)"`)
	loginFailRegex   = regexp.MustCompile(`^(\w+\s+\w+\s+\d{1,2}\s+\d+:\d+:\d+\s+\d+)\s+\[pid\s+(\d+)\]\s+\[([^\]]*)\]\s+FAIL\s+LOGIN:\s+Client\s+"([^"]+)"`)
	uploadOKRegex    = regexp.MustCompile(`^(\w+\s+\w+\s+\d{1,2}\s+\d+:\d+:\d+\s+\d+)\s+\[pid\s+(\d+)\]\s+\[([^\]]+)\]\s+OK\s+UPLOAD:\s+Client\s+"([^"]+)",\s+"([^"]+)",\s+(\d+)\s+bytes`)
	downloadOKRegex  = regexp.MustCompile(`^(\w+\s+\w+\s+\d{1,2}\s+\d+:\d+:\d+\s+\d+)\s+\[pid\s+(\d+)\]\s+\[([^\]]+)\]\s+OK\s+DOWNLOAD:\s+Client\s+"([^"]+)",\s+"([^"]+)",\s+(\d+)\s+bytes`)
	ftpResponseRegex = regexp.MustCompile(`^(\w+\s+\w+\s+\d{1,2}\s+\d+:\d+:\d+\s+\d+)\s+\[pid\s+(\d+)\]\s+\[([^\]]*)\]\s+FTP\s+response:\s+Client\s+"([^"]*)",\s+"([^"]*)"`)
)

type ExporterState struct {
	lastProcessedTime time.Time
	lastPosition      int64
	lastInode         uint64

	totalBytesUploaded   int64
	totalBytesDownloaded int64

	vsftpLogPosition int64
	vsftpLogInode    uint64

	clientLastActivity map[string]time.Time
	clientConnectTimes map[string]time.Time
	activeProcessIDs   map[string]time.Time
	clientLastConnect  map[string]time.Time

	// probeClientIP 记录健康检查探测连接的来源 IP，用于 summary_exclude 时过滤探测事件。
	probeClientIP string

	typeEventsMu        sync.Mutex
	scrapeSeq           int64
	pendingTypeCounts   map[string]int64
	pendingTypeSeq      map[string]int64
	committedTypeLabels map[string]bool

	lastUniqueClientUpdate time.Time
	lastProcessUpdate      time.Time
}

func NewExporterState() *ExporterState {
	now := time.Now()
	return &ExporterState{
		lastProcessedTime:   now,
		clientLastActivity:  make(map[string]time.Time),
		clientConnectTimes:  make(map[string]time.Time),
		activeProcessIDs:    make(map[string]time.Time),
		clientLastConnect:   make(map[string]time.Time),
		pendingTypeCounts:   make(map[string]int64),
		pendingTypeSeq:      make(map[string]int64),
		committedTypeLabels: make(map[string]bool),
	}
}

func readRemoteFile(sshMgr *SSHManager, filePath string, startPosition int64, lastInode uint64) ([]string, int64, uint64, error) {
	if sshMgr == nil {
		return readLocalFile(filePath, startPosition, lastInode)
	}

	slog.Debug("通过SSH读取文件", "path", filePath, "start_position", startPosition, "last_inode", lastInode)

	if !isValidFilePath(filePath) {
		return nil, 0, lastInode, fmt.Errorf("文件路径包含非法字符: %s", filePath)
	}

	command := fmt.Sprintf(`fp='%s'
start_pos=%d
last_ino=%d
max_lines=%d

read cur_size cur_ino <<< $(stat -c "%%s %%i" "$fp" 2>/dev/null || echo "0 0")

rotated=0
if [ "$last_ino" -ne 0 ] && [ "$cur_ino" -ne "$last_ino" ]; then
    rotated=1
elif [ "$cur_size" -lt "$start_pos" ]; then
    rotated=1
fi

if [ "$rotated" -eq 1 ]; then
    rp="${fp}.1"
    read rot_size rot_ino <<< $(stat -c "%%s %%i" "$rp" 2>/dev/null || echo "0 0")
    if [ "$rot_size" -gt "$start_pos" ]; then
        echo "ROTATED_OLD $rot_ino"
        tail -c +$((start_pos + 1)) "$rp" 2>/dev/null | head -n $max_lines
    else
        echo "ROTATED_NEW $cur_ino"
        head -n $max_lines "$fp" 2>/dev/null
    fi
else
    echo "OK $cur_ino"
    if [ "$start_pos" -gt 0 ]; then
        tail -c +$((start_pos + 1)) "$fp" 2>/dev/null | head -n $max_lines
    else
        head -n $max_lines "$fp" 2>/dev/null
    fi
fi`, filePath, startPosition, lastInode, maxLinesPerRead)

	rawOutput, err := sshMgr.Execute(command)
	if err != nil {
		return nil, 0, lastInode, fmt.Errorf("执行SSH命令失败: %w", err)
	}

	header, content, _ := strings.Cut(rawOutput, "\n")
	parts := strings.Fields(header)
	status := ""
	var curInode uint64
	if len(parts) >= 1 {
		status = parts[0]
	}
	if len(parts) >= 2 {
		curInode, _ = strconv.ParseUint(parts[1], 10, 64)
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

	var newPosition int64
	var newInode uint64

	switch status {
	case "ROTATED_OLD":
		slog.Warn("检测到远程日志轮转，先读取已轮转日志剩余内容", "path", filePath+".1", "last_position", startPosition)
		newPosition = startPosition + consumedBytes
		newInode = lastInode
	case "ROTATED_NEW":
		slog.Warn("检测到远程日志轮转，从新文件开头读取", "path", filePath, "last_position", startPosition)
		newPosition = consumedBytes
		newInode = curInode
	default: // OK
		newPosition = startPosition + consumedBytes
		newInode = curInode
	}

	slog.Debug("SSH读取文件成功", "lines", len(lines), "position", newPosition, "inode", newInode)
	return lines, newPosition, newInode, nil
}

func readLocalFile(filePath string, startPosition int64, lastInode uint64) ([]string, int64, uint64, error) {
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return nil, 0, lastInode, fmt.Errorf("打开本地文件失败: %w", err)
	}
	curSize := fileInfo.Size()
	curInode := getFileInode(fileInfo)

	rotated := false
	if lastInode != 0 && curInode != lastInode {
		rotated = true
	} else if curSize < startPosition {
		rotated = true
	}

	readPath := filePath
	readPos := startPosition
	targetInode := curInode

	if rotated {
		rotatedPath := filePath + ".1"
		rotInfo, err := os.Stat(rotatedPath)
		if err == nil && rotInfo.Size() > startPosition {
			slog.Warn("检测到日志轮转，先读取已轮转日志剩余内容", "path", rotatedPath, "last_position", startPosition)
			readPath = rotatedPath
			readPos = startPosition
			targetInode = lastInode
		} else {
			slog.Warn("检测到日志轮转，从新文件开头读取", "path", filePath, "last_position", startPosition)
			readPath = filePath
			readPos = 0
			targetInode = curInode
		}
	}

	file, err := os.Open(readPath)
	if err != nil {
		return nil, 0, lastInode, fmt.Errorf("打开本地文件失败: %w", err)
	}
	defer file.Close()

	if _, err := file.Seek(readPos, 0); err != nil {
		return nil, 0, lastInode, fmt.Errorf("定位文件位置失败: %w", err)
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
			return nil, 0, lastInode, fmt.Errorf("读取文件失败: %w", err)
		}
		if len(lines) >= maxLinesPerRead {
			break
		}
	}

	newPosition := readPos + bytesRead
	return lines, newPosition, targetInode, nil
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
		// clamp: 畸形日志可能出现负 fileSize(如损坏/非标准产出)。负值会被
		// prometheus counter.Add 当"计数减少"而 panic,进而崩溃整个监控协程
		// (prometheus client_golang v1.19 在 v<0 时 panic)。归零以保单调性(BUG-054)。
		if size < 0 {
			size = 0
		}
		fileSize = size
	}
	completed = (completionStatus == "c")

	return eventTime, direction, clientIP, fileSize, filePath, transferTime, username, completed
}

// extractFileExtension 提取文件路径的后缀（小写、不含点），无后缀时返回 "no_extension"，
// 用于 vsftp_files_by_type_total 的 file_type 标签（直接展示原始后缀，如 ts/mp4/mkv）。
func extractFileExtension(filePath string) string {
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(filepath.Base(filePath)), "."))
	if ext == "" {
		return "no_extension"
	}
	return ext
}

// recordTypeEvent 记录一次按后缀和方向的文件传输。已提交的标签直接 Inc()；
// 首次见到的标签先用 Add(0) 注册 0 值系列并暂存计数，待抓取到 0 采样后再提交，
// 使 increase()[$__range] 能观测到该标签的首次增量。
func (s *ExporterState) recordTypeEvent(fileType, direction string) {
	key := fileType + "\x00" + direction
	s.typeEventsMu.Lock()
	defer s.typeEventsMu.Unlock()
	if s.committedTypeLabels[key] {
		filesByTypeTotal.WithLabelValues(fileType, direction).Inc()
		return
	}
	if s.pendingTypeCounts[key] == 0 {
		filesByTypeTotal.WithLabelValues(fileType, direction).Add(0)
		s.pendingTypeSeq[key] = s.scrapeSeq
	}
	s.pendingTypeCounts[key]++
}

// commitPendingTypeEvents 在每次解析轮次结束时调用：将"已在至少一次抓取中暴露 0 值"的
// 暂存计数提交到计数器，此后该标签的增量直接走 Inc()。
func (s *ExporterState) commitPendingTypeEvents() {
	s.typeEventsMu.Lock()
	defer s.typeEventsMu.Unlock()
	for key, n := range s.pendingTypeCounts {
		if s.pendingTypeSeq[key] < s.scrapeSeq {
			fileType, direction, _ := strings.Cut(key, "\x00")
			filesByTypeTotal.WithLabelValues(fileType, direction).Add(float64(n))
			delete(s.pendingTypeCounts, key)
			delete(s.pendingTypeSeq, key)
			s.committedTypeLabels[key] = true
		}
	}
}

// bumpScrapeSeq 在每次 /metrics 抓取完成后调用。
func (s *ExporterState) bumpScrapeSeq() {
	s.typeEventsMu.Lock()
	s.scrapeSeq++
	s.typeEventsMu.Unlock()
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
	slog.Debug("开始解析FTP日志文件", "path", logPath, "position", state.lastPosition, "inode", state.lastInode)

	totalLinesProcessed := 0
	totalUploads := 0
	totalDownloads := 0
	totalIncomplete := 0

	totalBytesThisRound := int64(0)
	var earliestTime, latestTime time.Time

	for {
		lines, newPosition, newInode, err := readRemoteFile(sshMgr, logPath, state.lastPosition, state.lastInode)
		if err != nil {
			return fmt.Errorf("读取日志文件失败: %w", err)
		}

		state.lastPosition = newPosition
		state.lastInode = newInode

		if len(lines) == 0 {
			break
		}

		linesProcessed := 0
		uploadCount := 0
		downloadCount := 0
		incompleteCount := 0

		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}

			linesProcessed++

			eventTime, direction, clientIP, fileSize, filePath, transferTime, _, completed := parseStandardXferlog(line)
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
			ext := extractFileExtension(filePath)
			state.recordTypeEvent(ext, dirLabel)
			if transferTime > 0 {
				transferDurationSeconds.Observe(float64(transferTime))
			}
		}

		totalLinesProcessed += linesProcessed
		totalUploads += uploadCount
		totalDownloads += downloadCount
		totalIncomplete += incompleteCount

		if len(lines) < maxLinesPerRead {
			break
		}
	}

	// 带宽计算：基于本轮解析的所有日志时间范围
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

	state.commitPendingTypeEvents()

	slog.Debug("FTP日志解析完成", "lines", totalLinesProcessed, "uploads", totalUploads, "downloads", totalDownloads, "incomplete", totalIncomplete)
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
		strings.Contains(msg, "cannot find the file"),
		strings.Contains(msg, "not a regular file"),
		strings.Contains(msg, "failed to open file"),
		strings.Contains(msg, "can't open file"),
		strings.Contains(msg, "cannot open file"):
		return "file_not_found"
	case code == "552":
		return "quota_exceeded"
	case code == "553":
		return "file_name_not_allowed"
	case code == "421":
		return "service_unavailable"
	case code == "425", code == "426", code == "450", code == "451":
		return "data_connection_error"
	case code == "500", code == "501", code == "502", code == "503", code == "504":
		return "command_error"
	default:
		return "other"
	}
}

// classifyFTPNotice 在 classifyFTPError 之外,A1-A4 细分计数依据。
// 依据 vsftpd 官方配置文档(vsftpd_conf.html)的 idle_session_timeout、
// data_connection_timeout、max_clients、max_per_ip、pasv_min/max_port,
// 从 FTP response(code+message)中识别对应运行时事件。返回命中的分类集合:
//   - "idle_timeout"   :响应空闲超时(idle_session_timeout 触发)
//   - "data_conn_timeout":数据传输停滞无进展超时(data_connection_timeout 触发)
//   - "max_clients"    :达到全局连接上限(max_clients)
//   - "max_per_ip"     :单 IP 连接数超限(max_per_ip)
//   - "pasv_port"      :PASV 数据连接建立失败(PASV 端口范围耗尽或网络故障,见 KNOWN)
//
// 注意:返回值与 classifyFTPError 的 reason 相互独立,当前 ftpErrorsTotal 的
// 归类不受影响,保持向后兼容。
func classifyFTPNotice(code, message string) []string {
	msg := strings.ToLower(message)
	var hits []string

	switch {
	case code == "421" && strings.Contains(msg, "timeout"):
		hits = append(hits, "idle_timeout")
	case code == "426" && (strings.Contains(msg, "failure writing network stream") ||
		strings.Contains(msg, "transfer aborted")):
		hits = append(hits, "data_conn_timeout")
	case strings.Contains(msg, "maximum number of clients") || strings.Contains(msg, "too many clients"):
		// max_clients:全局连接数上限
		hits = append(hits, "max_clients")
	case strings.Contains(msg, "from your internet address") || strings.Contains(msg, "from your ip"):
		// max_per_ip:单 IP 连接数上限
		hits = append(hits, "max_per_ip")
	case code == "425" && (strings.Contains(msg, "establish connection") ||
		strings.Contains(msg, "data connection")):
		hits = append(hits, "pasv_port")
	}

	return hits
}

func parseVsftpdLog(config *Config, logPath string, state *ExporterState, sshMgr *SSHManager) error {
	if logPath == "" {
		return nil
	}

	slog.Debug("开始解析vsftpd日志文件", "path", logPath, "position", state.vsftpLogPosition, "inode", state.vsftpLogInode)

	totalLinesProcessed := 0
	totalConnects := 0
	totalLoginsOK := 0
	totalLoginsFail := 0

	// 仅 vsftpd.log 模式(未配置 xferlog)下,由本函数负责带宽与平均速度指标(BUG-042):
	// parseFTPLog 未运行时,bandwidthUsage / averageTransferSpeed 无人更新,恒为 0。
	transferBytesThisRound := int64(0)
	var transferEarliest, transferLatest time.Time

	for {
		lines, newPosition, newInode, err := readRemoteFile(sshMgr, logPath, state.vsftpLogPosition, state.vsftpLogInode)
		if err != nil {
			return fmt.Errorf("读取vsftpd日志文件失败: %w", err)
		}

		state.vsftpLogPosition = newPosition
		state.vsftpLogInode = newInode

		// 无论本轮是否有新日志行,都按时刷新客户端/进程活跃度指标:
		// 旧实现在 len(lines)==0 提前 break 后才更新,导致日志静默期
		// vsftp_unique_clients / vsftp_active_processes 永不衰减,停留历史非零值(BUG-031)。
		currentTime := time.Now()
		if currentTime.Sub(state.lastUniqueClientUpdate).Minutes() >= 1 {
			updateUniqueClientsMetric(state, currentTime)
			state.lastUniqueClientUpdate = currentTime
		}
		if currentTime.Sub(state.lastProcessUpdate).Minutes() >= 1 {
			updateActiveProcessesMetric(state, currentTime)
			state.lastProcessUpdate = currentTime
		}

		if len(lines) == 0 {
			break
		}

		linesProcessed := 0
		connectCount := 0
		loginOKCount := 0
		loginFailCount := 0

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

				// summary_exclude 开启时，忽略健康检查探测产生的连接
				if config.SummaryExclude && state.probeClientIP != "" && clientIP == state.probeClientIP {
					continue
				}

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

				// summary_exclude 开启时，忽略健康检查探测账号的登录事件
				if config.SummaryExclude && username == config.FTPUser {
					continue
				}

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
				username := matches[3]
				clientIP := matches[4]

				// summary_exclude 开启时,忽略健康检查探测账号的失败登录
				if config.SummaryExclude && (state.probeClientIP != "" && clientIP == state.probeClientIP || username == config.FTPUser) {
					continue
				}

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
					// 记录传输事件时间范围与字节,供带宽/平均速度计算(BUG-042)
					if eventTime, err := parseVsftpdTimestamp(matches[1]); err == nil {
						if transferEarliest.IsZero() || eventTime.Before(transferEarliest) {
							transferEarliest = eventTime
						}
						if transferLatest.IsZero() || eventTime.After(transferLatest) {
							transferLatest = eventTime
						}
					}
					// 形态异常的日志可能有负字节,clamp 防 counter 单调性 panic(BUG-054)
					if bytes < 0 {
						bytes = 0
					}
					transferBytesThisRound += bytes
					// xferlog 已启用时,传输字节由 parseFTPLog 累计,此处仅累计统计字节避免双计(BUG-038)
					if config.LogFilePath == "" {
						state.totalBytesUploaded += bytes
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
					// 记录传输事件时间范围与字节,供带宽/平均速度计算(BUG-042)
					if eventTime, err := parseVsftpdTimestamp(matches[1]); err == nil {
						if transferEarliest.IsZero() || eventTime.Before(transferEarliest) {
							transferEarliest = eventTime
						}
						if transferLatest.IsZero() || eventTime.After(transferLatest) {
							transferLatest = eventTime
						}
					}
					// 形态异常的日志可能有负字节,clamp 防 counter 单调性 panic(BUG-054)
					if bytes < 0 {
						bytes = 0
					}
					transferBytesThisRound += bytes
					// xferlog 已启用时,传输字节由 parseFTPLog 累计,此处仅累计统计字节避免双计(BUG-038)
					if config.LogFilePath == "" {
						state.totalBytesDownloaded += bytes
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
				parts := strings.SplitN(matches[5], " ", 2)
				code := parts[0]
				message := ""
				if len(parts) > 1 {
					message = parts[1]
				}
				if !strings.HasPrefix(code, "4") && !strings.HasPrefix(code, "5") {
					continue
				}

				// summary_exclude 开启时,忽略健康检查探测产生的错误响应:
				// 按探测来源 IP 或探测账号名过滤(BUG-046,NAT 场景下 IP 可能不一致)
				if config.SummaryExclude && (state.probeClientIP != "" && matches[4] == state.probeClientIP || matches[3] == config.FTPUser) {
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

				// A1-A4 细分计数:idle_session_timeout / data_connection_timeout /
				// max_clients / max_per_ip / PASV 端口失败(不改变 classifyFTPError 归类)
				for _, notice := range classifyFTPNotice(code, message) {
					switch notice {
					case "idle_timeout":
						vsftpIdleTimeoutTotal.Inc()
					case "data_conn_timeout":
						vsftpDataConnTimeoutTotal.Inc()
					case "max_clients":
						vsftpConnLimitRejectedTotal.WithLabelValues("max_clients").Inc()
					case "max_per_ip":
						vsftpConnLimitRejectedTotal.WithLabelValues("max_per_ip").Inc()
					case "pasv_port":
						vsftpPasvPortRejectionsTotal.Inc()
					}
				}
				continue
			}
		}

		totalLinesProcessed += linesProcessed
		totalConnects += connectCount
		totalLoginsOK += loginOKCount
		totalLoginsFail += loginFailCount

		if len(lines) < maxLinesPerRead {
			break
		}
	}

	// 仅 vsftpd.log 模式(未配置 xferlog)下,由本函数负责带宽与平均速度指标(BUG-042)
	if config.LogFilePath == "" {
		if !transferEarliest.IsZero() && !transferLatest.IsZero() {
			logTimeDiff := transferLatest.Sub(transferEarliest).Seconds()
			if logTimeDiff > 0 {
				bandwidthUsage.Set(float64(transferBytesThisRound) / logTimeDiff)
			}
		} else if transferBytesThisRound == 0 {
			bandwidthUsage.Set(0)
		}

		totalBytes := state.totalBytesUploaded + state.totalBytesDownloaded
		programRunTime := time.Since(state.lastProcessedTime).Seconds()
		if totalBytes > 0 && programRunTime > 0 {
			averageTransferSpeed.Set(float64(totalBytes) / programRunTime)
		}
	}

	slog.Debug("vsftpd日志解析完成",
		"lines", totalLinesProcessed,
		"connects", totalConnects,
		"logins_ok", totalLoginsOK,
		"logins_fail", totalLoginsFail,
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
// A line matches when either the local or the peer address ends with the FTP port,
// covering server-side sockets (local = FTP port) and client-side sockets on the same
// host (peer = FTP port). Each TCP connection is deduplicated by its (local, peer)
// address pair so that a single connection observed from both sockets is counted once.
func parseSSOutput(output string, ftpPort string) (total, established, closeWait int) {
	seen := make(map[string]struct{})
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
		// LISTEN 是监听套接字,不是连接,不应计入 vsftp_connections(BUG-043)
		if state == "LISTEN" {
			continue
		}
		localAddr := fields[3]
		peerAddr := fields[4]
		// 精确比较端口而非字符串后缀匹配:后缀匹配会把 :56069、:16069 等
		// 恰好以 FTP 端口结尾的随机客户端端口误当成 FTP 连接(BUG-053)。
		if parseSSPort(localAddr) != ftpPort && parseSSPort(peerAddr) != ftpPort {
			continue
		}
		key := ssConnKey(ssNormalizeAddr(localAddr), ssNormalizeAddr(peerAddr))
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
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

// parseSSPort 从 ss 输出的地址中提取端口号(最后一个冒号之后的部分)。
// ss 输出可能为 IPv4("1.2.3.4:6069")、带方括号的 IPv6("[::1]:6069")或
// IPv4-mapped IPv6("::ffff:1.2.3.4:6069")。用 LastIndex 提取最末端口段,
// 保证与 ftpPort 做精确比较(BUG-053)。
func parseSSPort(addr string) string {
	if i := strings.LastIndex(addr, ":"); i != -1 {
		return addr[i+1:]
	}
	return ""
}

// ssNormalizeAddr normalizes an address so IPv4-mapped IPv6 endpoints
// (e.g. "::ffff:172.25.234.200:6069") and their plain IPv4 form
// ("172.25.234.200:6069") deduplicate as the same endpoint.
func ssNormalizeAddr(addr string) string {
	return strings.TrimPrefix(addr, "::ffff:")
}

// ssConnKey returns a canonical key for a TCP connection so that the same
// connection observed from either socket side (local/peer swapped) dedupes.
func ssConnKey(a, b string) string {
	if a < b {
		return a + "|" + b
	}
	return b + "|" + a
}
