# CHANGELOG

## ArkHub 项目开发记录

### Phase 1：基础设施层（已完成 ✅）

**变更文件：**

#### 1. 部署与配置
- `deployments/docker-compose.yml` — Docker Compose 编排文件（单机模式）
  - PostgreSQL 15（单机模式，替代主从集群）
  - Redis 7（单机模式，替代 Cluster）
  - RocketMQ 5.1.4（单机模式，替代 NameServer + Broker 集群）
  - Nacos 2.2.3（单机模式，Derby 内嵌数据库，替代 MySQL + 多节点）
  - Prometheus 2.54.0（监控采集）
  - Grafana 10.2.0（监控仪表盘）
  - 每个服务均添加详细中文注释，说明用途和配置参数
  - 适配 Apple Silicon（M1/M2/M3）平台，使用 Rosetta 转译

- `deployments/prometheus/prometheus.yml` — Prometheus 监控配置
- `configs/nacos/nacos-config.json` — Nacos 初始配置

#### 2. 数据库
- `migrations/001_init_schema.sql` — 数据库初始化脚本
  - 用户资产表（user_balances）
  - 订单表（orders）
  - 成交记录表（trades）
  - NFT 资产表（nft_assets）
  - 回购记录表（buyback_records）
  - 链上同步断点表（sync_checkpoint）
  - 风控事件表（risk_events）
  - 包含索引创建和示例数据

#### 3. Go 代码骨架（带中文注释）
- `cmd/api-gateway/main.go` — API 网关入口（Phase 2 骨架）
  - 包含服务启动、路由注册、健康检查端点
  - 详细中文注释说明每个函数和流程

- `internal/pkg/db/postgres.go` — PostgreSQL 客户端封装
  - 连接池初始化、配置管理、健康检查
  - 包含连接池参数调优说明

- `internal/pkg/redis/redis.go` — Redis 缓存客户端封装
  - 常用操作封装（Get/Set/Delete）
  - 支持分布式锁、限流计数等扩展

- `internal/pkg/mq/rocketmq.go` — RocketMQ 消息队列客户端封装
  - 生产者、消费者封装
  - 支持同步/异步发送

- `internal/middleware/ratelimit.go` — 限流中间件
  - 令牌桶算法实现
  - 支持按用户ID/IP限流

- `internal/middleware/auth.go` — JWT 鉴权中间件
  - Token 生成与解析
  - 用户信息上下文传递

#### 4. 项目文档
- `README.md` — 项目说明文档
  - 详细的启动步骤（6 步）
  - 各中间件的访问方式和命令示例
  - 各中间件的 GUI 工具推荐
  - 常见问题 FAQ
  - Makefile 命令说明
  - 项目结构说明

- `Makefile` — 构建脚本
  - setup、start、stop、status、build、test、setup-db、clean
  - 每个命令的中文说明

- `go.mod` — Go 模块定义
- `CHANGELOG.md` — 变更记录

#### 5. 开发实施规则（写入 README.md）
- 中间件优先、单机模式优先、自动检测与安装
- 详细记录操作流程、完成标记、文件变更记录

**实现说明：**
- 所有中间件使用单机模式，便于本地开发和学习
- PostgreSQL 替代主从集群，Redis 替代 Cluster 模式
- RocketMQ 使用单 NameServer + 单 Broker
- Nacos 使用 Derby 内嵌数据库模式
- 所有配置和代码均添加详细中文注释
- Docker Compose 已成功启动（6/6 服务全部 healthy）

### Phase 2：网关层（已完成 ✅）

**变更文件：**

#### 1. 统一响应格式
- `internal/response/response.go` — 标准化 API 响应结构
  - Response 结构体：Code / Message / Data / TraceID
  - JSON() 返回成功响应，Error() 返回错误响应
  - 预设状态码常量（成功、鉴权、限流、服务等）

#### 2. JWT 工具包
- `internal/pkg/jwt/jwt.go` — JWT 生成、解析、验签工具函数
  - 使用 RS256 (RSA 非对称加密) 算法
  - GenerateToken()：RSA 私钥签名生成 Token
  - ParseToken()：RSA 公钥验签并解析 Claims
  - 自动生成 RSA 密钥对 (configs/private.pem, configs/public.pem)

#### 3. JWT 鉴权中间件
- `internal/middleware/auth.go` — 完整 JWT 鉴权中间件
  - AuthMiddleware()：从 Authorization Header 提取 Bearer Token
  - 验证 Token 签名和过期时间
  - 将 user_id 和 role 注入 gin.Context
  - GenerateToken() / RefreshToken() 工具函数

#### 4. 鉴权中心服务
- `cmd/auth-service/main.go` — 独立鉴权服务
  - POST `/auth/login` — 用户名密码登录，签发 JWT Token
  - POST `/auth/refresh` — 刷新 Access Token
  - GET `/auth/verify` — 验证 Token 有效性
  - POST `/auth/logout` — 登出（简化版，未实现黑名单）
  - 端口：8081

#### 5. Sentinel 限流熔断中间件
- `internal/middleware/ratelimit.go` — 完整限流熔断中间件
  - InitSentinel()：初始化 Sentinel 规则
  - RateLimitMiddleware()：三级限流（IP 60 req/min、用户 100 req/min、接口 1000 QPS）
  - CircuitBreakerMiddleware()：熔断（错误率 > 50% 持续 30s）
  - 降级恢复：返回 429 / 503 状态码

#### 6. API Gateway 完整实现
- `cmd/api-gateway/main.go` — 完整路由转发 + 中间件注册
  - 集成 Prometheus 指标采集（http_requests_total、http_request_duration_seconds）
  - 健康检查 /health 端点
  - 路由转发到后端服务（market、order、nft、buyback、risk、chain）
  - 使用 httputil.ReverseProxy 实现流式转发
  - 支持优雅关闭（Graceful Shutdown）
  - 端口：8080

#### 7. WebSocket Gateway
- `cmd/ws-gateway/main.go` — WebSocket 长连接服务
  - 心跳机制：PING/PONG 每 30 秒，超时 90 秒断开
  - 频道订阅：market.price、order.filled、nft.heat
  - 消息广播：基于 Hub 模式的广播机制
  - 支持 subscribe / unsubscribe / heartbeat 消息类型
  - 端口：8087

#### 8. Nginx 反向代理配置
- `deployments/nginx.conf` — Nginx 反向代理配置
  - `/api/` → API Gateway (localhost:8080)
  - `/ws` → WebSocket Gateway (localhost:8087)
  - 静态资源 /static/ → /var/www/static/
  - 健康检查 /health → API Gateway

#### 9. 集成测试
- `test/integration/gateway_test.go` — 网关集成测试
  - TestHealthEndpoint：验证健康检查端点
  - TestAuthMiddlewareMissingToken：验证鉴权中间件拒绝未授权请求

#### 10. 依赖更新
- `go.mod` / `go.sum` — 新增依赖
  - `github.com/golang-jwt/jwt/v5` — JWT 实现
  - `github.com/alibaba/sentinel-golang` — 限流熔断
  - `github.com/gorilla/websocket` — WebSocket 服务
  - `github.com/stretchr/testify` — 测试断言

**实现说明：**
- JWT 采用 RS256 非对称加密，公钥用于验签，私钥用于签发
- Sentinel 限流采用三级策略：IP 限流 + 用户限流 + 接口限流
- API Gateway 使用 httputil.ReverseProxy 实现流式响应转发
- WebSocket 采用 Hub 模式管理连接，支持多客户端广播
- 所有代码均添加中文注释，说明职责和实现逻辑

### Phase 3：行情聚合（待实现 ⏳）

**计划变更文件：**
- `cmd/market-data/` — 行情聚合服务
- `internal/market/` — 多协议适配器、滤波算法

### Phase 4：撮合引擎（待实现 ⏳）

**计划变更文件：**
- `cmd/matching-engine/` — 撮合引擎服务
- `internal/matching/` — Disruptor 队列、订单簿

### Phase 5：链上链下（待实现 ⏳）

**计划变更文件：**
- `cmd/chain-sync/` — 链上链下一致性服务
- `internal/chain/` — 双源校验器、事件同步引擎

### Phase 6：NFT 业务（待实现 ⏳）

**计划变更文件：**
- `cmd/nft-service/` — NFT 业务服务
- `internal/nft/` — 元数据解析器、稀有度算法

### Phase 7：回购统计（待实现 ⏳）

**计划变更文件：**
- `cmd/buyback-service/` — 回购统计服务
- `internal/buyback/` — 预聚合、缓存层

### Phase 8：风控服务（待实现 ⏳）

**计划变更文件：**
- `cmd/risk-service/` — 风控服务
- `internal/risk/` — 实时风控模型、异常检测
