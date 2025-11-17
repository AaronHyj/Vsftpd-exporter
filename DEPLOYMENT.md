# Vsftpd Exporter 部署指南

## 快速开始

### 方式一：直接运行二进制文件

1. **编译项目**
```bash
make build
```

2. **配置文件**
```bash
cp config.example.json config.json
# 编辑 config.json 填入实际配置
```

3. **运行**
```bash
./vsftp-exporter -config=./config.json
```

### 方式二：使用 Docker

1. **构建镜像**
```bash
docker build -t vsftp-exporter:latest .
```

2. **运行容器**
```bash
docker run -d \
  --name vsftp-exporter \
  -p 9101:9101 \
  -v $(pwd)/config.json:/app/config.json:ro \
  vsftp-exporter:latest
```

### 方式三：使用 Docker Compose（推荐）

1. **准备配置文件**
```bash
cp config.example.json config.json
# 编辑配置
```

2. **启动所有服务**
```bash
docker-compose up -d
```

这将启动：
- vsftp-exporter (端口 9101)
- Prometheus (端口 9090)
- Grafana (端口 3000)

3. **访问服务**
- Grafana: http://localhost:3000 (admin/admin)
- Prometheus: http://localhost:9090
- Metrics: http://localhost:9101/metrics

## 生产环境部署

### Systemd 服务配置

创建 `/etc/systemd/system/vsftp-exporter.service`:

```ini
[Unit]
Description=Vsftpd Prometheus Exporter
After=network.target

[Service]
Type=simple
User=prometheus
WorkingDirectory=/opt/vsftp-exporter
ExecStart=/opt/vsftp-exporter/vsftp-exporter -config=/opt/vsftp-exporter/config.json
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

启动服务：
```bash
sudo systemctl daemon-reload
sudo systemctl enable vsftp-exporter
sudo systemctl start vsftp-exporter
sudo systemctl status vsftp-exporter
```

## 监控验证

### 检查 Exporter 状态
```bash
# 健康检查
curl http://localhost:9101/health

# 查看指标
curl http://localhost:9101/metrics
```

### 检查日志
```bash
# Systemd 服务日志
sudo journalctl -u vsftp-exporter -f

# Docker 日志
docker logs -f vsftp-exporter
```

## 故障排查

### 常见问题

1. **无法连接 FTP 服务器**
   - 检查网络连接
   - 验证 FTP 端口是否正确
   - 确认用户名密码正确

2. **SSH 连接失败**
   - 检查 SSH 端口和凭据
   - 确认目标服务器 SSH 服务运行正常
   - 验证网络防火墙规则

3. **日志文件读取失败**
   - 确认日志文件路径正确
   - 检查文件读取权限
   - 验证 SSH 用户有权限读取日志

## 性能优化建议

- 根据 FTP 服务器负载调整 `check_interval`
- 定期清理旧日志文件
- 监控 Exporter 自身的资源使用
