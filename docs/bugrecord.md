# Bug 记录

本文件记录代码审查发现的问题、修复状态与相关说明。每条记录包含问题描述、影响、修复方案与验证方式。

## 审查信息

- 审查日期:2026-08-08(第一轮)/ 2026-08-08(第二轮)/ 2026-08-09(第三轮深度审查与修复)/ 2026-08-12(第四轮全面审查与修复)/ 2026-08-12(第五轮复查:连接数监听套接字污染、FTP握手超时计数、README 告警文档同步)/ 2026-08-12(第六轮复查:README 桶范围错误、FTP response 正则捕获用户名、NAT CONNECT 过滤限制)/ 2026-08-12(第七至十轮深度复查:并发安全、PromQL 语义、过滤条件优先级、四方一致性、冗余指标、测试隔离)/ 2026-08-19(第十一至十二轮专项复核:SSH 存活探测并发锁、FTP 握手 deadline 设置、连接数端口后缀误匹配、负 fileSize counter panic、README 指标来源与安全警示、告警名拼写)/ 2026-08-19(第十三轮:HTTP 服务与指标语义、PromQL 与单位一致性、运行时冒烟测试)/ 2026-08-20(第十四轮:配置校验边界、ss 去重逻辑、go 模块一致性、Makefile .PHONY)/ 2026-08-20(第十五轮:基于 vsftpd_conf.html 新增 A1-A4 连接限制/超时/ PASV 细分监控与告警/面板/文档)
- 审查范围:`cmd/` 全部源码(main.go / config.go / metrics.go / ssh.go / parsers.go / tests)、Makefile、`deploy/grafana-dashboard.json`、`deploy/alerts.yml.example`、`docs/bugrecord.md`、`README.md`
- 审查重点:仪表盘指标覆盖完整性、告警规则聚合正确性、summary_exclude 遗漏、活跃度指标静默期衰减、带宽告警改用 PromQL rate、仪表盘新增面板、连接数指标误计监听套接字、FTP 握手超时计数、README 与告警规则一致性、README 桶范围描述错误、FTP response 正则捕获用户名增强、NAT CONNECT 过滤限制
- 验证命令:`go build -o /dev/null ./cmd`、`go vet ./...`、`go test -v -race ./...`、`python3 -c "import json; json.load(open('deploy/grafana-dashboard.json'))"`

## Bug 列表

| ID | 严重程度 | 位置 | 问题摘要 | 状态 |
|----|----------|------|----------|------|
| BUG-001 | 高 | `cmd/main.go` checkFTPLogin | 登录探测与共享计数器耦合，探测行为污染指标且与服务端日志重复计数 | 已修复 |
| BUG-002 | 高 | `cmd/parsers.go` readRemoteFile | SSH 读日志使用 `dd bs=1`，对远程主机产生巨大 I/O 开销 | 已修复 |
| BUG-003 | 高 | `cmd/parsers.go` readRemoteFile | SSH 模式无日志轮转检测，logrotate 后数据静默丢失 | 已修复 |
| BUG-004 | 高 | `cmd/ssh.go` executeSSHCommand | SSH 命令执行无超时，远程命令挂起将永久阻塞监控协程 | 已修复 |
| BUG-005 | 高 | `cmd/parsers.go` parseFTPLog / parseVsftpdLog | xferlog 与 vsftpd.log 同时启用时上传/下载计数重复 | 已修复 |
| BUG-006 | 高 | `cmd/parsers.go` parseFTPLog | `userConnectionsTotal` 按传输计数，与按登录计数的语义冲突 | 已修复 |
| BUG-007 | 中 | `cmd/main.go` checkFTPLogin | 登录错误分类依赖字符串匹配，且 `connectionTimeoutsTotal` 对任何连接错误都计数 | 已修复 |
| BUG-008 | 中 | `cmd/main.go` healthCheckHandler | `/health` 恒为 healthy，`last_check_time` 无意义 | 已修复 |
| BUG-009 | 中 | `cmd/parsers.go` readLocalFile | 末行无换行时字节计数多算 1，可能触发误判轮转并重复计数 | 已修复 |
| BUG-010 | 中 | `cmd/parsers.go` | SSH/本地读日志无行数上限，超大日志内存耗尽 | 已修复 |
| BUG-011 | 低 | `cmd/metrics.go` | `maxConnectionsReachedTotal` 从未递增，告警永不触发 | 已修复 |
| BUG-012 | 低 | `cmd/main.go` / `cmd/parsers.go` | 死参数：checkFTPLogin 的 state、checkConnections 的 state、parseFTPLog 的 config | 已修复 |
| BUG-013 | 低 | `cmd/main.go` | 启动后首个采集周期内指标为空，health 无参考信息 | 已修复 |
| BUG-014 | 低 | `cmd/parsers.go` extractTimestamp | 生产代码死代码（仅测试引用） | 已修复 |
| BUG-015 | 低 | `cmd/ssh.go` | `InsecureIgnoreHostKey()` 存在 MITM 风险 | 已知限制 |
| BUG-016 | 低 | `configs/config.json` | 明文存储 SSH/FTP 密码 | 已知限制 |
| BUG-017 | 中 | `cmd/parsers.go` | `clientLastConnect` map 无清理逻辑，随新客户端 IP 无限增长（内存泄漏） | 已修复 |
| BUG-018 | 中 | `cmd/parsers.go` parseFTPLog / parseVsftpdLog | 单轮新增日志超过 1000 行时，position 直接跳到文件末尾，超限行被永久跳过（数据丢失） | 已修复 |
| BUG-019 | 低 | `cmd/parsers.go` | `state.lastBytesTransferred` 只写不读，死字段 | 已修复 |
| BUG-020 | 低 | `cmd/parsers.go` | `state.userClientMapping` 只写不读，死数据（随唯一用户名增长内存泄漏） | 已修复 |
| BUG-021 | 低 | `Makefile` / `cmd/main.go` | `-X main.buildTime=$(BUILD_TIME)` 引用了代码中不存在的变量 `main.buildTime` | 已修复 |
| BUG-022 | 低 | `cmd/main.go` healthCheckHandler | degraded 状态仍返回 HTTP 200，且 `probeResult.err` 未在响应中暴露 | 已修复 |
| BUG-023 | 低 | `cmd/main.go` checkFTPLogin | SSH 模式下登录探测仍直连 exporter→target:FTPPort；若 FTP 端口对 exporter 不可达则 `vsftp_login_success` 恒为 0 | 已知限制 |
| BUG-024 | 低 | `cmd/parsers.go` | 日志生产速率持续高于 1000 行/轮时，解析永远落后（backpressure 上限） | 已知限制 |
| BUG-025 | 低 | `cmd/parsers.go` remoteFileSize | 每轮每个日志文件额外一次 SSH 往返（stat），可合并到读取命令中 | 已修复 |
| BUG-026 | 高 | `cmd/parsers.go` parseStandardXferlog | 文件名中包含空格时，`fields` 拆分导致 direction/username/completionStatus 错位解析，将成功传输误判为传输失败错误 | 已修复 |
| BUG-027 | 中 | `cmd/parsers.go` parseFTPLog | 当本轮无传输日志（0 字节传输）时，`bandwidthUsage` 仪表盘指标未被重置为 0，导致带宽指标永久停留在历史非零峰值 | 已修复 |
| BUG-028 | 中 | `cmd/parsers.go` readLocalFile / readRemoteFile | 在读取到达文件末尾（EOF）时若 vsftpd 正在写入半行日志（无换行符 `\n`），会将半行日志消费并推进文件 position，导致下一轮日志拼接断裂并丢失事件 | 已修复 |
| BUG-029 | 低 | `cmd/parsers.go` loginFailRegex | 用户名中括号正则表达式使用 `+` (`\[([^\]]+)\]`)，导致匿名或未提供用户名失败登录产生 `[]` 时无法匹配 | 已修复 |
| BUG-030 | 中 | `cmd/parsers.go` parseVsftpdLog | 同一失败登录的 `FAIL LOGIN` 与 `FTP response: 530` 两行均递增 `authenticationErrorsTotal`，认证错误被重复计数 | 已修复 |
| BUG-031 | 中 | `cmd/parsers.go` parseVsftpdLog | 日志静默期(无新日志行)时 `vsftp_unique_clients` / `vsftp_active_processes` 不衰减,停留历史非零值 | 已修复 |
| BUG-032 | 低 | `deploy/alerts.yml.example` HighTransferErrorRate | 传输错误率告警未跨 `type` 标签聚合,单一类型错误率超限但总体超限时可能不告警,与描述"全部类型合计"不符 | 已修复 |
| BUG-033 | 低 | `cmd/parsers.go` parseVsftpdLog | `summary_exclude` 只过滤 CONNECT 与 OK LOGIN,探测账号的 FAIL LOGIN 与 530/FTP response 错误仍计入统计,凭据错误时污染 `failedLoginsTotal` / `authenticationErrorsTotal` / `ftpErrorsTotal` | 已修复 |
| BUG-034 | 低 | `deploy/alerts.yml.example` HighBandwidthUsage | 带宽告警使用 KNOWN-002 语义受限的 `vsftp_bandwidth_usage_bytes_per_second` Gauge,按文档建议应改用 PromQL `rate(vsftp_upload_bytes_total + vsftp_download_bytes_total[5m])` | 已修复 |
| BUG-035 | 低 | `deploy/grafana-dashboard.json` | 仪表盘缺少 `vsftp_bandwidth_usage_bytes_per_second` 与 `vsftp_average_transfer_speed_bytes_per_second` 面板,两个已导出指标无法观测 | 已修复 |
| BUG-036 | 低 | `deploy/grafana-dashboard.json` 面板22 | 错误速率趋势面板未包含 `vsftp_ftp_errors_total`(FTP 协议错误),错误监控不完整 | 已修复 |
| BUG-037 | 高 | `deploy/alerts.yml.example` HighBandwidthUsage | PromQL 语法错误:`rate(a + b[5m])` 中 `[5m]` 只能作用于选择器,`+` 连接瞬时向量与区间向量导致规则加载失败 | 已修复 |
| BUG-038 | 中 | `cmd/parsers.go` parseVsftpdLog | 双日志模式(xferlog + vsftpd.log)下 `state.totalBytesUploaded/Downloaded` 被两处累加,`vsftp_average_transfer_speed_bytes_per_second` 计算为真实值 2 倍(BUG-005 修复声称"不受影响"但不成立) | 已修复 |
| BUG-039 | 中 | `deploy/alerts.yml.example` 全部 rate 类告警 | `rate()` 单位为次/秒,阈值未乘 60 时与描述"次/分钟"相差 60 倍;如 `rate > 10` 实为 600 次/分钟 | 已修复 |
| BUG-040 | 中 | `deploy/alerts.yml.example` HighTransferErrorRate / HighFTPErrorRate | `sum(rate(...))` 聚合全部标签导致 `{{ $labels.instance }}` 为空,且多实例部署时跨实例全局聚合,阈值判断错误 | 已修复 |
| BUG-041 | 低 | `deploy/grafana-dashboard.json` 面板22 | 面板22 D/E 查询 `sum(rate(...))` 与面板104 的 `sum by (reason)` 风格不一致,改用 `sum by (job, instance)` | 已修复 |
| BUG-042 | 中 | `cmd/parsers.go` parseVsftpdLog | 仅 vsftpd.log 模式(未配置 xferlog)下 `bandwidthUsage` / `averageTransferSpeed` 恒为 0——两者只在 `parseFTPLog` 中更新,但 vsftpd.log-only 时 `parseFTPLog` 不运行 | 已修复 |
| BUG-043 | 中 | `cmd/parsers.go` parseSSOutput | `ss -tnH` 输出中的 LISTEN 监听套接字本地地址以 FTP 端口结尾被计入 `vsftp_connections`,导致空闲时连接数恒 ≥ 1(IPv4)或 ≥ 2(IPv4+IPv6 双监听),抬高了 HighConnectionCount 告警基线 | 已修复 |
| BUG-044 | 低 | `cmd/main.go` checkFTPLogin | 第二个 `ftp.Dial`(FTP 握手阶段)超时不计数 `connectionTimeoutsTotal`,FrequentConnectionTimeouts 告警统计不完整 | 已修复 |
| BUG-045 | 低 | `README.md` | 指标表 `vsftp_transfer_duration_seconds` 桶范围描述为"0.1s~102.4s",实际代码 `ExponentialBuckets(0.1, 2, 10)` 最大桶为 51.2s | 已修复 |
| BUG-046 | 中 | `cmd/parsers.go` ftpResponseRegex | FTP response 正则未捕获 `[user]` 用户名,NAT 场景下探测 IP 与日志 Client IP 不一致时 `summary_exclude` 无法按用户名过滤探测的错误响应(530 等) | 已修复 |
| BUG-047 | 低 | `README_EN.md` | 英文版 README 文档与中文版不同步:`vsftp_transfer_duration_seconds` 桶范围仍写 "0.1s~102.4s"(应为 51.2s);告警表缺失 `HighFTPErrorRate` 行 | 已修复 |
| BUG-048 | 中 | `cmd/config.go` | `os.ExpandEnv` 展开环境变量路径为死代码:`isValidFilePath`(正则排除 `$`、`{}`、`~`)在展开前先校验,任何含环境变量引用的路径都被拒绝,展开逻辑永远到不了,与 README"支持环境变量"承诺不符 | 已修复 |
| BUG-049 | 高 | `cmd/main.go` checkFTPLogin | `ftp.Dial` 用 `DialWithNetConn` 时跳过库自身的 context 超时,握手(220)与 `Login` 读取无 deadline;服务器接受 TCP 但迟迟不返回 220 时,`ftp.Dial`/`Login` 无限阻塞,监控协程(ticker 同步调用)卡死,探活与 /health 全部停滞 | 已修复 |
| BUG-050 | 中 | `cmd/metrics.go` | `vsftp_connection_login_delay_seconds` 桶 `ExponentialBuckets(0.001,2,15)` 最大桶 16.384s,但观测 guard 允许 `delay<=60`,17~60s 的观测全部落入 +Inf,面板 99 分位延迟失真;扩桶到 17 桶(最大 65.5s)覆盖 60s | 已修复 |
| BUG-051 | 中 | `cmd/vsftp-exporter_test.go` | 测试操作全局 package 级指标且无隔离:`TestParseFTPLogFilesByTypeCounter`/`TestParseFTPLogFilesByTypeColdStart` 依赖 CounterVec 从 0 起点,`go test -count>1` 或并行时互相污染而失败 | 已修复 |
| BUG-052 | 中 | `cmd/main.go`、`cmd/ssh.go` | IPv6 地址拼接错误:`isValidHost` 校验放行裸 IPv6(`::1`),但连接处用 `host+":"+port` 拼出 `::1:21`,`net.Dial` 报 "too many colons";改用 `net.JoinHostPort` | 已修复 |
| BUG-053 | 中 | `cmd/parsers.go` parseSSOutput | 端口匹配用 `strings.HasSuffix(":"+ftpPort)` 后缀比较:随机客户端端口(如 `:56069` 以 `:6069` 结尾)或 `:210`/`:2169`(以 `:21` 结尾)会被误判为 FTP 连接,污染 `vsftp_connections` 等连接数指标 | 已修复 |
| BUG-054 | 高 | `cmd/parsers.go` | 畸形 xferlog/vsftpd.log 出现**负 fileSize/bytes** 时,`strconv.ParseInt` 不报错但把负值传入 `prometheus counter.Add`(client_golang v1.19 在 v<0 时 `panic("counter cannot decrease")`),`runChecks` 无 recover,整个监控协程崩溃停止采集 | 已修复 |
| BUG-055 | 中 | `README.md`、`README_EN.md` | 指标来源标注错误:`vsftp_login_total`、`vsftp_last_login_time` 实际需 vsftpd.log 却放在"传输统计"表;`vsftp_client_files_total`、`vsftp_files_by_type_total` 实际来自 xferlog 却被标"需启用 vsftpd.log",误导用户配置 | 已修复 |
| BUG-056 | 低 | `README.md`、`README_EN.md` | 全篇无安全警示,`ssh.InsecureIgnoreHostKey()`(MITM 风险)与 config.json 明文密码均未告知用户 | 已修复 |
| BUG-057 | 低 | `README_EN.md` | 告警名拼写 `VsftpdExporterDown`(多 d)与 `deploy/alerts.yml.example`、`README.md` 的 `VsftpExporterDown` 不一致 | 已修复 |
| BUG-058 | 中 | `cmd/main.go` monitor 协程 | 监控协程在 ticker 循环里直接调用 `runChecks`,无任何 panic 兜底;BUG-054 已证明畸形输入可触发 `prometheus counter.Add(v<0)` panic,一旦发生整个监控协程崩溃、所有指标静默停滞,而 `/metrics`/`/health` 仍存活造成报警盲区 | 已修复 |
| BUG-059 | 中 | `README.md`、`README_EN.md` | 指标文档小节与表格损坏:带标签指标(`vsftp_client_files_total`/`vsftp_files_by_type_total`/登录指标)被拆分到 3 列表(传输统计)与 4 列表(客户端统计)之间,来源标注与文档列数不一致、`vsftp_client_files_total` 一度丢失;且 `Xferlog_file_path` 的"支持环境变量"未说明仅限本地模式 | 已修复 |
| BUG-060 | 中 | `README.md`、`README_EN.md`、`deploy/grafana-dashboard.json` | 查询示例与单位标注不一致:`vsftp_transfer_errors_total` 的 `type` 文档写 "(upload/download/timeout)" 但代码只产 upload/download(timeout 永不产生,`type="timeout"` 查询恒空);"每分钟"的 `rate()` 未乘 60(rate 实际是次/秒,注释与数值口径不符);带宽换算 `/1024/1024`(MiB) 却标注/渲染为 MB/s(dashboard 面板14 unit=MBs + 标题,README 注释),显示偏差约 4.6% | 已修复 |
| BUG-061 | 低 | `Makefile` | `.PHONY` 声明不全: `all`、`coverage`、`tidy`、`build-all`、`build-linux`、`build-windows`、`build-darwin` 未在 `.PHONY` 中声明。GNU make 默认把这些目标视为文件名(若存在同名文件则跳过 recipe),虽当前无同名文件无害,但重构时可能产生意外跳过。 | 已修复 |
| BUG-062 | 中 | `cmd/metrics.go` / `cmd/parsers.go` | 依据 vsftpd_conf.html 新增 A1-A4 细分监控:`vsftp_idle_timeout_total`(idle_session_timeout)、`vsftp_data_connection_timeout_total`(data_connection_timeout)、`vsftp_connection_limit_rejections_total{reason=max_clients|max_per_ip}`、`vsftp_pasv_port_rejections_total`;并新增 5 条告警、Grafana 面板与文档(英文 "A5" ASCII 未做,由用户权衡后跳过) | 已完成 |
| BUG-063 | 低 | `cmd/vsftp-exporter_test.go` | BUG-062 新增测试在文件末尾追加时多出一个空行,`gofmt -l` 报格式不符(gofmt:多个函数间仅应有一个空行)。 | 已修复 |

## 详细说明

### BUG-001：登录探测污染指标并重复计数（高）

**位置**：`cmd/main.go:58-78`

**问题**：

1. 探测使用真实账号每 `check_interval`（当前配置 2 秒）做一次完整登录，每次都会在服务端产生 `CONNECT` + `OK LOGIN` / `FAIL LOGIN` 事件，这些事件被 `parseVsftpdLog` 解析后计入 `clientConnectionsTotal`（携带 exporter 自身 IP）、`userLoginsTotal`、`ftpLoginTotal`、`ftpLoginTime`、`uniqueClients`、`activeProcesses`、`rapidReconnectionsTotal` 等指标，污染真实数据。
2. 探测失败时直接 `Inc()` `failedLoginsTotal` 与 `authenticationErrorsTotal`；而同一失败事件随后又会被日志解析器计数一次，造成双重计数。

**修复**：

- 探测只负责更新 `vsftp_login_success` Gauge；失败/认证错误计数完全交由日志解析器负责（日志解析器统计的是全部客户端的真实失败事件）。
- `connectionTimeoutsTotal` 仅在真正的网络超时（`net.Error.Timeout()`）时递增。

**遗留建议**（未在本轮实施）：探测产生的登录事件仍会出现在服务端日志并被统计。若需完全消除影响，建议：

1. 使用专用的只读探测账号，避免占用真实账号的连接配额；
2. 或将探测频率与日志解析频率解耦；
3. 或在部署侧排除 exporter 自身的客户端 IP。

### BUG-002：SSH 读日志使用 `dd bs=1`（高）

**位置**：`cmd/parsers.go:76`

**问题**：`dd if='...' bs=1 skip=N` 每次 `read()` 只读 1 字节，对大型日志文件是灾难性的系统调用开销，且每轮都会从 position 重新传输整个剩余文件。

**修复**：改用 `tail -c +N file`（单次高效读取）+ `head -n 1000` 限制行数。

### BUG-003：SSH 模式无日志轮转检测（高）

**位置**：`cmd/parsers.go:63-94`

**问题**：本地模式（`readLocalFile`）有 `size < startPosition` 的轮转检测，SSH 模式没有。logrotate 后 `dd skip` 越过 EOF 输出为空，position 永不复位，后续数据静默丢失。

**修复**：SSH 读取前检测远程文件大小，若 `size < startPosition` 则从头读取。

### BUG-004：SSH 命令执行无超时（高）

**位置**：`cmd/ssh.go:91-104`

**问题**：`session.Output()` 无限等待。远程 `ss`/`cat`/`tail`/`stat` 一旦挂起，整个监控协程永久阻塞。本地模式有 10 秒超时（context），SSH 模式没有。

**修复**：`executeSSHCommand` 通过 goroutine + `time.After` 实现 10 秒超时，超时后强制关闭 session。

### BUG-005：双日志源重复计数（高）

**位置**：`cmd/parsers.go:382-408`

**问题**：同时启用 xferlog 与 vsftpd.log 时，同一上传/下载事件被两边各计数一次（`ftpUploadTotal`、`uploadBytesTotal`、`ftpDownloadTotal`、`downloadBytesTotal`、`clientFilesTotal`）。

**修复**：xferlog 是传输计数的权威来源。当 `Xferlog_file_path` 已配置时，`parseVsftpdLog` 不再递增传输类指标；仅当未配置 xferlog 时才由 vsftpd.log 承担。`state.totalBytesUploaded/Downloaded` 与平均速度计算不受影响。

### BUG-006：userConnectionsTotal 语义冲突（高）

**位置**：`cmd/parsers.go:253`

**问题**：`userConnectionsTotal` 在 `parseFTPLog` 中按"完成传输次数"递增，在 `parseVsftpdLog` 中按"登录次数"递增，同一指标混用两种语义。

**修复**：移除 `parseFTPLog` 中的传输计数，该指标只按登录事件（OK LOGIN）计数，与 README 中"需启用 vsftpd.log"的描述一致。

### BUG-007：登录错误分类不可靠（中）

**位置**：`cmd/main.go:68`

**问题**：使用 `strings.Contains(err.Error(), "530")` 判断认证失败，脆弱的字符串匹配。库实际返回 `*textproto.Error{Code:...}`。

**修复**：改用 `errors.As(err, &protoErr)` 并检查 `protoErr.Code == 530`。

### BUG-008：/health 恒为 healthy（中）

**位置**：`cmd/main.go:35-56`

**问题**：`Store(now)` 后立即读回，`last_check_time` 恒为当前时间；状态不反映真实 FTP 可用性。

**修复**：记录最近一次探测结果（时间 + 成功/失败）。失败时 `status` 返回 `degraded`，`last_check_time` 为最近探测时间。

### BUG-009：本地日志末行字节计数多算（中）

**位置**：`cmd/parsers.go:96-131`

**问题**：`bytesRead += len(scanner.Bytes()) + 1` 假设每行都有换行符。当日志文件末行无换行时多算 1 字节，导致 `newPosition` 超出文件大小，下一轮误判为轮转并从头重读，产生重复计数。

**修复**：改用 `bufio.Reader.ReadString('\n')` 按真实字节长度（含换行符）计数。

### BUG-010：日志读取无内存上限（中）

**位置**：`cmd/parsers.go`

**问题**：SSH 模式 `session.Output` 将整个剩余文件载入内存；`maxLinesPerRead=1000` 只限制处理、不限制读取。超大日志可导致 exporter OOM。

**修复**：本地与 SSH 读取均在 1000 行处停止（`head -n 1000` / 读取循环上限），position 按实际消费字节推进。

### BUG-011：maxConnectionsReachedTotal 永不触发（低）

**位置**：`cmd/metrics.go:87`

**问题**：指标已注册但从未递增，对应告警永远不会触发。

**修复**：解析 vsftpd.log 的 `530` 响应时，若消息包含 `maximum number of clients`（vsftpd 达到 `max_clients` 限制的日志），递增该指标。

### BUG-012：死参数（低）

**位置**：`cmd/main.go:58`、`cmd/parsers.go:506`、`cmd/parsers.go:177`

**问题**：`checkFTPLogin` 的 `state`、`checkConnections` 的 `state`、`parseFTPLog` 的 `config` 参数均未使用。

**修复**：移除未使用参数。

### BUG-013：启动后首个采集周期内无指标（低）

**位置**：`cmd/main.go:124-158`

**问题**：监控协程启动后需等待一个 `check_interval` 才开始采集，期间指标为空。

**修复**：进入 ticker 循环前立即执行一轮采集。

### BUG-014：extractTimestamp 死代码（低）

**位置**：`cmd/parsers.go:436-457`

**问题**：该函数仅在单元测试中引用，生产代码未使用。

**修复**：第三轮审查中清理该死代码及测试中对应的单测和未引用的正则定义。

### BUG-015：SSH 不校验主机密钥（低）

**位置**：`cmd/ssh.go:77`

**问题**：`ssh.InsecureIgnoreHostKey()` 存在中间人攻击风险。

**建议**：改用 known_hosts 校验（`ssh.KnownHosts`）。

### BUG-016：明文密码（低）

**位置**：`configs/config.json`

**问题**：SSH/FTP 密码明文存储在配置文件中。

**建议**：配合 systemd `LoadCredential` / 环境变量 / 密钥认证使用。

### BUG-017：clientLastConnect map 无限增长（中）

**位置**：`cmd/parsers.go` `ExporterState` / `updateUniqueClientsMetric`

**问题**：`clientLastConnect` 记录每个客户端 IP 的最近一次连接时间，用于快速重连检测。`updateUniqueClientsMetric` 只清理 `clientLastActivity` 和 `clientConnectTimes`，从未清理 `clientLastConnect`。每个曾出现过的客户端 IP 都会永久保留一条记录，长期运行的 exporter 该 map 无限增长（慢速内存泄漏）。

**修复**：在 `updateUniqueClientsMetric` 清理过期 IP 时同步删除 `clientLastConnect` 条目（写 `clientLastConnect` 的路径均同时写 `clientLastActivity`，遍历后者可完整覆盖）。

### BUG-018：单轮日志超限导致数据丢失（中）

**位置**：`cmd/parsers.go` parseFTPLog / parseVsftpdLog

**问题**：旧实现先读取**整个剩余文件**（本地：全文件；SSH：`dd` 读全尾部），处理时只消费前 1000 行，却把 `state.lastPosition` 设置为读取位置（文件末尾）。当一轮新增日志超过 1000 行时，超限的后续行被永久跳过，产生静默数据丢失。

**修复**：读取阶段即限制在 1000 行（本地循环上限 / SSH `head -n 1000`），position 只推进实际消费的字节数，超限行留待下一轮继续处理。（第一轮修复 BUG-010 时一并解决。）

### BUG-019：lastBytesTransferred 死字段（低）

**位置**：`cmd/parsers.go:40`

**问题**：`state.lastBytesTransferred` 只在 parseFTPLog 中累加（`+= totalBytesThisRound`），从未被读取，属于写后不用的死数据。

**修复**：第三轮审查中移除 `ExporterState` 中的 `lastBytesTransferred` 字段及其累加逻辑。

### BUG-020：userClientMapping 死数据与内存泄漏（低）

**位置**：`cmd/parsers.go:46`

**问题**：`userClientMapping`（用户名→IP）只在 OK LOGIN 时写入，从未被读取，属于只写不用的 map，且从未清理，随唯一用户名无上限增长导致内存泄漏。

**修复**：第三轮审查中移除 `ExporterState` 中的 `userClientMapping` 字段及其写入逻辑与测试断言。

### BUG-021：Makefile 引用不存在的 buildTime 变量（低）

**位置**：`Makefile:9` / `cmd/main.go`

**问题**：`-X main.buildTime=$(BUILD_TIME)` 注入的是 `main.buildTime`，但 `cmd/main.go` 中并不存在该变量，该 ldflags 注入无效。

**修复**：在 `main.go` 中声明 `buildTime` 变量，并在 `HealthStatus` 及 `/health` 响应中暴露。

### BUG-022：/health 未区分 HTTP 状态码及错误隐蔽（低）

**位置**：`cmd/main.go` healthCheckHandler

**问题**：`degraded` 状态下仍返回 HTTP 200，仅靠响应体 `status` 字段区分；`probeResult.err`（失败原因）也未在响应中暴露，外部健康检查器无法根据 HTTP code 告警。

**修复**：`degraded` 状态时返回 HTTP 503 Service Unavailable 并在响应体中引入 `error` 字段暴露具体探活报错。

### BUG-023：SSH 模式下探测仍直连 FTP 端口（低）

**位置**：`cmd/main.go` checkFTPLogin

**问题**：SSH 模式仅用于日志与 `ss` 采集，登录探测仍由 exporter 直接 TCP 连接 `target_host:ftp_port`。若 FTP 端口仅对目标主机内网开放、exporter 无法直连（只能 SSH），则 `vsftp_login_success` 恒为 0，即使服务正常。

**建议**：探测失败时回退通过 SSH 执行 FTP 探测（如 `ftp`/`nc`），或允许单独配置探测目标。

### BUG-024：日志生产速率超上限时永久落后（低）

**位置**：`cmd/parsers.go`

**问题**：每轮最多消费 1000 行。若日志生产速率持续高于 1000 行/轮（如高并发环境 + 极短 `check_interval`），解析位置将永远追不上文件尾部，指标持续滞后。

**建议**：监控解析滞后程度（如记录 `lastPosition` 与当前文件大小的差值）并告警；或按需提高 `maxLinesPerRead`。

### BUG-025：SSH 轮转检测额外一次往返（低）

**位置**：`cmd/parsers.go` readRemoteFile / remoteFileSize

**问题**：每轮每个日志文件先 `stat -c %s` 再读取，增加一次额外 SSH 往返。

**修复**：第三轮审查中将 `stat` 与 `tail`/`cat` 合并至单条 SSH 命令执行（`s=$(stat ...); if [ "$s" -lt pos ]; then echo ROTATED; ... else echo OK; ... fi`），在保证远程轮转检测的同时消除了额外往返开销。

### BUG-026：parseStandardXferlog 文件名包含空格解析错位（高）

**位置**：`cmd/parsers.go` parseStandardXferlog

**问题**：标准 xferlog 日志按空格分隔。若传输的文件名路径包含空格，`strings.Fields(line)` 拆分后的字段数 `n > 18`。旧实现硬编码索引用 `fields[11]` 取 direction、`fields[13]` 取 username、`fields[17]` 取 completionStatus，导致字段全部错位。例如 `completionStatus` 误取到 `*` 而非 `c`，导致成功的带有空格文件名的传输被错误归类为传输失败，并触发 `vsftp_transfer_errors_total` 指标累加。

**修复**：xferlog 结尾 9 个字段（`transferType` 至 `completionStatus`）固定无空格，从切片末尾逆向索引（如 `direction` 为 `fields[n-7]`，`username` 为 `fields[n-5]`，`completionStatus` 为 `fields[n-1]`），文件名合并 `fields[8:n-9]`。

### BUG-027：bandwidthUsage 带宽指标在无传输时无法降零（中）

**位置**：`cmd/parsers.go` parseFTPLog

**问题**：`bandwidthUsage.Set(...)` 仅在 `!earliestTime.IsZero() && !latestTime.IsZero()` 且 `logTimeDiff > 0` 时调用。当某个采样周期内无新的传输日志（`totalBytesThisRound == 0`）时，条件不满足，`bandwidthUsage` 仪表盘 Gauge 保留上一周期计算出的非零数值，导致在 FTP 停止传输后带宽指标永久停留在历史峰值。

**修复**：当 `totalBytesThisRound == 0` 时，显式调用 `bandwidthUsage.Set(0)`，确保空闲时带宽指标准确回落为 0。

### BUG-028：EOF 处消费未完成写入的半行日志（中）

**位置**：`cmd/parsers.go` readLocalFile / readRemoteFile

**问题**：vsftpd 在并发写入日志时，可能正好在 exporter 采样瞬间仅写入半行日志（无末尾 `\n`）。旧实现在 EOF 处也会将该半行日志消费并推进文件 offset/position。下一周期 vsftpd 补齐该行剩余部分及 `\n` 时，exporter 从中途读取后半行，导致前后两半行均因正则匹配失败而被丢弃，丢失传输/登录事件。

**修复**：在读取末尾（EOF）检测末行是否有 `\n`。若末行无换行符，视为尚未写入完成的脏数据，不追加至待处理行切片且不推进 `newPosition`，留待下一周期整行读取。

### BUG-029：loginFailRegex 无法匹配空用户名 FAIL LOGIN（低）

**位置**：`cmd/parsers.go` loginFailRegex

**问题**：`loginFailRegex` 使用 `\[([^\]]+)\]` 匹配用户名。当客户端在未发送 USER 命令或匿名尝试失败产生 `[pid 1234] [] FAIL LOGIN: Client "x.x.x.x"` 时，中括号内字符数为 0，`+` 量词导致正则匹配失败，遗漏登录失败事件。

**修复**：将 `\[([^\]]+)\]` 修改为 `\[([^\]]*)\]`，允许中括号内部为空字符串。

### BUG-030：认证错误重复计数（中）

**位置**：`cmd/parsers.go` parseVsftpdLog

**问题**：vsftpd 对同一次失败登录会同时输出 `FAIL LOGIN` 事件行与 `FTP response: "530 ..."` 响应行。原实现两处都递增 `vsftp_authentication_errors_total`，导致认证错误被计 2 次。

**修复**：
1. `authenticationErrorsTotal` 仅由 `530` 响应行递增，`FAIL LOGIN` 分支只保留 `failedLoginsTotal`（登录尝试次数）。
2. FTP 响应解析泛化为任意 `4xx/5xx`，新增 `vsftp_ftp_errors_total{reason}` 按原因分类计数，`reason` 取值：`auth_failed` / `max_connections` / `service_unavailable` / `data_connection_error` / `command_error` / `dir_not_found` / `file_not_found` / `permission_denied` / `quota_exceeded` / `file_name_not_allowed` / `other`。


### BUG-031:活跃度指标在日志静默期不衰减(中)

**位置**:`cmd/parsers.go` parseVsftpdLog

**问题**:`vsftp_unique_clients` 和 `vsftp_active_processes` 的更新逻辑在 `parseVsftpdLog` 的 `for` 循环内部,位于 `len(lines)==0` 提前 break 之后。当 vsftpd 日志无新行时(静默期),这两个指标不会被刷新,5 分钟超时清理逻辑不执行,客户端/进程数停留在历史非零值,即使所有客户端已断开。

**修复**:将 `updateUniqueClientsMetric` 与 `updateActiveProcessesMetric` 的调用移到 `len(lines)==0` break 之前,确保每轮 `parseVsftpdLog` 调用(无论是否有新日志行)都会按时执行清理。1 分钟节流逻辑保持不变。

### BUG-032:传输错误率告警未跨类型聚合(低)

**位置**:`deploy/alerts.yml.example` HighTransferErrorRate

**问题**:`vsftp_transfer_errors_total` 带有 `type` 标签(取值 `upload` / `download`)。原告警表达式 `rate(vsftp_transfer_errors_total[5m]) > 5` 是向量表达式,要求每个 `type` 标签值单独满足阈值,而非所有类型合计。当上传错误率 3 次/分钟、下载错误率 3 次/分钟时,合计 6 次/分钟超限但不告警,与描述"传输错误率超过 5 次/分钟"不符。

**修复**:改为 `sum(rate(vsftp_transfer_errors_total[5m])) > 5`,按全部类型合计判断。

### BUG-033:summary_exclude 未过滤探测失败事件(低)

**位置**:`cmd/parsers.go` parseVsftpdLog

**问题**:`summary_exclude=true` 时,CONNECT 按 `clientIP == probeClientIP` 过滤,OK LOGIN 按 `username == config.FTPUser` 过滤。但以下分支未做过滤:
1. FAIL LOGIN 事件:探测账号凭据错误时 vsftpd 记录 `FAIL LOGIN` 行,计入 `failedLoginsTotal`,但探测失败本质是 exporter 自身行为,不应污染登录失败统计。
2. FTP 响应 4xx/5xx 事件:探测失败产生的 `530 Login incorrect` 响应计入 `authenticationErrorsTotal` 与 `ftpErrorsTotal`,同样污染认证错误统计。

**修复**:
1. FAIL LOGIN 分支:在解析 `username` 与 `clientIP` 后,增加 `summary_exclude` 过滤条件 `clientIP == state.probeClientIP || username == config.FTPUser`。
2. FTP 响应分支:在递增错误计数前,增加 `summary_exclude` 过滤条件 `clientIP == state.probeClientIP`(响应行中 `matches[3]` 为客户端 IP)。

### BUG-034:带宽告警基于语义受限的 Gauge(低)

**位置**:`deploy/alerts.yml.example` HighBandwidthUsage

**问题**:`vsftp_bandwidth_usage_bytes_per_second` 的计算基于日志事件时间戳区间,单事件轮次(时间跨度=0)不更新,且 vsftpd.log-only 模式下恒为 0。KNOWN-002 已明确建议以 PromQL `rate()` 为主。原告警基于该有缺陷的 Gauge,可能漏报或误报。

**修复**:将告警表达式改为 `rate(vsftp_upload_bytes_total + vsftp_download_bytes_total[5m]) > 104857600`(即 100 MB/s),基于原始计数器,语义明确且无单事件缺陷。

### BUG-035:仪表盘缺少已导出指标的观测面板(低)

**位置**:`deploy/grafana-dashboard.json`

**问题**:exporter 导出 `vsftp_bandwidth_usage_bytes_per_second`(Gauge) 和 `vsftp_average_transfer_speed_bytes_per_second`(Gauge) 两个指标,但仪表盘没有任何面板展示这两个指标。`HighBandwidthUsage` 告警依赖前者,但无法在仪表盘上观测其历史趋势。

**修复**:在"传输统计"行之后新增"⚡ 带宽与速度指标"行,包含:
- 实时带宽面板 (`vsftp_bandwidth_usage_bytes_per_second`,timeseries,提示 KNOWN-002 语义限制)
- 平均传输速度面板 (`vsftp_average_transfer_speed_bytes_per_second`,stat,提示 KNOWN-003 语义)

### BUG-036:错误速率趋势面板未包含 FTP 协议错误(低)

**位置**:`deploy/grafana-dashboard.json` 面板22

**问题**:面板22"错误速率趋势"展示了登录失败、认证错误、连接超时、传输错误四种速率,但未包含 `vsftp_ftp_errors_total`(FTP 协议错误,按原因分类)。BUG-030 新增的 FTP 协议错误监控不完整。

**修复**:在面板22的 targets 中新增第5条查询 `sum(rate(vsftp_ftp_errors_total{job="$job", instance="$instance"}[5m])) * 60`,legendFormat 为"FTP 协议错误"。

### BUG-037:带宽告警 PromQL 语法错误(高)

**位置**:`deploy/alerts.yml.example` HighBandwidthUsage

**问题**:第四轮修复 BUG-034 时,将告警表达式写成 `rate(vsftp_upload_bytes_total + vsftp_download_bytes_total[5m]) > 104857600`。PromQL 语法规定 `[5m]` 区间选择器只能作用于指标选择器,不能作用于算术表达式:`rate(a + b[5m])` 中 `b[5m]` 是区间向量,与瞬时向量 `a` 相加类型不匹配,Prometheus 规则加载时会报错,导致整组告警规则无法生效。

**修复**:改为 `rate(vsftp_upload_bytes_total[5m]) + rate(vsftp_download_bytes_total[5m]) > 104857600`,分别对两个计数器取 rate 再相加。

### BUG-038:双日志模式下平均速度字节双重累加(中)

**位置**:`cmd/parsers.go` parseVsftpdLog OK UPLOAD / OK DOWNLOAD

**问题**:`state.totalBytesUploaded` 与 `state.totalBytesDownloaded` 用于计算 `vsftp_average_transfer_speed_bytes_per_second`(总字节/总运行时长)。当 xferlog 与 vsftpd.log 同时启用时(默认配置 `configs/config.json` 即如此),同一传输事件在 `parseFTPLog`(第406/412行)和 `parseVsftpdLog`(第644/660行)各累加一次,平均速度指标虚高为真实值 2 倍。BUG-005 修复时声称"state.totalBytesUploaded/Downloaded 与平均速度计算不受影响",实际不成立。

**修复**:在 `parseVsftpdLog` 的 OK UPLOAD/OK DOWNLOAD 分支中,将 `state.totalBytes*` 累加移入 `if config.LogFilePath == ""` 守卫内,与传输计数指标(BUG-005 修复)保持一致:xferlog 启用时由 xferlog 累计,否则由 vsftpd.log 累计。

### BUG-039:rate 类告警阈值单位错误(中)

**位置**:`deploy/alerts.yml.example` 全部 rate 类告警

**问题**:PromQL `rate()` 返回**每秒**事件数,但告警描述均写"次/分钟"。原表达式如 `rate(vsftp_failed_logins_total[5m]) > 10` 实际阈值是 10 次/秒 = 600 次/分钟,与描述"超过 10 次/分钟"相差 60 倍,导致阈值形同虚设(正常流量即可触发)或描述误导。

**修复**:所有 rate 类告警表达式乘以 60(`rate(...[5m]) * 60 > N`),使阈值单位与描述一致(次/分钟)。涉及:HighFailedLoginRate、HighTransferErrorRate、FrequentConnectionTimeouts、FrequentAuthenticationErrors、RapidReconnections、MaxConnectionsReached、HighFTPErrorRate。

### BUG-040:sum 聚合丢失 instance 标签(中)

**位置**:`deploy/alerts.yml.example` HighTransferErrorRate / HighFTPErrorRate

**问题`:第四轮新增/修改的 `sum(rate(...))` 聚合了**全部**标签(包括 `instance`、`job`),导致:1) 描述模板 `{{ $labels.instance }}` 为空;2) 多实例部署时跨实例全局求和,任一实例超限会以聚合值判断,且告警丢失实例归属信息。

**修复**:改为 `sum by (job, instance) (rate(...[5m]) * 60)`,按实例保留标签后再判断阈值。

### BUG-041:仪表盘面板22 sum 风格不一致(低)

**位置**:`deploy/grafana-dashboard.json` 面板22

**问题**:面板22 的 D/E 查询使用 `sum(rate(...))` 而面板104 使用 `sum by (reason)`。虽然面板查询中已用 `job`/`instance` 选择器过滤,`sum` 只聚合 `type`/`reason` 标签,但为与告警规则风格一致、避免聚合到空标签,改为 `sum by (job, instance)`。

**修复**:面板22 D/E 查询改为 `sum by (job, instance) (rate(...[5m])) * 60`。

### BUG-042:vsftpd.log-only 模式带宽与平均速度恒为 0(中)

**位置**:`cmd/parsers.go` parseVsftpdLog

**问题**:`bandwidthUsage`(`vsftp_bandwidth_usage_bytes_per_second`)与 `averageTransferSpeed`(`vsftp_average_transfer_speed_bytes_per_second`)只在 `parseFTPLog`(xferlog 解析)中更新。当部署只启用 vsftpd.log(未配置 `Xferlog_file_path`)时,`parseFTPLog` 不运行,而 `parseVsftpdLog` 虽然累加了 `state.totalBytesUploaded/Downloaded`,却从未更新这两个 Gauge,导致:
- `vsftp_bandwidth_usage_bytes_per_second` 恒为 0(仪表盘"实时带宽"面板与 `HighBandwidthUsage` 告警失效)
- `vsftp_average_transfer_speed_bytes_per_second` 恒为 0(仪表盘"平均传输速度"面板失效)

**修复**:在 `parseVsftpdLog` 的 OK UPLOAD/OK DOWNLOAD 分支解析事件时间戳(`matches[1]`),跟踪本轮传输事件的最早/最晚时间与总字节;在函数末尾(`config.LogFilePath == ""` 时)复用与 `parseFTPLog` 相同的带宽与平均速度计算逻辑,更新两个 Gauge。双日志模式不受影响(xferlog 仍是权威来源)。

### BUG-043:监听套接字被计入连接数指标(中)

**位置**:`cmd/parsers.go` parseSSOutput

**问题**:`parseSSOutput` 通过本地/对端地址是否以 `:<FTP端口>` 结尾判断连接是否相关。但 `ss -tnH` 输出中的 **LISTEN 监听套接字**行(如 `LISTEN 0 128 0.0.0.0:6069 0.0.0.0:*`)的本地地址同样以 FTP 端口结尾,被计入 `vsftp_connections`。后果:
- 没有任何活跃连接时,`vsftp_connections` 恒 ≥ 1(仅 IPv4 监听)或 ≥ 2(IPv4+IPv6 双监听)
- `HighConnectionCount` 告警基线被抬高,空闲态连接数显示错误

**修复**:在 `parseSSOutput` 中跳过 `state == "LISTEN"` 的行。ESTAB/CLOSE-WAIT/TIME-WAIT 等真实连接状态不受影响。

### BUG-044:FTP 握手阶段超时未计数(低)

**位置**:`cmd/main.go` checkFTPLogin

**问题**:`checkFTPLogin` 有两个拨号阶段:
1. `net.Dialer.Dial`(TCP 连接)——超时已计入 `connectionTimeoutsTotal` ✓
2. `ftp.Dial`(FTP banner/握手)——超时只返回错误,**未计数** `connectionTimeoutsTotal`

场景:服务器接受 TCP 连接但不响应 FTP banner 时,握手超时不会触发 `FrequentConnectionTimeouts` 告警(该告警只统计阶段 1 的超时)。

**修复**:在 `ftp.Dial` 失败分支同样通过 `errors.As(err, &net.Error)` 检查 `Timeout()` 并计数 `connectionTimeoutsTotal`。

### BUG-045:README 桶范围描述错误(低)

**位置**:`README.md`

**问题**:指标表 `vsftp_transfer_duration_seconds` 描述为"桶: 0.1s~102.4s 指数分布",但实际代码 `prometheus.ExponentialBuckets(0.1, 2, 10)` 生成 10 个桶,最大桶为 **51.2s**(0.1×2⁹)。

**修复**:将 README 中的 "102.4s" 改为 "51.2s"。

### BUG-046:FTP response 正则未捕获用户名,summary_exclude 不完整(中)

**位置**:`cmd/parsers.go` ftpResponseRegex、FTP response 分支

**问题**:`ftpResponseRegex` 使用 `.*` 跳过 `[pid]` 与 `FTP response` 之间的 `[user]` 部分,未捕获用户名。FTP response 分支的 `summary_exclude` 仅按探测来源 IP 过滤。在 NAT/网关场景下,导出器探测连接的本地出口 IP 与 vsftpd.log 中记录的 Client IP(经 NAT 转换后)可能不一致,导致探测产生的错误响应(如 `530 Login incorrect`)无法被过滤,污染 `vsftp_ftp_errors_total` 和 `vsftp_authentication_errors_total`。

**修复**:
1. 修改 `ftpResponseRegex` 为 `\[pid\s+(\d+)\]\s+\[([^\]]*)\]\s+FTP\s+response:` 以捕获 `[user]`(分组 3)。
2. 更新分组索引:原分组 3(clientIP)→4,原分组 4(code+message)→5。
3. FTP response 过滤条件新增 `matches[3] == config.FTPUser`(用户名匹配),与 FAIL LOGIN 分支保持一致。
4. 同步更新 `TestFTPResponseRegex` 的测试用例(增加 username 字段与新断言)。
5. 新增 `TestFTPResponseSummaryExcludeByUsername` 回归测试,验证 NAT 场景下 ostore 的 530 按用户名过滤。

### BUG-047:英文版 README 与中文版不同步(低)

**位置**:`README_EN.md`

**问题**:此前 BUG-045 修复中文版 `README.md` 时,遗漏了英文版 `README_EN.md`:
1. `vsftp_transfer_duration_seconds` 桶范围仍写 "0.1s~102.4s"(应为 "0.1s~51.2s")。
2. 告警表缺失 `HighFTPErrorRate` 行(中文版已新增该告警)。

**修复**:
1. 将英文版桶范围改为 "0.1s~51.2s exponential"。
2. 在 `HighBandwidthUsage` 与 `MaxConnectionsReached` 之间补入 `HighFTPErrorRate` 行,与 `deploy/alerts.yml.example` 及中文版保持一致。

### BUG-048:环境变量路径展开是死代码(中)

**位置**:`cmd/config.go` isValidFilePath / expandLogFilePath

**问题**:`expandLogFilePath` 里的 `os.ExpandEnv` 意图支持 `$VAR`/`${VAR}` 路径(README 承诺支持环境变量),但 `isValidFilePath`(正则 `^[a-zA-Z0-9/_.\-]+$`)在任何展开**之前**先对原始路径校验,而该正则排除 `$`、`{`、`}`、`~`。因此任何含环境变量引用的路径都在 `isValidFilePath` 处被拒,`os.ExpandEnv` 对任何能通过校验的输入都不产生效果——是死代码,环境变量路径实际永远无法使用。

**修复**:将校验顺序调整为——本地模式先 `expandLogFilePath`(展开 env + 绝对化 + clean),再对**展开后的最终路径**做 `isValidFilePath` 校验与 `checkLogFileAccess`;SSH 模式保持对原始路径校验(因原始路径会拼入远端 shell 命令 `fp='...'`,须排除单引号等注入字符,且 SSH 模式下不展开环境变量)。

### BUG-049:FTP 握手与登录读取无超时,可致监控协程永久阻塞(高)

**位置**:`cmd/main.go` checkFTPLogin

**问题**:`checkFTPLogin` 同时使用 `ftp.DialWithNetConn(conn)`(提供现成 TCP 连接)与 `ftp.DialWithTimeout(10s)`。经核对 jlaffaye/ftp v0.2.0 源码:当 `DialWithNetConn` 设置了 `dialFunc` 后,`Dial` 内部 `if dialFunc == nil` 分支(其中根据 context 建立超时)被跳过,`DialWithTimeout` 只设置 `do.dialer.Timeout`,而该 dialer 在 `DialWithNetConn` 下根本不使用。结果握手阶段读取 220 横幅(`ReadResponse`)与 `Login` 读取均无任何 deadline。

**影响**:服务器接受 TCP 连接但迟迟不返回 220(或登录时挂起)时,`ftp.Dial`/`ftpConn.Login` 无限阻塞。由于 `checkFTPLogin` 在监控协程的 ticker 循环里**同步**调用,阻塞会卡死整个监控协程:`lastProbe` 不再更新、后续 check 全部停止,`/health` 与探活指标失效。

**修复**:在调用 `ftp.Dial` 前对底层 `conn` 显式 `conn.SetDeadline(time.Now().Add(10s))`,覆盖握手与登录读取;再用 `defer conn.SetDeadline(time.Time{})`(LIFO 在 `defer ftpConn.Quit()` 之后先执行)清除,确保 `Quit` 能正常发出。

### BUG-050:连接登录延迟直方图桶上界与观测范围不匹配(中)

**位置**:`cmd/metrics.go` connectionLoginDelaySeconds

**问题**:观测 guard 为 `delay>=0 && delay<=60`(parsers.go),但 `connectionLoginDelaySeconds` 桶 `ExponentialBuckets(0.001, 2, 15)` 最大桶仅 16.384s。17~60s 的合法观测值全部落入 `+Inf` 桶,导致面板 27 的 99 分位延迟严重失真。

**修复**:扩桶到 `ExponentialBuckets(0.001, 2, 17)`(最大桶 65.536s),覆盖 60s 观测上限;同步更新 README 中英文桶范围描述("1ms~16s" → "1ms~65s")。

### BUG-051:文件类型计数测试因全局指标污染在 count>1 时失败(中)

**位置**:`cmd/vsftp-exporter_test.go`

**问题**:`TestParseFTPLogFilesByTypeCounter` 与 `TestParseFTPLogFilesByTypeColdStart` 操作全局 `filesByTypeTotal` CounterVec 并依赖其从 0 起点。`-count=1`(默认)时 Go 在同一进程内重复执行测试,首次运行遗留的 CounterVec 值污染第二次运行(`committedTypeLabels` 状态 + 非零起点),`go test -count=2 -run '...FilesByType...'` 失败。CI 若做了重复执行或并行会随机翻红。

**修复**:在两个测试函数开头调用 `filesByTypeTotal.Reset()`,使每次运行从干净状态开始(已用 `go test -count=2` 验证通过)。

### BUG-052:IPv6 地址字面量拼接错误导致连接失败(中)

**位置**:`cmd/main.go` checkFTPLogin、`cmd/ssh.go` createSSHClient

**问题**:`isValidHost` 通过 `net.ParseIP` 放行裸 IPv6(如 `::1`),但所有连接处用 `host+":"+port` 拼接出 `::1:21`,`net.Dial`/`ssh.Dial` 解析报 `too many colons in address`。方括号写法 `[::1]` 又会被 `isValidHost` 判为非 IP 且域名校验失败而拒绝。结果是:IPv6 地址要么被校验拒绝,要么校验通过但运行时必然连不上(FTP 与 SSH 两条路径都中招)。

**修复**:所有 `host+":"+port` 拼接改为 `net.JoinHostPort(config.TargetHost, config.XPort)`,IPv4 与 IPv6 均正确(`[::1]:21`)。



### BUG-053:连接数指标端口后缀误匹配(中)

**位置**:`cmd/parsers.go` parseSSOutput

**问题**:用 `strings.HasSuffix(localAddr, ":"+ftpPort)` / `HasSuffix(peerAddr, portSuffix)` 判断某行是否是该 FTP 端口的连接。这是**字符串后缀匹配**而非**端口精确比较**:任何以 FTP 端口串结尾的端口都会被误判。例如目标端口 `6069` 时,随机客户端端口 `:56069`(客户端连接 MySQL):`HasSuffix("172.25.234.5:56069", ":6069")` 为 true → 误计为一条 FTP 连接;FTP 端口 `21` 时 `:210`、`:321`、`:2169` 等全部以 `:21` 结尾被误计。污染 `vsftp_connections`/`vsftp_established_connections`/`vsftp_close_wait_connections`(直接 Set)。

**修复**:改为按地址最后一个冒号之后的**完整端口 token** 与 `ftpPort` 精确比较,新增 `parseSSPort(addr)`(用 `strings.LastIndex` 提取末段端口),IPv4/IPv6(`[::1]:21`)/IPv4-mapped(`::ffff:1.2.3.4:21`)均正确处理。

### BUG-054:负 fileSize 触发 counter panic,监控协程崩溃(高)

**位置**:`cmd/parsers.go` parseStandardXferlog 与 vsftpd.log 的 bytes 解析

**问题**:xferlog 第 8 字段(fileSize)或 vsftpd.log 的传输字节为**有符号负数**(畸形/损坏日志)时,`strconv.ParseInt("-23", 10, 64)` 不返回 error,负值被 `uploadBytesTotal.Add(float64(-23))` 等消费。Prometheus client_golang v1.19 的 `counter.Add` 在 `v < 0` 时直接 `panic("counter cannot decrease in value")`;`cmd/` 无任何 `recover()`,panic 沿 `parseFTPLog → runChecks → 监控协程` 一路向上,导致**整个监控协程终止、所有指标停止更新**(已验证可复现 panic)。对比 transferTime 有 `>0` 保护、direction 有 else 兜底,唯独大小没有防护。

**修复**:在解析处对负值 clamp 为 0——`parseStandardXferlog` 的 fileSize,及 vsftpd.log upload/download 两个分支的 `bytes`;保证传入 counter 的值非负、counter 单调性不被破坏。附回归测试验证不再 panic。

### BUG-055:README 指标来源标注错误(中)

**位置**:`README.md`、`README_EN.md` 指标表

**问题**:`vsftp_login_total`、`vsftp_last_login_time` 由 `parseVsftpdLog` 的 OK LOGIN 分支产生(**需 vsftpd.log**),却放在"传输统计指标"(暗示只配 xferlog);`vsftp_client_files_total`、`vsftp_files_by_type_total` 由 `parseFTPLog`(xferlog)产生(**不需 vsftpd.log**),却被放在"需启用 vsftpd.log"的客户端统计表。只配 xferlog 时两个登录指标恒为 0,与此处文档暗示冲突,误导配置。

**修复**:将 `vsftp_login_total`/`vsftp_last_login_time` 移入"客户端和用户统计指标(需启用 vsftpd.log)"并注明来源;将 `vsftp_client_files_total`/`vsftp_files_by_type_total` 移入"传输统计指标"并注明来源为 xferlog。中英文同步。

### BUG-056:README 缺少安全警示(低)

**位置**:`README.md`、`README_EN.md`

**问题**:全篇无安全提示。`cmd/ssh.go` 用 `ssh.InsecureIgnoreHostKey()`(不对主机密钥校验,MITM 风险),且 `configs/config.json` 明文存 SSH/FTP 密码,但用户文档从未告知,bugrecord 已记为限制而不为人知。

**修复**:SSH 远程监控小节补充"安全警示":主机密钥不校验的 MITM 风险、明文密码建议 `chmod 600`、监听端口勿暴露公网。中英文同步。

### BUG-057:英文 README 告警名拼写不一致(低)

**位置**:`README_EN.md`

**问题**:告警表将 `VsftpExporterDown`(与 `deploy/alerts.yml.example`、`README.md` 一致)误写为 `VsftpdExporterDown`(多一个 d),查表时会找不到实际告警。

**修复**:统一为 `VsftpExporterDown`。


### BUG-058:监控协程无 panic 兜底,异常可将采集静默停摆(中)

**位置**:`cmd/main.go` monitor 协程

**问题**:监控协程(ticker goroutine)直接调用 `runChecks`,该调用链没有 `recover()`。BUG-054 已证明畸形 xferlog/vsftpd.log 的负值会触发 `prometheus counter.Add(v<0)` panic(已修复),但这说明"契约外部异常可能把 panic 抛上来"。一旦 `runChecks` 内出现任何未捕获 panic,监控协程 goroutine 直接终止,所有指标不再更新;而 HTTP `/metrics`、`/health` 由独立 goroutine 服务,仍返回旧值——exporter 表面"存活"但数据停滞,构成告警盲区。

**修复**:新增 `safeRunChecks` 包装函数,用 `defer recover()` 捕获 panic 并记录日志;监控协程的初始调用与 ticker 周期调用均改用 `safeRunChecks`。panic 时记录一条 error 日志、跳过本轮,下一轮自然恢复(协程不退出,周期性重新执行)。


### BUG-059:README 指标表来源标注与表格损坏(中)

**位置**:`README.md`、`README_EN.md` 指标表

**问题**:指标文档小节划分不清晰——带 `client_ip`/`file_type` 标签的指标被放进只有 3 列(名称/类型/说明)的"传输统计指标"表,与 4 列表(含"标签"列)的"客户端和用户统计指标"混用;`vsftp_client_files_total` 一度从表中丢失;`Xferlog_file_path` 行写"支持环境变量"却未注明**仅本地 `need_ssh=false` 模式才展开**(SSH 模式不展开且拒绝 `$`/引号/空白等注入字符)。导致文档列数渲染不齐、指标来源(需 vsftpd.log vs 不需)误导用户。

**修复**:
1. 将带标签指标(`vsftp_client_files_total`、`vsftp_files_by_type_total`、`vsftp_login_total`、`vsftp_last_login_time`)统一放入 4 列表,并给无标签的登录指标补 `-` 标签列,保证每列对齐。
2. 表标题改为中性的"客户端、用户与文件统计指标",不再隐含"需启用 vsftpd.log";各指标在说明中标注来源(xferlog / vsftpd.log)。
3. `Xferlog_file_path` 说明补充环境变量仅本地模式展开、SSH 模式路径受限的边界。中英文同步。



### BUG-060:查询示例与单位标注不一致(中)

**位置**:`README.md`、`README_EN.md`、`deploy/grafana-dashboard.json`

**问题**(PromQL 全量核对 + 独立验证发现):
1. **`type` 标签取值不符**:`vsftp_transfer_errors_total` 文档描述 `type` 为 "(upload/download/timeout)",但代码 (`parsers.go:399/401`) 只通过 `WithLabelValues("upload"/"download")` 递增,**`"timeout"` 永不产生** → `type="timeout"` 的查询恒空。
2. **"每分钟"口径错**:README/EN 多处 `# 每分钟传输文件数`、`# 各后缀文件每分钟传输数` 注释对应 `rate(...[5m])`,而 `rate` 返回的是**每秒**速率,未乘 `* 60`,注释与数值口径不符(仅 `ts upload rate` 一处正确乘了 60)。
3. **MB/s vs MiB/s**:带宽换算 `/1024/1024`(二进制 MiB)却标注/渲染为 MB/s(dashboard 面板14 `unit=MBs`、`axisLabel=MB/s`、标题 "(MB/s)";README 注释 "(MB/s)"),显示值偏差约 4.6%。

**修复**:
1. 两 README 的 `type` 改为 "upload/download"。
2. 两 README 的"每分钟"查询补 `* 60`,使其确实为次/分钟。
3. dashboard 面板14 `unit` 改 `decmibs`、`axisLabel` 改 `MiB/s`、标题改 "(MiB/s)",两 README 注释同步为 "(MiB/s)",与 `/1024/1024` 的实际 MiB 语义一致(并与此前带宽告警统一的 "100 MiB/s" 口径一致)。


### BUG-061:Makefile .PHONY 声明不全(低)

**位置**:`Makefile`

**问题**:`.PHONY: build run test clean fmt vet install help` 只包含 8 个目标,
`all`、`coverage`、`tidy`、`build-all`、`build-linux`、`build-windows`、`build-darwin` 均未在 `.PHONY` 中声明。
GNU make 默认将未声明为 `.PHONY` 的目标视为文件名目标:若存在同名文件则跳过 recipe。
当前不存在同名文件,实际无害,但重构或创建同名文件时可能产生意外跳过。

**修复**:在 `.PHONY` 行补全所有 target。


### BUG-062:基于 vsftpd_conf.html 新增连接限制/超时细分监控(A1-A4)(中)

**背景**:对照 vsftpd 官方配置文档(vsftpd_conf.html),原有 TS(错误与异常)面板把 idle 超时、数据连接超时、
max_clients / max_per_ip 连接上限、PASV 端口失败等事件一律并入 `ftp_errors_total{reason=...}`,
无法单独观测 4xx/5xx 响应所对应的 vsftpd 配置项运行时事件。本次按文档新增四类细分指标:

| 新指标 | 对应配置项 | 匹配依据(FTP response) |
|--------|-----------|------------------------|
| `vsftp_idle_timeout_total` | `idle_session_timeout` | `421` + 消息含 `timeout` |
| `vsftp_data_connection_timeout_total` | `data_connection_timeout` | `426` + 消息含 `failure writing network stream` / `transfer aborted` |
| `vsftp_connection_limit_rejections_total{reason=max_clients}` | `max_clients` | 消息含 `maximum number of clients` / `too many clients` |
| `vsftp_connection_limit_rejections_total{reason=max_per_ip}` | `max_per_ip` | 消息含 `from your internet address` / `from your ip` |
| `vsftp_pasv_port_rejections_total` | `pasv_min/max_port` | `425` + 消息含 `establish connection` / `data connection` |

**实现**:在 `parsers.go` 新增 `classifyFTPNotice(code, message) []string`,于 FTP response 处理段并行计数,
**不改变**原有 `classifyFTPError` 的 `ftpErrorsTotal` 归类,保持向后兼容。新指标在 `metrics.go` 注册。

**配套**:`deploy/alerts.yml.example` 新增 5 条告警(HighIdleTimeoutRate / HighDataConnTimeoutRate /
MaxClientsReached / MaxPerIpReached / HighPasvPortFailures);`deploy/grafana-dashboard.json` 新增
"🔒 连接限制与超时" 区块(5 个 stat + 1 个 rate timeseries,in waiting 已有面板位移);README 中英文更新指标与告警表。

**验证**:新增 `TestClassifyFTPNotice`(12 案例)与 `TestParseVsftpdLogA1A4Counters`(端到端 5 计数)。
`go build -o /dev/null ./cmd`、`go vet ./...`、`go test -race -count=1 ./cmd`、JSON/YAML 校验全部通过。

**说明**:A5(ASCII 模式传输计数,源自 `transfer_type` 字段)在用户权衡后未实现——xferlog 已有上传/下载计数;
ASCII 仅表示文本/二进制模式差异,监控 ROI 低,详见对话记录。



### BUG-063:测试文件多余空行(gofmt 报格式不符)(低)

**位置**:`cmd/vsftp-exporter_test.go` 文件末尾追加的 A1-A4 测试块前。

**问题**:在文件末尾追加测试时,`TestClassifyFTPNotice` 前多余了一个空行(多个顶层函数之间
按 gofmt 规范应只有一个空行),`gofmt -l` 将该文件标记为格式不符。不影响编译与测试,但违反 gofmt。

**修复**:`gofmt -w cmd/vsftp-exporter_test.go` 移除多余空行。

## 已知特性说明

| 编号 | 说明 |
|------|------|
| KNOWN-002 | `vsftp_bandwidth_usage_bytes_per_second` 语义有限：单事件轮次（时间跨度=0）不更新；事件时间跨度大（日志空档）时会算出一个偏小的"平均带宽"。建议以 PromQL `rate(vsftp_upload_bytes_total + vsftp_download_bytes_total[5m])` 为主。 |
| KNOWN-003 | `vsftp_average_transfer_speed_bytes_per_second` 的除数是程序总运行时长（`state.lastProcessedTime` 仅在启动时初始化、不再更新），即"总字节/总运行时长"；字段名 `lastProcessedTime` 有误导性。 |
| KNOWN-004 | 探测污染过滤依赖 `summary_exclude` 与 `probeClientIP`:`OK LOGIN`/`FAIL LOGIN`/FTP response 可同时按探测用户名 `FTPUser` 或 IP 过滤(`parsers.go:606/639/727`);但 `CONNECT` 事件只含 IP、只能按 `probeClientIP` 过滤,且 `probeClientIP` 仅在探测成功时由 `conn.LocalAddr()` 设置——若探测自启动起持续失败(`probeClientIP==""`)或经 NAT 后源 IP 与日志 Client IP 不一致,`CONNECT`/`clientConnectionsTotal`/`rapidReconnections` 会漏。建议用专用探测账号并设置 `summary_exclude=true`(OK/FAIL/FTP-response 已默认覆盖)。 |
| KNOWN-005 | `state.probeClientIP` 仅在探测成功时更新;探测失败(如 FTP 端口不可达)时保留上一次成功值。因 exporter 自身 IP 一般稳定,影响有限;若 exporter 网络出口 IP 变化,`summary_exclude` 可能无法过滤新的探测来源 IP。 |
| KNOWN-006 | CONNECT 事件无用户名信息,仅含来源 IP。NAT/网关场景下,探测连接的来源 IP 经转换后与 `state.probeClientIP`(探测本地出口 IP)不一致,`summary_exclude` 无法按 IP 过滤 CONNECT 事件,导致 `vsftp_client_connections_total` 被探测污染(每次探测 +1)。这是日志信息不足导致的固有限制,非代码缺陷;需部署侧排除 exporter 自身 IP 才能完全消除。 |
| KNOWN-007 | `vsftp_user_connections_total` 与 `vsftp_user_logins_total` 在 OK LOGIN 分支同步递增(BUG-006 修复后遗留),两者永远相等,无独立统计意义。仪表盘仅使用 `vsftp_user_logins_total`。保留该指标仅为向后兼容。 |
| KNOWN-008 | `/health` 在首次探测完成前(`lastProbe.checkTime` 为零值)返回 HTTP 200 healthy。BUG-049 修复后首次探测最迟 10s 内完成,故该"未探测即健康"窗口通常短暂可接受;但在监控协程因其他原因长时间未运行探测时,`/health` 可能误报健康。 |
| KNOWN-009 | 日志尾部存在"永久半行"(永不出现换行、如进程崩溃/外部截断残留)时,`readLocalFile`/`readRemoteFile` 每次读到该半行都会 break 且不推进 position,若其后有新行追加则永远读不到,静默丢失后续事件。正常条件下的尾部半行随后会补全(BUG-009/028 已处理),永久半行属异常残留;修复需跨轮次状态机(连续 N 轮不补全即强制消费),当前作为已知限制记录。 |
| KNOWN-010 | `cmd/ssh.go` `GetClient` 在持有 `m.mu` 锁期间执行 `m.client.NewSession()` 做存活探测,该网络 I/O 无 deadline;若 SSH 连接"半死"(TCP 收包但不响应),`NewSession` 无限阻塞并持锁不放,导致关停时 `Close` 等锁挂死。正常监控(单协程、SSH 响应正常)不受影响;修复需重构锁模型或给会话探测加超时,作为已知限制。 |
| KNOWN-011 | 高基数标签无界增长:`vsftp_client_connections_total`/`vsftp_client_files_total`(标签 `client_ip`/`client_ip,direction`)、`vsftp_user_logins_total`/`vsftp_user_connections_total`(标签 `username`,取自 vsftpd 日志未校验)、`vsftp_files_by_type_total`(标签 `file_type`,见 KNOWN-012)。这些 CounterVec 系列一经创建永不删除,基数随"历史见过"的客户端 IP/用户名/后缀单调增长(每 IP 最多 3 条系列;1 万 IP→3 万系列,10 万 IP→30 万系列,Prometheus 内存数百 MB + 每次抓取文本数十 MB)。公网部署长期运行(含扫描/僵尸网络)风险显著;单主机/内网部署基数小。要根治需改为"活跃集 Gauge + 无活动删除"或前缀聚合,涉及指标语义变更;当前保留 counter 语义,部署侧可用 `metric_relabel_configs` drop 或 recording rule 缓解(注意不降低摄入)。 |
| KNOWN-012 | `vsftp_files_by_type_total` 的 `file_type` 标签取文件最后一段后缀,常规几十~几百种;但攻击者上传随机后缀(如 `x.a1b2c3`)可制造新系列,理论上也是无界标签。建议上限合并到 `other` 或白名单,当前作为已知限制。 |
| KNOWN-013 | 本地模式环境变量展开(BUG-048)基于 `os.ExpandEnv` 单次展开:`$VAR` 贪婪匹配,`$HOMExferlog` 会被当整体未定义变量而变空;字面 `$$` 会被吞成空。已建议用户用 `${VAR}` 分隔形式,并在 README 的 `Xferlog_file_path` 说明中写明。 |
| KNOWN-014 | `deploy/prometheus.yml.example` 的 vsftp-exporter job target、README 配置示例、`grafana-dashboard.json` 的 instance 变量默认值若不一致(job 用 `vsftp-exporter:9101`、其余用 `localhost:9101`),首次打开 dashboard 时 instance 默认选中会无数据片刻。已统一为 `localhost:9101`。 |
| KNOWN-015 | `deploy/vsftpd-exporter.service` 的 `ExecStart` 原先指向自包含的 `/usr/local/vsftp-exporter/...` 布局,与 `make install`(装到 `/usr/local/bin/`)及 README systemd 示例(`/usr/local/bin/vsftp-exporter -config=/etc/vsftp-exporter/config.json`)不一致。已统一为与 README/Makefile 一致的布局。 |
| KNOWN-016 | `vsftp_files_by_type_total` 冷启动标签的"0 暴露→提交增量"时序在**并发/慢抓取**下存在理论窗口:`pendingTypeSeq[key] < scrapeSeq` 的提交由"哪个 /metrics 抓取先完成"驱动,而非"哪个抓取实际看到了该标签的 0 采样"(`parsers.go:312`);若某快照早于标签注册、却先完成 bump,则该标签首个增量可能在真实 0 采样被任何抓取输出前就提交,`increase()` 低记一次(经典 counter hole)。单写者 check goroutine + 正常单个 Prometheus 串行抓取下极难触发(μs 级窗口);非崩溃、counter 仍单调。属设计权衡,要彻底消除需为每个 pending 标签记录"是否已被某次抓取输出过"而非仅比较 seq。 |
| KNOWN-017 | `vsftp_pasv_port_rejections_total`(A4)基于 `425` FTP response 消息匹配,而 `425` 无 "pasv 端口" 专属字样,无法在日志层面与普通 425 数据连接建立失败(网络/防火墙)完全区分;统计的是"PASV/数据连接建立失败"总数,端口范围耗尽是主因之一但非唯一。当作数据连接建立失败的监控信号,而非精确的端口耗尽计数。 |
| KNOWN-018 | `vsftp_idle_timeout_total`(A1)匹配 `421` + 消息含 `timeout`;vsftpd 的 `idle_session_timeout` 触发时通常报 `421 Timeout.`,消息含 "timeout" 判据可靠。极少数自定义 banner 或非标准响应若也在 421 里含 "timeout" 字样会被计入,实际冲突概率极低。 |
