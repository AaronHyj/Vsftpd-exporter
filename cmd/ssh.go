package main

import (
	"fmt"
	"log/slog"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

// SSHManager 管理 SSH 连接的复用和重连
type SSHManager struct {
	config *Config
	client *ssh.Client
	mu     sync.Mutex
}

func NewSSHManager(config *Config) *SSHManager {
	return &SSHManager{config: config}
}

// GetClient 获取 SSH 连接，断开时自动重连
func (m *SSHManager) GetClient() (*ssh.Client, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.client != nil {
		// 通过创建 session 检测连接是否存活
		session, err := m.client.NewSession()
		if err == nil {
			session.Close()
			return m.client, nil
		}
		slog.Warn("SSH连接已断开，尝试重连", "error", err)
		m.client.Close()
		m.client = nil
	}

	client, err := createSSHClient(m.config)
	if err != nil {
		return nil, fmt.Errorf("SSH连接失败: %w", err)
	}
	m.client = client
	return m.client, nil
}

// Execute 通过复用的 SSH 连接执行命令
func (m *SSHManager) Execute(command string) (string, error) {
	client, err := m.GetClient()
	if err != nil {
		return "", err
	}
	return executeSSHCommand(client, command, sshCommandTimeout)
}

// Close 关闭 SSH 连接
func (m *SSHManager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.client != nil {
		err := m.client.Close()
		m.client = nil
		slog.Info("SSH连接已关闭")
		return err
	}
	return nil
}

func createSSHClient(config *Config) (*ssh.Client, error) {
	sshConfig := &ssh.ClientConfig{
		User: config.SSHUser,
		Auth: []ssh.AuthMethod{
			ssh.Password(config.SSHPassword),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}

	address := config.TargetHost + ":" + config.SSHPort
	client, err := ssh.Dial("tcp", address, sshConfig)
	if err != nil {
		return nil, fmt.Errorf("SSH连接失败: %w", err)
	}

	slog.Info("SSH连接成功", "user", config.SSHUser, "host", config.TargetHost, "port", config.SSHPort)
	return client, nil
}

const sshCommandTimeout = 10 * time.Second

func executeSSHCommand(client *ssh.Client, command string, timeout time.Duration) (string, error) {
	session, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("创建SSH会话失败: %w", err)
	}
	defer session.Close()

	type result struct {
		output []byte
		err    error
	}
	resultCh := make(chan result, 1)
	go func() {
		output, err := session.Output(command)
		resultCh <- result{output, err}
	}()

	select {
	case res := <-resultCh:
		if res.err != nil {
			return "", fmt.Errorf("执行SSH命令失败: %w", res.err)
		}
		return string(res.output), nil
	case <-time.After(timeout):
		session.Close()
		return "", fmt.Errorf("执行SSH命令超时: %s", command)
	}
}
