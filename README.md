# ArkHub - 加密货币/NFT 综合交易平台

> 面向现货、合约、NFT 交易的综合性数字资产金融平台，采用 Go 语言开发，支持高并发低延迟撮合、多源行情聚合、链上链下一致性保障等核心能力。

## 项目简介

ArkHub 是一个面向现货、合约、NFT 交易的综合性数字资产金融平台。核心能力包括：

- **高并发低延迟**：Disruptor 无锁队列 + 内存订单簿，撮合延迟仅 10ms，百万级 TPS
- **抗干扰行情聚合**：14 路异构行情源通过中位数滤波 + 异常剔除算法融合，指数价格精准
- **链上链下强一致**：双源校验 + 异步补偿 + 事件监听三重保障，数据一致性达 100%
- **轻量多链扩展**：统一区块链交互 SDK，新增公链仅需 2 周
- **实时风控与运营**：Spark Streaming 实时交易风控模型 + WebSocket 热点推送

## 技术栈

| 技术领域 | 具体技术 | 用途说明 |
|---------|---------|---------|
| **开发语言** | Go 1.22+ | 主后端服务开发 |
| **数据存储** | PostgreSQL 15+ | 订单、交易、回购统计等业务数据持久化 |
| **缓存** | Redis 7+ | 热点数据缓存、订单簿快照、稀有度计算结果 |
| **消息队列** | RocketMQ 5.0+ | 价格推送、成交通知、链上事件异步处理 |
| **配置中心** | Nacos 2.2+ | 多源行情配置热更新、服务注册与发现 |
| **监控** | Prometheus + Grafana | 系统/业务指标采集与可视化 |
| **容器化** | Docker + Docker Compose | 服务部署与环境隔离 |

## 快速开始

### 前置要求

在启动项目前，请确保你的开发环境已安装以下工具：

| 工具 | 版本要求 | 下载地址 |
|------|---------|---------|
| Docker Desktop | 最新版 | https://www.docker.com/products/docker-desktop/ |
| Go | 1.22+ | https://golang.org/dl/ |
| Make | 系统自带或 GNU Make | `brew install make` (macOS) |

### 第一步：启动基础设施（中间件）

```bash
# 方式一：使用 Makefile（推荐）
make setup

# 方式二：手动启动 Docker Compose
cd deployments
docker-compose up -d
```

> **部署方式说明**：本项目支持两种部署方式
>
> | 方式 | 说明 | 适用场景 |
> |------|------|---------|
> | **本地开发** | 中间件用 Docker，Go 服务本地编译 | 日常开发、调试 |
> | **Docker 部署** | 所有服务都用 Docker | 完整测试、演示 |
>
> 默认 `make setup` 只启动中间件，Go 服务需本地编译。如需完整 Docker 部署，执行 `make docker-up`。

### 第二步：检查服务状态

```bash
# 使用 Makefile
make status

# 或手动检查
docker-compose -f deployments/docker-compose.yml ps
```

### 第三步：验证各中间件服务

#### 1. PostgreSQL（数据库）

```bash
# 命令行连接
psql -h localhost -U arhub -d arhub -W
# 密码：arhub123

# 或使用 Docker 容器内连接
docker exec -it arhub-postgres psql -U arhub -d arhub
```

**Web 管理工具推荐**：
- **DBeaver**（免费）：https://dbeaver.io/
- **pgAdmin**（PostgreSQL 官方）：https://www.pgadmin.org/
- **DataGrip**（JetBrains，付费）

**连接信息**：
- 主机：`localhost`
- 端口：`5432`
- 数据库：`arhub`
- 用户名：`arhub`
- 密码：`arhub123`

#### 2. Redis（缓存）

```bash
# 命令行连接
redis-cli -p 6379

# 验证连接
redis-cli -p 6379 ping
# 预期输出：PONG

# 查看键值（示例）
redis-cli -p 6379 keys '*'
```

**GUI 工具推荐**：
- **RedisInsight**（Redis 官方，免费）：https://redis.com/redis-insight/
- **Medis**（macOS，付费）

**连接信息**：
- 主机：`localhost`
- 端口：`6379`
- 密码：无（单机模式未启用密码）

#### 3. RocketMQ（消息队列）

```bash
# 验证 NameServer 是否启动
curl http://localhost:9876
# 预期输出：显示 RocketMQ 版本信息

# 发送测试消息（需安装 RocketMQ 客户端）
docker exec -it arhub-rocketmq sh -c "
  cd /home/rocketmq/rocketmq-5.1.4/bin &&
  sh tools.sh org.apache.rocketmq.example.quickstart.Producer
"
```

**管理工具**：RocketMQ 目前主要通过命令行管理，暂无官方 Web UI。

#### 4. Nacos（配置中心）

```bash
# 访问 Nacos Web UI
open http://localhost:8848/nacos
# 默认账号密码：nacos / nacos
```

**Web UI 地址**：http://localhost:8848/nacos

**功能说明**：
- **配置管理**：查看和修改应用配置（如行情权重、价格限幅等）
- **服务列表**：查看已注册的服务实例
- **命名空间**：支持 dev / test / prod 环境隔离

#### 5. Prometheus（监控采集）

```bash
# 访问 Prometheus Web UI
open http://localhost:9090
```

**Web UI 地址**：http://localhost:9090

**常用查询**：
- `up` — 查看所有监控目标的健康状态
- `rate(http_requests_total[5m])` — 查看 HTTP 请求速率
- `go_goroutines` — 查看 Go 协程数量

#### 6. Grafana（监控仪表盘）

```bash
# 访问 Grafana Web UI
open http://localhost:3000
# 默认账号密码：admin / admin
```

**Web UI 地址**：http://localhost:3000

**首次使用步骤**：
1. 登录后进入 "Configuration" → "Data Sources"
2. 添加 Prometheus 数据源，URL 填 `http://arhub-prometheus:9090`
3. 创建 Dashboard，添加 Panel，选择 Prometheus 查询

### 第四步：初始化数据库

```bash
# 方式一：使用 Makefile（推荐）
make setup-db

# 方式二：手动执行
psql -h localhost -U arhub -d arhub -f migrations/001_init_schema.sql
```

### 第五步：构建项目

```bash
# 使用 Makefile（推荐）
make build

# 构建完成后，二进制文件位于 bin/ 目录
ls -la bin/
```

### 第六步：运行测试

```bash
# 使用 Makefile
make test
```

## 项目结构

```
ArkHub/
├── cmd/                          # 各阶段可执行入口（每个子目录为一个独立服务）
│   ├── api-gateway/              # Phase 2：API 网关服务
│   ├── auth-service/             # Phase 2：鉴权中心服务
│   ├── ws-gateway/               # Phase 2：WebSocket 网关服务
│   ├── matching-engine/          # Phase 4：订单撮合引擎
│   ├── market-data/              # Phase 3：行情聚合服务
│   ├── chain-sync/               # Phase 5：链上链下一致性服务
│   ├── nft-service/              # Phase 6：NFT 业务服务
│   ├── buyback-service/          # Phase 7：回购统计服务
│   └── risk-service/             # Phase 8：风控服务
├── internal/                     # 私有代码（不对外暴露）
│   ├── pkg/                      # 公共包（数据库、Redis、MQ 客户端封装、JWT）
│   │   ├── db/                   # PostgreSQL 客户端封装
│   │   ├── redis/                # Redis 缓存客户端封装
│   │   ├── mq/                   # RocketMQ 消息队列客户端封装
│   │   └── jwt/                  # JWT 工具包（RS256 非对称加密）
│   ├── middleware/               # 限流、熔断、鉴权等中间件
│   └── response/                 # 统一响应格式（Phase 2）
├── pkg/                          # 公共代码库（可独立使用）
│   └── utils/                    # 工具函数（日期、字符串、加密等）
├── api/                          # API 定义（OpenAPI / Proto）
│   └── openapi/
├── configs/                      # 配置文件
│   └── nacos/                    # Nacos 配置模板
├── deployments/                  # Docker / K8s 部署脚本
│   ├── docker-compose.yml        # 基础设施编排（单机模式）
│   └── prometheus/               # 监控配置
├── migrations/                   # 数据库初始化脚本
│   └── 001_init_schema.sql       # 初始库表结构
├── test/                         # 集成测试脚本
│   └── integration/
├── docs/                         # 文档
│   └── setup/                    # 部署说明
├── Makefile                      # 构建脚本
├── go.mod                        # Go 模块管理
├── CHANGELOG.md                  # 变更记录
└── README.md                     # 项目说明
```

## Makefile 命令说明

| 命令 | 说明 | 示例 |
|------|------|------|
| `make setup` | 一键启动所有中间件 | `make setup` |
| `make start` | 启动 Docker Compose 服务 | `make start` |
| `make stop` | 停止 Docker Compose 服务 | `make stop` |
| `make status` | 查看所有容器运行状态 | `make status` |
| `make build` | 编译所有 Go 服务 | `make build` |
| `make test` | 运行所有测试 | `make test` |
| `make setup-db` | 初始化数据库 | `make setup-db` |
| `make clean` | 清理容器和构建产物 | `make clean` |

## 开发实施规则

### 1. 中间件优先

实现业务代码前，先确保 Docker Desktop 中有对应中间件服务。所有中间件通过 `deployments/docker-compose.yml` 一键启动。

### 2. 单机模式优先

所有中间件默认使用单机模式，便于本地开发和快速启动。生产环境可平滑迁移至集群模式。

| 中间件 | 单机模式 | 集群模式 | 说明 |
|--------|---------|---------|------|
| PostgreSQL | 单节点 | 主从复制 | 单机满足开发需求 |
| Redis | 单节点 | Cluster 模式 | 单机满足开发需求 |
| RocketMQ | 单 NameServer + 单 Broker | 多节点集群 | 单机满足开发需求 |
| Nacos | Derby 内嵌数据库 | MySQL + 多节点 | 单机满足开发需求 |

### 3. 自动检测与安装

若本地缺少中间件，执行 `make setup` 自动创建并启动 Docker Compose 环境。

### 4. 详细记录操作流程

每个部署和配置步骤记录到 `docs/setup/` 目录下的对应文档。

### 5. 完成标记

每个阶段完成后，在技术方案文档中标记完成状态（✅/❌）。

### 6. 文件变更记录

每次修改后更新 `CHANGELOG.md`，记录变更文件列表。

## 中间件访问信息速查

| 服务 | 地址 | 用户名/密码 | 说明 |
|------|------|-----------|------|
| **PostgreSQL** | localhost:5432 | arhub / arhub123 | 业务数据库 |
| **Redis** | localhost:6379 | - | 缓存服务 |
| **RocketMQ** | localhost:9876 | - | 消息队列 |
| **Nacos** | http://localhost:8848/nacos | nacos / nacos | 配置中心 |
| **Prometheus** | http://localhost:9090 | - | 监控采集 |
| **Grafana** | http://localhost:3000 | admin / admin | 监控仪表盘 |

## Go 服务访问信息（Docker 部署时）

| 服务 | 本地地址 | 容器内地址 | 端口 |
|------|---------|-----------|------|
| **API Gateway** | http://localhost:8080 | http://api-gateway:8080 | 8080 |
| **撮合引擎** | http://localhost:8081 | http://matching-engine:8081 | 8081 |
| **行情聚合** | http://localhost:8082 | http://market-data:8082 | 8082 |
| **Auth Service** | http://localhost:8088 | http://auth-service:8088 | 8088 |
| **WS Gateway** | http://localhost:8087 | http://ws-gateway:8087 | 8087 |
| **NFT 服务** | http://localhost:8084 | http://nft-service:8084 | 8084 |
| **回购统计** | http://localhost:8085 | http://buyback-service:8085 | 8085 |
| **风控服务** | http://localhost:8086 | http://risk-service:8086 | 8086 |

### Docker 部署命令

```bash
# 构建所有 Go 服务的 Docker 镜像
make docker-build

# 启动所有服务（中间件 + Go 服务）
make docker-up

# 查看所有服务日志
make docker-logs

# 停止并删除所有容器
make docker-down
```

## 阶段实现状态

| 阶段 | 模块名称 | 状态 | 说明 |
|------|---------|------|------|
| Phase 1 | 基础设施层 | ✅ 已完成 | Docker Compose、数据库、Makefile、中文注释 |
| Phase 2 | 网关层 | ✅ 已完成 | API Gateway、鉴权中心、Sentinel 限流熔断、WebSocket Gateway、Nginx |
| Phase 3 | 行情聚合 | ⏳ 待实现 | 多源行情、滤波、融合 |
| Phase 4 | 撮合引擎 | ⏳ 待实现 | Disruptor、订单簿、撮合 |
| Phase 5 | 链上链下 | ⏳ 待实现 | 双源校验、异步补偿 |
| Phase 6 | NFT 业务 | ⏳ 待实现 | 元数据、稀有度、热度 |
| Phase 7 | 回购统计 | ⏳ 待实现 | 预聚合、缓存、降级 |
| Phase 8 | 风控服务 | ⏳ 待实现 | 实时风控、异常检测 |

## 验证清单

| 检查项 | 命令 | 通过标准 |
|--------|------|---------|
| Docker Compose 启动 | `make setup` | 所有容器状态 `healthy` |
| PostgreSQL 连接 | `psql -h localhost -U arhub` | 连接成功 |
| Redis 连接 | `redis-cli -p 6379 ping` | 返回 `PONG` |
| Nacos 访问 | `curl http://localhost:8848/nacos` | 返回 200 |
| Prometheus | `curl http://localhost:9090/-/healthy` | 返回 200 |
| Grafana | `curl http://localhost:3000/api/health` | 返回 200 |
| Go 编译 | `make build` | 无报错 |
| JWT 签发 | `curl -X POST http://localhost:8088/auth/login -d '{"username":"test"}'` | 返回 JWT Token |
| JWT 验证 | `curl -H "Authorization: Bearer <token>" http://localhost:8088/auth/verify` | 返回用户信息 |
| 限流测试 | `ab -n 100 -c 10 http://localhost:8080/api/v1/market/price` | 超限时返回 429 |
| WebSocket | `wscat -c ws://localhost:8087/ws` | 连接成功，心跳正常 |
| Nginx 代理 | `curl http://localhost/api/health` | 正确转发到 API Gateway |

## 常见问题

### Q1: Docker Compose 启动失败（端口被占用）

**问题**：提示 `bind: address already in use`

**解决**：
```bash
# 查找占用端口的进程
lsof -i :5432  # PostgreSQL
lsof -i :6379  # Redis
lsof -i :9876  # RocketMQ

# 或修改 docker-compose.yml 中的端口映射
# 例如将 5432:5432 改为 5433:5432
```

### Q2: Nacos 在 Apple Silicon (M1/M2/M3) 上无法启动

**问题**：提示 `no matching manifest for linux/arm64`

**解决**：已在 docker-compose.yml 中添加了 `platform: linux/amd64`，Docker Desktop 会自动使用 Rosetta 转译。

### Q3: 数据库初始化脚本没有自动执行

**问题**：PostgreSQL 表结构未创建

**解决**：
```bash
# 手动执行初始化脚本
make setup-db

# 或
psql -h localhost -U arhub -d arhub -f migrations/001_init_schema.sql
```

### Q4: 如何重置所有中间件数据

**问题**：需要清空所有数据重新来过

**解决**：
```bash
# 停止并删除容器和数据卷
make clean

# 重新启动
make setup
```

## 贡献指南

1. Fork 本仓库
2. 创建 feature 分支 (`git checkout -b feature/xxx`)
3. 提交代码 (`git commit -m 'feat: xxx'`)
4. 推送到分支 (`git push origin feature/xxx`)
5. 创建 Pull Request

## 许可证

MIT License
