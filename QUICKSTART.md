# 快速开始指南

## 5 分钟快速部署

### 前置要求

- Docker 和 Docker Compose（推荐）
- 或 Go 1.24+（源码编译）

### 方式一：Docker Compose（最简单）⭐

```bash
# 1. 克隆项目
git clone <repository-url>
cd Vsftpd-exporter

# 2. 配置文件
cp config.example.json config.json
# 编辑 config.json，填入你的 FTP 服务器信息

# 3. 一键启动
docker-compose up -d

# 4. 验证服务
curl http://localhost:9101/health
```

**访问服务**:
- Exporter Metrics: http://localhost:9101/metrics
- Prometheus: http://localhost:9090
- Grafana: http://localhost:3000 (admin/admin)

### 方式二：直接运行二进制文件

```bash
# 1. 编译
make build

# 2. 配置
cp config.example.json config.json
# 编辑配置文件

# 3. 运行
./vsftp-exporter -config=./config.json
```

### 方式三：从源码运行

```bash
# 1. 安装依赖
go mod download

# 2. 配置
cp config.example.json config.json

# 3. 运行
go run vsftp-exporter.go -config=./config.json
```

## 配置说明

最小配置示例 (`config.json`):

```json
{
    "target_host": "192.168.1.100",
    "ftp_port": "21",
    "ftp_user": "ftpuser",
    "ftp_password": "password",
    "need_ssh": false,
    "Xferlog_file_path": "/var/log/xferlog",
    "listen_port": "9101",
    "check_interval": 30
}
```

### SSH 远程监控配置

如果 FTP 服务器在远程主机上：

```json
{
    "target_host": "192.168.1.100",
    "ftp_port": "21",
    "ftp_user": "ftpuser",
    "ftp_password": "password",
    "need_ssh": true,
    "ssh_port": "22",
    "ssh_user": "root",
    "ssh_password": "ssh_password",
    "Xferlog_file_path": "/var/log/xferlog",
    "listen_port": "9101",
    "check_interval": 30,
    "vsftplog_enabled": true,
    "vsftplog_file_path": "/var/log/vsftpd.log"
}
```

## 验证部署

### 1. 检查健康状态

```bash
curl http://localhost:9101/health
```

预期输出:
```json
{
  "status": "healthy",
  "timestamp": "2025-11-17T15:00:00Z",
  "uptime": "5m30s",
  "version": "1.0.0"
}
```

### 2. 查看指标

```bash
curl http://localhost:9101/metrics | grep vsftp
```

应该看到类似输出:
```
vsftp_login_success 1
vsftp_connections 5
vsftp_established_connections 3
vsftp_files_sent_total 120
vsftp_files_received_total 85
...
```

### 3. 访问 Grafana 仪表板

1. 打开浏览器访问: http://localhost:3000
2. 登录（默认: admin/admin）
3. 导航到 Dashboards → Vsftpd FTP 服务器监控仪表盘

## 常见问题

### Q: 无法连接 FTP 服务器

**A**: 检查以下项目:
```bash
# 1. 测试 FTP 连接
telnet <target_host> <ftp_port>

# 2. 检查用户名密码
ftp <target_host>

# 3. 查看 exporter 日志
docker logs vsftp-exporter
# 或
journalctl -u vsftp-exporter -f
```

### Q: SSH 连接失败

**A**: 验证 SSH 访问:
```bash
# 测试 SSH 连接
ssh <ssh_user>@<target_host>

# 检查日志文件权限
ssh <ssh_user>@<target_host> "ls -l /var/log/xferlog"
```

### Q: 指标不更新

**A**: 检查日志文件:
```bash
# 确认日志文件存在且有新内容
tail -f /var/log/xferlog

# 检查 exporter 是否正在读取
curl http://localhost:9101/metrics | grep vsftp_files
```

### Q: Grafana 仪表板显示 "No Data"

**A**: 验证数据链路:
```bash
# 1. 检查 Prometheus 是否抓取数据
curl http://localhost:9090/api/v1/targets

# 2. 查询 Prometheus
curl 'http://localhost:9090/api/v1/query?query=vsftp_login_success'

# 3. 检查 Grafana 数据源配置
# Grafana → Configuration → Data Sources → Prometheus
```

## 下一步

- 📖 阅读 [完整文档](README.md)
- 🚀 查看 [部署指南](DEPLOYMENT.md)
- 📊 了解 [Grafana 仪表板](GRAFANA_DASHBOARD.md)
- ⚠️ 配置 [告警规则](alerts.yml)

## 获取帮助

- 查看日志: `docker logs vsftp-exporter`
- 运行测试: `make test`
- 验证配置: `./vsftp-exporter -config=./config.json`

## 生产环境建议

1. **使用 Systemd 服务** - 参考 [DEPLOYMENT.md](DEPLOYMENT.md)
2. **配置告警** - 使用提供的 [alerts.yml](alerts.yml)
3. **定期备份** - 备份 Grafana 仪表板和 Prometheus 数据
4. **监控 Exporter** - 配置 `VsftpExporterDown` 告警
5. **日志轮转** - 确保 FTP 日志文件定期轮转

---

**需要帮助？** 查看 [故障排查指南](README.md#故障排除) 或提交 Issue。
