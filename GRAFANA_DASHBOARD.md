# Grafana 仪表板使用指南

## 仪表板概述

`grafana-dashboard.json` 是一个完整的 Vsftpd FTP 服务器监控仪表板配置文件，包含以下监控面板：

## 📊 面板布局

### Row 1: 服务状态概览
实时显示 FTP 服务的核心状态指标

1. **FTP 服务状态** - 显示服务是否在线（绿色=在线，红色=离线）
2. **总连接数** - 当前 FTP 总连接数
3. **活跃连接数** - ESTABLISHED 状态的连接数
4. **唯一客户端数** - 最近5分钟内活跃的不同客户端IP数量
5. **并发传输数** - 当前正在进行的文件传输数
6. **活跃进程数** - 当前运行的 vsftpd 进程数

### Row 2: 传输统计
文件传输相关的统计信息

7. **上传文件总数** - 累计上传的文件数量
8. **下载文件总数** - 累计下载的文件数量
9. **登录总次数** - FTP 登录的累计次数
10. **最后登录时间** - 最近一次成功登录的时间戳

11. **连接状态趋势** - 时间序列图，显示总连接数、活跃连接、等待关闭连接的变化趋势
12. **传输速率 (MB/s)** - 时间序列图，显示上传和下载的实时速率

## 导入仪表板

### 方式一：通过 Grafana UI 导入

1. 登录 Grafana (默认: http://localhost:3000)
2. 点击左侧菜单 "+" → "Import"
3. 点击 "Upload JSON file"
4. 选择 `grafana-dashboard.json` 文件
5. 选择 Prometheus 数据源
6. 点击 "Import"

### 方式二：通过 API 导入

```bash
curl -X POST \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -d @grafana-dashboard.json \
  http://localhost:3000/api/dashboards/db
```

### 方式三：使用 Docker Compose 自动导入

如果使用项目提供的 `docker-compose.yml`，仪表板会自动配置。

## 使用说明

### 变量选择

仪表板顶部有两个下拉菜单：

- **Job**: 选择监控任务（默认: vsftp-exporter）
- **Instance**: 选择监控实例（如: localhost:9101）

### 时间范围

- 默认显示最近 1 小时的数据
- 可以通过右上角的时间选择器调整
- 支持自动刷新（默认 30 秒）

### 面板交互

- **点击图例**: 隐藏/显示特定指标
- **拖动选择**: 放大特定时间范围
- **双击**: 重置缩放
- **悬停**: 查看详细数值

## 告警配置

可以为以下面板配置告警：

1. **FTP 服务状态** - 服务离线告警
2. **总连接数** - 连接数过高告警
3. **活跃连接数** - 活跃连接异常告警

### 配置告警示例

1. 点击面板标题 → "Edit"
2. 切换到 "Alert" 标签
3. 点击 "Create Alert"
4. 设置告警条件，例如：
   ```
   WHEN last() OF query(A, 5m, now) IS BELOW 1
   ```
5. 配置通知渠道
6. 保存

## 自定义仪表板

### 添加新面板

1. 点击仪表板右上角的 "Add panel"
2. 选择可视化类型
3. 配置查询，例如：
   ```promql
   rate(vsftp_upload_total{job="$job", instance="$instance"}[5m])
   ```
4. 调整面板设置
5. 保存

### 可用的 Prometheus 查询示例

```promql
# 每分钟传输文件数
rate(vsftp_upload_total[1m]) + rate(vsftp_download_total[1m])

# 传输错误率
rate(vsftp_transfer_errors_total[5m])

# 平均传输速度 (MB/s)
vsftp_average_transfer_speed_bytes_per_second / 1024 / 1024

# 活跃用户数
count(rate(vsftp_user_logins_total[5m]) > 0)

# 客户端连接分布 (Top 10)
topk(10, rate(vsftp_client_connections_total[5m]))

# 传输耗时 P95
histogram_quantile(0.95, rate(vsftp_transfer_duration_seconds_bucket[5m]))

# 带宽使用率
vsftp_bandwidth_usage_bytes_per_second / 1024 / 1024

# 登录失败率
rate(vsftp_failed_logins_total[5m])
```

## 性能优化建议

1. **调整刷新间隔**: 根据需要调整自动刷新时间（默认 30 秒）
2. **限制时间范围**: 查看长时间范围数据时，使用较大的时间间隔
3. **使用变量**: 利用 Job 和 Instance 变量过滤数据
4. **面板缓存**: Grafana 会自动缓存查询结果

## 故障排查

### 仪表板显示 "No Data"

1. 检查 Prometheus 数据源配置
2. 验证 vsftp-exporter 是否正常运行
3. 确认 Prometheus 正在抓取指标
4. 检查时间范围是否正确

### 查询超时

1. 减小时间范围
2. 增加查询间隔
3. 优化 Prometheus 配置

### 指标不更新

1. 检查 vsftp-exporter 日志
2. 验证 FTP 服务是否有活动
3. 确认日志文件路径正确

## 扩展建议

可以添加以下面板来增强监控：

1. **按文件类型统计** - 显示不同文件扩展名的传输量
2. **客户端地理分布** - 如果有 GeoIP 数据
3. **传输耗时热力图** - 显示传输时间分布
4. **错误统计** - 详细的错误类型分析
5. **用户活动排行** - Top N 活跃用户
6. **带宽使用趋势** - 长期带宽使用分析

## 导出和分享

### 导出仪表板

1. 点击仪表板设置（齿轮图标）
2. 选择 "JSON Model"
3. 复制 JSON 内容或下载文件

### 分享仪表板

1. 点击 "Share" 按钮
2. 选择分享方式：
   - **Link**: 生成分享链接
   - **Snapshot**: 创建快照
   - **Export**: 导出为 JSON

## 版本历史

- **v1.0** (2025-11-17)
  - 初始版本
  - 包含 14 个基础监控面板
  - 支持服务状态、传输统计、连接监控

## 相关文档

- [README.md](README.md) - 项目总体说明
- [DEPLOYMENT.md](DEPLOYMENT.md) - 部署指南
- [alerts.yml](alerts.yml) - Prometheus 告警规则
- [prometheus.yml](prometheus.yml) - Prometheus 配置
