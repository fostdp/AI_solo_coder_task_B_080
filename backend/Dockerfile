# 多阶段构建 - 后端服务
# 构建阶段：使用 golang:1.21-alpine 编译 Go 代码
FROM golang:1.21-alpine AS builder

# 设置 Go 环境变量
# GO111MODULE=on 启用 Go Modules
# CGO_ENABLED=0 禁用 CGO，生成静态二进制文件
# GOOS=linux GOARCH=amd64 指定目标平台为 Linux amd64
ENV GO111MODULE=on \
    CGO_ENABLED=0 \
    GOOS=linux \
    GOARCH=amd64

# 设置工作目录
WORKDIR /app

# 安装 ca-certificates（用于 HTTPS 证书验证）和 tzdata（用于时区设置）
RUN apk add --no-cache ca-certificates tzdata

# 复制 go.mod 和 go.sum，下载依赖（利用 Docker 缓存）
COPY go.mod go.sum ./
RUN go mod download

# 复制所有源代码
COPY . .

# 编译 Go 应用，输出到 /app/aqueduct-monitor
RUN go build -o /app/aqueduct-monitor .

# 运行阶段：使用 alpine:3.19 作为基础镜像，减小镜像体积
FROM alpine:3.19 AS runtime

# 安装运行时依赖：ca-certificates 和 tzdata
RUN apk add --no-cache ca-certificates tzdata

# 设置时区为 Asia/Shanghai
ENV TZ=Asia/Shanghai

# 从 builder 阶段复制编译好的二进制文件到 /usr/local/bin/
COPY --from=builder /app/aqueduct-monitor /usr/local/bin/

# 复制 .env 配置文件到 /app/.env
COPY .env /app/.env

# 设置工作目录为 /app
WORKDIR /app

# 暴露 8080 端口
EXPOSE 8080

# 设置入口点
ENTRYPOINT ["/usr/local/bin/aqueduct-monitor"]
