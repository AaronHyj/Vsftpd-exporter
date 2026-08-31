# Vsftpd Exporter for Prometheus

一个用于监控 vsftpd FTP 服务器的 Prometheus exporter，通过解析 xferlog 传输日志和 vsftpd.log 详细日志，提供全面的 FTP 服务性能和状态监控指标。

[![Go Version](https://img.shields.io/badge/Go-1.24+-00ADD8?style=flat&logo=go)](https://golang.org)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

## 快速开始

```bash
# 编译
make build

# 复制并修改配置
cp configs/config.example.json configs/config.json

# 运行
./vsftp-exporter

# 访问服务
# Metrics:     http://localhost:9101/metrics
# Health:      http://localhost:9101/health
```

## 项目简介

Vsftpd Exporter 通过以下方式采集监控数据：

1. **FTP 连接探测** — 定期尝试登录 FTP 服务器，验证服务可用性
2. **连接状态统计** — 通过 `ss -tnH` 统计 FTP 端口的连接状态（ESTABLISHED / CLOSE_WAIT 等）
3. **xferlog 日志解析** — 增量读取标准 xferlog，提取上传/下载文件数、字节数、传输耗时、客户端 IP、文件后缀（如 ts/mp4/mkv）等
4. **vsftpd.log 日志解析** — 增量读取 vsftpd 详细日志，提取 CONNECT/LOGIN 事件、用户活动、进程信息等
5. **SSH 远程采集** — 支持通过 SSH 连接到远程服务器读取日志和执行 ss

所有采集任务按 `check_interval` 配置的间隔周期性执行。

### 主要功能

- **连接监控**: 实时统计 FTP 总连接数、ESTABLISHED 连接数、CLOSE_WAIT 连接数
- **传输统计**: 上传/下载文件数、字节数、传输耗时分布（Histogram）、平均传输速度、带宽使用率
- **错误监控**: 登录失败、传输错误（按类型分类）、连接超时、认证错误、最大连接数限制
- **用户与客户端分析**: 按用户名统计登录/连接数，按客户端 IP 统计连接/文件传输数
- **文件类型统计**: 将 xferlog 中的文件按原始后缀统计（如 ts/mp4/mkv），以横向柱状图展示指定时间段各后缀传输的文件总数
- **高级检测**: 快速重连检测（30 秒内同 IP 重连）、连接到登录延迟分布、活跃进程数
- **SSH 远程监控**: 通过 SSH 连接远程服务器读取日志文件和执行命令
- **健康检查**: 提供 `/health` 端点，返回 JSON 格式的服务状态信息

## 架构概览

```text
┌─────────────────────────────────────────────────────┐
│                  vsftp-exporter                      │
│                                                      │
│  ┌──────────────┐  ┌──────────────┐  ┌────────────┐ │
│  │ FTP Login    │  │ ss           │  │ Log Parser │ │
│  │ Checker      │  │ Checker      │  │ (xferlog + │ │
│  │              │  │              │  │ vsftpd.log)│ │
│  └──────┬───────┘  └──────┬───────┘  └─────┬──────┘ │
│         │                 │                 │        │
│         │    ┌────────────┴─────────┐       │        │
│         │    │  SSH Client (可选)   │       │        │
│         │    └──────────────────────┘       │        │
│         │                                   │        │
│  ┌──────┴───────────────────────────────────┴──────┐ │
│  │           Prometheus Metrics Registry           │ │
│  └─────────────────────┬───────────────────────────┘ │
│                        │                             │
│              ┌─────────┴─────────┐                   │
│              │  HTTP Server      │                   │
│              │  /metrics         │                   │
│              │  /health          │                   │
│              └───────────────────┘                   │
└─────────────────────────────────────────────────────┘
         ▲                              ▲
         │                              │
    Prometheus                     Health Check
    (scrape)                       (监控系统)
```

## 安装和编译

### 系统要求

- Go 1.24 或更高版本
- 运行中的 vsftpd FTP 服务器
- 对 FTP 日志文件的读取权限（本地模式）或 SSH 访问权限（远程模式）

### 编译安装

```bash
# 克隆项目
git clone <repository-url>
cd Vsftpd-exporter

# 下载依赖
go mod download

# 编译
make build
```

### 交叉编译

```bash
make build-linux     # Linux amd64
make build-windows   # Windows amd64
make build-darwin    # macOS amd64
make build-all       # 所有平台
```

### Make 目标

| 命令 | 说明 |
| ---- | ---- |
| `make build` | 构建二进制文件 |
| `make run` | 构建并运行程序 |
| `make test` | 运行测试（含 race 检测和覆盖率） |
| `make coverage` | 生成 HTML 覆盖率报告 |
| `make fmt` | 格式化代码 |
| `make vet` | 代码静态检查 |
| `make tidy` | 整理 Go 模块依赖 |
| `make install` | 安装到 /usr/local/bin |
| `make clean` | 清理构建产物 |

### 依赖包

| 包 | 版本 | 用途 |
| -- | ---- | ---- |
| `github.com/jlaffaye/ftp` | v0.2.0 | FTP 客户端，用于连接探测 |
| `github.com/prometheus/client_golang` | v1.19.1 | Prometheus 客户端库 |
| `golang.org/x/crypto` | v0.43.0 | SSH 客户端，用于远程采集 |
| `pgregory.net/rapid` | v1.2.0 | 属性测试（测试依赖） |
| `github.com/davecgh/go-spew` | v1.1.1 | 测试断言（间接依赖） |

## 配置说明

### 配置文件 (configs/config.json)

复制 `configs/config.example.json` 为 `configs/config.json` 并修改：

```bash
cp configs/config.example.json configs/config.json
```

```json
{
    "target_host": "localhost",
    "ftp_port": "21",
    "ftp_user": "your_ftp_username",
    "ftp_password": "your_ftp_password",
    "need_ssh": false,
    "ssh_port": "22",
    "ssh_user": "your_ssh_username",
    "ssh_password": "your_ssh_password",
    "Xferlog_file_path": "/var/log/xferlog",
    "listen_port": "9101",
    "check_interval": 30,
    "vsftplog_enabled": true,
    "vsftplog_file_path": "/var/log/vsftpd.log",
    "summary_exclude": false
}
```

### 配置项详解

| 配置项 | 类型 | 必需 | 默认值 | 说明 |
| ------ | ---- | ---- | ------ | ---- |
| `target_host` | string | 是 | - | 目标服务器地址，支持 IP 或域名（会验证格式） |
| `ftp_port` | string | 否 | `21` | FTP 端口号（1-65535） |
| `ftp_user` | string | 是 | - | FTP 用户名（最长 64 字符，仅字母数字下划线连字符） |
| `ftp_password` | string | 是 | - | FTP 密码（最长 128 字符） |
| `need_ssh` | bool | 否 | `false` | 是否通过 SSH 连接远程服务器采集数据 |
| `ssh_port` | string | 否 | `22` | SSH 端口 |
| `ssh_user` | string | 否 | - | SSH 用户名（`need_ssh=true` 时必需） |
| `ssh_password` | string | 否 | - | SSH 密码（`need_ssh=true` 时必需） |
| `Xferlog_file_path` | string | 否 | - | xferlog 传输日志路径,支持相对路径与环境变量(仅本地 need_ssh=false 模式展开,SSH 模式不展开且路径不允许 `$`/单引号/空白等特殊字符);环境变量建议用 `${VAR}` 形式避免贪婪匹配,`os.ExpandEnv` 单次展开(字面 `$$` 会变空) |
| `listen_port` | string | 否 | `9101` | Exporter HTTP 监听端口 |
| `check_interval` | int | 否 | `30` | 采集间隔（1-3600 秒） |
| `vsftplog_enabled` | bool | 否 | `false` | 是否启用 vsftpd.log 详细日志解析 |
| `vsftplog_file_path` | string | 否 | - | vsftpd.log 文件路径 |
| `summary_exclude` | bool | 否 | `false` | 为 `true` 时，登录次数、连接数、快速重连等汇总统计排除健康检查探测账号（`ftp_user`）及其来源 IP 产生的事件，避免探测干扰真实统计数据 |

> **注意**: `configs/config.json` 已在 `.gitignore` 中排除，不会被提交到版本库。请使用 `configs/config.example.json` 作为模板。

## 使用方法

### 启动 Exporter

```bash
# 使用默认配置文件 (configs/config.json)
./vsftp-exporter

# 指定配置文件路径
./vsftp-exporter -config=/path/to/config.json

# 指定日志级别 (debug/info/warn/error，默认 info)
./vsftp-exporter -log-level=debug
```

### 验证运行状态

```bash
# 检查指标端点
curl http://localhost:9101/metrics

# 检查健康状态（返回 JSON）
curl http://localhost:9101/health
```

健康检查返回示例：

```json
{
  "status": "healthy",
  "timestamp": "2025-10-15T16:04:42+08:00",
  "uptime": "2h30m15s",
  "last_check_time": "2025-10-15T16:04:42+08:00",
  "version": "1.0.0",
  "build_time": "2025-10-15T06:00:00_UTC"
}
```

- `status`：`healthy` 表示最近一次 FTP 登录探测成功，`degraded` 表示探测失败（FTP 服务不可达或登录失败）。`degraded` 时返回 HTTP 503，并附带 `error` 字段说明失败原因。
- `last_check_time`：最近一次 FTP 探测发生的时间。

> **注意（对 vsftpd 服务端的影响）**：exporter 会使用配置的真实账号周期性地执行 FTP 登录探测（`vsftp_login_success` 指标）。每次探测都会在服务端产生一次真实的连接与认证，占用连接配额并出现在 vsftpd 日志中。建议：
> 1. 为探测创建**专用的只读账号**，避免占用真实用户配额；
> 2. 探测频率不要设置过小（`check_interval` 建议 ≥ 30 秒）；
> 3. 探测本身已不再计入 `vsftp_failed_logins_total` / `vsftp_authentication_errors_total`，这些计数完全来自日志解析器对真实客户端事件的统计，避免与服务端日志重复计数。

### SSH 远程监控

当需要监控远程服务器上的 vsftpd 时：

```json
{
    "target_host": "192.168.1.100",
    "need_ssh": true,
    "ssh_port": "22",
    "ssh_user": "root",
    "ssh_password": "your_password",
    "Xferlog_file_path": "/var/log/xferlog",
    "vsftplog_enabled": true,
    "vsftplog_file_path": "/var/log/vsftpd.log",
    "summary_exclude": true
}
```

SSH 模式下，exporter 会通过 SSH 执行 `ss -tnH` 统计连接状态，用 `tail -c +N` / `cat` 增量读取日志，并用 `stat` 检测日志轮转。SSH 用户需要有读取日志文件和执行 ss 的权限。
> **安全警示**>> - **SSH 主机密钥不校验(MITM 风险)**:exporter 使用 `ssh.InsecureIgnoreHostKey()`(见 `cmd/ssh.go`),对 SSH 服务器的主机密钥不做验证,存在中间人攻击风险。请仅在可信内网中使用,并限制监听端口与访问来源。> - **明文密码**:`configs/config.json` 以明文保存 SSH/FTP 密码。请通过文件权限(`chmod 600`)限制访问,并切勿将配置文件提交到版本库(`configs/config.json` 已被 `.gitignore` 排除)。> - **监听端口**:默认 `listen_port` `9101` 请勿直接暴露到公网,建议配合防火墙/反向代理限制访问。
### systemd 服务

创建 `/etc/systemd/system/vsftp-exporter.service`：

```ini
[Unit]
Description=Vsftpd Prometheus Exporter
After=network.target

[Service]
Type=simple
User=prometheus
ExecStart=/usr/local/bin/vsftp-exporter -config=/etc/vsftp-exporter/config.json -log-level=info
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now vsftp-exporter
```

## 监控指标

### 连接状态指标

| 指标名称 | 类型 | 说明 |
| -------- | ---- | ---- |
| `vsftp_login_success` | Gauge | FTP 登录探测状态（1=成功, 0=失败） |
| `vsftp_connections` | Gauge | 当前 FTP 端口总连接数 |
| `vsftp_established_connections` | Gauge | ESTABLISHED 状态连接数 |
| `vsftp_close_wait_connections` | Gauge | CLOSE_WAIT 状态连接数 |

### 传输统计指标

| 指标名称 | 类型 | 说明 |
| -------- | ---- | ---- |
| `vsftp_upload_total` | Counter | 上传操作总次数 |
| `vsftp_download_total` | Counter | 下载操作总次数 |
| `vsftp_upload_bytes_total` | Counter | 上传字节总数 |
| `vsftp_download_bytes_total` | Counter | 下载字节总数 |
| `vsftp_transfer_duration_seconds` | Histogram | 传输耗时分布（桶: 0.1s~51.2s 指数分布） |
| `vsftp_average_transfer_speed_bytes_per_second` | Gauge | 平均传输速度（字节/秒） |
| `vsftp_bandwidth_usage_bytes_per_second` | Gauge | 当前带宽使用率（字节/秒） |

### 错误和异常指标

| 指标名称 | 类型 | 标签 | 说明 |
| -------- | ---- | ---- | ---- |
| `vsftp_failed_logins_total` | Counter | - | 登录失败总次数（按 FAIL LOGIN 事件） |
| `vsftp_transfer_errors_total` | Counter | `type` | 传输错误总数(upload/download) |
| `vsftp_connection_timeouts_total` | Counter | - | 连接超时总次数 |
| `vsftp_authentication_errors_total` | Counter | - | 认证错误总次数（仅统计 530 响应，已去除 FAIL LOGIN 重复计数） |
| `vsftp_max_connections_reached_total` | Counter | - | 达到最大连接数限制次数 |
| `vsftp_ftp_errors_total` | Counter | `reason` | FTP 协议错误按原因分类计数，`reason` 取值见下表 |
| `vsftp_idle_timeout_total` | Counter | - | 空闲会话超时次数(idle_session_timeout 触发,421 Timeout) |
| `vsftp_data_connection_timeout_total` | Counter | - | 数据连接停滞超时次数(data_connection_timeout 触发,426) |
| `vsftp_connection_limit_rejections_total` | Counter | `reason` | 达到连接数限制被拒绝次数,`reason` 取值 `max_clients`(全局)/`max_per_ip`(单IP) |
| `vsftp_pasv_port_rejections_total` | Counter | - | PASV 数据连接建立失败次数(pasv_min/max_port 范围耗尽或网络故障) |

`vsftp_ftp_errors_total` 的 `reason` 标签取值：

| `reason` | 含义 | 典型 FTP 响应 |
| -------- | ---- | ---- |
| `auth_failed` | 认证失败（密码错误、用户不存在等） | `530 Login incorrect.` |
| `max_connections` | 达到连接数上限被拒绝 | `421 Too many connections`、`530 Maximum number of clients reached` |
| `service_unavailable` | 服务不可用（关闭控制连接） | `421 Service not available, closing control connection.` |
| `data_connection_error` | 数据连接建立/传输失败 | `425 Can't open data connection.`、`426 Connection closed; transfer aborted.`、`450/451` |
| `command_error` | 客户端命令语法/未实现/顺序错误 | `500 Unknown command.`、`501`、`502`、`503 Bad sequence of commands.`、`504` |
| `dir_not_found` | 目标目录不存在 | `550 Failed to change directory.` |
| `file_not_found` | 目标文件不存在或无法打开 | `550 No such file or directory.`、`550 Not a regular file.` |
| `permission_denied` | 权限不足 | `550 Permission denied.` |
| `quota_exceeded` | 磁盘配额超限 | `552 Exceeded storage allocation.` |
| `file_name_not_allowed` | 文件名/创建操作不允许 | `553 Could not create file.` |
| `other` | 其他 4xx/5xx 错误 | - |

> 说明：`vsftp_failed_logins_total` 按 `FAIL LOGIN` 事件计数，`vsftp_authentication_errors_total` 仅按 `530` 响应行计数。vsftpd 对同一次失败登录会同时输出 `FAIL LOGIN` 与 `530 FTP response` 两行，二者不再重复累加认证错误。

### 客户端、用户与文件统计指标

| 指标名称 | 类型 | 标签 | 说明 |
| -------- | ---- | ---- | ---- |
| `vsftp_client_connections_total` | Counter | `client_ip` | 按客户端 IP 统计的连接总数 |
| `vsftp_unique_clients` | Gauge | - | 最近 5 分钟内活跃的唯一客户端数 |
| `vsftp_user_logins_total` | Counter | `username` | 按用户名统计的成功登录总数 |
| `vsftp_user_connections_total` | Counter | `username` | 按用户名统计的连接总数 |
| `vsftp_login_total` | Counter | `-` | FTP 登录总次数(来源:vsftpd.log) |
| `vsftp_last_login_time` | Gauge | `-` | 最后成功登录的 Unix 时间戳(来源:vsftpd.log) |
| `vsftp_client_files_total` | Counter | `client_ip`, `direction` | 按客户端 IP 和方向统计的文件传输数(来源:xferlog) |
| `vsftp_files_by_type_total` | Counter | `file_type`, `direction` | 按文件后缀和方向统计的传输文件数(来源:xferlog),`file_type` 为小写后缀(如 `ts`/`mp4`/`mkv`),无后缀为 `no_extension`;vsftpd 内部/遍历文件(`.listing` 目录列表缓存、`*.writing` 上传临时文件)被过滤,不计入。首次见到的标签先以 0 值注册、下一次抓取后再提交增量,保证 `increase($__range)` 能观测到首次增量 |
| `vsftp_internal_transfers_total` | Counter | `direction` | vsftpd 内部/遍历产生的传输次数(目录列表缓存 `.listing`、上传临时文件 `*.writing`),与业务上传/下载计数分离,便于观察遍历行为本身 |

### 高级监控指标（需启用 vsftpd.log）

| 指标名称 | 类型 | 说明 |
| -------- | ---- | ---- |
| `vsftp_connection_login_delay_seconds` | Histogram | CONNECT 到 LOGIN 的延迟分布(桶: 1ms~65s 指数分布) |
| `vsftp_rapid_reconnections_total` | Counter | 快速重连次数（同一 IP 30 秒内重连） |
| `vsftp_active_processes` | Gauge | 最近 5 分钟内活跃的 vsftpd 进程数 |

## Prometheus 配置

`deploy/prometheus.yml.example` 提供了抓取配置模板。手动配置时添加：

```yaml
scrape_configs:
  - job_name: 'vsftp-exporter'
    static_configs:
      - targets: ['localhost:9101']
        labels:
          service: 'vsftpd'
          environment: 'production'
    scrape_interval: 30s
    scrape_timeout: 10s
    metrics_path: /metrics
```

### 告警规则

`deploy/alerts.yml.example` 包含以下告警：

| 告警名称 | 严重级别 | 触发条件 |
| -------- | -------- | -------- |
| VsftpdServiceDown | critical | FTP 服务不可用超过 2 分钟 |
| HighFailedLoginRate | warning | 5 分钟内登录失败率 > 10 次/分钟 |
| HighTransferErrorRate | warning | 5 分钟内传输错误率 > 5 次/分钟 |
| HighConnectionCount | warning | 连接数 > 100 持续 5 分钟 |
| HighCloseWaitConnections | warning | CLOSE_WAIT 连接 > 20 持续 10 分钟 |
| FrequentConnectionTimeouts | warning | 连接超时率 > 3 次/分钟 |
| FrequentAuthenticationErrors | warning | 认证错误率 > 5 次/分钟（可能暴力破解） |
| RapidReconnections | info | 快速重连率 > 10 次/分钟 |
| HighBandwidthUsage | info | 带宽 > 100 MiB/s (104857600 B/s) |
| HighFTPErrorRate | warning | 5 分钟内 FTP 协议错误率 > 5 次/分钟(全部原因合计) |
| MaxConnectionsReached | warning | 达到最大连接数限制 |
| HighIdleTimeoutRate | warning | 空闲会话超时率过高(>5 次/分钟) |
| HighDataConnTimeoutRate | warning | 数据连接超时率过高(>5 次/分钟) |
| MaxClientsReached | warning | 触及全局 max_clients 上限 |
| MaxPerIpReached | warning | 触及单 IP 连接数上限(max_per_ip) |
| HighPasvPortFailures | warning | PASV 数据连接建立失败率过高(>5 次/分钟) |
| VsftpExporterDown | critical | Exporter 自身不可用超过 2 分钟 |

启用告警规则：在 `prometheus.yml` 中取消 `rule_files` 的注释：

```yaml
rule_files:
  - "alerts.yml"
```

## Grafana 仪表板

`deploy/grafana-dashboard.json` 提供了预配置的仪表板，包含以下面板：

- 服务状态概览：FTP 服务状态、总连接数、活跃连接数、唯一客户端数、活跃进程数
- 传输统计：上传/下载次数、字节增量、登录总次数（均为所选时间段内增量）、最后登录时间（日期/时分秒两行显示）、带宽与传输速度（所选时间段内上传/下载/总平均带宽）、连接状态趋势图、传输速率图 (MB/s)
- 错误监控:登录失败/认证错误/连接超时/最大连接数限制/快速重连,以及 FTP 协议错误总计与所选时间范围增量(stat)
- 文件类型统计:按后缀聚合的传输计数(`vsftp_files_by_type_total`)
- 内部/遍历传输统计:`vsftp_internal_transfers_total`(目录列表缓存 `.listing`、上传临时文件 `*.writing` 等 vsftpd 内部/遍历传输,已从业务上传/下载计数中分离),含总数 stat 与按方向速率图
- 连接限制与超时(位于仪表盘最底部):空闲会话超时、数据连接超时、全局连接上限(max_clients)、单 IP 连接超限(max_per_ip)、PASV 连接失败及对应速率趋势

仪表板特性：

- 支持 `job`、`group`、`instance` 变量切换(`group` 来自 Prometheus 抓取配置的 `group` 标签,可选择 All)
- 计数类与带宽面板均按仪表板所选时间段计算（`increase(...[$__range])`），并非进程启动以来的累计值
- 默认 30 秒自动刷新
- 中文面板标题

### 导入方式

登录 Grafana → 点击 "+" → "Import" → 上传 `deploy/grafana-dashboard.json` → 在数据源选择器中选中你的 Prometheus 实例（仪表盘通过 `${DS_PROMETHEUS}` 变量引用数据源，不依赖固定名称）

### 常用 PromQL 查询

```promql
# 服务可用性
vsftp_login_success

# 每分钟传输文件数
rate(vsftp_upload_total[1m]) * 60 + rate(vsftp_download_total[1m]) * 60

# 平均传输速度 (MiB/s)
vsftp_average_transfer_speed_bytes_per_second / 1024 / 1024

# 上传/下载流量 (MiB/s)
rate(vsftp_upload_bytes_total[5m]) / 1024 / 1024
rate(vsftp_download_bytes_total[5m]) / 1024 / 1024

# 活跃用户数
count(rate(vsftp_user_logins_total[5m]) > 0)

# Top 10 客户端连接(所选时间范围内连接总数)
topk(10, sum by (client_ip) (increase(vsftp_client_connections_total[$__range])))

# 按原因分类的 FTP 错误
sum by (reason) (rate(vsftp_ftp_errors_total[5m]))

# 目标目录不存在错误
rate(vsftp_ftp_errors_total{reason="dir_not_found"}[5m])

# 各后缀文件每分钟传输数（上传+下载）
sum by (file_type) (rate(vsftp_files_by_type_total[5m]) * 60)

# ts 文件上传速率 (个/分钟)
rate(vsftp_files_by_type_total{file_type="ts", direction="upload"}[5m]) * 60

# 某时间段各后缀文件传输总数（例如近 1 小时，仪表盘横向柱状图使用）
sum by (file_type) (increase(vsftp_files_by_type_total[$__range]))
```

## CI/CD

项目使用 Gitea Actions 工作流：

- **Build & Package** (`.gitea/workflows/build-package.yml`)：仅在推送 `v*` 标签时触发，执行 gofmt 检查、`go vet`、单元测试，串行构建并打包 Linux/Windows/macOS 多平台二进制，上传构建产物，并创建或更新 Gitea Release 及上传资产。通过 `GOFLAGS=-p=1`、`GOMAXPROCS=1`、`CGO_ENABLED=0`、`GOMEMLIMIT=180MiB`、`GOGC=30` 及单 job 串行执行，将峰值内存控制在 200 MiB 以内。

- **Legacy CI** (`.github/workflows/ci.yml`)：GitHub Actions 遗留配置（push/PR 到 main/develop 时执行格式检查、静态分析、测试、构建），仅用于 GitHub，Gitea 上不会运行。

## 项目结构

```text
.
├── cmd/                       # Go 源码
│   ├── main.go                # 程序入口、HTTP 服务、信号处理
│   ├── config.go              # 配置加载与验证
│   ├── metrics.go             # Prometheus 指标定义与注册
│   ├── parsers.go             # 日志解析、连接检查
│   ├── ssh.go                 # SSH 连接管理
│   ├── vsftp-exporter_test.go # 单元测试
│   └── property_test.go       # 属性测试
├── configs/                   # 配置文件
│   ├── config.example.json    # 配置文件模板
│   └── config.json            # 实际配置文件（.gitignore 排除）
├── deploy/                    # 部署辅助
│   ├── prometheus.yml.example # Prometheus 抓取配置模板
│   ├── alerts.yml.example     # Prometheus 告警规则模板
│   ├── grafana-dashboard.json # Grafana 仪表板配置
│   └── vsftpd-exporter.service # systemd 服务文件
├── docs/                      # 文档
│   └── bugrecord.md           # Bug 记录（审查发现与修复状态）
├── .gitea/workflows/          # Gitea Actions CI/CD
│   └── build-package.yml      # 构建、打包、发布工作流
├── .github/workflows/         # GitHub Actions 遗留工作流
│   └── ci.yml                 # 持续集成（仅 GitHub）
├── Makefile                   # 构建、测试、交叉编译
├── go.mod / go.sum            # Go 模块依赖
├── README.md                  # 文档（中文）
├── README_EN.md               # 文档（英文）
└── LICENSE                    # MIT 许可证
```

## 故障排除

**Exporter 启动失败**

- 检查 `configs/config.json` 格式是否正确（JSON 语法）
- 确认所有必需字段已填写（`target_host`、`ftp_user`、`ftp_password`）
- 检查端口号范围（1-65535）和检查间隔范围（1-3600 秒）

**无法连接 FTP 服务器**

- 确认 FTP 服务器地址和端口正确
- 检查用户名密码是否有效
- 检查防火墙和网络连通性
- 查看日志中的 `[ERROR]` 信息

**日志解析无数据**

- 确认日志文件路径正确且有读取权限
- 检查 vsftpd 是否配置了 `xferlog_enable=YES`
- 如使用 SSH 模式，确认 SSH 用户有读取日志文件的权限
- 日志文件为空是正常的（vsftpd 刚启动或无传输活动）

**SSH 连接失败**

- 确认目标服务器 SSH 服务正常运行
- 检查 SSH 端口、用户名、密码是否正确
- 确认网络可达性

**指标数据不更新**

- 检查 `check_interval` 配置是否合理
- 确认 FTP 服务有实际活动
- 查看 exporter 日志输出

**同时启用 xferlog 与 vsftpd.log 时的计数说明**

传输类指标（`vsftp_upload_total`、`vsftp_download_total`、`vsftp_upload_bytes_total`、`vsftp_download_bytes_total`、`vsftp_client_files_total`）以 xferlog 为权威来源；同时启用 vsftpd.log 时不会重复计数。`vsftp_user_connections_total` 按登录事件（OK LOGIN）计数，需要启用 vsftpd.log。

### 日志级别与状态持久化

通过 `-log-level` 参数控制日志输出级别，默认 `info`。可选值：`debug`、`info`、`warn`、`error`。

```bash
# 调试模式，输出所有日志（含每轮解析详情）
./vsftp-exporter -log-level=debug

# 只输出警告和错误
./vsftp-exporter -log-level=warn
```

日志读取位置默认持久化到 `/tmp/vsftp-exporter-state.json`(可用 `-state-file` 覆盖)。
exporter 重启后从上次读取位置继续解析,既不重放历史日志,也不丢失重启期间产生的新日志:

```bash
# 指定状态文件位置(需确保目录可写)
./vsftp-exporter -state-file=/var/lib/vsftp-exporter/state.json
```

## 性能说明

- 采用增量日志读取，每次只处理新增内容（最多 1000 行/轮次）
- SSH 模式使用 `tail -c +N` 增量读取，替代逐字节 `dd bs=1`，显著降低远程主机 I/O 开销；支持远程日志轮转检测
- SSH 命令执行带 10 秒超时，避免远程命令挂起阻塞采集
- 预编译正则表达式，避免重复编译开销
- 支持日志文件轮转检测
- 典型资源占用：内存 < 50MB，CPU < 5%

## 贡献指南

1. Fork 本项目
2. 创建特性分支 (`git checkout -b feature/your-feature`)
3. 确保测试通过 (`make test`)
4. 确保代码格式规范 (`make fmt && make vet`)
5. 提交 Pull Request

## 许可证

本项目采用 [MIT 许可证](LICENSE)。
