# 项目结构说明

## �� 目录结构

```
Vsftpd-exporter/
├── 📄 核心代码
│   ├── vsftp-exporter.go          # 主程序（1668 行）
│   └── vsftp-exporter_test.go     # 单元测试（275 行）
│
├── ⚙️ 配置文件
│   ├── config.json                 # 实际配置（不提交到 Git）
│   ├── config.example.json         # 配置模板
│   ├── prometheus.yml              # Prometheus 配置示例
│   └── alerts.yml                  # 告警规则（12 条规则）
│
├── 🐳 容器化
│   ├── Dockerfile                  # Docker 镜像构建
│   └── docker-compose.yml          # 完整监控栈部署
│
├── 📊 监控可视化
│   ├── grafana-dashboard.json      # Grafana 仪表板（14 个面板）
│   └── GRAFANA_DASHBOARD.md        # 仪表板使用指南
│
├── 📚 文档
│   ├── README.md                   # 项目总览和使用说明
│   ├── QUICKSTART.md               # 5 分钟快速开始
│   ├── DEPLOYMENT.md               # 部署指南
│   ├── AGENTS.md                   # 开发规范
│   ├── CHANGELOG.md                # 更新日志
│   ├── OPTIMIZATION_SUMMARY.md     # 优化总结报告
│   └── PROJECT_STRUCTURE.md        # 本文件
│
├── 🔧 构建和工具
│   ├── Makefile                    # 构建脚本
│   ├── go.mod                      # Go 模块定义
│   ├── go.sum                      # 依赖校验和
│   ├── validate_dashboard.py       # 仪表板验证工具
│   └── final_check.sh              # 最终验证脚本
│
├── 🤖 CI/CD
│   └── .github/workflows/
│       ├── ci.yml                  # 持续集成
│       └── release.yml             # 自动发布
│
├── 📝 其他
│   ├── .gitignore                  # Git 忽略规则
│   ├── LICENSE                     # MIT 许可证
│   └── log/                        # 日志样本目录
│
└── 🔨 构建产物（不提交）
    └── vsftp-exporter              # 编译后的二进制文件
```

## 📄 文件说明

### 核心代码

#### vsftp-exporter.go
主程序文件，包含：
- FTP 连接监控
- 日志解析（xferlog 和 vsftpd.log）
- SSH 远程支持
- Prometheus 指标导出
- 40+ 监控指标

#### vsftp-exporter_test.go
单元测试文件，包含：
- 8 个测试函数
- 38 个测试用例
- 3 个性能基准测试
- 竞态检测支持

### 配置文件

#### config.json
实际使用的配置文件（包含敏感信息，不提交到 Git）

#### config.example.json
配置文件模板，包含：
- 所有配置项说明
- 默认值示例
- 注释说明

#### prometheus.yml
Prometheus 配置示例，包含：
- 抓取配置
- 目标定义
- 标签设置

#### alerts.yml
告警规则配置，包含 12 条规则：
- 服务状态告警
- 性能告警
- 错误告警
- 安全告警

### 容器化

#### Dockerfile
多阶段构建配置：
- 构建阶段：编译 Go 程序
- 运行阶段：Alpine Linux + 必要工具
- 非 root 用户运行
- 健康检查配置

#### docker-compose.yml
完整监控栈，包含：
- vsftp-exporter
- Prometheus
- Grafana
- 网络和卷配置

### 监控可视化

#### grafana-dashboard.json
完整的 Grafana 仪表板：
- 14 个监控面板
- 2 个面板分组
- 变量支持（Job、Instance）
- 自动刷新（30 秒）

#### GRAFANA_DASHBOARD.md
仪表板使用指南：
- 导入方法
- 面板说明
- 查询示例
- 自定义建议

### 文档

#### README.md
项目主文档，包含：
- 项目介绍
- 安装说明
- 配置指南
- 使用方法
- 故障排查

#### QUICKSTART.md
快速开始指南：
- 5 分钟部署
- 3 种部署方式
- 常见问题
- 验证步骤

#### DEPLOYMENT.md
详细部署指南：
- 多种部署方式
- 生产环境配置
- Systemd 服务
- 监控验证

#### AGENTS.md
开发规范：
- 代码风格
- 提交规范
- 测试要求
- 安全建议

#### CHANGELOG.md
更新日志：
- 版本历史
- 新增功能
- 优化改进
- 待办事项

#### OPTIMIZATION_SUMMARY.md
优化总结报告：
- 优化内容
- 性能对比
- 统计数据
- 后续计划

### 构建和工具

#### Makefile
构建脚本，支持命令：
- `make build` - 构建
- `make test` - 测试
- `make coverage` - 覆盖率
- `make fmt` - 格式化
- `make vet` - 静态检查
- `make clean` - 清理
- `make build-all` - 交叉编译

#### validate_dashboard.py
仪表板验证工具：
- JSON 格式验证
- 结构完整性检查
- 统计信息输出

#### final_check.sh
最终验证脚本：
- 文件完整性检查
- 代码质量检查
- 测试执行
- 构建验证

### CI/CD

#### .github/workflows/ci.yml
持续集成配置：
- 代码格式检查
- 静态分析
- 单元测试
- 覆盖率报告

#### .github/workflows/release.yml
自动发布配置：
- 交叉编译
- 创建 Release
- 上传构建产物

## 📊 统计信息

### 代码量
- 主程序: 1,668 行
- 测试代码: 275 行
- 总代码: 1,943 行

### 文件数量
- 总文件: 25 个
- Go 源文件: 2 个
- 配置文件: 8 个
- 文档文件: 7 个
- 脚本文件: 4 个

### 监控指标
- Prometheus 指标: 40+ 个
- Grafana 面板: 14 个
- 告警规则: 12 条

### 测试覆盖
- 测试函数: 8 个
- 测试用例: 38 个
- 基准测试: 3 个

## 🔄 开发流程

### 1. 克隆项目
```bash
git clone <repository-url>
cd Vsftpd-exporter
```

### 2. 安装依赖
```bash
go mod download
```

### 3. 开发
```bash
# 编辑代码
vim vsftp-exporter.go

# 格式化
make fmt

# 静态检查
make vet

# 运行测试
make test
```

### 4. 构建
```bash
make build
```

### 5. 运行
```bash
./vsftp-exporter -config=./config.json
```

### 6. 提交
```bash
git add .
git commit -m "feat: 添加新功能"
git push
```

## 📦 依赖包

### 直接依赖
- `github.com/jlaffaye/ftp` v0.2.0 - FTP 客户端
- `github.com/prometheus/client_golang` v1.19.1 - Prometheus 客户端
- `golang.org/x/crypto` v0.43.0 - SSH 支持

### 间接依赖
- `github.com/beorn7/perks` v1.0.1
- `github.com/cespare/xxhash/v2` v2.2.0
- `github.com/prometheus/client_model` v0.5.0
- `github.com/prometheus/common` v0.48.0
- `github.com/prometheus/procfs` v0.12.0
- `google.golang.org/protobuf` v1.33.0

## 🎯 关键特性

### 监控能力
- ✅ FTP 连接状态监控
- ✅ 文件传输统计
- ✅ 用户活动跟踪
- ✅ 客户端连接分析
- ✅ 错误和异常监控
- ✅ 性能指标收集

### 技术特性
- ✅ 并发安全
- ✅ 增量日志读取
- ✅ SSH 远程支持
- ✅ 日志轮转检测
- ✅ 优雅关闭
- ✅ 健康检查

### 部署特性
- ✅ Docker 容器化
- ✅ Docker Compose 一键部署
- ✅ Systemd 服务支持
- ✅ 跨平台编译
- ✅ CI/CD 自动化

## 🔗 相关链接

- [项目主页](README.md)
- [快速开始](QUICKSTART.md)
- [部署指南](DEPLOYMENT.md)
- [开发规范](AGENTS.md)
- [更新日志](CHANGELOG.md)
- [优化报告](OPTIMIZATION_SUMMARY.md)
- [Grafana 指南](GRAFANA_DASHBOARD.md)

---

**最后更新**: 2025-11-17  
**项目版本**: v1.0.0
