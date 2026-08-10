# Bug 记录

本文件记录代码审查发现的问题、修复状态与相关说明。每条记录包含问题描述、影响、修复方案与验证方式。

## 审查信息

- 审查日期：2026-08-07（第一轮）/ 2026-08-07（第二轮）/ 2026-08-09（第三轮深度审查与修复）
- 审查范围：`cmd/` 全部源码（main.go / config.go / metrics.go / ssh.go / parsers.go / tests）及 Makefile
- 审查重点：登录探测与采样污染、SSH读取性能与轮转、文件名包含空格解析、未换行半行保护、健康检查与死代码死字段清理
- 验证命令：`go build -o /dev/null ./cmd`、`go vet ./...`、`go test -v -race ./...`

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
3. `max_connections` 分类覆盖原 `530 ... maximum number of clients` 及 `421 ... too many connections` 变体。

## 已知特性说明

| 编号 | 说明 |
|------|------|
| KNOWN-002 | `vsftp_bandwidth_usage_bytes_per_second` 语义有限：单事件轮次（时间跨度=0）不更新；事件时间跨度大（日志空档）时会算出一个偏小的"平均带宽"。建议以 PromQL `rate(vsftp_upload_bytes_total + vsftp_download_bytes_total[5m])` 为主。 |
| KNOWN-003 | `vsftp_average_transfer_speed_bytes_per_second` 的除数是程序总运行时长（`state.lastProcessedTime` 仅在启动时初始化、不再更新），即"总字节/总运行时长"；字段名 `lastProcessedTime` 有误导性。 |
| KNOWN-004 | 登录探测的成功事件仍会计入日志派生态指标（`vsftp_user_logins_total`、`vsftp_ftp_login_total`、`vsftp_user_connections_total`、`vsftp_client_connections_total` 等）。需使用专用探测账号或在部署侧排除 exporter 自身 IP 才能完全消除（承接 BUG-001 遗留建议）。 |
