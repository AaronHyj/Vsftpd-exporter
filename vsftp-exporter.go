// Package main implements a Prometheus exporter for vsftpd FTP server metrics.
// This exporter collects various metrics from vsftpd including:
// - FTP login status and connection counts
// - File transfer statistics (uploads/downloads)
// - Connection state monitoring
// - Log file parsing for historical data
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/jlaffaye/ftp"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"golang.org/x/crypto/ssh"
)

// Logger 结构化日志记录器
type Logger struct {
	logger *log.Logger
}

// NewLogger 创建新的日志记录器
func NewLogger() *Logger {
	return &Logger{
		logger: log.New(os.Stdout, "", log.LstdFlags|log.Lshortfile),
	}
}

// Info 记录信息级别日志
func (l *Logger) Info(msg string, args ...interface{}) {
	l.logger.Printf("[INFO] "+msg, args...)
}

// Warn 记录警告级别日志
func (l *Logger) Warn(msg string, args ...interface{}) {
	l.logger.Printf("[WARN] "+msg, args...)
}

// Error 记录错误级别日志
func (l *Logger) Error(msg string, args ...interface{}) {
	l.logger.Printf("[ERROR] "+msg, args...)
}

// Debug 记录调试级别日志
func (l *Logger) Debug(msg string, args ...interface{}) {
	l.logger.Printf("[DEBUG] "+msg, args...)
}

// 全局日志记录器
var logger = NewLogger()

// Config 定义了vsftpd exporter的配置结构
// 包含FTP服务器连接信息、监控参数和日志文件路径等配置项
type Config struct {
	TargetHost        string `json:"target_host"`        // 目标服务器地址，支持IP地址或域名
	FTPPort           string `json:"ftp_port"`           // FTP服务器端口，默认为21
	FTPUser           string `json:"ftp_user"`           // FTP登录用户名，用于连接测试
	FTPPassword       string `json:"ftp_password"`       // FTP登录密码，用于连接测试
	NeedSSH           bool   `json:"need_ssh"`           // 是否需要通过SSH连接到目标服务器
	SSHPort           string `json:"ssh_port"`           // SSH连接端口，默认22
	SSHUser           string `json:"ssh_user"`           // SSH登录用户名
	SSHPassword       string `json:"ssh_password"`       // SSH登录密码
	LogFilePath       string `json:"Xferlog_file_path"`  // vsftpd日志文件路径，用于解析传输统计
	ListenPort        string `json:"listen_port"`        // Prometheus metrics HTTP服务监听端口，默认9100
	CheckInterval     int    `json:"check_interval"`     // 监控检查间隔时间（秒），默认30秒
	VsftplogEnabled   bool   `json:"vsftplog_enabled"`   // 是否启用vsftpd日志解析
	VsftplogFilePath  string `json:"vsftplog_file_path"` // vsftpd详细日志文件路径
}

// ExporterState 维护导出器的运行时状态
// 用于跟踪日志文件读取位置和文件句柄，支持日志轮转检测
type ExporterState struct {
	lastProcessedTime time.Time
	ctx               context.Context
	cancel            context.CancelFunc
	logFile           *os.File // 当前打开的日志文件句柄
	lastPosition      int64    // 上次读取到的文件位置，用于增量读取
	
	// 新增字段用于跟踪传输统计
	totalBytesUploaded   int64     // 累计上传字节数
	totalBytesDownloaded int64     // 累计下载字节数
	lastBandwidthCheck   time.Time // 上次带宽检查时间
	lastBytesTransferred int64     // 上次检查时的总传输字节数
	activeTransfers      int       // 当前活跃传输数
	transferStartTimes   map[string]time.Time // 传输开始时间映射

	// === 基于vsftpd.log的新增状态跟踪字段 ===
	
	// vsftpd日志文件相关
	vsftpLogFile         *os.File // vsftpd.log文件句柄
	vsftpLogPosition     int64    // vsftpd.log上次读取位置
	
	// 客户端和用户活动跟踪
	clientLastActivity   map[string]time.Time // 客户端IP -> 最后活动时间
	clientConnectTimes   map[string]time.Time // 客户端IP -> 最后连接时间（用于计算登录延迟）
	userClientMapping    map[string]string    // 用户名 -> 客户端IP映射
	activeProcessIDs     map[string]time.Time // 进程ID -> 最后活动时间
	
	// 快速重连检测
	clientLastConnect    map[string]time.Time // 客户端IP -> 上次连接时间（用于检测快速重连）
	
	// 统计缓存（用于定期更新Gauge类型指标）
	lastUniqueClientUpdate time.Time // 上次更新唯一客户端数量的时间
	lastProcessUpdate      time.Time // 上次更新活跃进程数的时间
}

// Prometheus指标定义
// 这些指标用于监控vsftpd FTP服务器的各种状态和活动
var (
	// ftpLoginSuccess 表示FTP服务器登录状态
	// 值为1表示最近一次登录测试成功，0表示失败
	ftpLoginSuccess = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "vsftp_login_success",
		Help: "Indicates if the login to the FTP server is successful (1 for success, 0 for failure).",
	})

	// ftpConnections 当前FTP连接总数
	// 通过netstat命令统计得出
	ftpConnections = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "vsftp_connections",
		Help: "Current number of FTP connections.",
	})

	// establishedConnections 处于ESTABLISHED状态的FTP连接数
	// 表示当前活跃的FTP数据传输连接
	establishedConnections = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "vsftp_established_connections",
		Help: "Number of ESTABLISHED FTP connections.",
	})

	// closeWaitConnections 处于CLOSE_WAIT状态的FTP连接数
	// 表示等待关闭的连接，可能指示连接泄漏问题
	closeWaitConnections = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "vsftp_close_wait_connections",
		Help: "Number of CLOSE_WAIT FTP connections.",
	})

	// filesDownloaded 从FTP服务器下载的文件总数
	// 从日志文件中解析得出的累计值
	filesDownloaded = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "vsftp_files_received_total",
		Help: "Total number of files received (downloaded) from the FTP server.",
	})

	// filesUploaded 上传到FTP服务器的文件总数
	// 从日志文件中解析得出的累计值
	filesUploaded = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "vsftp_files_sent_total",
		Help: "Total number of files sent (uploaded) to the FTP server.",
	})

	// ftpLoginTime 最后一次成功FTP登录的时间戳
	// Unix时间戳格式，用于监控登录活动
	ftpLoginTime = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "vsftp_last_login_time",
		Help: "Timestamp of last successful FTP login.",
	})

	// ftpLoginTotal FTP登录总次数计数器
	// 从日志文件中解析的累计登录次数
	ftpLoginTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "vsftp_login_total",
		Help: "Total number of FTP logins.",
	})

	// ftpUploadTotal FTP上传操作总次数计数器
	// 从日志文件中解析的累计上传次数
	ftpUploadTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "vsftp_upload_total",
		Help: "Total number of FTP uploads.",
	})

	// ftpDownloadTotal FTP下载操作总次数计数器
	// 从日志文件中解析的累计下载次数
	ftpDownloadTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "vsftp_download_total",
		Help: "Total number of FTP downloads.",
	})

	// 新增的监控指标
	
	// uploadBytesTotal 上传字节总数
	// 统计上传的总字节数
	uploadBytesTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "vsftp_upload_bytes_total",
		Help: "Total number of bytes uploaded.",
	})

	// downloadBytesTotal 下载字节总数
	// 统计下载的总字节数
	downloadBytesTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "vsftp_download_bytes_total",
		Help: "Total number of bytes downloaded.",
	})

	// transferDurationSeconds 文件传输耗时分布（histogram）
	// 记录文件传输操作的耗时分布
	transferDurationSeconds = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "vsftp_transfer_duration_seconds",
		Help:    "Duration of file transfers in seconds.",
		Buckets: prometheus.ExponentialBuckets(0.1, 2, 10), // 0.1s到102.4s的指数分布
	})

	// concurrentTransfers 当前并发传输数
	// 实时统计正在进行的文件传输数量
	concurrentTransfers = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "vsftp_concurrent_transfers",
		Help: "Current number of concurrent file transfers.",
	})

	// averageTransferSpeed 平均传输速度
	// 计算最近一段时间的平均传输速度（字节/秒）
	averageTransferSpeed = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "vsftp_average_transfer_speed_bytes_per_second",
		Help: "Average transfer speed in bytes per second.",
	})

	// failedLoginsTotal 登录失败总次数
	// 统计FTP登录失败的累计次数
	failedLoginsTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "vsftp_failed_logins_total",
		Help: "Total number of failed login attempts.",
	})

	// transferErrorsTotal 传输错误总数（按类型分类）
	// 统计不同类型的传输错误次数
	transferErrorsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "vsftp_transfer_errors_total",
		Help: "Total number of transfer errors by type.",
	}, []string{"type"})

	// connectionTimeoutsTotal 连接超时总次数
	// 统计FTP连接超时的累计次数
	connectionTimeoutsTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "vsftp_connection_timeouts_total",
		Help: "Total number of connection timeouts.",
	})

	// authenticationErrorsTotal 认证错误总次数
	// 统计认证失败的累计次数
	authenticationErrorsTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "vsftp_authentication_errors_total",
		Help: "Total number of authentication errors.",
	})

	// maxConnectionsReachedTotal 达到最大连接数限制的次数
	// 统计服务器达到最大连接数限制的累计次数
	maxConnectionsReachedTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "vsftp_max_connections_reached_total",
		Help: "Total number of times max connections limit was reached.",
	})

	// bandwidthUsage 带宽使用率
	// 实时监控当前的带宽使用情况（字节/秒）
	bandwidthUsage = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "vsftp_bandwidth_usage_bytes_per_second",
		Help: "Current bandwidth usage in bytes per second.",
	})

	// fileCountByExtension 按文件扩展名统计的文件数量
	// 统计不同文件扩展名的传输次数
	fileCountByExtension = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "vsftp_file_count_by_extension",
		Help: "Number of files transferred by extension.",
	}, []string{"extension"})

	// === 基于vsftpd.log的新增监控指标 ===

	// clientConnectionsTotal 按客户端IP统计连接总数
	// 从vsftpd.log中解析CONNECT事件，按客户端IP分类统计
	clientConnectionsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "vsftp_client_connections_total",
		Help: "Total number of connections by client IP address.",
	}, []string{"client_ip"})

	// uniqueClients 当前活跃的唯一客户端数量
	// 统计最近一段时间内有活动的不同客户端IP数量
	uniqueClients = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "vsftp_unique_clients",
		Help: "Number of unique client IP addresses with recent activity.",
	})

	// userLoginsTotal 按用户名统计登录总数
	// 从vsftpd.log中解析OK LOGIN事件，按用户名分类统计
	userLoginsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "vsftp_user_logins_total",
		Help: "Total number of successful logins by username.",
	}, []string{"username"})

	// userConnectionsTotal 按用户名统计连接总数
	// 关联CONNECT和LOGIN事件，按用户名统计连接数
	userConnectionsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "vsftp_user_connections_total",
		Help: "Total number of connections by username.",
	}, []string{"username"})

	// connectionLoginDelaySeconds 连接到登录的时间延迟分布
	// 统计从CONNECT事件到OK LOGIN事件的时间间隔
	connectionLoginDelaySeconds = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "vsftp_connection_login_delay_seconds",
		Help:    "Time delay between connection and successful login in seconds.",
		Buckets: prometheus.ExponentialBuckets(0.001, 2, 15), // 1ms到16s的指数分布
	})

	// rapidReconnectionsTotal 快速重连次数统计
	// 统计30秒内同一IP的重复连接次数
	rapidReconnectionsTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "vsftp_rapid_reconnections_total",
		Help: "Total number of rapid reconnections (same IP within 30 seconds).",
	})

	// activeProcesses 当前活跃的vsftpd进程数
	// 从vsftpd.log中统计不同进程ID的数量
	activeProcesses = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "vsftp_active_processes",
		Help: "Number of active vsftpd processes based on log entries.",
	})

	// clientActivityByHour 按小时统计客户端活动
	// 统计不同时间段的客户端连接活动
	clientActivityByHour = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "vsftp_client_activity_by_hour",
		Help: "Client connection activity by hour of day.",
	}, []string{"hour"})

	// loginFailuresByClient 按客户端IP统计登录失败次数
	// 从vsftpd.log中解析登录失败事件
	loginFailuresByClient = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "vsftp_login_failures_by_client",
		Help: "Number of login failures by client IP address.",
	}, []string{"client_ip"})

	// clientFilesTotal 按客户端IP统计上传和下载文件数量
	// 从xferlog中解析文件传输记录，按客户端IP和传输方向分类统计
	clientFilesTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "vsftp_client_files_total",
		Help: "Total number of files transferred by client IP address and direction.",
	}, []string{"client_ip", "direction"})
)

// init 初始化函数，在程序启动时自动执行
// 负责向Prometheus注册所有的监控指标
func init() {
	// 注册所有Prometheus指标到默认注册表
	// 这些指标将通过/metrics端点暴露给Prometheus
	prometheus.MustRegister(ftpLoginSuccess)     // FTP登录状态指标
	prometheus.MustRegister(ftpConnections)      // FTP连接数指标
	prometheus.MustRegister(establishedConnections) // 活跃连接数指标
	prometheus.MustRegister(closeWaitConnections)   // 等待关闭连接数指标
	prometheus.MustRegister(filesDownloaded)     // 文件下载总数指标
	prometheus.MustRegister(filesUploaded)       // 文件上传总数指标
	prometheus.MustRegister(ftpLoginTime)        // 最后登录时间指标
	prometheus.MustRegister(ftpLoginTotal)       // 登录总次数计数器
	prometheus.MustRegister(ftpUploadTotal)      // 上传总次数计数器
	prometheus.MustRegister(ftpDownloadTotal)    // 下载总次数计数器

	// 注册新增的监控指标
	prometheus.MustRegister(uploadBytesTotal)          // 上传字节总数指标
	prometheus.MustRegister(downloadBytesTotal)        // 下载字节总数指标
	prometheus.MustRegister(transferDurationSeconds)   // 传输耗时分布指标
	prometheus.MustRegister(concurrentTransfers)       // 并发传输数指标
	prometheus.MustRegister(averageTransferSpeed)      // 平均传输速度指标
	prometheus.MustRegister(failedLoginsTotal)         // 登录失败总次数指标
	prometheus.MustRegister(transferErrorsTotal)       // 传输错误总数指标
	prometheus.MustRegister(connectionTimeoutsTotal)   // 连接超时总次数指标
	prometheus.MustRegister(authenticationErrorsTotal) // 认证错误总次数指标
	prometheus.MustRegister(maxConnectionsReachedTotal) // 最大连接数限制次数指标
	prometheus.MustRegister(bandwidthUsage)            // 带宽使用率指标
	prometheus.MustRegister(fileCountByExtension)      // 按扩展名统计文件数量指标

	// 注册基于vsftpd.log的新增监控指标
	prometheus.MustRegister(clientConnectionsTotal)      // 按客户端IP统计连接总数指标
	prometheus.MustRegister(uniqueClients)               // 唯一客户端数量指标
	prometheus.MustRegister(userLoginsTotal)             // 按用户名统计登录总数指标
	prometheus.MustRegister(userConnectionsTotal)        // 按用户名统计连接总数指标
	prometheus.MustRegister(connectionLoginDelaySeconds) // 连接到登录延迟分布指标
	prometheus.MustRegister(rapidReconnectionsTotal)     // 快速重连次数指标
	prometheus.MustRegister(activeProcesses)             // 活跃进程数指标
	prometheus.MustRegister(clientActivityByHour)        // 按小时统计客户端活动指标
	prometheus.MustRegister(loginFailuresByClient)       // 按客户端IP统计登录失败指标
	prometheus.MustRegister(clientFilesTotal)            // 按客户端IP统计文件传输数量指标
}

// main 程序主入口函数
// 负责初始化配置、启动监控协程、设置HTTP服务器和处理优雅关闭
func main() {
	// 解析命令行参数
	configFile := flag.String("config", "config.json", "配置文件路径")
	flag.Parse()

	// 第一步：加载并验证配置文件
	// 配置文件包含FTP服务器信息、监控参数等关键设置
	logger.Info("正在加载配置文件: %s", *configFile)
	config, err := loadAndValidateConfig(*configFile)
	if err != nil {
		logger.Error("配置加载失败: %v", err)
		os.Exit(1)
	}
	logger.Info("配置加载成功，目标服务器: %s:%s", config.TargetHost, config.FTPPort)

	// 第二步：初始化导出器运行时状态
	// 用于维护日志文件句柄和读取位置等状态信息
	state := &ExporterState{
		transferStartTimes: make(map[string]time.Time),
		lastBandwidthCheck: time.Now(),
	}

	// 第三步：创建上下文用于优雅关闭
	// 当收到终止信号时，通过context通知所有协程停止工作
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 第四步：设置系统信号处理
	// 监听SIGINT(Ctrl+C)和SIGTERM信号，用于优雅关闭程序
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	logger.Info("信号处理器已设置")

	// 第五步：启动后台监控协程
	// 定期执行FTP连接测试、连接数统计和日志解析等监控任务
	logger.Info("启动监控协程，检查间隔: %d秒", config.CheckInterval)
	go func() {
		// 创建定时器，按配置的间隔执行监控任务
		ticker := time.NewTicker(time.Duration(config.CheckInterval) * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				// 收到停止信号，退出监控协程
				logger.Info("监控协程收到停止信号")
				return
			case <-ticker.C:
				// 定时器触发，执行监控任务
				
				// 任务1：检查FTP服务器连接状态
				// 尝试登录FTP服务器以验证服务可用性
				if err := checkFTPLogin(config, state); err != nil {
					logger.Error("FTP连接检查失败: %v", err)
					ftpLoginSuccess.Set(0) // 设置登录失败状态
				} else {
					ftpLoginSuccess.Set(1) // 设置登录成功状态
				}

				// 任务2：统计当前FTP连接数
				// 通过netstat命令获取网络连接状态
				if err := checkConnections(config, state); err != nil {
					logger.Error("连接检查失败: %v", err)
				}
				
				// 任务3：解析FTP日志文件
				// 从vsftpd日志中提取传输统计信息
				if config.LogFilePath != "" {
					if err := parseFTPLog(config, config.LogFilePath, state); err != nil {
						logger.Error("解析FTP日志失败: %v", err)
					}
				}
				
				// 任务4：解析vsftpd详细日志文件
				// 从vsftpd.log中提取连接和登录统计信息
				if config.VsftplogEnabled && config.VsftplogFilePath != "" {
					if err := parseVsftpdLog(config, config.VsftplogFilePath, state); err != nil {
						logger.Error("解析vsftpd日志失败: %v", err)
					}
				}
			}
		}
	}()

	// 第六步：配置并启动HTTP服务器
	// 提供Prometheus metrics端点和健康检查端点
	server := &http.Server{
		Addr:    ":" + config.ListenPort, // 监听配置的端口
		Handler: nil,                     // 使用默认的HTTP多路复用器
	}

	// 注册HTTP路由处理器
	http.Handle("/metrics", promhttp.Handler())    // Prometheus指标端点
	http.HandleFunc("/health", healthCheckHandler) // 健康检查端点

	// 在单独的协程中启动HTTP服务器，避免阻塞主线程
	go func() {
		logger.Info("Exporter 启动，监听端口 %s", config.ListenPort)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			// 如果不是正常关闭导致的错误，则记录错误并退出
			logger.Error("HTTP服务器启动失败: %v", err)
			os.Exit(1)
		}
	}()

	// 第七步：等待终止信号并执行优雅关闭
	// 程序将在此处阻塞，直到收到SIGINT或SIGTERM信号
	<-sigChan
	logger.Info("收到关闭信号，开始优雅关闭...")

	// 开始优雅关闭流程
	logger.Info("正在关闭服务器...")
	// 取消context，通知所有协程停止工作
	cancel()

	// 清理资源：关闭日志文件句柄
	if state.logFile != nil {
		if err := state.logFile.Close(); err != nil {
			logger.Error("关闭日志文件失败: %v", err)
		} else {
			logger.Info("日志文件已关闭")
		}
	}

	// 优雅关闭HTTP服务器
	// 给服务器5秒时间完成当前正在处理的请求
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("服务器关闭失败: %v", err)
	} else {
		logger.Info("服务器已优雅关闭")
	}
}

// loadAndValidateConfig 加载并验证配置文件
func loadAndValidateConfig(file string) (*Config, error) {
	var config Config
	configFile, err := os.Open(file)
	if err != nil {
		return nil, fmt.Errorf("打开配置文件失败: %w", err)
	}
	defer configFile.Close()

	byteValue, err := io.ReadAll(configFile)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	err = json.Unmarshal(byteValue, &config)
	if err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}

	// 设置默认值
	if config.FTPPort == "" {
		config.FTPPort = "21"
	}
	if config.ListenPort == "" {
		config.ListenPort = "9100"
	}
	if config.CheckInterval <= 0 {
		config.CheckInterval = 30
	}

	// 验证必需配置项
	if config.TargetHost == "" {
		return nil, fmt.Errorf("目标主机地址不能为空")
	}
	// 验证目标主机地址格式
	if !isValidHost(config.TargetHost) {
		return nil, fmt.Errorf("目标主机地址格式无效: %s", config.TargetHost)
	}

	if config.FTPUser == "" {
		return nil, fmt.Errorf("FTP用户名不能为空")
	}
	// 验证用户名长度和字符
	if len(config.FTPUser) > 64 || !isValidUsername(config.FTPUser) {
		return nil, fmt.Errorf("FTP用户名格式无效或过长")
	}

	if config.FTPPassword == "" {
		return nil, fmt.Errorf("FTP密码不能为空")
	}
	// 验证密码长度
	if len(config.FTPPassword) > 128 {
		return nil, fmt.Errorf("FTP密码过长（最大128字符）")
	}

	// 验证端口范围
	ftpPort, err := strconv.Atoi(config.FTPPort)
	if err != nil {
		return nil, fmt.Errorf("FTP端口号格式无效: %s", config.FTPPort)
	}
	if ftpPort < 1 || ftpPort > 65535 {
		return nil, fmt.Errorf("FTP端口必须在1-65535范围内")
	}

	listenPort, err := strconv.Atoi(config.ListenPort)
	if err != nil {
		return nil, fmt.Errorf("监听端口号格式无效: %s", config.ListenPort)
	}
	if listenPort < 1 || listenPort > 65535 {
		return nil, fmt.Errorf("监听端口必须在1-65535范围内")
	}

	// 验证检查间隔
	if config.CheckInterval < 1 || config.CheckInterval > 3600 {
		return nil, fmt.Errorf("检查间隔必须在1-3600秒范围内")
	}

	// 验证日志文件路径（如果提供）
	if config.LogFilePath != "" {
		// 扩展路径（支持环境变量和相对路径）
		expandedPath, err := expandLogFilePath(config.LogFilePath)
		if err != nil {
			return nil, fmt.Errorf("日志文件路径处理失败: %w", err)
		}
		// 更新配置中的路径为扩展后的绝对路径
		config.LogFilePath = expandedPath
		
		// 使用增强的日志文件检查函数
		if err := checkLogFileAccess(config.LogFilePath); err != nil {
			return nil, fmt.Errorf("日志文件路径验证失败: %w", err)
		}
	} else {
		logger.Warn("未配置日志文件路径，将无法解析FTP传输日志")
	}

	return &config, nil
}

// healthCheckHandler 处理健康检查请求
// 提供简单的HTTP健康检查端点，用于监控系统检查服务状态
// 返回HTTP 200状态码和"OK"文本，表示服务正常运行
func healthCheckHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK) // 设置HTTP状态码为200
	w.Write([]byte("OK"))        // 返回简单的OK响应
}

// isValidHost 验证主机地址格式（IP地址或域名）
func isValidHost(host string) bool {
	// 检查是否为有效的IP地址
	if net.ParseIP(host) != nil {
		return true
	}
	// 检查是否为有效的域名
	if len(host) == 0 || len(host) > 253 {
		return false
	}
	// 简单的域名格式验证
	domainRegex := regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9]))*$`)
	return domainRegex.MatchString(host)
}

// isValidUsername 验证用户名格式（字母、数字、下划线、连字符）
func isValidUsername(username string) bool {
	if len(username) == 0 {
		return false
	}
	// 用户名只能包含字母、数字、下划线和连字符
	usernameRegex := regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
	return usernameRegex.MatchString(username)
}

// createSSHClient 创建SSH客户端连接
// 根据配置建立到目标服务器的SSH连接，支持密码认证
// 参数:
//   config: 包含SSH连接信息的配置对象
// 返回:
//   *ssh.Client: 成功时返回SSH客户端，失败时返回nil
//   error: 连接失败时返回错误信息
func createSSHClient(config *Config) (*ssh.Client, error) {
	
	// 设置SSH客户端配置
	sshConfig := &ssh.ClientConfig{
		User: config.SSHUser,
		Auth: []ssh.AuthMethod{
			ssh.Password(config.SSHPassword),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // 注意：生产环境应该验证主机密钥
		Timeout:         10 * time.Second,
	}
	
	// 建立SSH连接
	address := config.TargetHost + ":" + config.SSHPort
	client, err := ssh.Dial("tcp", address, sshConfig)
	if err != nil {
		return nil, fmt.Errorf("SSH连接失败: %w", err)
	}
	
	// 添加SSH连接成功的INFO日志
	logger.Info("SSH连接成功: %s@%s:%s", config.SSHUser, config.TargetHost, config.SSHPort)
	
	return client, nil
}

// executeSSHCommand 通过SSH执行远程命令
// 在目标服务器上执行指定命令并返回输出结果
// 参数:
//   client: SSH客户端连接
//   command: 要执行的命令
// 返回:
//   string: 命令输出结果
//   error: 执行失败时返回错误信息
func executeSSHCommand(client *ssh.Client, command string) (string, error) {
	// 创建SSH会话
	session, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("创建SSH会话失败: %w", err)
	}
	defer session.Close()
	
	// 执行命令并获取输出
	output, err := session.Output(command)
	if err != nil {
		return "", fmt.Errorf("执行SSH命令失败: %w", err)
	}
	
	return string(output), nil
}

// checkFTPLogin 检查FTP服务器连接和登录状态
// 尝试连接到配置的FTP服务器并使用提供的凭据进行登录
// 用于验证FTP服务的可用性和认证配置的正确性
// 参数:
//   config: 包含FTP连接信息的配置对象
//   state: 导出器状态对象，用于更新相关指标
// 返回:
//   error: 如果连接或登录失败则返回错误，成功则返回nil
func checkFTPLogin(config *Config, state *ExporterState) error {
	// 设置连接超时
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 建立到FTP服务器的连接
	conn, err := ftp.Dial(config.TargetHost + ":" + config.FTPPort)
	if err != nil {
		// 检查是否为超时错误
		if ctx.Err() == context.DeadlineExceeded {
			connectionTimeoutsTotal.Inc()
			return fmt.Errorf("连接FTP服务器超时: %w", err)
		}
		connectionTimeoutsTotal.Inc()
		return fmt.Errorf("连接FTP服务器失败: %w", err)
	}
	defer conn.Quit() // 确保连接在函数结束时关闭

	// 尝试使用配置的用户名和密码登录
	err = conn.Login(config.FTPUser, config.FTPPassword)
	if err != nil {
		// 区分认证错误和其他登录失败
		if strings.Contains(err.Error(), "530") || strings.Contains(err.Error(), "authentication") || strings.Contains(err.Error(), "login") {
			authenticationErrorsTotal.Inc()
			failedLoginsTotal.Inc()
		} else {
			failedLoginsTotal.Inc()
		}
		return fmt.Errorf("FTP登录失败: %w", err)
	}

	return nil // 登录成功
}

// checkConnections 检查FTP服务器的网络连接状态
// 根据配置决定是本地执行netstat还是通过SSH远程执行
// 分别统计总连接数、已建立连接数和等待关闭连接数
// 参数:
//   config: 包含FTP端口信息和SSH配置的配置对象
//   state: 导出器状态对象（当前未使用但保留用于扩展）
// 返回:
//   error: 如果执行netstat命令失败则返回错误，成功则返回nil
func checkConnections(config *Config, state *ExporterState) error {
	var output string
	
	if config.NeedSSH {
		// 通过SSH远程执行netstat命令
		
		// 创建SSH客户端
		sshClient, err := createSSHClient(config)
		if err != nil {
			return fmt.Errorf("创建SSH连接失败: %w", err)
		}
		defer sshClient.Close()
		
		// 远程执行netstat命令
		output, err = executeSSHCommand(sshClient, "netstat -anp")
		if err != nil {
			return fmt.Errorf("SSH远程执行netstat命令失败: %w", err)
		}
	} else {
		// 本地执行netstat命令
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		
		cmd := exec.CommandContext(ctx, "netstat", "-anp")
		outputBytes, err := cmd.Output()
		if err != nil {
			if ctx.Err() == context.DeadlineExceeded {
				return fmt.Errorf("netstat命令执行超时")
			}
			return fmt.Errorf("执行netstat命令失败: %w", err)
		}
		output = string(outputBytes)
	}

	// 记录完整的netstat输出（截取前100行避免日志过长）
	lines := strings.Split(output, "\n")
	// 处理netstat输出（移除了DEBUG日志输出）

	// 解析netstat输出，统计连接数
	totalConnections := 0      // 总连接数计数器
	establishedCount := 0      // 已建立连接数计数器
	closeWaitCount := 0        // 等待关闭连接数计数器
	listenCount := 0           // 监听端口数计数器
	otherStateCount := 0       // 其他状态连接数计数器


	// 遍历每一行，查找包含FTP端口的连接
	for _, line := range lines {
		if line == "" {
			continue
		}
		
		// 检查是否包含FTP端口，支持多种格式匹配
		portPattern := ":" + config.FTPPort
		if strings.Contains(line, portPattern) {
			// 检查是否为vsftpd进程的连接
			if strings.Contains(line, "vsftpd") || strings.Contains(line, "ftp") {
				totalConnections++ // 发现FTP端口连接，总数加1
				
				// 根据连接状态进行分类统计
				if strings.Contains(line, "ESTABLISHED") {
					establishedCount++ // 已建立的连接
				} else if strings.Contains(line, "CLOSE_WAIT") {
					closeWaitCount++ // 等待关闭的连接
				} else if strings.Contains(line, "LISTEN") {
					listenCount++ // 监听端口
				} else {
					otherStateCount++
				}
			} else {
				// 即使没有进程信息，也尝试按端口匹配
				// 这是为了兼容某些系统上netstat -p可能需要root权限的情况
				if strings.Contains(line, "tcp") && strings.Contains(line, portPattern) {
					totalConnections++
					
					if strings.Contains(line, "ESTABLISHED") {
						establishedCount++
					} else if strings.Contains(line, "CLOSE_WAIT") {
						closeWaitCount++
					} else if strings.Contains(line, "LISTEN") {
						listenCount++
					} else {
						otherStateCount++
					}
				}
			}
		}
	}

	// 更新Prometheus指标
	ftpConnections.Set(float64(totalConnections))           // 设置总连接数指标
	establishedConnections.Set(float64(establishedCount))   // 设置已建立连接数指标
	closeWaitConnections.Set(float64(closeWaitCount))       // 设置等待关闭连接数指标
	
	return nil // 统计完成
}

// extractTimestamp 从日志行中提取时间戳
// 支持多种常见的时间戳格式，将其转换为Unix时间戳
// 用于跟踪FTP活动的最后发生时间
// 参数:
//   line: 包含时间戳的日志行文本
// 返回:
//   int64: 成功解析则返回Unix时间戳，失败则返回0
func extractTimestamp(line string) int64 {
	// 尝试匹配第一种时间戳格式：YYYY-MM-DD HH:MM:SS
	// 例如：2024-01-15 14:30:25
	timeRegex1 := regexp.MustCompile(`(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2})`)
	
	// 尝试匹配第二种时间戳格式：Mon Jan _2 HH:MM:SS YYYY
	// 例如：Mon Jan 15 14:30:25 2024
	timeRegex2 := regexp.MustCompile(`(\w{3} \w{3}\s+\d{1,2} \d{2}:\d{2}:\d{2} \d{4})`)
	
	// 尝试解析第一种格式
	if match := timeRegex1.FindString(line); match != "" {
		if t, err := time.Parse("2006-01-02 15:04:05", match); err == nil {
			return t.Unix() // 返回Unix时间戳
		}
	}
	
	// 尝试解析第二种格式
	if match := timeRegex2.FindString(line); match != "" {
		if t, err := time.Parse("Mon Jan _2 15:04:05 2006", match); err == nil {
			return t.Unix() // 返回Unix时间戳
		}
	}
	
	return 0 // 无法解析时间戳，返回0
}

// parseTransferLog 解析传输日志，提取字节数、文件名和传输时间
func parseTransferLog(line, direction string) (bytes int64, filename string, duration float64) {
	// 示例日志格式："OK UPLOAD: Client "192.168.1.100", "/path/to/file.txt", 1024 bytes, 1.5 seconds"
	// 或者："OK DOWNLOAD: Client "192.168.1.100", "/path/to/file.txt", 2048 bytes, 2.3 seconds"
	
	// 提取字节数
	if bytesMatch := regexp.MustCompile(`(\d+)\s+bytes`).FindStringSubmatch(line); len(bytesMatch) > 1 {
		if b, err := strconv.ParseInt(bytesMatch[1], 10, 64); err == nil {
			bytes = b
		}
	}
	
	// 提取文件名
	if filenameMatch := regexp.MustCompile(`"([^"]+\.[^"]+)"`).FindStringSubmatch(line); len(filenameMatch) > 1 {
		filename = filenameMatch[1]
	}
	
	// 提取传输时间
	if durationMatch := regexp.MustCompile(`([0-9.]+)\s+seconds`).FindStringSubmatch(line); len(durationMatch) > 1 {
		if d, err := strconv.ParseFloat(durationMatch[1], 64); err == nil {
			duration = d
		}
	}
	
	// 如果没有找到具体信息，使用默认值
	if bytes == 0 {
		bytes = 1024 // 默认1KB
	}
	if filename == "" {
		filename = "unknown.txt"
	}
	if duration == 0 {
		duration = 1.0 // 默认1秒
	}
	
	return bytes, filename, duration
}

// extractFileExtension 从文件名中提取扩展名
func extractFileExtension(filename string) string {
	if filename == "" {
		return ""
	}
	
	// 获取文件扩展名
	ext := strings.ToLower(filepath.Ext(filename))
	
	// 只返回我们关心的扩展名
	switch ext {
	case ".xml", ".ts", ".jpg", ".m3u8", ".png":
		return ext
	default:
		return ""
	}
}

// expandLogFilePath 扩展日志文件路径，支持环境变量和相对路径
// 参数:
//   path: 原始路径，可能包含环境变量或相对路径
// 返回:
//   string: 扩展后的绝对路径
//   error: 如果路径处理失败则返回错误
func expandLogFilePath(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("日志文件路径不能为空")
	}

	// 扩展环境变量
	expandedPath := os.ExpandEnv(path)

	// 转换为绝对路径
	absPath, err := filepath.Abs(expandedPath)
	if err != nil {
		return "", fmt.Errorf("无法转换为绝对路径: %w", err)
	}

	// 清理路径（移除多余的分隔符等）
	cleanPath := filepath.Clean(absPath)

	return cleanPath, nil
}

// testLogFileAccess 测试日志文件的访问性
// 这是一个独立的测试函数，可以在程序启动时或需要时调用
// 参数:
//   logPath: 日志文件路径
// 返回:
//   bool: 文件是否可访问
//   string: 详细的测试结果信息
func testLogFileAccess(logPath string) (bool, string) {
	var results []string
	
	// 测试路径扩展
	expandedPath, err := expandLogFilePath(logPath)
	if err != nil {
		return false, fmt.Sprintf("路径扩展失败: %v", err)
	}
	results = append(results, fmt.Sprintf("✓ 路径扩展成功: %s -> %s", logPath, expandedPath))
	
	// 测试文件访问性
	err = checkLogFileAccess(expandedPath)
	if err != nil {
		return false, fmt.Sprintf("访问性检查失败: %v\n已完成的检查:\n%s", err, strings.Join(results, "\n"))
	}
	results = append(results, "✓ 文件访问性检查通过")
	
	// 测试文件读取（读取前几行）
	file, err := os.Open(expandedPath)
	if err != nil {
		return false, fmt.Sprintf("文件打开失败: %v\n已完成的检查:\n%s", err, strings.Join(results, "\n"))
	}
	defer file.Close()
	
	scanner := bufio.NewScanner(file)
	lineCount := 0
	for scanner.Scan() && lineCount < 3 {
		lineCount++
	}
	if err := scanner.Err(); err != nil {
		return false, fmt.Sprintf("文件读取失败: %v\n已完成的检查:\n%s", err, strings.Join(results, "\n"))
	}
	results = append(results, fmt.Sprintf("✓ 文件读取测试通过，读取了 %d 行", lineCount))
	
	return true, fmt.Sprintf("所有测试通过:\n%s", strings.Join(results, "\n"))
}

// checkLogFileAccess 检查日志文件的存在性、权限和可读性
// 提供详细的错误信息帮助诊断问题
// 参数:
//   logPath: 日志文件路径
// 返回:
//   error: 如果检查失败则返回详细错误信息，成功则返回nil
func checkLogFileAccess(logPath string) error {
	// 检查路径是否为空
	if logPath == "" {
		return fmt.Errorf("日志文件路径为空")
	}

	// 检查文件是否存在
	fileInfo, err := os.Stat(logPath)
	if os.IsNotExist(err) {
		// 检查父目录是否存在
		dir := filepath.Dir(logPath)
		if _, dirErr := os.Stat(dir); os.IsNotExist(dirErr) {
			return fmt.Errorf("日志文件不存在且父目录不存在: %s (父目录: %s)", logPath, dir)
		}
		return fmt.Errorf("日志文件不存在: %s (请检查vsftpd配置中的xferlog_file设置)", logPath)
	}
	if err != nil {
		return fmt.Errorf("无法访问日志文件: %s, 错误: %v", logPath, err)
	}

	// 检查是否为常规文件
	if !fileInfo.Mode().IsRegular() {
		return fmt.Errorf("指定路径不是常规文件: %s (文件类型: %s)", logPath, fileInfo.Mode().String())
	}

	// 检查文件是否可读
	file, err := os.Open(logPath)
	if err != nil {
		if os.IsPermission(err) {
			return fmt.Errorf("没有读取日志文件的权限: %s (当前用户可能需要读取权限)", logPath)
		}
		return fmt.Errorf("无法打开日志文件: %s, 错误: %v", logPath, err)
	}
	file.Close()

	// 检查文件大小（可选警告）
	if fileInfo.Size() == 0 {
		logger.Warn("日志文件为空: %s (这可能是正常的，如果vsftpd刚启动)", logPath)
	}

	return nil
}

// readRemoteFile 通过SSH读取远程文件内容
// 支持增量读取，只读取从指定位置开始的新内容
func readRemoteFile(config *Config, filePath string, startPosition int64) ([]string, int64, error) {
	if !config.NeedSSH {
		// 如果不需要SSH，直接读取本地文件
		return readLocalFile(filePath, startPosition)
	}

	// 添加SSH读取文件开始的INFO日志
	logger.Info("通过SSH连接到 %s 读取文件: %s", config.TargetHost, filePath)

	// 创建SSH连接
	sshClient, err := createSSHClient(config)
	if err != nil {
		return nil, 0, fmt.Errorf("创建SSH连接失败: %w", err)
	}
	defer sshClient.Close()

	// 使用tail命令从指定位置开始读取文件
	var command string
	if startPosition > 0 {
		// 使用dd命令跳过已读取的字节
		command = fmt.Sprintf("dd if=%s bs=1 skip=%d 2>/dev/null", filePath, startPosition)
	} else {
		// 读取整个文件
		command = fmt.Sprintf("cat %s", filePath)
	}

	output, err := executeSSHCommand(sshClient, command)
	if err != nil {
		return nil, 0, fmt.Errorf("执行SSH命令失败: %w", err)
	}

	lines := strings.Split(output, "\n")
	// 移除最后一个空行
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	// 计算新的位置
	newPosition := startPosition + int64(len(output))

	// 添加SSH读取文件成功的INFO日志
	logger.Info("SSH读取文件成功，读取 %d 行，新位置: %d", len(lines), newPosition)

	return lines, newPosition, nil
}

// readLocalFile 读取本地文件内容（用于不需要SSH的情况）
func readLocalFile(filePath string, startPosition int64) ([]string, int64, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, 0, fmt.Errorf("打开本地文件失败: %w", err)
	}
	defer file.Close()

	// 跳到指定位置
	if _, err := file.Seek(startPosition, 0); err != nil {
		return nil, 0, fmt.Errorf("定位文件位置失败: %w", err)
	}

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		return nil, 0, fmt.Errorf("读取文件失败: %w", err)
	}

	// 获取当前文件位置
	currentPos, err := file.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, 0, fmt.Errorf("获取文件位置失败: %w", err)
	}

	return lines, currentPos, nil
}

// parseStandardXferlog 解析标准xferlog格式
// 标准格式：Wed Oct 15 16:04:42 2025 1 172.25.235.63 19236361 /txt/yd_platform.txt b _ o g dstore ftp 0 * c
// 字段说明：时间戳 传输时间(秒) 客户端IP 文件大小(字节) 文件路径 传输类型 特殊动作标志 方向 访问模式 用户名 服务名 认证方法 认证用户ID 完成状态
func parseStandardXferlog(line string) (direction string, clientIP string, fileSize int64, filePath string, transferTime int, username string, completed bool) {
	fields := strings.Fields(line)
	if len(fields) < 18 {
		return "", "", 0, "", 0, "", false
	}

	// 解析各字段 (基于标准xferlog格式)
	// 格式: Wed Oct 15 16:04:42 2025 1 172.25.235.63 19236361 /txt/yd_platform.txt b _ o g dstore ftp 0 * c
	transferTimeStr := fields[5]  // 传输时间（秒）
	clientIP = fields[6]          // 客户端IP
	fileSizeStr := fields[7]      // 文件大小（字节）
	filePath = fields[8]          // 文件路径
	direction = fields[11]        // 方向：o（出站/下载）或i（入站/上传）
	username = fields[13]         // 用户名
	completionStatus := fields[17] // 完成状态：c（完成）或i（未完成）

	// 解析传输时间
	if t, err := strconv.Atoi(transferTimeStr); err == nil {
		transferTime = t
	}

	// 解析文件大小
	if size, err := strconv.ParseInt(fileSizeStr, 10, 64); err == nil {
		fileSize = size
	}

	// 检查是否完成
	completed = (completionStatus == "c")

	return direction, clientIP, fileSize, filePath, transferTime, username, completed
}

// parseFTPLog 解析FTP日志文件并更新相关指标
// 支持标准xferlog格式和SSH远程读取
// 采用增量读取方式，只处理自上次读取以来新增的日志内容
// 参数:
//   config: 配置对象，包含SSH连接信息
//   logPath: FTP日志文件的完整路径
//   state: 导出器状态对象，用于维护读取位置
// 返回:
//   error: 如果文件操作或解析失败则返回错误，成功则返回nil
func parseFTPLog(config *Config, logPath string, state *ExporterState) error {
	// 添加开始解析的INFO日志
	logger.Info("开始解析FTP日志文件: %s，从位置 %d 开始", logPath, state.lastPosition)
	
	// 读取日志文件内容
	lines, newPosition, err := readRemoteFile(config, logPath, state.lastPosition)
	if err != nil {
		return fmt.Errorf("读取日志文件失败: %w", err)
	}

	linesProcessed := 0
	loginCount := 0
	uploadCount := 0
	downloadCount := 0
	const maxLinesPerRead = 1000 // 限制每次处理的行数

	// 用于计算带宽的临时变量
	currentTime := time.Now()
	totalBytesThisRound := int64(0)

	for _, line := range lines {
		if linesProcessed >= maxLinesPerRead {
			break
		}
		
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		
		linesProcessed++

		// 尝试解析标准xferlog格式
		direction, clientIP, fileSize, filePath, transferTime, username, completed := parseStandardXferlog(line)
		
		if direction != "" && completed {
			// 更新客户端连接统计
			if clientIP != "" {
				clientConnectionsTotal.WithLabelValues(clientIP).Inc()
			}

			// 更新用户统计
			if username != "" {
				userConnectionsTotal.WithLabelValues(username).Inc()
			}

			// 根据方向统计上传/下载
			if direction == "i" { // 入站 = 上传到服务器
				uploadCount++
				ftpUploadTotal.Inc()
				filesUploaded.Inc() // 更新Gauge指标
				
				// 统计按客户端IP的上传文件数量
				if clientIP != "" {
					clientFilesTotal.WithLabelValues(clientIP, "upload").Inc()
				}
				
				if fileSize > 0 {
					uploadBytesTotal.Add(float64(fileSize))
					state.totalBytesUploaded += fileSize
					totalBytesThisRound += fileSize
				}
				
				// 记录传输时间
				if transferTime > 0 {
					transferDurationSeconds.Observe(float64(transferTime))
				}
				
				// 统计文件扩展名
				if ext := extractFileExtension(filePath); ext != "" {
					fileCountByExtension.WithLabelValues(ext).Inc()
				}
				
			} else if direction == "o" { // 出站 = 从服务器下载
				downloadCount++
				ftpDownloadTotal.Inc()
				filesDownloaded.Inc() // 更新Gauge指标
				
				// 统计按客户端IP的下载文件数量
				if clientIP != "" {
					clientFilesTotal.WithLabelValues(clientIP, "download").Inc()
				}
				
				if fileSize > 0 {
					downloadBytesTotal.Add(float64(fileSize))
					state.totalBytesDownloaded += fileSize
					totalBytesThisRound += fileSize
				}
				
				// 记录传输时间
				if transferTime > 0 {
					transferDurationSeconds.Observe(float64(transferTime))
				}
				
				// 统计文件扩展名
				if ext := extractFileExtension(filePath); ext != "" {
					fileCountByExtension.WithLabelValues(ext).Inc()
				}
			}
		}

		// 兼容旧格式的解析（保留向后兼容性）
		// 解析登录成功的日志
		if strings.Contains(line, "OK LOGIN") {
			loginCount++
			// 尝试解析时间戳
			if timestamp := extractTimestamp(line); timestamp > 0 {
				ftpLoginTime.Set(float64(timestamp))
			}
			ftpLoginTotal.Inc()
		}

		// 解析登录失败的日志
		if strings.Contains(line, "FAIL LOGIN") || strings.Contains(line, "530") {
			failedLoginsTotal.Inc()
			if strings.Contains(line, "530") {
				authenticationErrorsTotal.Inc()
			}
		}

		// 解析传输错误
		if strings.Contains(line, "FAIL UPLOAD") {
			transferErrorsTotal.WithLabelValues("upload").Inc()
		} else if strings.Contains(line, "FAIL DOWNLOAD") {
			transferErrorsTotal.WithLabelValues("download").Inc()
		} else if strings.Contains(line, "timeout") || strings.Contains(line, "TIMEOUT") {
			transferErrorsTotal.WithLabelValues("timeout").Inc()
			connectionTimeoutsTotal.Inc()
		}

		// 解析最大连接数限制
		if strings.Contains(line, "max connections") || strings.Contains(line, "connection limit") {
			maxConnectionsReachedTotal.Inc()
		}
	}

	// 更新读取位置
	state.lastPosition = newPosition

	// 更新并发传输数
	concurrentTransfers.Set(float64(state.activeTransfers))

	// 计算和更新带宽使用率
	if !state.lastBandwidthCheck.IsZero() {
		timeDiff := currentTime.Sub(state.lastBandwidthCheck).Seconds()
		if timeDiff > 0 {
			bytesDiff := totalBytesThisRound
			bandwidthRate := float64(bytesDiff) / timeDiff
			bandwidthUsage.Set(bandwidthRate)
			
			// 计算平均传输速度
			totalBytes := state.totalBytesUploaded + state.totalBytesDownloaded
			if totalBytes > 0 && timeDiff > 0 {
				averageSpeed := float64(totalBytes) / timeDiff
				averageTransferSpeed.Set(averageSpeed)
			}
		}
	}
	state.lastBandwidthCheck = currentTime
	state.lastBytesTransferred += totalBytesThisRound

	// 添加完成解析的INFO日志
	logger.Info("FTP日志解析完成，处理 %d 行，上传: %d，下载: %d", linesProcessed, uploadCount, downloadCount)

	return nil
}

// parseVsftpdLog 解析vsftpd.log文件，提取连接和登录事件信息
// 支持解析CONNECT和OK LOGIN事件，更新相关的监控指标
// 支持SSH远程读取
func parseVsftpdLog(config *Config, logPath string, state *ExporterState) error {
	if logPath == "" {
		return nil // 如果没有配置vsftpd.log路径，直接返回
	}

	// 添加开始解析的INFO日志
	logger.Info("开始解析vsftpd日志文件: %s", logPath)

	// 初始化状态映射（如果尚未初始化）
	if state.clientLastActivity == nil {
		state.clientLastActivity = make(map[string]time.Time)
	}
	if state.clientConnectTimes == nil {
		state.clientConnectTimes = make(map[string]time.Time)
	}
	if state.userClientMapping == nil {
		state.userClientMapping = make(map[string]string)
	}
	if state.activeProcessIDs == nil {
		state.activeProcessIDs = make(map[string]time.Time)
	}
	if state.clientLastConnect == nil {
		state.clientLastConnect = make(map[string]time.Time)
	}

	// 读取vsftpd日志文件内容
	lines, newPosition, err := readRemoteFile(config, logPath, state.vsftpLogPosition)
	if err != nil {
		return fmt.Errorf("读取vsftpd日志文件失败: %w", err)
	}

	// 更新读取位置
	state.vsftpLogPosition = newPosition

	linesProcessed := 0
	connectCount := 0
	loginCount := 0
	currentTime := time.Now()

	// 正则表达式用于解析vsftpd.log格式
	// 示例: Wed Oct 15 15:34:29 2025 [pid 2] CONNECT: Client "172.25.235.63"
	// 示例: Wed Oct 15 15:34:29 2025 [pid 1] [ostore] OK LOGIN: Client "172.25.235.63"
	connectRegex := regexp.MustCompile(`^(\w+\s+\w+\s+\d+\s+\d+:\d+:\d+\s+\d+)\s+\[pid\s+(\d+)\]\s+CONNECT:\s+Client\s+"([^"]+)"`)
	loginRegex := regexp.MustCompile(`^(\w+\s+\w+\s+\d+\s+\d+:\d+:\d+\s+\d+)\s+\[pid\s+(\d+)\]\s+\[([^\]]+)\]\s+OK\s+LOGIN:\s+Client\s+"([^"]+)"`)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		linesProcessed++

		// 解析CONNECT事件
		if matches := connectRegex.FindStringSubmatch(line); matches != nil {
			timeStr := matches[1]
			processID := matches[2]
			clientIP := matches[3]

			// 解析时间戳
			eventTime, err := parseVsftpdTimestamp(timeStr)
			if err != nil {
				logger.Warn("解析时间戳失败: %s, 错误: %v", timeStr, err)
				continue
			}

			// 更新指标
			clientConnectionsTotal.WithLabelValues(clientIP).Inc()
			connectCount++

			// 更新状态跟踪
			state.clientLastActivity[clientIP] = eventTime
			state.clientConnectTimes[clientIP] = eventTime
			state.activeProcessIDs[processID] = eventTime

			// 检测快速重连（30秒内同一IP重连）
			if lastConnect, exists := state.clientLastConnect[clientIP]; exists {
				if eventTime.Sub(lastConnect).Seconds() <= 30 {
					rapidReconnectionsTotal.Inc()
				}
			}
			state.clientLastConnect[clientIP] = eventTime

			// 按小时统计客户端活动
			hour := fmt.Sprintf("%02d", eventTime.Hour())
			clientActivityByHour.WithLabelValues(hour).Inc()
		}

		// 解析OK LOGIN事件
		if matches := loginRegex.FindStringSubmatch(line); matches != nil {
			timeStr := matches[1]
			processID := matches[2]
			username := matches[3]
			clientIP := matches[4]

			// 解析时间戳
			eventTime, err := parseVsftpdTimestamp(timeStr)
			if err != nil {
				logger.Warn("解析时间戳失败: %s, 错误: %v", timeStr, err)
				continue
			}

			// 更新指标
			userLoginsTotal.WithLabelValues(username).Inc()
			userConnectionsTotal.WithLabelValues(username).Inc()
			loginCount++

			// 更新状态跟踪
			state.clientLastActivity[clientIP] = eventTime
			state.userClientMapping[username] = clientIP
			state.activeProcessIDs[processID] = eventTime

			// 计算连接到登录的延迟
			if connectTime, exists := state.clientConnectTimes[clientIP]; exists {
				delay := eventTime.Sub(connectTime).Seconds()
				if delay >= 0 && delay <= 60 { // 合理的延迟范围（0-60秒）
					connectionLoginDelaySeconds.Observe(delay)
				}
			}
		}
	}

	// 定期更新Gauge类型指标（每分钟更新一次）
	if currentTime.Sub(state.lastUniqueClientUpdate).Minutes() >= 1 {
		updateUniqueClientsMetric(state, currentTime)
		state.lastUniqueClientUpdate = currentTime
	}

	if currentTime.Sub(state.lastProcessUpdate).Minutes() >= 1 {
		updateActiveProcessesMetric(state, currentTime)
		state.lastProcessUpdate = currentTime
	}

	// 添加完成解析的INFO日志
	logger.Info("vsftpd日志解析完成，处理 %d 行，连接: %d，登录: %d", linesProcessed, connectCount, loginCount)

	return nil
}

// parseVsftpdTimestamp 解析vsftpd.log中的时间戳格式
// 格式: Wed Oct 15 15:34:29 2025
func parseVsftpdTimestamp(timeStr string) (time.Time, error) {
	// vsftpd使用的时间格式
	layout := "Mon Jan 2 15:04:05 2006"
	return time.Parse(layout, timeStr)
}

// updateUniqueClientsMetric 更新唯一客户端数量指标
// 统计最近5分钟内有活动的不同客户端IP数量
func updateUniqueClientsMetric(state *ExporterState, currentTime time.Time) {
	activeClients := 0
	cutoffTime := currentTime.Add(-5 * time.Minute) // 5分钟内的活动

	for clientIP, lastActivity := range state.clientLastActivity {
		if lastActivity.After(cutoffTime) {
			activeClients++
		} else {
			// 清理过期的客户端记录
			delete(state.clientLastActivity, clientIP)
			delete(state.clientConnectTimes, clientIP)
		}
	}

	uniqueClients.Set(float64(activeClients))
}

// updateActiveProcessesMetric 更新活跃进程数量指标
// 统计最近5分钟内有活动的不同进程ID数量
func updateActiveProcessesMetric(state *ExporterState, currentTime time.Time) {
	activeProcessCount := 0
	cutoffTime := currentTime.Add(-5 * time.Minute) // 5分钟内的活动

	for processID, lastActivity := range state.activeProcessIDs {
		if lastActivity.After(cutoffTime) {
			activeProcessCount++
		} else {
			// 清理过期的进程记录
			delete(state.activeProcessIDs, processID)
		}
	}

	activeProcesses.Set(float64(activeProcessCount))
}

