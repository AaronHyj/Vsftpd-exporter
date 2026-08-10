# Vsftpd Exporter for Prometheus

A Prometheus exporter for monitoring vsftpd FTP servers. It parses the xferlog transfer log and the vsftpd.log verbose log to provide comprehensive FTP service performance and status metrics.

[![Go Version](https://img.shields.io/badge/Go-1.24+-00ADD8?style=flat&logo=go)](https://golang.org)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

## Quick Start

```bash
# Build
make build

# Copy and edit the configuration
cp configs/config.example.json configs/config.json

# Run
./vsftp-exporter

# Access the service
# Metrics:     http://localhost:9101/metrics
# Health:      http://localhost:9101/health
```

## Overview

Vsftpd Exporter collects monitoring data in the following ways:

1. **FTP login probe** — periodically attempts to log in to the FTP server to verify availability
2. **Connection state statistics** — inspects FTP port connection states (ESTABLISHED / CLOSE_WAIT, etc.) via `ss -tnH`
3. **xferlog parsing** — incrementally reads the standard xferlog to extract upload/download file counts, bytes, transfer durations, client IPs, and file extensions (e.g. ts/mp4/mkv)
4. **vsftpd.log parsing** — incrementally reads the vsftpd verbose log to extract CONNECT/LOGIN events, user activity, and process information
5. **SSH remote collection** — supports connecting to a remote server over SSH to read logs and run `ss`

All collection tasks run periodically at the `check_interval`.

### Key Features

- **Connection monitoring**: real-time totals for FTP connections, ESTABLISHED connections, and CLOSE_WAIT connections
- **Transfer statistics**: upload/download file counts, bytes, transfer duration distribution (Histogram), average transfer speed, and bandwidth usage
- **Error monitoring**: failed logins, transfer errors (by type), connection timeouts, authentication errors, and max-connection limit hits
- **User & client analytics**: logins/connections per username, connections/file transfers per client IP
- **File type statistics**: counts transferred files by their raw extension (e.g. ts/mp4/mkv) and shows counts and rates per extension over a time range
- **Advanced detection**: rapid reconnection detection (same IP reconnecting within 30 s), connect-to-login latency distribution, active process count
- **SSH remote monitoring**: reads log files and executes commands on a remote server over SSH
- **Health check**: provides a `/health` endpoint returning service status as JSON

## Architecture

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
│         │    │  SSH Client (optional)│       │        │
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
    (scrape)                       (monitoring)
```

## Installation and Build

### Requirements

- Go 1.24 or later
- A running vsftpd FTP server
- Read access to the FTP log files (local mode) or SSH access (remote mode)

### Build

```bash
# Clone the project
git clone <repository-url>
cd Vsftpd-exporter

# Download dependencies
go mod download

# Build
make build
```

### Cross Compilation

```bash
make build-linux     # Linux amd64
make build-windows   # Windows amd64
make build-darwin    # macOS amd64
make build-all       # all platforms
```

### Make Targets

| Command | Description |
| ---- | ---- |
| `make build` | Build the binary |
| `make run` | Build and run the program |
| `make test` | Run tests (with race detection and coverage) |
| `make coverage` | Generate an HTML coverage report |
| `make fmt` | Format the code |
| `make vet` | Run static analysis |
| `make tidy` | Tidy Go module dependencies |
| `make install` | Install to /usr/local/bin |
| `make clean` | Clean build artifacts |

### Dependencies

| Package | Version | Purpose |
| -- | ---- | ---- |
| `github.com/jlaffaye/ftp` | v0.2.0 | FTP client used for the login probe |
| `github.com/prometheus/client_golang` | v1.19.1 | Prometheus client library |
| `golang.org/x/crypto` | v0.43.0 | SSH client used for remote collection |
| `pgregory.net/rapid` | v1.2.0 | Property testing (test dependency) |
| `github.com/davecgh/go-spew` | v1.1.1 | Test assertions (indirect dependency) |

## Configuration

### Configuration File (configs/config.json)

Copy `configs/config.example.json` to `configs/config.json` and edit it:

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
    "vsftplog_file_path": "/var/log/vsftpd.log"
}
```

### Configuration Options

| Option | Type | Required | Default | Description |
| ------ | ---- | ---- | ------ | ---- |
| `target_host` | string | Yes | - | Target server address, IP or domain (validated) |
| `ftp_port` | string | No | `21` | FTP port (1-65535) |
| `ftp_user` | string | Yes | - | FTP username (max 64 chars, alphanumeric underscore hyphen) |
| `ftp_password` | string | Yes | - | FTP password (max 128 chars) |
| `need_ssh` | bool | No | `false` | Collect data from a remote server over SSH |
| `ssh_port` | string | No | `22` | SSH port |
| `ssh_user` | string | No | - | SSH username (required when `need_ssh=true`) |
| `ssh_password` | string | No | - | SSH password (required when `need_ssh=true`) |
| `Xferlog_file_path` | string | No | - | Path to the xferlog transfer log; supports environment variables and relative paths |
| `listen_port` | string | No | `9101` | Exporter HTTP listen port |
| `check_interval` | int | No | `30` | Collection interval (1-3600 seconds) |
| `vsftplog_enabled` | bool | No | `false` | Enable vsftpd.log verbose log parsing |
| `vsftplog_file_path` | string | No | - | Path to the vsftpd.log file |

> **Note**: `configs/config.json` is excluded via `.gitignore` and will not be committed. Use `configs/config.example.json` as a template.

## Usage

### Start the Exporter

```bash
# Use the default config file (configs/config.json)
./vsftp-exporter

# Specify a config file path
./vsftp-exporter -config=/path/to/config.json

# Set the log level (debug/info/warn/error, default info)
./vsftp-exporter -log-level=debug
```

### Verify Runtime Status

```bash
# Check the metrics endpoint
curl http://localhost:9101/metrics

# Check health status (returns JSON)
curl http://localhost:9101/health
```

Example health check response:

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

- `status`: `healthy` means the most recent FTP login probe succeeded; `degraded` means it failed (FTP service unreachable or login failed). When `degraded`, the endpoint returns HTTP 503 and includes an `error` field with the failure reason.
- `last_check_time`: the time of the most recent FTP probe.

> **Note (impact on the vsftpd server)**: the exporter periodically performs FTP login probes using a configured real account (the `vsftp_login_success` metric). Every probe produces a real connection and authentication on the server, consuming a connection slot and appearing in the vsftpd logs. Recommendations:
> 1. Create a **dedicated read-only account** for probes to avoid consuming real user slots;
> 2. Do not set the probe frequency too low (`check_interval` ≥ 30 s is recommended);
> 3. Probes no longer increment `vsftp_failed_logins_total` / `vsftp_authentication_errors_total`; those counts come exclusively from the log parser for real client events, avoiding duplicate counting against the server logs.

### SSH Remote Monitoring

To monitor vsftpd on a remote server:

```json
{
    "target_host": "192.168.1.100",
    "need_ssh": true,
    "ssh_port": "22",
    "ssh_user": "root",
    "ssh_password": "your_password",
    "Xferlog_file_path": "/var/log/xferlog",
    "vsftplog_enabled": true,
    "vsftplog_file_path": "/var/log/vsftpd.log"
}
```

In SSH mode the exporter runs `ss -tnH` to collect connection states, uses `tail -c +N` / `cat` for incremental log reads, and `stat` for log rotation detection. The SSH user needs read access to the log files and permission to run `ss`.

### systemd Service

Create `/etc/systemd/system/vsftp-exporter.service`:

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

## Metrics

### Connection State Metrics

| Metric | Type | Description |
| -------- | ---- | ---- |
| `vsftp_login_success` | Gauge | FTP login probe status (1=success, 0=failure) |
| `vsftp_connections` | Gauge | Current total connections on the FTP port |
| `vsftp_established_connections` | Gauge | Number of ESTABLISHED connections |
| `vsftp_close_wait_connections` | Gauge | Number of CLOSE_WAIT connections |

### Transfer Metrics

| Metric | Type | Description |
| -------- | ---- | ---- |
| `vsftp_login_total` | Counter | Total number of FTP logins |
| `vsftp_upload_total` | Counter | Total number of upload operations |
| `vsftp_download_total` | Counter | Total number of download operations |
| `vsftp_upload_bytes_total` | Counter | Total bytes uploaded |
| `vsftp_download_bytes_total` | Counter | Total bytes downloaded |
| `vsftp_transfer_duration_seconds` | Histogram | Transfer duration distribution (buckets: 0.1s~102.4s exponential) |
| `vsftp_average_transfer_speed_bytes_per_second` | Gauge | Average transfer speed (bytes/s) |
| `vsftp_bandwidth_usage_bytes_per_second` | Gauge | Current bandwidth usage (bytes/s) |
| `vsftp_last_login_time` | Gauge | Unix timestamp of the last successful login |

### Error Metrics

| Metric | Type | Labels | Description |
| -------- | ---- | ---- | ---- |
| `vsftp_failed_logins_total` | Counter | - | Total failed login attempts (by FAIL LOGIN events) |
| `vsftp_transfer_errors_total` | Counter | `type` | Total transfer errors (upload/download/timeout) |
| `vsftp_connection_timeouts_total` | Counter | - | Total connection timeouts |
| `vsftp_authentication_errors_total` | Counter | - | Total authentication errors (530 responses only; FAIL LOGIN is no longer double-counted) |
| `vsftp_max_connections_reached_total` | Counter | - | Times the max-connections limit was reached |
| `vsftp_ftp_errors_total` | Counter | `reason` | FTP protocol errors counted by reason; see table below |

Possible `reason` label values for `vsftp_ftp_errors_total`:

| `reason` | Meaning | Typical FTP response |
| -------- | ---- | ---- |
| `auth_failed` | Authentication failure (wrong password, unknown user, etc.) | `530 Login incorrect.` |
| `max_connections` | Rejected due to connection limit | `421 Too many connections`, `530 Maximum number of clients reached` |
| `service_unavailable` | Service unavailable (closing control connection) | `421 Service not available, closing control connection.` |
| `data_connection_error` | Data connection setup/transfer failure | `425 Can't open data connection.`, `426 Connection closed; transfer aborted.`, `450/451` |
| `command_error` | Client command syntax/not implemented/sequence error | `500 Unknown command.`, `501`, `502`, `503 Bad sequence of commands.`, `504` |
| `dir_not_found` | Target directory does not exist | `550 Failed to change directory.` |
| `file_not_found` | Target file missing or cannot be opened | `550 No such file or directory.`, `550 Not a regular file.` |
| `permission_denied` | Insufficient permissions | `550 Permission denied.` |
| `quota_exceeded` | Disk quota exceeded | `552 Exceeded storage allocation.` |
| `file_name_not_allowed` | File name/create operation not allowed | `553 Could not create file.` |
| `other` | Any other 4xx/5xx error | - |

> Note: `vsftp_failed_logins_total` counts `FAIL LOGIN` events; `vsftp_authentication_errors_total` counts only `530` response lines. vsftpd emits both a `FAIL LOGIN` line and a `530 FTP response` line for the same failed login; authentication errors are no longer summed from both.

### Client and User Metrics (requires vsftpd.log)

| Metric | Type | Labels | Description |
| -------- | ---- | ---- | ---- |
| `vsftp_client_connections_total` | Counter | `client_ip` | Total connections per client IP |
| `vsftp_unique_clients` | Gauge | - | Number of unique active clients in the last 5 minutes |
| `vsftp_user_logins_total` | Counter | `username` | Total successful logins per username |
| `vsftp_user_connections_total` | Counter | `username` | Total connections per username |
| `vsftp_client_files_total` | Counter | `client_ip`, `direction` | File transfers per client IP and direction |
| `vsftp_files_by_type_total` | Counter | `file_type`, `direction` | Files transferred per file extension and direction; `file_type` is the lowercase extension directly (e.g. `ts`/`mp4`/`mkv`), or `no_extension` for files without one |

### Advanced Metrics (requires vsftpd.log)

| Metric | Type | Description |
| -------- | ---- | ---- |
| `vsftp_connection_login_delay_seconds` | Histogram | CONNECT-to-LOGIN delay distribution (buckets: 1ms~16s) |
| `vsftp_rapid_reconnections_total` | Counter | Rapid reconnections (same IP reconnecting within 30 s) |
| `vsftp_active_processes` | Gauge | Number of active vsftpd processes in the last 5 minutes |

## Prometheus Configuration

`deploy/prometheus.yml.example` provides a scrape config template. Add the following manually:

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

### Alert Rules

`deploy/alerts.yml.example` contains the following alerts:

| Alert | Severity | Condition |
| -------- | -------- | -------- |
| VsftpdServiceDown | critical | FTP service unavailable for more than 2 minutes |
| HighFailedLoginRate | warning | Failed login rate > 10/min over 5 minutes |
| HighTransferErrorRate | warning | Transfer error rate > 5/min over 5 minutes |
| HighConnectionCount | warning | Connections > 100 for 5 minutes |
| HighCloseWaitConnections | warning | CLOSE_WAIT connections > 20 for 10 minutes |
| FrequentConnectionTimeouts | warning | Connection timeout rate > 3/min |
| FrequentAuthenticationErrors | warning | Authentication error rate > 5/min (possible brute force) |
| RapidReconnections | info | Rapid reconnection rate > 10/min |
| HighBandwidthUsage | info | Bandwidth > 100 MB/s |
| MaxConnectionsReached | warning | Max-connections limit reached |
| VsftpdExporterDown | critical | Exporter itself unavailable for more than 2 minutes |

Enable alert rules by uncommenting `rule_files` in `prometheus.yml`:

```yaml
rule_files:
  - "alerts.yml"
```

## Grafana Dashboard

`deploy/grafana-dashboard.json` provides a preconfigured dashboard with the following panels:

- Service status overview: FTP service status, total connections, active connections, unique clients, active processes
- Transfer statistics: total uploads/downloads, total logins, last login time, connection state trend, transfer rate (MB/s)
- Error monitoring: failed logins, authentication errors, connection timeouts, max-connections limit, rapid reconnections, and FTP protocol error rates split by `reason`

Dashboard features:

- `job` and `instance` variable switching
- 30-second auto refresh by default
- Panel titles in Chinese

### Import

Log in to Grafana → click "+" → "Import" → upload `deploy/grafana-dashboard.json` → select your Prometheus instance in the data source picker (the dashboard references the data source via the `${DS_PROMETHEUS}` variable, so no fixed name is required).

### Useful PromQL Queries

```promql
# Service availability
vsftp_login_success

# Files transferred per minute
rate(vsftp_upload_total[1m]) + rate(vsftp_download_total[1m])

# Average transfer speed (MB/s)
vsftp_average_transfer_speed_bytes_per_second / 1024 / 1024

# Upload/download throughput (MB/s)
rate(vsftp_upload_bytes_total[5m]) / 1024 / 1024
rate(vsftp_download_bytes_total[5m]) / 1024 / 1024

# Active users
count(rate(vsftp_user_logins_total[5m]) > 0)

# Top 10 client connections
topk(10, rate(vsftp_client_connections_total[5m]))

# FTP errors by reason
sum by (reason) (rate(vsftp_ftp_errors_total[5m]))

# Directory-not-found errors
rate(vsftp_ftp_errors_total{reason="dir_not_found"}[5m])

# Files transferred per extension per minute (upload + download)
sum by (file_type) (rate(vsftp_files_by_type_total[5m]))

# ts upload rate (files per minute)
rate(vsftp_files_by_type_total{file_type="ts", direction="upload"}[5m]) * 60

# Total files transferred per extension in a time range (e.g. last 1 hour)
sum by (file_type) (increase(vsftp_files_by_type_total[1h]))
```

## CI/CD

The project uses a Gitea Actions workflow:

- **Build & Package** (`.gitea/workflows/build-package.yml`): triggered only on push of a `v*` tag. Runs gofmt check, `go vet`, unit tests, builds/packages multi-platform binaries (Linux/Windows/macOS), uploads artifacts, and creates/updates a Gitea Release with assets. Peak memory stays within ~200 MiB via `GOFLAGS=-p=1`, `GOMAXPROCS=1`, `CGO_ENABLED=0`, `GOMEMLIMIT=180MiB`, `GOGC=30`, serialized multi-platform builds, and a single job.

- **Legacy CI** (`.github/workflows/ci.yml`): a legacy GitHub Actions workflow (runs format check, static analysis, tests, build on push/PR to main/develop). GitHub only; it is not used on Gitea.

## Project Structure

```text
.
├── cmd/                       # Go source code
│   ├── main.go                # Entry point, HTTP server, signal handling
│   ├── config.go              # Config loading and validation
│   ├── metrics.go             # Prometheus metric definitions and registration
│   ├── parsers.go             # Log parsing, connection checks
│   ├── ssh.go                 # SSH connection management
│   ├── vsftp-exporter_test.go # Unit tests
│   └── property_test.go       # Property tests
├── configs/                   # Config files
│   ├── config.example.json    # Config file template
│   └── config.json            # Actual config (excluded by .gitignore)
├── deploy/                    # Deployment helpers
│   ├── prometheus.yml.example # Prometheus scrape config template
│   ├── alerts.yml.example     # Prometheus alert rules template
│   ├── grafana-dashboard.json # Grafana dashboard config
│   └── vsftpd-exporter.service # systemd service file
├── docs/                      # Documentation
│   └── bugrecord.md           # Bug records (review findings and fix status)
├── .gitea/workflows/          # Gitea Actions CI/CD
│   └── build-package.yml      # Build, package, and release workflow
├── .github/workflows/         # Legacy GitHub Actions workflows
│   └── ci.yml                 # Continuous integration (GitHub only)
├── Makefile                   # Build, test, cross compilation
├── go.mod / go.sum            # Go module dependencies
├── README.md                  # Documentation (Chinese)
├── README_EN.md               # Documentation (English)
└── LICENSE                    # MIT license
```

## Troubleshooting

**Exporter fails to start**

- Check that `configs/config.json` is valid JSON
- Confirm all required fields are set (`target_host`, `ftp_user`, `ftp_password`)
- Check the port range (1-65535) and the check interval range (1-3600 seconds)

**Cannot connect to the FTP server**

- Verify the FTP server address and port
- Check the username and password
- Check the firewall and network connectivity
- Look for `[ERROR]` entries in the logs

**No data from log parsing**

- Confirm the log file path is correct and readable
- Check that vsftpd is configured with `xferlog_enable=YES`
- In SSH mode, confirm the SSH user can read the log files
- An empty log is normal (vsftpd just started or no transfer activity)

**SSH connection fails**

- Confirm the SSH service on the target server is running
- Check the SSH port, username, and password
- Confirm network reachability

**Metrics do not update**

- Check the `check_interval` setting
- Confirm the FTP service has real activity
- Inspect the exporter logs

**Double counting when both xferlog and vsftpd.log are enabled**

Transfer metrics (`vsftp_upload_total`, `vsftp_download_total`, `vsftp_upload_bytes_total`, `vsftp_download_bytes_total`, `vsftp_client_files_total`) use xferlog as the authoritative source; enabling vsftpd.log does not double-count them. `vsftp_user_connections_total` is counted per login event (OK LOGIN) and requires vsftpd.log.

### Log Levels

The `-log-level` flag controls the log output level, default `info`. Valid values: `debug`, `info`, `warn`, `error`.

```bash
# Debug mode, output all logs (including per-round parse details)
./vsftp-exporter -log-level=debug

# Only warnings and errors
./vsftp-exporter -log-level=warn
```

## Performance

- Incremental log reading: only newly added content is processed each round (max 1000 lines/round)
- SSH mode uses `tail -c +N` incremental reads instead of byte-by-byte `dd bs=1`, significantly reducing remote I/O; supports remote log rotation detection
- SSH commands run with a 10-second timeout to prevent hanging
- Precompiled regular expressions avoid recompilation overhead
- Log file rotation detection supported
- Typical resource usage: memory < 50MB, CPU < 5%

## Contributing

1. Fork this repository
2. Create a feature branch (`git checkout -b feature/your-feature`)
3. Ensure tests pass (`make test`)
4. Ensure the code is formatted and passes static checks (`make fmt && make vet`)
5. Submit a Pull Request

## License

This project is licensed under the [MIT license](LICENSE).
