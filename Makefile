# ============================================================
# ArkHub Makefile
# 职责：封装常用命令，简化开发、构建、部署流程
# 支持两种部署方式：
#   1. 本地开发：go run / go build（不依赖 Docker）
#   2. Docker 部署：docker-compose up --build（推荐）
# ============================================================

.PHONY: build test clean setup start stop status setup-db docker-build docker-up docker-down docker-logs help

# 默认目标
.DEFAULT_GOAL := help

# 变量定义
PROJECT_NAME := arhub
DOCKER_COMPOSE := docker-compose -f deployments/docker-compose.yml

# ============================================================
# 帮助信息（默认显示）
# ============================================================
help:
	@echo "============================================================"
	@echo "  ArkHub 构建工具"
	@echo "============================================================"
	@echo ""
	@echo "【本地开发命令】"
	@echo "  make setup       - 启动所有中间件（Docker Compose，不含Go服务）"
	@echo "  make start       - 启动 Docker Compose 服务（含Go服务）"
	@echo "  make stop        - 停止 Docker Compose 服务"
	@echo "  make status      - 查看所有容器运行状态"
	@echo "  make build       - 本地编译所有 Go 服务（不依赖 Docker）"
	@echo "  make test        - 运行所有测试"
	@echo "  make setup-db    - 初始化数据库"
	@echo "  make clean       - 清理容器和构建产物"
	@echo ""
	@echo "【Docker 部署命令】"
	@echo "  make docker-build  - 构建所有 Go 服务的 Docker 镜像"
	@echo "  make docker-up     - 启动所有服务（中间件 + Go 服务）"
	@echo "  make docker-down   - 停止并删除所有容器"
	@echo "  make docker-logs   - 查看所有服务日志"
	@echo "============================================================"

# ============================================================
# 本地开发命令
# ============================================================

# 启动中间件（不含Go服务，适合本地开发）
setup:
	@echo "🚀 启动中间件服务..."
	$(DOCKER_COMPOSE) up -d postgres redis rocketmq nacos prometheus grafana
	@echo "✅ 中间件启动完成"
	@echo ""
	@echo "下一步：本地编译 Go 服务"
	@echo "  make build"

# 启动所有服务（中间件 + Go 服务，需要先用 docker-build 构建镜像）
start:
	@echo "🚀 启动所有服务（中间件 + Go 服务）..."
	$(DOCKER_COMPOSE) up -d
	@echo "✅ 所有服务启动完成"

# 停止所有服务
stop:
	@echo "🛑 停止所有服务..."
	$(DOCKER_COMPOSE) down
	@echo "✅ 服务已停止"

# 查看服务状态
status:
	@echo "📊 服务状态:"
	$(DOCKER_COMPOSE) ps

# 本地编译所有 Go 服务
build:
	@echo "🔨 本地编译 Go 服务..."
	go mod tidy
	go build -o bin/api-gateway ./cmd/api-gateway
	go build -o bin/matching-engine ./cmd/matching-engine
	go build -o bin/market-data ./cmd/market-data
	go build -o bin/chain-sync ./cmd/chain-sync
	go build -o bin/nft-service ./cmd/nft-service
	go build -o bin/buyback-service ./cmd/buyback-service
	go build -o bin/risk-service ./cmd/risk-service
	@echo "✅ 本地编译完成"

# 运行测试
test:
	@echo "🧪 运行测试..."
	go test ./... -v
	@echo "✅ 测试完成"

# 初始化数据库
setup-db:
	@echo "🗄️  初始化数据库..."
	psql -h localhost -U arhub -d arhub -f migrations/001_init_schema.sql
	@echo "✅ 数据库初始化完成"

# 清理容器和构建产物
clean:
	@echo "🧹 清理..."
	$(DOCKER_COMPOSE) down -v
	rm -rf bin/
	@echo "✅ 清理完成"

# ============================================================
# Docker 部署命令
# ============================================================

# 构建所有 Go 服务的 Docker 镜像
# 说明：使用 docker-compose build，会自动根据 Dockerfile 构建每个服务的镜像
docker-build:
	@echo "🐳 构建 Docker 镜像..."
	$(DOCKER_COMPOSE) build
	@echo "✅ Docker 镜像构建完成"

# 启动所有服务（中间件 + Go 服务）
# 说明：如果镜像不存在，会自动构建
docker-up:
	@echo "🐳 启动所有服务（中间件 + Go 服务）..."
	$(DOCKER_COMPOSE) up -d
	@echo "✅ 所有服务启动完成"
	@echo ""
	@echo "服务访问地址："
	@echo "  API Gateway:  http://localhost:8080"
	@echo "  Matching:     http://localhost:8081"
	@echo "  Market Data:  http://localhost:8082"
	@echo "  Chain Sync:   http://localhost:8083"
	@echo "  NFT Service:  http://localhost:8084"
	@echo "  Buyback:      http://localhost:8085"
	@echo "  Risk:         http://localhost:8086"

# 停止并删除所有容器
docker-down:
	@echo "🛑 停止并删除所有容器..."
	$(DOCKER_COMPOSE) down
	@echo "✅ 容器已删除"

# 查看所有服务日志
docker-logs:
	@echo "📋 查看所有服务日志..."
	$(DOCKER_COMPOSE) logs -f

# 查看单个服务日志（用法：make docker-logs-api）
docker-logs-api:
	$(DOCKER_COMPOSE) logs -f api-gateway
docker-logs-matching:
	$(DOCKER_COMPOSE) logs -f matching-engine
docker-logs-market:
	$(DOCKER_COMPOSE) logs -f market-data
