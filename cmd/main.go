package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/textproto"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/jlaffaye/ftp"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type HealthStatus struct {
	Status        string    `json:"status"`
	Timestamp     time.Time `json:"timestamp"`
	Uptime        string    `json:"uptime"`
	LastCheckTime string    `json:"last_check_time,omitempty"`
	Error         string    `json:"error,omitempty"`
	Version       string    `json:"version"`
	BuildTime     string    `json:"build_time,omitempty"`
}

var (
	startTime  = time.Now()
	lastProbe  atomic.Value
	appVersion = "1.0.0"
	buildTime  = "unknown"
)

type probeResult struct {
	ok        bool
	checkTime time.Time
	err       string
}

func healthCheckHandler(w http.ResponseWriter, r *http.Request) {
	status := HealthStatus{
		Status:    "healthy",
		Timestamp: time.Now(),
		Uptime:    time.Since(startTime).String(),
		Version:   appVersion,
		BuildTime: buildTime,
	}

	statusCode := http.StatusOK
	if res, ok := lastProbe.Load().(probeResult); ok && !res.checkTime.IsZero() {
		status.LastCheckTime = res.checkTime.Format(time.RFC3339)
		if !res.ok {
			status.Status = "degraded"
			status.Error = res.err
			statusCode = http.StatusServiceUnavailable
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	if err := json.NewEncoder(w).Encode(status); err != nil {
		slog.Error("编码健康检查响应失败", "error", err)
	}
}

func checkFTPLogin(config *Config) (probeIP string, err error) {
	conn, err := (&net.Dialer{Timeout: 10 * time.Second}).Dial("tcp", net.JoinHostPort(config.TargetHost, config.FTPPort))
	if err != nil {
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			connectionTimeoutsTotal.Inc()
		}
		return "", fmt.Errorf("连接FTP服务器失败: %w", err)
	}
	if tcpAddr, ok := conn.LocalAddr().(*net.TCPAddr); ok {
		probeIP = tcpAddr.IP.String()
	}

	// FTP handshake (the 220 banner read inside Dial) and login reads have no
	// built-in deadline: once DialWithNetConn is used, the library skips its own
	// context timeout, so DialWithTimeout only covers the TCP connect stage.
	// Set an explicit deadline so the monitor goroutine cannot block forever if
	// the server accepts TCP but never replies (BUG-049).
	conn.SetDeadline(time.Now().Add(10 * time.Second))
	ftpConn, err := ftp.Dial(net.JoinHostPort(config.TargetHost, config.FTPPort), ftp.DialWithNetConn(conn), ftp.DialWithTimeout(10*time.Second))
	if err != nil {
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			connectionTimeoutsTotal.Inc()
		}
		conn.Close()
		return probeIP, fmt.Errorf("初始化FTP连接失败: %w", err)
	}
	defer ftpConn.Quit()
	defer conn.SetDeadline(time.Time{}) // clear so defer ftpConn.Quit() can send QUIT

	if err := ftpConn.Login(config.FTPUser, config.FTPPassword); err != nil {
		var protoErr *textproto.Error
		if errors.As(err, &protoErr) && protoErr.Code == 530 {
			return probeIP, fmt.Errorf("FTP登录失败（认证被拒绝 530）: %w", err)
		}
		return probeIP, fmt.Errorf("FTP登录失败: %w", err)
	}

	return probeIP, nil
}

// safeRunChecks 包装 runChecks:任何 panic 都被捕获并记录日志,避免单个异常
// (如畸形日志行触发 prometheus counter panic,见 BUG-054)导致整个监控协程
// 崩溃、指标静默停滞而 exporter 表面仍存活(HTTP /metrics 依旧响应旧值)。
func safeRunChecks(config *Config, state *ExporterState, sshMgr *SSHManager) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("运行检查时发生panic,已恢复", "panic", r)
		}
	}()
	runChecks(config, state, sshMgr)
}

func runChecks(config *Config, state *ExporterState, sshMgr *SSHManager) {
	probeRes := probeResult{checkTime: time.Now()}
	if probeIP, err := checkFTPLogin(config); err != nil {
		slog.Error("FTP连接检查失败", "error", err)
		ftpLoginSuccess.Set(0)
		probeRes.err = err.Error()
	} else {
		ftpLoginSuccess.Set(1)
		probeRes.ok = true
		state.probeClientIP = probeIP
	}
	lastProbe.Store(probeRes)

	if err := checkConnections(config, sshMgr); err != nil {
		slog.Error("连接检查失败", "error", err)
	}

	if config.LogFilePath != "" {
		if err := parseFTPLog(config.LogFilePath, state, sshMgr); err != nil {
			slog.Error("解析FTP日志失败", "error", err)
		}
	}

	if config.VsftplogEnabled && config.VsftplogFilePath != "" {
		if err := parseVsftpdLog(config, config.VsftplogFilePath, state, sshMgr); err != nil {
			slog.Error("解析vsftpd日志失败", "error", err)
		}
	}
}

func main() {
	configFile := flag.String("config", "configs/config.json", "配置文件路径")
	logLevel := flag.String("log-level", "info", "日志级别 (debug/info/warn/error)")
	stateFile := flag.String("state-file", "/tmp/vsftp-exporter-state.json", "日志读取位置持久化文件路径(跨重启恢复,默认 /tmp/vsftp-exporter-state.json)")
	flag.Parse()

	var level slog.Level
	switch strings.ToLower(*logLevel) {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: level,
	})))

	slog.Info("正在加载配置文件", "path", *configFile)
	config, err := loadAndValidateConfig(*configFile)
	if err != nil {
		slog.Error("配置加载失败", "error", err)
		os.Exit(1)
	}
	slog.Info("配置加载成功", "host", config.TargetHost, "port", config.FTPPort)

	state := NewExporterState()
	state.stateFilePath = *stateFile

	var sshMgr *SSHManager
	if config.NeedSSH {
		sshMgr = NewSSHManager(config)
		slog.Info("SSH连接管理器已创建", "host", config.TargetHost, "port", config.SSHPort)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	slog.Info("信号处理器已设置")

	slog.Info("启动监控协程", "interval_seconds", config.CheckInterval)
	// 首次启动将日志读取位置初始化到文件末尾(tail -f 语义),避免重启后
	// 从文件头重放历史日志导致 upload/download/bytes 计数被历史填充(BUG-071)。
	if config.LogFilePath != "" {
		initLogReadPosition(sshMgr, config.LogFilePath, state, false)
	}
	if config.VsftplogEnabled && config.VsftplogFilePath != "" {
		initLogReadPosition(sshMgr, config.VsftplogFilePath, state, true)
	}

	go func() {
		safeRunChecks(config, state, sshMgr)

		ticker := time.NewTicker(time.Duration(config.CheckInterval) * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				slog.Info("监控协程收到停止信号")
				return
			case <-ticker.C:
				safeRunChecks(config, state, sshMgr)
			}
		}
	}()

	mux := http.NewServeMux()
	metricsHandler := promhttp.Handler()
	// 抓取完成后标记一次新的抓取，触发 vsftp_files_by_type_total 暂存计数的提交：
	// 首次见到的标签先在抓取中暴露 0 值，再提交增量，避免 increase() 丢失首次增量。
	mux.Handle("/metrics", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		metricsHandler.ServeHTTP(w, r)
		state.bumpScrapeSeq()
	}))
	mux.HandleFunc("/health", healthCheckHandler)

	server := &http.Server{
		Addr:    ":" + config.ListenPort,
		Handler: mux,
	}

	go func() {
		slog.Info("Exporter 启动", "port", config.ListenPort)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("HTTP服务器启动失败", "error", err)
			os.Exit(1)
		}
	}()

	<-sigChan
	slog.Info("收到关闭信号，开始优雅关闭")

	cancel()

	if sshMgr != nil {
		if err := sshMgr.Close(); err != nil {
			slog.Error("关闭SSH连接失败", "error", err)
		}
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Error("服务器关闭失败", "error", err)
	} else {
		slog.Info("服务器已优雅关闭")
	}
}
