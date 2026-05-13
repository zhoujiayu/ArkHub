# ArkHub 系统说明书

> 本文档描述 ArkHub 各服务的职责、端口、调用关系及数据流，帮助开发者快速理解系统架构。

---

## 目录

- [1. 系统架构总览](#1-系统架构总览)
- [2. 服务清单](#2-服务清单)
- [3. 服务调用关系](#3-服务调用关系)
- [4. 数据流](#4-数据流)
- [5. 基础设施依赖](#5-基础设施依赖)
- [6. 用户端到服务端执行路径](#6-用户端到服务端执行路径)
- [7. 前端交互（待实现）](#7-前端交互待实现)
- [8. 快速熟悉代码](#8-快速熟悉代码)
- [9. 启动顺序](#9-启动顺序)

---

## 1. 系统架构总览

```
                         外部用户 / 客户端
                                 |
                                 ▼
                    ┌──────────────────────┐
                    │   Nginx (端口 80)    │
                    └──────────┬───────────┘
                               |
           ┌───────────────────┼───────────────────┐
           ▼                   ▼                   ▼
    ┌──────────┐       ┌───────────┐       ┌───────────┐
    │ API      │       │ WS        │       │ Auth      │
    │ Gateway  │       │ Gateway   │       │ Service   │
    │ :8080    │       │ :8087     │       │ :8088     │
    └────┬─────┘       └───────────┘       └───────────┘
         |
         |  HTTP 反向代理
         |
    ┌────┴─────────────────────────────────────────────────┐
    │                                                         │
┌───▼────────────┐ ┌────────────┐ ┌────────────┐ ┌──────────┐
│ Market Data    │ │ Matching   │ │ Chain Sync │ │ Buyback  │
│ 行情聚合       │ │ Engine     │ │ 链上链下   │ │ Service  │
│ :8082          │ │ 撮合引擎   │ │ :8084      │ │ :8086    │
└────────────────┘ └──────┬─────┘ └────────────┘ └──────────┘
                          |
                    ┌─────┴─────┐
                    ▼           ▼
            ┌──────────┐ ┌──────────┐
            │ NFT      │ │ Risk     │
            │ Service  │ │ Service  │
            │ :8085    │ │ :8089    │
            └──────────┘ └──────────┘
```

---

## 2. 服务清单

| 服务 | 端口 | 职责 | 状态 |
|------|------|------|------|
| **api-gateway** | 8080 | HTTP 统一入口，路由转发、JWT 鉴权、限流、熔断 | 已完成 |
| **auth-service** | 8088 | 用户注册 / 登录 / JWT 签发 / 验证 / 刷新 / 注销（Redis+PG 双写 Token 黑名单） | 已完成 |
| **ws-gateway** | 8087 | WebSocket 长连接管理、消息广播、心跳检测 | 已完成 |
| **market-data** | 8082 | 多源行情聚合、异常剔除、加权融合、EIP-712 签名 | 已完成 |
| **matching-engine** | 8083 | 分布式撮合引擎（Disruptor + Redis + PostgreSQL） | 已完成 |
| **chain-sync** | 8084 | 链上链下一致性（双源校验、异步补偿、事件同步） | 已完成 |
| **nft-service** | 8085 | NFT 元数据、稀有度、热度评分、欺诈检测 | 已完成 |
| **buyback-service** | 8086 | 回购统计、定时聚合、Redis 缓存、降级策略 | 已完成 |
| **risk-service** | 8089 | 实时风控、异常检测 | 待实现 |

---

## 3. 服务调用关系

### 3.1 API Gateway 路由映射

API Gateway (`cmd/api-gateway/main.go`) 将外部请求按路径前缀路由到对应的后端服务：

| 路径前缀 | 目标服务 | 实际端口 |
|---------|---------|---------|
| `/api/v1/market/*` | market-data | 8082 |
| `/api/v1/order/*` | matching-engine | 8083 |
| `/api/v1/chain/*` | chain-sync | 8084 |
| `/api/v1/nft/*` | nft-service | 8085 |
| `/api/v1/buyback/*` | buyback-service | 8086 |
| `/api/v1/risk/*` | risk-service | 8089 |

### 3.2 各服务内部依赖

```
api-gateway
  ├── internal/middleware    (AuthMiddleware, RateLimit, CircuitBreaker)
  ├── internal/response      (统一 JSON 响应格式)
  └── internal/pkg/jwt       (JWT 解析)

auth-service
  └── internal/pkg/jwt       (JWT 生成 / 验证 / 刷新)

matching-engine
  ├── internal/matching      (Disruptor, OrderBook, Matcher, DistributedOrderBook, CircuitBreaker, Trade)
  └── internal/pkg/db        (PostgreSQL 连接)

market-data
  └── internal/market        (数据源适配器、过滤算法、融合算法、EIP-712 签名)

chain-sync
  ├── internal/chain           (Client, Validator, Compensation, Sync, Reconciler)
  └── internal/pkg/db

nft-service
  └── internal/nft             (IPFS, Parser, Rarity, Heat, Fraud)

buyback-service
  ├── internal/buyback         (Aggregator, Cache, Fallback, Handler)
  └── internal/pkg/db

ws-gateway
  └── （无内部依赖，独立长连接管理）
```

---

## 4. 数据流

### 4.1 行情聚合 → 撮合引擎

```
┌──────────────┐     ┌──────────────────┐     ┌────────────────┐
│ 多源行情     │     │ market-data      │     │ matching-engine│
│ (Binance,  │────▶│ · Z-Score 异常剔除│────▶│ · 内存订单簿   │
│  OKX, 内部) │     │ · 中位数滤波      │     │ · Disruptor   │
│             │     │ · 加权融合        │     │ · 撮合匹配     │
└──────────────┘     │ · EIP-712 签名   │     └───────┬────────┘
                     └──────────────────┘             │
                                                      ▼
                                              ┌──────────────┐
                                              │ PostgreSQL   │
                                              │ 成交记录     │
                                              └──────────────┘
```

1. **收集**：从 Binance、OKX、平台内部 3 个源实时拉取价格
2. **清洗**：Z-Score 异常剔除 → 中位数二次过滤
3. **融合**：加权融合得到指数价格
4. **签名**：EIP-712 预言机签名（供链上合约验证）
5. **分发**：WebSocket 广播 + Redis / MQ 推送

### 4.2 订单提交 → 撮合 → 成交记录

```
客户端
  │
  │ POST /api/v1/order
  ▼
┌─────────────┐     ┌─────────────┐     ┌──────────────┐
│ API Gateway │────▶│ Matching    │────▶│ PostgreSQL   │
│             │     │ Engine      │     │ 成交记录     │
└─────────────┘     │ · Disruptor │     └──────────────┘
                    │ · Matcher   │
                    │ · OrderBook │
                    └──────┬──────┘
                           │
                    ┌──────▼──────┐
                    │ Redis       │
                    │ 分布式订单簿│
                    └─────────────┘
```

1. **接收**：客户端通过 API Gateway 提交订单
2. **入队**：订单写入 Disruptor 无锁环形队列
3. **撮合**：Matcher 从队列消费订单，在内存订单簿中实时匹配
4. **持久化**：成交记录异步写入 PostgreSQL
5. **同步**：分布式订单簿同步到 Redis Sorted Set

### 4.3 链上链下一致性

```
┌────────────┐     ┌─────────────┐     ┌──────────────┐
│ 区块链 RPC │────▶│ Chain Sync  │────▶│ PostgreSQL   │
│ (Ethereum) │     │ · 双源校验   │     │ 本地状态     │
└────────────┘     │ · 事件同步   │     └──────────────┘
                   │ · 异步补偿   │
                   │ · 数据校准   │
                   └─────────────┘
```

1. **监听**：WebSocket 订阅链上事件（Transfer、Swap 等）
2. **校验**：双源校验器对比链上状态与本地数据库
3. **补偿**：发现不一致时，异步补偿任务自动重试
4. **校准**：定时全量对账，发现重组时自动回滚

---

## 5. 基础设施依赖

| 组件 | 端口 | 用途 |
|------|------|------|
| **PostgreSQL** | 5432 | 业务数据持久化（订单、成交、链上数据） |
| **Redis** | 6379 | 热点缓存、订单簿快照、熔断器状态 |
| **RocketMQ** | 9876 | 消息队列（价格推送、成交通知、链上事件） |
| **Nacos** | 8848 | 配置中心、服务注册发现 |
| **Prometheus** | 9090 | 指标采集 |
| **Grafana** | 3000 | 监控仪表盘 |

---

## 6. 用户端到服务端执行路径

以下描述一个完整的**用户交易场景**，展示从用户操作到后端服务执行的完整链路。

### 6.1 典型场景：用户登录并下单

```
┌──────────┐     ┌────────────┐     ┌──────────────┐     ┌──────────────┐
│ 用户     │     │ 前端页面   │     │ API Gateway  │     │ Auth Service │
│          │     │ (待实现)   │     │ :8080        │     │ :8088        │
└────┬─────┘     └──────┬─────┘     └──────┬───────┘     └──────┬───────┘
     │                  │                  │                      │
     │  1. 输入账号密码   │                  │                      │
     │─────────────────▶│                  │                      │
     │                  │  POST /auth/login│                      │
     │                  │─────────────────▶│                      │
     │                  │                  │    POST /auth/login  │
     │                  │                  │─────────────────────▶│
     │                  │                  │                      │
     │                  │                  │    {token, expires}  │
     │                  │                  │◀─────────────────────│
     │                  │  {token}         │                      │
     │                  │◀─────────────────│                      │
     │  2. 保存 Token   │                  │                      │
     │◀─────────────────│                  │                      │
     │                  │                  │                      │
     │  3. 浏览行情页面 │                  │                      │
     │─────────────────▶│                  │                      │
     │                  │  GET /api/v1/market/index-price          │
     │                  │  Header: Bearer &lt;token&gt;                 │
     │                  │─────────────────▶│                      │
     │                  │                  │  验证 JWT Token      │
     │                  │                  │  (AuthMiddleware)    │
     │                  │                  │                      │
     │                  │                  │──────────────────────┐
     │                  │                  │                      │
     │                  │                  │◀─────────────────────│
     │                  │  转发到 market-data :8082              │
     │                  │◀─────────────────│                      │
     │                  │                  │                      │
     │  4. 提交订单     │                  │                      │
     │  (买入 BTC/USDT) │                  │                      │
     │─────────────────▶│                  │                      │
     │                  │  POST /api/v1/order/order             │
     │                  │  Header: Bearer &lt;token&gt;                │
     │                  │  Body: {symbol, side, price, qty}     │
     │                  │─────────────────▶│                      │
     │                  │                  │  验证 JWT Token      │
     │                  │                  │  限流检查            │
     │                  │                  │  熔断检查            │
     │                  │                  │                      │
     │                  │                  │──────────────────────┐
     │                  │                  │                      │
     │                  │                  │◀─────────────────────│
     │                  │  转发到 matching-engine :8083        │
     │                  │◀─────────────────│                      │
     │                  │                  │                      │
     │  5. 查看订单簿   │                  │                      │
     │─────────────────▶│                  │                      │
     │                  │  GET /api/v1/order/orderbook          │
     │                  │─────────────────▶│                      │
     │                  │                  │  转发到 :8083        │
     │                  │◀─────────────────│                      │
     │◀─────────────────│                  │                      │
     │                  │                  │                      │
     │  6. 查看成交记录 │                  │                      │
     │─────────────────▶│                  │                      │
     │                  │  GET /api/v1/order/trades             │
     │                  │─────────────────▶│                      │
     │                  │                  │  转发到 :8083        │
     │                  │◀─────────────────│                      │
     │◀─────────────────│                  │                      │
```

### 6.2 Auth Service 角色说明

**Auth Service** 是独立的鉴权中心，负责用户全生命周期管理：

| 端点 | 方法 | 说明 |
|------|------|------|
| `/auth/register` | POST | 用户注册（bcrypt 加密密码，存入 PostgreSQL） |
| `/auth/login` | POST | 用户登录（验证 bcrypt 密码，签发 JWT） |
| `/auth/verify` | GET | 验证 Token（Header: `Authorization: Bearer <token>`） |
| `/auth/refresh` | POST | 刷新 Token（Body 传旧 Token） |
| `/auth/logout` | POST | 注销（Header: `Authorization: Bearer <token>`，Token 加入黑名单） |

**Token 黑名单机制**：

```
用户调用 /auth/logout
      │
      ▼
┌─────────────────────┐
│ 解析 Token 获取过期时间 │
└──────────┬──────────┘
           │
     ┌─────┴──────┐
     ▼            ▼
┌────────┐   ┌──────────┐
│ Redis  │   │PostgreSQL│
│黑名单  │   │黑名单    │
│(TTL)   │   │(持久化)  │
└────────┘   └──────────┘
     │            │
     └──────┬─────┘
            ▼
    后续请求验证时
    先查 Redis → 再查 PostgreSQL
```

**设计特点**：
- **密码安全**：使用 bcrypt 加密（Cost=10），不可逆存储
- **双写黑名单**：Redis 提供高性能查询，PostgreSQL 提供持久化兜底
- **降级策略**：Redis 故障时，验证逻辑自动降级到 PostgreSQL
- **自动过期**：Redis 中的黑名单条目设置 TTL = Token 剩余有效期，过期自动清理

---

## 7. 前端交互（待实现）

以下页面和交互逻辑尚未实现，可作为后续开发参考。

> **注意**：登录/注册的后端 API 已实现（`POST /auth/login`、`POST /auth/register`），前端页面可直接对接。

### 7.1 待实现页面清单

| 页面 | 路径 | 交互说明 | 调用后端 |
|------|------|----------|----------|
| **登录页** | `/login` | 输入用户名密码 → 获取 JWT → 存入 localStorage | `POST /auth/login` ✅ 后端已完成 |
| **注册页** | `/register` | 输入用户信息 → 创建账号 | `POST /auth/register` ✅ 后端已完成 |
| **行情页** | `/market` | 展示 BTC/USDT 实时行情、K 线图 | `GET /api/v1/market/index-price` + WS |
| **交易页** | `/trade` | 下单面板（限价/市价）、订单簿深度图 | `POST /api/v1/order` + `GET /api/v1/orderbook` |
| **订单管理** | `/orders` | 查看当前委托、历史成交 | `GET /api/v1/order` + `GET /api/v1/trades` |
| **资产页** | `/assets` | 查看账户余额、充值提现记录 | （需新增 Asset Service） |
| **NFT 市场** | `/nft` | 浏览 NFT、查看元数据、计算稀有度 | `GET /api/v1/nft/metadata` + `POST /api/v1/nft/rarity` |
| **回购统计** | `/buyback` | 展示回购数据图表 | `GET /api/v1/buyback/stats` |
| **个人中心** | `/profile` | 修改密码、查看登录历史 | （待实现） |

### 7.2 前端技术栈建议（待确定）

| 领域 | 技术选型 | 说明 |
|------|----------|------|
| 框架 | React / Vue 3 | 组件化开发 |
| 状态管理 | Pinia / Redux Toolkit | 管理用户状态、Token |
| HTTP 客户端 | Axios | 统一封装请求拦截器（注入 Token） |
| WebSocket | 原生 WebSocket / Socket.io | 实时行情推送 |
| 图表 | TradingView / ECharts | K 线图、深度图 |
| UI 组件 | Ant Design / Element Plus | 快速搭建后台管理界面 |

### 7.3 前端请求封装示例（参考）

```typescript
// 封装 Axios 请求拦截器，自动注入 JWT Token
import axios from 'axios';

const api = axios.create({
  baseURL: '/api/v1',
  timeout: 10000,
});

// 请求拦截器：自动在 Header 中携带 Token
api.interceptors.request.use((config) => {
  const token = localStorage.getItem('token');
  if (token) {
    config.headers.Authorization = `Bearer ${token}`;
  }
  return config;
});

// 响应拦截器：Token 过期时自动刷新或跳转登录页
api.interceptors.response.use(
  (response) => response,
  (error) => {
    if (error.response?.status === 401) {
      localStorage.removeItem('token');
      window.location.href = '/login';
    }
    return Promise.reject(error);
  }
);

export default api;
```

---

## 8. 快速熟悉代码

### 8.1 阅读顺序（建议）

| 顺序 | 阶段 | 目标 |
|------|------|------|
| 1 | 跑起来 | `make setup` + `make build`，确认中间件全部 healthy |
| 2 | 看入口 | 从 `cmd/api-gateway/main.go` 开始，理解路由转发 |
| 3 | 看鉴权 | `cmd/auth-service/main.go` + `internal/middleware/auth.go` |
| 4 | 看行情 | `cmd/market-data/main.go` + `internal/market/` |
| 5 | 看撮合 | `cmd/matching-engine/main.go` + `internal/matching/`（最复杂） |
| 6 | 看链上 | `cmd/chain-sync/main.go` + `internal/chain/` |
| 7 | 看 NFT | `cmd/nft-service/main.go` + `internal/nft/` |
| 8 | 看回购 | `cmd/buyback-service/main.go` + `internal/buyback/` |
| 9 | 看配置 | `configs/` + `migrations/` |

### 8.2 关键文件速查

| 你想了解... | 文件 |
|-------------|------|
| 某个服务怎么启动的 | `cmd/<service>/main.go` |
| 服务间怎么互相调用 | `cmd/api-gateway/main.go` 里的 `serviceMap` |
| 数据库表结构 | `migrations/001_init_schema.sql` |
| 公共工具函数 | `internal/pkg/` 和 `pkg/` |
| 统一响应格式 | `internal/response/response.go` |
| 限流/熔断/鉴权逻辑 | `internal/middleware/` |

---

## 9. 启动顺序

### 9.1 中间件（先启动）

```bash
cd /Users/luca/luca/web3/arkHub/ArkHub
make setup
```

### 9.2 Go 服务（按顺序启动）

开发模式下逐个启动：

```bash
# 1. 鉴权服务（其他服务依赖 JWT）
make run-auth

# 2. 行情聚合
make run-market

# 3. 撮合引擎
make run-matching

# 4. 链上链下
make run-chain

# 5. NFT 服务
make run-nft

# 6. 回购统计
make run-buyback

# 7. WebSocket 网关
make run-ws

# 8. API 网关（最后启动，依赖以上所有服务）
make run-gateway
```

---

### 9.3 本地 Go 服务一键启动 / 停止（推荐开发使用）

先编译，再一键后台启动所有本地 Go 服务（使用 `nohup` 后台运行）：

```bash
# 一键后台启动所有本地服务
make run-all

# 查看服务是否启动成功
ps aux | grep "bin/"

# 查看日志
tail -f logs/api-gateway.log
tail -f logs/auth-service.log
tail -f logs/matching-engine.log

# 一键停止所有本地服务
make stop-go
```

| 命令 | 说明 | 适用场景 |
|------|------|----------|
| `make run-all` | 编译并后台启动所有 Go 服务 | 日常开发，快速启动 |
| `make stop-go` | 停止所有本地 Go 服务 | 开发结束或重启 |
| `make build` | 仅编译所有 Go 服务到 `bin/` 目录 | 检查编译是否通过 |

**本地服务日志位置**：`logs/*.log`

---

### 9.4 Docker 一键启动 / 停止（推荐测试 / 演示）

Docker 方式将中间件和 Go 服务全部容器化：

```bash
# 构建 Docker 镜像
make docker-build

# 启动所有服务（中间件 + Go 服务）
make docker-up

# 查看服务状态
make status

# 查看日志
make docker-logs
make docker-logs-api
make docker-logs-auth
make docker-logs-matching

# 停止并删除所有容器
make docker-down
```

**Docker 方式服务访问**：

| 服务 | 地址 |
|------|------|
| API Gateway | http://localhost:8080 |
| Auth Service | http://localhost:8088 |
| WS Gateway | ws://localhost:8087/ws |
| Matching Engine | http://localhost:8083 |
| Market Data | http://localhost:8082 |
| Chain Sync | http://localhost:8084 |
| NFT Service | http://localhost:8085 |
| Buyback | http://localhost:8086 |

---

### 9.5 两种部署方式对比

| 方式 | 命令 | 优点 | 缺点 | 适用场景 |
|------|------|------|------|----------|
| **本地 Go 服务** | `make run-all` / `make stop-go` | 热重载快、调试方便、资源占用低 | 需手动管理进程、无容器隔离 | 日常开发 |
| **Docker 全量** | `make docker-up` / `make docker-down` | 一键完整环境、环境隔离、便于部署 | 构建慢、占用资源多、调试不便 | 集成测试、演示 |

> 💡 **建议**：日常开发使用 **本地 Go 服务**（`make run-all`），集成测试或演示时使用 **Docker 全量**（`make docker-up`）。

---

## 附录：HTTP 端点速查

| 服务 | 端点 | 方法 | 说明 |
|------|------|------|------|
| api-gateway | `/health` | GET | 健康检查 |
| api-gateway | `/api/v1/market/*` | ANY | 转发到 market-data |
| api-gateway | `/api/v1/order/*` | ANY | 转发到 matching-engine |
| auth-service | `/auth/register` | POST | 用户注册（bcrypt 加密） |
| auth-service | `/auth/login` | POST | 登录获取 JWT |
| auth-service | `/auth/verify` | POST | 验证 JWT（检查黑名单） |
| auth-service | `/auth/refresh` | POST | 刷新 JWT（检查黑名单） |
| auth-service | `/auth/logout` | POST | 注销（Token 加入黑名单） |
| market-data | `/api/v1/index-price` | GET | 获取指数价格 |
| market-data | `/ws` | WS | WebSocket 实时行情 |
| matching-engine | `/api/v1/order` | POST | 提交订单 |
| matching-engine | `/api/v1/order/cancel` | POST | 取消订单 |
| matching-engine | `/api/v1/orderbook` | GET | 获取订单簿快照 |
| matching-engine | `/api/v1/trades` | GET | 查询成交记录 |
| chain-sync | `/api/v1/validate` | POST | 双源校验 |
| chain-sync | `/api/v1/compensate` | POST | 手动触发补偿 |
| chain-sync | `/api/v1/reconcile` | GET | 数据校准 |
| nft-service | `/api/v1/nft/metadata` | GET | 拉取 NFT 元数据 |
| nft-service | `/api/v1/nft/rarity` | POST | 计算稀有度 |
| nft-service | `/api/v1/nft/heat` | POST | 计算热度 |
| buyback-service | `/api/v1/buyback/stats` | GET | 回购统计 |
| buyback-service | `/api/v1/buyback/refresh` | POST | 手动刷新统计 |
| ws-gateway | `/ws` | WS | WebSocket 连接 |
