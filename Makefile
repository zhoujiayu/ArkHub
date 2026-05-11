# ============================================================
# ArkHub Makefile
# 职责：封装常用命令，简化开发、构建、部署流程
# 支持两种部署方式：
#   1. 本地开发：go run / go build（不依赖 Docker）
#   2. Docker 部署：docker-compose up --build（推荐）
# ============================================================

.PHONY: build test clean setup start stop status setup-db docker-build docker-up docker-down docker-logs help infra-up infra-down infra-status run-api run-auth run-ws run-all stop-go docker-go-up docker-go-down

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
	@echo "【中间件管理（Docker）】"
	@echo "  make infra-up       - 启动所有中间件（PostgreSQL, Redis, RocketMQ, Nacos, Prometheus, Grafana）"
	@echo "  make infra-down     - 停止所有中间件"
	@echo "  make infra-status   - 查看中间件运行状态"
	@echo ""
	@echo "【Go 服务本地运行（开发模式）】"
	@echo "  make run-api        - 运行 API Gateway（端口 8080）"
	@echo "  make run-auth       - 运行鉴权中心（端口 8088）"
	@echo "  make run-ws         - 运行 WebSocket Gateway（端口 8087）"
	@echo "  make run-all        - 一键后台运行所有本地 Go 服务"
	@echo "  make stop-go        - 一键停止所有本地 Go 服务"
	@echo ""
	@echo "【本地开发命令】"
	@echo "  make setup          - 启动所有中间件（同 infra-up，兼容旧版）"
	@echo "  make start          - 启动所有服务（中间件 + Go 服务）"
	@echo "  make stop           - 停止所有服务"
	@echo "  make status         - 查看所有容器运行状态"
	@echo "  make build          - 本地编译所有 Go 服务（不依赖 Docker）"
	@echo "  make test           - 运行所有测试"
	@echo "  make setup-db       - 初始化数据库"
	@echo "  make clean          - 清理容器和构建产物"
	@echo ""
	@echo "【Docker 部署命令】"
	@echo "  make docker-build   - 构建所有 Go 服务的 Docker 镜像"
	@echo "  make docker-up      - 启动所有服务（中间件 + Go 服务）"
	@echo "  make docker-down    - 停止并删除所有容器"
	@echo "  make docker-go-up   - 一键启动 Docker 中的 Go 服务"
	@echo "  make docker-go-down - 一键停止 Docker 中的 Go 服务"
	@echo "  make docker-logs    - 查看所有服务日志"
	@echo "============================================================"

# ============================================================
# 中间件管理（本地开发）
# ============================================================

# 启动中间件（不含Go服务，适合本地开发）
infra-up:
	@echo "🚀 启动中间件服务..."
	$(DOCKER_COMPOSE) up -d postgres redis rocketmq nacos prometheus grafana
	@echo "✅ 中间件启动完成"
	@echo ""
	@echo "中间件访问地址："
	@echo "  PostgreSQL:   localhost:5432  (arhub / arhub123)"
	@echo "  Redis:        localhost:6379"
	@echo "  RocketMQ:     localhost:9876"
	@echo "  Nacos:        http://localhost:8848/nacos  (nacos / nacos)"
	@echo "  Prometheus:   http://localhost:9090"
	@echo "  Grafana:      http://localhost:3000  (admin / admin)"
	@echo ""
	@echo "下一步：启动 Go 服务"
	@echo "  make run-api"
	@echo "  make run-auth"
	@echo "  make run-ws"

# 停止中间件
infra-down:
	@echo "🛑 停止中间件服务..."
	$(DOCKER_COMPOSE) down
	@echo "✅ 中间件已停止"

# 查看中间件状态
infra-status:
	@echo "📊 中间件状态："
	$(DOCKER_COMPOSE) ps

# ============================================================
# Go 服务本地运行（开发模式）
# ============================================================

# 运行 API Gateway
run-api:
	@echo "🚀 启动 API Gateway..."
	go run cmd/api-gateway/main.go

# 运行鉴权中心
run-auth:
	@echo "🚀 启动鉴权中心..."
	go run cmd/auth-service/main.go

# 运行 WebSocket Gateway
run-ws:
	@echo "🚀 启动 WebSocket Gateway..."
	go run cmd/ws-gateway/main.go

# 一键后台运行所有本地 Go 服务（开发调试用）
run-all: build
	@echo "🚀 后台启动所有本地 Go 服务..."
	@mkdir -p logs
	@nohup ./bin/api-gateway > logs/api-gateway.log 2>&1 &
	@nohup ./bin/auth-service > logs/auth-service.log 2>&1 &
	@nohup ./bin/ws-gateway > logs/ws-gateway.log 2>&1 &
	@echo "✅ 所有本地服务已后台启动"
	@echo ""
	@echo "服务访问地址："
	@echo "  API Gateway:    http://localhost:8080"
	@echo "  Auth Service:   http://localhost:8088"
	@echo "  WS Gateway:     ws://localhost:8087/ws"
	@echo ""
	@echo "日志查看："
	@echo "  tail -f logs/api-gateway.log"
	@echo "  tail -f logs/auth-service.log"
	@echo "  tail -f logs/ws-gateway.log"

# 一键停止所有本地 Go 服务
stop-go:
	@echo "🛑 停止所有本地 Go 服务..."
	@pkill -f "bin/api-gateway" || true
	@pkill -f "bin/auth-service" || true
	@pkill -f "bin/ws-gateway" || true
	@pkill -f "cmd/api-gateway/main.go" || true
	@pkill -f "cmd/auth-service/main.go" || true
	@pkill -f "cmd/ws-gateway/main.go" || true
	@echo "✅ 所有本地 Go 服务已停止"

# ============================================================
# 本地开发命令（兼容旧版）
# ============================================================

# 启动中间件（同 infra-up，兼容旧版命令）
setup: infra-up

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
	@echo "📊 服务状态："
	$(DOCKER_COMPOSE) ps

# 本地编译所有 Go 服务
build:
	@echo "🔨 本地编译 Go 服务..."
	go mod tidy
	@for dir in cmd/*/; do \
		service=$$(basename $$dir); \
		if [ -f "$$dir/main.go" ]; then \
			echo "  编译 $$service..."; \
			go build -o bin/$$service ./$$dir || echo "  ⚠️  $$service 编译失败"; \
		fi; \
	done
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
	rm -rf logs/
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
	@echo "  API Gateway:    http://localhost:8080"
	@echo "  Auth Service:   http://localhost:8088"
	@echo "  WS Gateway:     ws://localhost:8087/ws"
	@echo "  Matching:       http://localhost:8081"
	@echo "  Market Data:    http://localhost:8082"
	@echo "  Chain Sync:     http://localhost:8083"
	@echo "  NFT Service:    http://localhost:8084"
	@echo "  Buyback:        http://localhost:8085"
	@echo "  Risk:           http://localhost:8086"

# 停止并删除所有容器
docker-down:
	@echo "🛑 停止并删除所有容器..."
	$(DOCKER_COMPOSE) down
	@echo "✅ 容器已删除"

# 一键启动 Docker 中的 Go 服务（不启动中间件）
docker-go-up:
	@echo "🐳 启动 Docker 中的 Go 服务..."
	$(DOCKER_COMPOSE) up -d api-gateway auth-service ws-gateway matching-engine market-data chain-sync nft-service buyback-service risk-service
	@echo "✅ Docker Go 服务启动完成"
	@echo ""
	@echo "服务访问地址："
	@echo "  API Gateway:    http://localhost:8080"
	@echo "  Auth Service:   http://localhost:8088"
	@echo "  WS Gateway:     ws://localhost:8087/ws"
	@echo "  Matching:       http://localhost:8081"
	@echo "  Market Data:    http://localhost:8082"
	@echo "  Chain Sync:     http://localhost:8083"
	@echo "  NFT Service:    http://localhost:8084"
	@echo "  Buyback:        http://localhost:8085"
	@echo "  Risk:           http://localhost:8086"

# 一键停止 Docker 中的 Go 服务（不停止中间件）
docker-go-down:
	@echo "🛑 停止 Docker 中的 Go 服务..."
	$(DOCKER_COMPOSE) stop api-gateway auth-service ws-gateway matching-engine market-data chain-sync nft-service buyback-service risk-service
	@echo "✅ Docker Go 服务已停止"

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
