# 构建阶段
FROM golang:1.24-alpine AS builder

# 安装必要的构建工具
RUN apk add --no-cache git make

# 设置工作目录
WORKDIR /build

# 复制 go mod 文件
COPY go.mod go.sum ./

# 下载依赖
RUN go mod download

# 复制源代码
COPY cmd/ ./cmd/

# 构建应用
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -ldflags '-extldflags "-static"' -o vsftp-exporter ./cmd

# 运行阶段
FROM alpine:latest

# 安装必要的运行时工具（用于 netstat 命令）
RUN apk --no-cache add ca-certificates iproute2 openssh-client

# 创建非 root 用户
RUN addgroup -g 1000 exporter && \
    adduser -D -u 1000 -G exporter exporter

# 设置工作目录
WORKDIR /app

# 从构建阶段复制二进制文件
COPY --from=builder /build/vsftp-exporter .

# 复制配置文件模板
COPY configs/config.example.json ./config.example.json

# 修改文件所有者
RUN chown -R exporter:exporter /app

# 切换到非 root 用户
USER exporter

# 暴露端口
EXPOSE 9101

# 健康检查
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:9101/health || exit 1

# 启动命令
ENTRYPOINT ["./vsftp-exporter"]
CMD ["-config=/app/config.json"]
