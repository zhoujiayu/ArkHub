# Phase 02: 网关层与基础服务实现设计

> 对应方案：`design/tech-specs/phase-02-gateway.md`
> 状态：已批准，待实现

## 1. 设计概要

本阶段实现 ArkHub 的 API Gateway、鉴权中心、限流熔断和 WebSocket Gateway，提供统一入口和安全控制。

## 2. 模块划分与依赖

### 2.1 模块清单

| 模块 | 目标文件 | 依赖 |
|------|---------|------|
| **统一响应格式** | `internal/response/response.go` | 无 (基础) |
| **API Gateway** | `cmd/api-gateway/main.go` | 统一响应, 中间件 |
| **鉴权中心** | `cmd/auth-service/main.go` | JWT, Redis |
| **JWT 中间件** | `internal/middleware/auth.go` | JWT 库 |
| **限流熔断** | `internal/middleware/ratelimit.go` | Sentinel-Go |
| **WebSocket Gateway** | `cmd/ws-gateway/main.go` | Redis |
| **集成测试** | `test/integration/gateway_test.go` | 全部模块 |
| **Nginx 配置** | `deployments/nginx.conf` | Nginx |

### 2.2 依赖关系

```
                     ┌─────────────────────────────────────┐
                     │         Nginx (反向代理)             │
                     └───────────────┬───────────────────────┘
                                     │
                     ┌───────────────▼───────────────────────┐
                     │         API Gateway                   │
                     │  (路由分发 + 统一响应 + 中间件)        │
                     └───────┬───────────────────┬──────────┘
                             │                   │
              ┌──────────────▼────┐    ┌───────▼──────────┐
              │   JWT 中间件      │    │  Sentinel 限流   │
              └────────┬──────────┘    └───────┬──────────┘
                       │                      │
              ┌────────▼────────┐    ┌───────▼──────────┐
              │  Auth Service   │    │  WS Gateway      │
              └─────────────────┘    └──────────────────┘
```

## 3. 核心设计决策

### 3.1 JWT 设计

- **算法**: RS256 (RSA 非对称加密)
- **库**: `github.com/golang-jwt/jwt/v5`
- **Token 结构**:
  ```json
  {
    "user_id": "string",
    "role": "admin|user|guest",
    "exp": 1699999999,
    "iat": 1699999999
  }
  ```
- **签发策略**: Access Token (15分钟) + Refresh Token (7天)
- **密钥管理**: RSA 公私钥对，公钥可公开分发用于验签

### 3.2 Sentinel 限流策略

| 策略类型 | 维度 | 阈值 | 触发条件 |
|---------|------|------|---------|
| IP 限流 | 固定窗口 | 60 req/min | 单 IP 请求频率 |
| 用户限流 | 令牌桶 | 100 req/min | 单用户请求频率 |
| 接口限流 | 滑动窗口 | 1000 QPS | 单接口请求频率 |
| 熔断 | 错误率 | 50% 持续 30s | 服务不可用 |
| 降级恢复 | 渐进 | 每 10s 允许 1 req | 直至成功 |

### 3.3 WebSocket 设计

- **协议**: 基于 gorilla/websocket
- **心跳**: PING/PONG，30秒间隔，超时 90秒
- **频道**: `market.price`, `order.filled`, `nft.heat`
- **广播**: Redis Pub/Sub 多节点同步
- **消息格式**:
  ```json
  {
    "type": "subscribe|unsubscribe|heartbeat|data",
    "channel": "market.price|order.filled|nft.heat",
    "payload": {}
  }
  ```

### 3.4 API Gateway 路由

| 前缀 | 目标服务 | 备注 |
|------|---------|------|
| `/api/v1/market` | market-data-service | 行情聚合 |
| `/api/v1/order` | matching-engine | 订单撮合 |
| `/api/v1/nft` | nft-service | NFT 业务 |
| `/api/v1/buyback` | buyback-service | 回购统计 |
| `/api/v1/risk` | risk-service | 风控服务 |
| `/auth` | auth-service | 鉴权服务 |
| `/ws` | ws-gateway | WebSocket |

## 4. 数据模型

### 4.1 JWT Claims

```go
type Claims struct {
    UserID    string `json:"user_id"`
    Role      string `json:"role"`
    ExpiresAt int64  `json:"exp"`
    IssuedAt  int64  `json:"iat"`
}
```

### 4.2 统一响应

```go
type Response struct {
    Code    int         `json:"code"`
    Message string      `json:"message"`
    Data    interface{} `json:"data,omitempty"`
    TraceID string      `json:"trace_id"`
}
```

### 4.3 WebSocket 消息

```go
type WSMessage struct {
    Type    string                 `json:"type"`
    Channel string                 `json:"channel"`
    Payload map[string]interface{} `json:"payload"`
}
```

## 5. 验证标准

| 检查项 | 验证方式 | 通过标准 |
|--------|---------|---------|
| 路由转发 | curl /api/v1/market/price | HTTP 200，正确转发 |
| JWT 签发 | curl -X POST /auth/login | 返回有效 Access + Refresh Token |
| JWT 验证 | curl -H "Authorization: Bearer ..." | 验证通过，返回用户信息 |
| 限流生效 | ab -n 1000 -c 100 | 超限时返回 429 |
| 熔断生效 | 模拟服务故障 | 返回 503，触发降级 |
| WS 连接 | wscat -c ws://localhost/ws | 连接成功，心跳正常 |
| 消息广播 | 多客户端订阅 | 消息同步推送 |

## 6. 风险与应对

| 风险 | 影响 | 应对措施 |
|------|------|---------|
| JWT 密钥泄露 | 安全风险 | RSA 非对称加密，公钥可分发 |
| WS 连接泄漏 | 内存泄漏 | 心跳检测 + 自动清理 |
| 限流误伤 | 正常用户被拒绝 | 提供白名单机制 |
| Sentinel 学习成本 | 开发延迟 | 文档 + 示例 + 渐进式集成 |

## 7. 交付清单

- [ ] `internal/response/response.go`
- [ ] `cmd/api-gateway/main.go` (完整路由转发)
- [ ] `cmd/auth-service/main.go`
- [ ] `internal/middleware/auth.go` (完整 JWT 验证)
- [ ] `internal/middleware/ratelimit.go` (Sentinel 限流熔断)
- [ ] `cmd/ws-gateway/main.go`
- [ ] `test/integration/gateway_test.go`
- [ ] `deployments/nginx.conf`
