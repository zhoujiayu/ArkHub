# ============================================================
# ArkHub Go 服务 Dockerfile
# 采用多阶段构建（Multi-stage Build），减小最终镜像体积
# 阶段1（builder）：编译 Go 二进制文件
# 阶段2（runtime）： 仅包含运行所需的二进制文件和配置文件
# ============================================================

# ============================================================
# 阶段1：构建阶段（Builder）
# 使用 Go 官方镜像，包含完整的编译工具链
# ============================================================
FROM golang:1.22-alpine AS builder

# 设置构建环境变量
ENV GO111MODULE=on    # 启用 Go Modules
ENV CGO_ENABLED=0   # 禁用 CGO，生成静态链接的可执行文件（Alpine 无 glibc）
ENV GOOS=linux        # 目标操作系统
ENV GOARCH=amd64      # 目标架构

# 安装编译依赖（Git、Make 等）
RUN apk add --no-cache git make

# 设置工作目录
WORKDIR /build

# 先复制模块定义文件，利用 Docker 缓存机制
# 如果 go.mod/go.sum 没变，不需要重新下载依赖
COPY go.mod go.sum ./
RUN go mod download

# 复制项目源代码
COPY . .

# 构建参数：指定要编译的服务名（如 api-gateway、matching-engine）
ARG SERVICE_NAME=api-gateway
ARG SERVICE_PORT=8080

# 编译 Go 服务
# -ldflags="-s -w"：去除符号表和调试信息，减小二进制体积
RUN go build -ldflags="-s -w" -o bin/${SERVICE_NAME} ./cmd/${SERVICE_NAME}

# ============================================================
# 阶段2：运行阶段（Runtime）
# 使用最小化的 Alpine 镜像，仅包含运行所需的依赖
# ============================================================
FROM alpine:latest

# 安装 ca-certificates（HTTPS 请求需要）和 tzdata（时区支持）
RUN apk --no-cache add ca-certificates tzdata

# 设置工作目录
WORKDIR /app

# 构建参数：服务名和端口
ARG SERVICE_NAME=api-gateway
ARG SERVICE_PORT=8080

# 从构建阶段复制编译好的二进制文件
COPY --from=builder /build/bin/${SERVICE_NAME} /app/server

# 暴露服务端口
EXPOSE ${SERVICE_PORT}

# 设置容器启动时执行的命令
ENTRYPOINT ["/app/server"]
