# Vsftpd Exporter for Prometheus

一个用于监控 vsftpd FTP 服务器的 Prometheus exporter，提供全面的 FTP 服务性能和状态监控指标。

## 项目简介

Vsftpd Exporter 是一个专门为 vsftpd FTP 服务器设计的 Prometheus 监控导出器。它通过解析 FTP 日志文件、检查 FTP 连接状态和执行健康检查来收集各种监控指标，帮助运维人员实时监控 FTP 服务的性能和健康状态。

### 主要功能

- **连接监控**: 实时监控 FTP 连接数、并发传输数等
- **传输统计**: 统计文件上传/下载次数、传输字节数、传输速度等
- **错误监控**: 监控登录失败、传输错误、连接超时等异常情况
- **性能分析**: 提供传输耗时分布、带宽使用率等性能指标
- **文件统计**: 按文件扩展名统计传输的文件类型
- **健康检查**: 定期检查 FTP 服务可用性

## 安装和编译

### 系统要求

- Go 1.19 或更高版本
- 运行中的 vsftpd FTP 服务器
- 对 FTP 日志文件的读取权限

### 编译安装

```bash
# 克隆项目
git clone <repository-url>
cd Vsftpd-exporter

# 下载依赖
go mod download

# 编译
go build -o vsftp-exporter vsftp-exporter.go

# 或者直接运行
go run vsftp-exporter.go
```

### 依赖包

- `github.com/jlaffaye/ftp v0.2.0` - FTP 客户端库
- `github.com/prometheus/client_golang v1.19.1` - Prometheus 客户端库

## 配置说明

### 配置文件 (config.json)

```json
{
    "ftp_host": "localhost",          // FTP 服务器地址
    "ftp_port": "21",                 // FTP 服务器端口
    "ftp_user": "testuser",           // FTP 用户名
    "ftp_password": "testpass",       // FTP 密码
    "log_file_path": "/var/log/xferlog", // FTP 日志文件路径
    "listen_port": "9100",            // Exporter 监听端口
    "check_interval": 30               // 检查间隔（秒）
}
```

### 配置项详解

| 配置项 | 类型 | 必需 | 默认值 | 说明 |
|--------|------|------|--------|------|
| `ftp_host` | string | 是 | localhost | FTP 服务器主机地址 |
| `ftp_port` | string | 是 | 21 | FTP 服务器端口号 |
| `ftp_user` | string | 是 | - | FTP 登录用户名 |
| `ftp_password` | string | 是 | - | FTP 登录密码 |
| `log_file_path` | string | 是 | /var/log/xferlog | vsftpd 日志文件路径 |
| `listen_port` | string | 否 | 9100 | Exporter HTTP 服务监听端口 |
| `check_interval` | int | 否 | 30 | 监控检查间隔时间（秒） |

## 使用方法

### 启动 Exporter

```bash
# 使用默认配置文件
./vsftp-exporter

# 指定配置文件路径
./vsftp-exporter -config=/path/to/config.json
```

### 验证运行状态

```bash
# 检查指标端点
curl http://localhost:9100/metrics

# 检查健康状态
curl http://localhost:9100/health
```

### 系统服务配置

创建 systemd 服务文件 `/etc/systemd/system/vsftp-exporter.service`:

```ini
[Unit]
Description=Vsftpd Prometheus Exporter
After=network.target

[Service]
Type=simple
User=prometheus
ExecStart=/usr/local/bin/vsftp-exporter
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

启动服务:

```bash
sudo systemctl daemon-reload
sudo systemctl enable vsftp-exporter
sudo systemctl start vsftp-exporter
```

## 监控指标

### 连接状态指标

| 指标名称 | 类型 | 说明 |
|----------|------|------|
| `ftp_login_success` | Gauge | FTP 登录成功状态 (1=成功, 0=失败) |
| `ftp_connections` | Gauge | 当前 FTP 总连接数 |
| `established_connections` | Gauge | 已建立的连接数 |
| `close_wait_connections` | Gauge | 等待关闭的连接数 |
| `vsftp_concurrent_transfers` | Gauge | 当前并发传输数 |

### 传输统计指标

| 指标名称 | 类型 | 标签 | 说明 |
|----------|------|------|------|
| `files_downloaded` | Counter | - | 文件下载总次数 |
| `files_uploaded` | Counter | - | 文件上传总次数 |
| `vsftp_transfer_bytes_total` | Counter | direction | 传输字节总数 (upload/download) |
| `vsftp_transfer_duration_seconds` | Histogram | - | 文件传输耗时分布 |
| `vsftp_average_transfer_speed_bytes_per_second` | Gauge | - | 平均传输速度 (字节/秒) |
| `vsftp_bandwidth_usage_bytes_per_second` | Gauge | - | 当前带宽使用率 (字节/秒) |

### 错误和异常指标

| 指标名称 | 类型 | 标签 | 说明 |
|----------|------|------|------|
| `vsftp_failed_logins_total` | Counter | - | 登录失败总次数 |
| `vsftp_transfer_errors_total` | Counter | type | 传输错误总数 (upload/download/timeout) |
| `vsftp_connection_timeouts_total` | Counter | - | 连接超时总次数 |
| `vsftp_authentication_errors_total` | Counter | - | 认证错误总次数 |
| `vsftp_max_connections_reached_total` | Counter | - | 达到最大连接数限制的次数 |

### 文件统计指标

| 指标名称 | 类型 | 标签 | 说明 |
|----------|------|------|------|
| `vsftp_file_count_by_extension` | Counter | extension | 按文件扩展名统计的文件数量 |

### 性能指标

| 指标名称 | 类型 | 说明 |
|----------|------|------|
| `ftp_login_time` | Gauge | FTP 登录响应时间 (毫秒) |
| `ftp_login_total` | Counter | FTP 登录尝试总次数 |
| `ftp_upload_total` | Counter | FTP 上传操作总次数 |
| `ftp_download_total` | Counter | FTP 下载操作总次数 |

## Prometheus 配置

在 Prometheus 配置文件中添加以下 job 配置:

```yaml
scrape_configs:
  - job_name: 'vsftp-exporter'
    static_configs:
      - targets: ['localhost:9100']
    scrape_interval: 30s
    scrape_timeout: 10s
    metrics_path: /metrics
```

### 告警规则示例

```yaml
groups:
  - name: vsftp-alerts
    rules:
      - alert: VsftpdDown
        expr: ftp_login_success == 0
        for: 1m
        labels:
          severity: critical
        annotations:
          summary: "Vsftpd service is down"
          description: "Vsftpd service has been down for more than 1 minute"
      
      - alert: HighFailedLogins
        expr: increase(vsftp_failed_logins_total[5m]) > 10
        for: 0m
        labels:
          severity: warning
        annotations:
          summary: "High number of failed FTP logins"
          description: "More than 10 failed logins in the last 5 minutes"
      
      - alert: HighTransferErrors
        expr: increase(vsftp_transfer_errors_total[5m]) > 5
        for: 0m
        labels:
          severity: warning
        annotations:
          summary: "High number of transfer errors"
          description: "More than 5 transfer errors in the last 5 minutes"
```

## Grafana 仪表板

### 推荐的仪表板面板

1. **服务状态概览**
   - FTP 服务可用性
   - 当前连接数
   - 登录成功率

2. **传输统计**
   - 上传/下载文件数量趋势
   - 传输字节数统计
   - 平均传输速度

3. **性能监控**
   - 传输耗时分布
   - 带宽使用率
   - 并发传输数

4. **错误监控**
   - 登录失败趋势
   - 传输错误统计
   - 连接超时次数

### 示例查询语句

```promql
# 服务可用性
ftp_login_success

# 每分钟传输文件数
rate(files_uploaded[1m]) + rate(files_downloaded[1m])

# 传输错误率
rate(vsftp_transfer_errors_total[5m]) / rate(vsftp_transfer_bytes_total[5m])

# 平均传输速度
vsftp_average_transfer_speed_bytes_per_second / 1024 / 1024
```

## 故障排除

### 常见问题

**Q: Exporter 启动失败，提示配置文件错误**

A: 检查 config.json 文件格式是否正确，确保所有必需字段都已填写。

**Q: 无法连接到 FTP 服务器**

A: 检查以下项目：
- FTP 服务器地址和端口是否正确
- 用户名和密码是否有效
- 网络连接是否正常
- 防火墙设置是否允许连接

**Q: 日志解析失败**

A: 确认：
- 日志文件路径是否正确
- 是否有读取日志文件的权限
- vsftpd 日志格式是否为标准格式

**Q: 指标数据不更新**

A: 检查：
- FTP 服务是否有活动
- 日志文件是否在更新
- check_interval 配置是否合理

### 调试模式

启用详细日志输出：

```bash
./vsftp-exporter -debug
```

### 日志级别

- INFO: 正常运行信息
- WARN: 警告信息
- ERROR: 错误信息
- DEBUG: 调试信息

## 性能优化

### 建议配置

- 对于高负载环境，建议将 `check_interval` 设置为 15-30 秒
- 确保日志文件定期轮转，避免文件过大影响解析性能
- 监控 Exporter 自身的资源使用情况

### 资源使用

- 内存使用: 通常 < 50MB
- CPU 使用: 通常 < 5%
- 磁盘 I/O: 主要用于读取日志文件

## 贡献指南

我们欢迎社区贡献！请遵循以下步骤：

1. Fork 本项目
2. 创建特性分支 (`git checkout -b feature/AmazingFeature`)
3. 提交更改 (`git commit -m 'Add some AmazingFeature'`)
4. 推送到分支 (`git push origin feature/AmazingFeature`)
5. 创建 Pull Request

### 开发规范

- 遵循 Go 代码规范
- 添加适当的注释和文档
- 确保所有测试通过
- 更新相关文档

### 报告问题

如果发现 bug 或有功能建议，请在 GitHub Issues 中提交详细信息。

## 许可证

本项目采用 MIT 许可证。详细信息请查看 [LICENSE](LICENSE) 文件。

## 更新日志

### v1.0.0
- 初始版本发布
- 支持基本的 FTP 监控指标
- 提供 Prometheus 集成

---

**维护者**: [Your Name]
**项目主页**: [Repository URL]
**问题反馈**: [Issues URL]
