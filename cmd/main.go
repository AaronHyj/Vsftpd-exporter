package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
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
	Version       string    `json:"version"`
}

var (
	startTime       = time.Now()
	lastHealthCheck atomic.Value
	appVersion      = "1.0.0"
)

func healthCheckHandler(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	lastHealthCheck.Store(now)

	status := HealthStatus{
		Status:    "healthy",
		Timestamp: now,
		Uptime:    time.Since(startTime).String(),
		Version:   appVersion,
	}

	if lastCheck, ok := lastHealthCheck.Load().(time.Time); ok && !lastCheck.IsZero() {
		status.LastCheckTime = lastCheck.Format(time.RFC3339)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(status); err != nil {
		slog.Error("编码健康检查响应失败", "error", err)
	}
}

func checkFTPLogin(config *Config, state *ExporterState) error {
	conn, err := ftp.Dial(config.TargetHost+":"+config.FTPPort, ftp.DialWithTimeout(10*time.Second))
	if err != nil {
		connectionTimeoutsTotal.Inc()
		return fmt.Errorf("连接FTP服务器失败: %w", err)
	}
	defer conn.Quit()

	err = conn.Login(config.FTPUser, config.FTPPassword)
	if err != nil {
		if strings.Contains(err.Error(), "530") || strings.Contains(err.Error(), "authentication") || strings.Contains(err.Error(), "login") {
			authenticationErrorsTotal.Inc()
			failedLoginsTotal.Inc()
		} else {
			failedLoginsTotal.Inc()
		}
		return fmt.Errorf("FTP登录失败: %w", err)
	}

	return nil
}

func main() {
	configFile := flag.String("config", "configs/config.json", "配置文件路径")
	logLevel := flag.String("log-level", "info", "日志级别 (debug/info/warn/error)")
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
	go func() {
		ticker := time.NewTicker(time.Duration(config.CheckInterval) * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				slog.Info("监控协程收到停止信号")
				return
			case <-ticker.C:
				if err := checkFTPLogin(config, state); err != nil {
					slog.Error("FTP连接检查失败", "error", err)
					ftpLoginSuccess.Set(0)
				} else {
					ftpLoginSuccess.Set(1)
				}

				if err := checkConnections(config, state, sshMgr); err != nil {
					slog.Error("连接检查失败", "error", err)
				}

				if config.LogFilePath != "" {
					if err := parseFTPLog(config, config.LogFilePath, state, sshMgr); err != nil {
						slog.Error("解析FTP日志失败", "error", err)
					}
				}

				if config.VsftplogEnabled && config.VsftplogFilePath != "" {
					if err := parseVsftpdLog(config, config.VsftplogFilePath, state, sshMgr); err != nil {
						slog.Error("解析vsftpd日志失败", "error", err)
					}
				}
			}
		}
	}()

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
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
