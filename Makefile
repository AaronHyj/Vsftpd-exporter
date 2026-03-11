.PHONY: build run test clean fmt vet install help

# 变量定义
BINARY_NAME=vsftp-exporter
GO=go
GOFLAGS=-v
VERSION?=1.0.0
BUILD_TIME=$(shell date -u '+%Y-%m-%d_%H:%M:%S')
LDFLAGS=-ldflags "-X main.appVersion=$(VERSION) -X main.buildTime=$(BUILD_TIME)"

# 默认目标
all: fmt vet build

# 构建二进制文件
build:
	@echo "正在构建 $(BINARY_NAME)..."
	$(GO) build $(GOFLAGS) $(LDFLAGS) -o $(BINARY_NAME) ./cmd
	@echo "构建完成: $(BINARY_NAME)"

# 运行程序
run: build
	@echo "启动 $(BINARY_NAME)..."
	./$(BINARY_NAME) -config=./configs/config.json

# 运行测试
test:
	@echo "运行测试..."
	$(GO) test -v -race -coverprofile=coverage.txt -covermode=atomic ./...
	@echo "测试完成"

# 查看测试覆盖率
coverage: test
	@echo "生成覆盖率报告..."
	$(GO) tool cover -html=coverage.txt -o coverage.html
	@echo "覆盖率报告已生成: coverage.html"

# 格式化代码
fmt:
	@echo "格式化代码..."
	$(GO) fmt ./...

# 代码检查
vet:
	@echo "运行代码检查..."
	$(GO) vet ./...

# 整理依赖
tidy:
	@echo "整理依赖..."
	$(GO) mod tidy

# 安装到系统
install: build
	@echo "安装 $(BINARY_NAME) 到 /usr/local/bin/..."
	sudo cp $(BINARY_NAME) /usr/local/bin/
	@echo "安装完成"

# 清理构建文件
clean:
	@echo "清理构建文件..."
	rm -f $(BINARY_NAME)
	rm -f coverage.txt coverage.html
	@echo "清理完成"

# 交叉编译
build-linux:
	@echo "构建 Linux 版本..."
	GOOS=linux GOARCH=amd64 $(GO) build $(LDFLAGS) -o $(BINARY_NAME)-linux-amd64 ./cmd

build-windows:
	@echo "构建 Windows 版本..."
	GOOS=windows GOARCH=amd64 $(GO) build $(LDFLAGS) -o $(BINARY_NAME)-windows-amd64.exe ./cmd

build-darwin:
	@echo "构建 macOS 版本..."
	GOOS=darwin GOARCH=amd64 $(GO) build $(LDFLAGS) -o $(BINARY_NAME)-darwin-amd64 ./cmd

build-all: build-linux build-windows build-darwin
	@echo "所有平台构建完成"

# 帮助信息
help:
	@echo "可用的 make 目标:"
	@echo "  make build        - 构建二进制文件"
	@echo "  make run          - 构建并运行程序"
	@echo "  make test         - 运行测试"
	@echo "  make coverage     - 生成测试覆盖率报告"
	@echo "  make fmt          - 格式化代码"
	@echo "  make vet          - 运行代码检查"
	@echo "  make tidy         - 整理依赖"
	@echo "  make install      - 安装到系统"
	@echo "  make clean        - 清理构建文件"
	@echo "  make build-all    - 交叉编译所有平台"
	@echo "  make help         - 显示此帮助信息"
