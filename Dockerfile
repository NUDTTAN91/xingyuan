# 星垣 - 监控端
# Author: tan91
# GitHub: https://github.com/NUDTTAN91
# Project: https://github.com/NUDTTAN91/xingyuan

# 多阶段构建 - 构建阶段
FROM golang:1.21-alpine AS builder

# 设置工作目录
WORKDIR /build

# 使用国内镜像源加速
RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories && \
    apk add --no-cache git gcc musl-dev

# 设置 Go 国内代理
ENV GO111MODULE=on \
    GOPROXY=https://goproxy.cn,direct

# 复制依赖文件
COPY go.mod ./

# 复制源代码
COPY . .

# 下载依赖并生成go.sum
RUN go mod download && go mod tidy

# 编译二进制文件（启用CGO以支持SQLite）
# 设置C编译器标志以兼容Alpine Linux
ENV CGO_CFLAGS="-D_LARGEFILE64_SOURCE"
RUN CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build \
    -tags "sqlite_omit_load_extension" \
    -ldflags="-w -s" \
    -o xingyuan-monitor \
    .

# 运行阶段 - 使用最小化镜像
FROM alpine:latest

# 设置作者信息
LABEL Author="tan91"
LABEL GitHub="https://github.com/NUDTTAN91"
LABEL Blog="https://blog.csdn.net/ZXW_NUDT"
LABEL description="Xingyuan Monitor - Linux Server Monitoring Agent"
LABEL version="1.0.0"
LABEL note="This container requires host system access via bind mounts"

# 使用国内镜像源加速
RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories && \
    apk --no-cache add ca-certificates tzdata docker-cli

# 设置时区为上海
ENV TZ=Asia/Shanghai

# 设置默认监听端口
ENV SERVER_PORT=80

# 创建工作目录
WORKDIR /app

# 从构建阶段复制编译好的二进制文件
COPY --from=builder /build/xingyuan-monitor .

# 复制静态文件
COPY --from=builder /build/static ./static

# 创建非root用户（但暂时以root运行以访问docker.sock）
RUN addgroup -g 1000 xingyuan && \
    adduser -D -u 1000 -G xingyuan xingyuan && \
    chown -R xingyuan:xingyuan /app

# 注意：为了访问Docker Socket，需要以root用户运行
# USER xingyuan

# 暴露端口（实际端口由SERVER_PORT环境变量控制，默认80）
EXPOSE ${SERVER_PORT:-80}

# 健康检查（使用环境变量SERVER_PORT指定的端口）
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:${SERVER_PORT:-80}/api/metrics || exit 1

# 启动命令（不再指定端口，由环境变量控制）
ENTRYPOINT ["./xingyuan-monitor"]
