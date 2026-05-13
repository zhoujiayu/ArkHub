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

### Phase 3：行情聚合（已完成 ✅）

**变更文件：**

#### 1. 多协议行情适配器
- `internal/market/adapter.go` — 多协议行情适配器
  - `MarketDataSource` 接口：统一 REST / WebSocket / FIX / Internal 协议抽象
  - `RESTSource`：HTTP 轮询适配器，支持超时控制（5s）
  - `WebSocketSource`：WebSocket 长连接适配器，支持并发安全读写
  - `FIXSource`：FIX 协议适配器（占位，用于传统金融机构对接）
  - `InternalSource`：平台自有现货成交数据源，权重最高（0.4），并发安全
  - `PriceTick`：统一行情数据结构（Source / Symbol / Price / Timestamp）

#### 2. 滤波算法与异常剔除
- `internal/market/filter.go` — 中位数滤波 + 异常剔除算法
  - `MedianFilter(prices)`：排序后取中位数，对极端值不敏感，O(n log n)
  - `RemoveOutliers(prices, threshold)`：Z-Score 异常剔除，默认阈值 3σ
  - `RemoveOutliersIQR(prices)`：IQR 四分位距法，对极端异常鲁棒
  - `calcMeanStd()`：均值和标准差计算（内部辅助函数）
  - `percentile()`：百分位数计算（内部辅助函数）

#### 3. 加权融合引擎
- `internal/market/fusion.go` — 加权融合引擎
  - `SourcePrice`：带权重的价格源结构体
  - `WeightedFusion(sources)`：按权重计算加权平均价格
  - `WeightedFusionWithMedian(ticks, weights)`：先中位数滤波再按权重融合
    - 偏离中位数 10% 以上的价格自动剔除
    - 全部偏离时退化为中位数（兜底策略）

#### 4. EIP-712 预言机签名
- `internal/market/signer.go` — EIP-712 预言机签名模块
  - `SignOraclePrice(symbol, price, timestamp, privateKey)`：生成 65 字节 EIP-712 签名
  - `VerifyOraclePrice(symbol, price, timestamp, signature)`：验证签名并恢复公钥地址
  - `OraclePriceData` / `OraclePriceTypedData`：EIP-712 结构化数据类型
  - `HashEIP712Message(domainHash, typedDataHash)`：完整 EIP-712 签名消息哈希
  - 价格精度 8 位小数，使用 `go-ethereum` 库实现 keccak256 哈希和 secp256k1 签名

#### 5. 行情聚合服务主入口
- `cmd/market-data/main.go` — 行情聚合服务主入口
  - `aggregationEngine()`：定时聚合引擎（500ms 轮询）
    - 收集多源价格 → 中位数滤波 → Z-Score 异常剔除 → 加权融合 → EIP-712 签名 → 多通道推送
  - `collectPrices(sources)`：并发收集多源价格，使用 `sync.WaitGroup`
  - `broadcastToWS(data)`：WebSocket 广播推送
  - HTTP 端点：
    - `GET /health` — 健康检查
    - `GET /api/v1/index-price` — 获取当前指数价格
    - `GET /api/v1/sources` — 获取数据源列表及状态
    - `WS /ws` — WebSocket 实时订阅指数价格
  - 端口：8082

#### 6. 集成测试
- `test/integration/market_test.go` — 行情聚合服务集成测试（15 个用例全部通过）
  - 适配器测试：`TestInternalSource`、`TestRESTSource`、`TestWebSocketSource`
  - 中位数滤波测试：`TestMedianFilter`（奇偶元素、含极端值、空切片）
  - Z-Score 异常剔除测试：`TestRemoveOutliers`（正常、含异常值、阈值调整）
  - IQR 异常剔除测试：`TestRemoveOutliersIQR`
  - 加权融合测试：`TestWeightedFusion`、`TestWeightedFusionWithMedian`（含异常值场景）
  - EIP-712 签名测试：`TestSignAndVerifyOraclePrice`、`TestSignOraclePrice_NilKey`、`TestVerifyOraclePrice_InvalidLength`
  - 端到端聚合测试：`TestFullAggregationPipeline`（14 路数据 → 中位数滤波 → 加权融合）
  - 并发安全测试：`TestInternalSource_Concurrent`（100 goroutine 并发读写）
  - 性能基准测试：`BenchmarkMedianFilter`、`BenchmarkRemoveOutliers`、`BenchmarkWeightedFusion`

#### 7. API 文档
- `docs/market-api.md` — 行情聚合服务 API 文档
  - 接口列表：健康检查、获取指数价格、获取数据源列表、WebSocket 实时订阅
  - 核心算法说明：中位数滤波、Z-Score 异常剔除、加权融合、EIP-712 签名
  - 错误码说明、部署命令、监控与告警、配置示例

#### 8. 依赖更新
- `go.mod` / `go.sum` — 新增依赖
  - `github.com/ethereum/go-ethereum` — EIP-712 签名、keccak256 哈希
  - `github.com/decred/dcrd/dcrec/secp256k1/v4` — secp256k1 椭圆曲线签名

**实现说明：**
- 多协议适配器采用接口抽象，新增数据源只需实现 `MarketDataSource` 接口
- 中位数滤波 + Z-Score 双重保障：中位数提供基准，Z-Score 识别并剔除异常
- 加权融合支持两种模式：纯权重融合 / 中位数滤波后融合（推荐，异常值自动剔除）
- EIP-712 签名使用 `go-ethereum` 库，生产环境应从 KMS/HSM 读取私钥
- 服务启动时生成测试私钥，实际生产环境需替换为安全密钥管理方案
- 所有代码均添加中文注释，说明职责和实现逻辑

### Phase 4：撮合引擎（已完成 ✅）

**变更文件：**

#### 1. Disruptor 无锁队列
- `internal/matching/disruptor.go` — 无锁环形队列（Disruptor 简化版）
  - `RingBuffer` 结构体：预分配环形缓冲区，size 必须是 2 的幂次方
  - `Put(order)`：CAS 无锁写入，队列满时返回 `ErrRingBufferFull`
  - `Get()`：CAS 无锁读取，队列空时返回 `ErrRingBufferEmpty`
  - `Len()` / `Cap()`：查询当前元素数量和总容量
  - 缓存行对齐防止 false sharing，预分配内存避免 GC 压力
  - 基准测试：`BenchmarkRingBuffer` — ~270 万 ops/s，0 内存分配

#### 2. 内存订单簿
- `internal/matching/orderbook.go` — 内存订单簿（价格优先 + 时间优先）
  - `OrderBook` 结构体：使用 `map[float64][]*Order` 按价格分组存储
  - `AddOrder(order)`：按方向添加到对应队列，同一价格按时间排序（FIFO）
  - `RemoveOrder(id, side, price)`：从指定价格层级移除订单
  - `GetBestBuy()` / `GetBestSell()`：获取最优买/卖价格
  - `PeekBestBuy()` / `PeekBestSell()`：查看最优订单（不移除）
  - `PopBestBuy()` / `PopBestSell()`：取出并移除最优订单
  - `Snapshot()`：获取订单簿快照（深拷贝，线程安全）
  - 基准测试：`BenchmarkOrderBookAdd` — ~94 ns/op

#### 3. 撮合算法
- `internal/matching/matcher.go` — 撮合引擎
  - `Matcher` 结构体：撮合引擎核心，维护订单簿和成交记录通道
  - `Match(order)`：对传入订单进行撮合，返回成交记录列表
    - 买单：从卖单队列找最低价匹配（价格交叉条件：买价 >= 卖价）
    - 卖单：从买单队列找最高价匹配（价格交叉条件：卖价 <= 买价）
  - `matchBuy(order)` / `matchSell(order)`：撮合逻辑实现
  - `execute(buy, sell)`：执行单次撮合，成交价格为被动单价格
  - `CancelOrder(id, side, price)`：取消订单（从订单簿中移除）
  - 支持完全成交、部分成交、多笔撮合、无法撮合（加入订单簿）
  - 基准测试：`BenchmarkMatcher` — ~95 ns/op

#### 4. 成交记录与异步持久化
- `internal/matching/trade.go` — 成交记录生成与异步持久化
  - `Trade` 结构体：记录完整成交信息（买卖订单、价格、数量、时间）
  - `TradeStore` 接口：抽象存储层，支持内存和数据库实现
  - `MemoryTradeStore`：内存存储实现，基于 `map[string]*Trade`
    - `Save(trade)`：保存成交记录
    - `GetByID(id)`：根据 ID 查询
    - `GetByOrderID(id)`：根据订单 ID 查询
    - `GetBySymbol(symbol, limit)`：查询指定交易对成交记录
  - `AsyncTradeWriter`：异步批量写入器
    - 批量写入（默认 100 条）+ 定时写入（默认 100ms）
    - 支持优雅关闭：关闭时 flush 剩余数据

#### 5. 自动熔断
- `internal/matching/circuit.go` — 自动熔断器（三态模型）
  - `State`：Closed（正常）/ Open（熔断）/ HalfOpen（半开）
  - `NewCircuitBreaker(threshold, timeout, windowSize)`：创建熔断器
  - `Call(fn)`：执行被熔断保护的函数
    - Closed：正常执行，统计错误次数
    - Open：拒绝所有请求，返回 `ErrCircuitOpen`
    - HalfOpen：允许探测请求（默认 3 次），成功后恢复 Closed
  - `Reset()`：手动重置熔断器状态
  - 支持响应时间窗口统计（`GetAverageLatency()`）

#### 6. 撮合引擎服务主入口
- `cmd/matching-engine/main.go` — 撮合引擎服务主入口
  - 初始化组件：订单簿、Disruptor 队列、熔断器、成交记录存储
  - `consumeOrders()`：从 Disruptor 队列消费订单并进行撮合
  - HTTP API：
    - `POST /api/v1/order` — 提交订单
    - `POST /api/v1/order/cancel` — 取消订单
    - `GET /api/v1/orderbook` — 获取订单簿快照
    - `GET /api/v1/trades` — 获取成交记录
    - `GET /api/v1/circuit/status` — 获取熔断器状态
    - `GET /health` — 健康检查
  - 端口：8083

#### 7. 集成测试
- `test/integration/matching_test.go` — 撮合引擎集成测试（全部通过）
  - Disruptor 测试：`TestDisruptorQueue`（基本读写、队列满、并发读写 1000 笔）
  - 订单簿测试：`TestOrderBook`（添加/移除、价格排序、同一价格多订单）
  - 撮合测试：`TestMatcher`（简单撮合、部分成交、无法撮合、多笔撮合、取消订单）
  - 成交记录测试：`TestTradeStore`（保存/查询、多笔记录）
  - 熔断测试：`TestCircuitBreaker`（熔断和恢复、重置）
  - 性能基准：`BenchmarkRingBuffer`、`BenchmarkOrderBookAdd`、`BenchmarkMatcher`

**实现说明：**
- Disruptor 基于 CAS 原子操作，无锁设计，CPU 缓存友好
- 内存订单簿使用 `map[float64][]*Order`，同一价格内按时间排序（FIFO）
- 撮合算法支持完全成交、部分成交、多笔撮合
- 成交记录异步持久化，批量写入减少 I/O 压力
- 熔断器三态模型（Closed/Open/HalfOpen），支持自动恢复
- 所有代码均添加中文注释，说明职责和实现逻辑

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
