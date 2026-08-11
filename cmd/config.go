package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
)

// 文件路径验证正则表达式（防止命令注入）
var validFilePathRegex = regexp.MustCompile(`^[a-zA-Z0-9/_.\-]+$`)

var (
	domainRegex   = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9]))*$`)
	usernameRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
)

type Config struct {
	TargetHost       string `json:"target_host"`
	FTPPort          string `json:"ftp_port"`
	FTPUser          string `json:"ftp_user"`
	FTPPassword      string `json:"ftp_password"`
	NeedSSH          bool   `json:"need_ssh"`
	SSHPort          string `json:"ssh_port"`
	SSHUser          string `json:"ssh_user"`
	SSHPassword      string `json:"ssh_password"`
	LogFilePath      string `json:"Xferlog_file_path"`
	ListenPort       string `json:"listen_port"`
	CheckInterval    int    `json:"check_interval"`
	VsftplogEnabled  bool   `json:"vsftplog_enabled"`
	VsftplogFilePath string `json:"vsftplog_file_path"`
	SummaryExclude   bool   `json:"summary_exclude"`
}

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

	if config.FTPPort == "" {
		config.FTPPort = "21"
	}
	if config.ListenPort == "" {
		config.ListenPort = "9101"
	}
	if config.CheckInterval <= 0 {
		config.CheckInterval = 30
	}

	if config.TargetHost == "" {
		return nil, fmt.Errorf("目标主机地址不能为空")
	}
	if !isValidHost(config.TargetHost) {
		return nil, fmt.Errorf("目标主机地址格式无效: %s", config.TargetHost)
	}

	if config.FTPUser == "" {
		return nil, fmt.Errorf("FTP用户名不能为空")
	}
	if len(config.FTPUser) > 64 || !isValidUsername(config.FTPUser) {
		return nil, fmt.Errorf("FTP用户名格式无效或过长")
	}

	if config.FTPPassword == "" {
		return nil, fmt.Errorf("FTP密码不能为空")
	}
	if len(config.FTPPassword) > 128 {
		return nil, fmt.Errorf("FTP密码过长（最大128字符）")
	}

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

	if config.CheckInterval < 1 || config.CheckInterval > 3600 {
		return nil, fmt.Errorf("检查间隔必须在1-3600秒范围内")
	}

	if config.NeedSSH {
		if config.SSHUser == "" {
			return nil, fmt.Errorf("SSH用户名不能为空（need_ssh=true时必须配置）")
		}
		if config.SSHPassword == "" {
			return nil, fmt.Errorf("SSH密码不能为空（need_ssh=true时必须配置）")
		}
		if config.SSHPort == "" {
			config.SSHPort = "22"
		}
		sshPort, err := strconv.Atoi(config.SSHPort)
		if err != nil || sshPort < 1 || sshPort > 65535 {
			return nil, fmt.Errorf("SSH端口号格式无效: %s", config.SSHPort)
		}
	}

	if config.VsftplogEnabled && config.VsftplogFilePath == "" {
		return nil, fmt.Errorf("启用vsftpd日志解析时，vsftplog_file_path不能为空")
	}

	if config.LogFilePath != "" {
		if !isValidFilePath(config.LogFilePath) {
			return nil, fmt.Errorf("日志文件路径包含非法字符: %s", config.LogFilePath)
		}
		// SSH 模式下文件在远程服务器，不检查本地是否存在
		if !config.NeedSSH {
			expandedPath, err := expandLogFilePath(config.LogFilePath)
			if err != nil {
				return nil, fmt.Errorf("日志文件路径处理失败: %w", err)
			}
			config.LogFilePath = expandedPath

			if err := checkLogFileAccess(config.LogFilePath); err != nil {
				return nil, fmt.Errorf("日志文件路径验证失败: %w", err)
			}
		}
	} else {
		slog.Warn("未配置日志文件路径，将无法解析FTP传输日志")
	}

	if config.VsftplogEnabled && config.VsftplogFilePath != "" {
		if !isValidFilePath(config.VsftplogFilePath) {
			return nil, fmt.Errorf("vsftpd日志文件路径包含非法字符: %s", config.VsftplogFilePath)
		}
		if !config.NeedSSH {
			expandedPath, err := expandLogFilePath(config.VsftplogFilePath)
			if err != nil {
				return nil, fmt.Errorf("vsftpd日志文件路径处理失败: %w", err)
			}
			config.VsftplogFilePath = expandedPath

			if err := checkLogFileAccess(config.VsftplogFilePath); err != nil {
				return nil, fmt.Errorf("vsftpd日志文件路径验证失败: %w", err)
			}
		}
	}

	return &config, nil
}

func isValidHost(host string) bool {
	if net.ParseIP(host) != nil {
		return true
	}
	if len(host) == 0 || len(host) > 253 {
		return false
	}
	return domainRegex.MatchString(host)
}

func isValidUsername(username string) bool {
	if len(username) == 0 {
		return false
	}
	return usernameRegex.MatchString(username)
}

func isValidFilePath(filePath string) bool {
	if filePath == "" {
		return false
	}
	return validFilePathRegex.MatchString(filePath)
}

func expandLogFilePath(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("日志文件路径不能为空")
	}

	expandedPath := os.ExpandEnv(path)

	absPath, err := filepath.Abs(expandedPath)
	if err != nil {
		return "", fmt.Errorf("无法转换为绝对路径: %w", err)
	}

	cleanPath := filepath.Clean(absPath)
	return cleanPath, nil
}

func checkLogFileAccess(logPath string) error {
	if logPath == "" {
		return fmt.Errorf("日志文件路径为空")
	}

	fileInfo, err := os.Stat(logPath)
	if os.IsNotExist(err) {
		dir := filepath.Dir(logPath)
		if _, dirErr := os.Stat(dir); os.IsNotExist(dirErr) {
			return fmt.Errorf("日志文件不存在且父目录不存在: %s (父目录: %s)", logPath, dir)
		}
		return fmt.Errorf("日志文件不存在: %s (请检查vsftpd配置中的xferlog_file设置)", logPath)
	}
	if err != nil {
		return fmt.Errorf("无法访问日志文件: %s, 错误: %v", logPath, err)
	}

	if !fileInfo.Mode().IsRegular() {
		return fmt.Errorf("指定路径不是常规文件: %s (文件类型: %s)", logPath, fileInfo.Mode().String())
	}

	file, err := os.Open(logPath)
	if err != nil {
		if os.IsPermission(err) {
			return fmt.Errorf("没有读取日志文件的权限: %s (当前用户可能需要读取权限)", logPath)
		}
		return fmt.Errorf("无法打开日志文件: %s, 错误: %v", logPath, err)
	}
	file.Close()

	if fileInfo.Size() == 0 {
		slog.Warn("日志文件为空", "path", logPath)
	}

	return nil
}
