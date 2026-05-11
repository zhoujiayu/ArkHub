# 阶段二：网关层与基础服务技术方案

> 阶段目标：实现 API Gateway、鉴权中心、限流熔断、WebSocket Gateway，提供统一入口和安全控制。
> 预计工期：5 天
> 依赖阶段：阶段一

---

## 0. 设计原因

### 0.1 为什么网关要分层设计

**业务背景**：
- 平台同时服务 Web、App、第三方 API、链上预言机 4 类客户端
- 不同客户端的鉴权、限流、协议要求不同（Web 用 HTTPS，预言机用 EIP-712 签名）
- 若不拆分网关，单点故障将导致全平台不可用

**设计决策**：

| 分层 | 职责 | 技术选型 | 选择原因 |
|------|------|---------|---------|
| **Nginx 反向代理层** | SSL 终止、静态资源、负载均衡 | Nginx | 性能卓越，C10K 问题解决方案 |
| **API Gateway** | HTTP 路由、鉴权、限流 | Go + gin / echo | 灵活可编程，支持自定义中间件 |
| **WebSocket Gateway** | 长连接管理、消息广播 | Go + gorilla/websocket | 单节点 10 万连接，Redis Pub/Sub 多节点同步 |
| **鉴权中心** | JWT 签发、验证、刷新 | Go + jwt-go | 无状态设计，水平扩展 |

**业务价值**：
- Nginx 扛住 80% 静态请求，API Gateway 专注业务路由
- WebSocket 独立部署，避免长连接阻塞 HTTP 短连接
- 单节点故障时，健康检查自动剔除，不影响整体服务

### 0.2 为什么 JWT + Sentinel 的组合

| 维度 | JWT | Session | 选择原因 |
|------|-----|---------|---------|
| **状态管理** | 无状态 | 有状态 | 支撑百万级用户，无需共享 Session 存储 |
| **扩展性** | 水平扩展无限制 | 需 Redis 共享 | 降低基础设施复杂度 |
| **性能** | 验证只需验签 | 需查询 Redis | 减少一次网络 RTT |

| 维度 | Sentinel | 自研限流 | 选择原因 |
|------|---------|---------|---------|
| **熔断策略** | 错误率 + 慢调用 + 异常数 | 仅错误率 | 多维度熔断，更精准 |
| **降级能力** | 自动 fallback | 需手动实现 | 降低开发成本 |
| **生态** | 阿里开源，文档丰富 | 需自维护 | 社区活跃，问题可快速解决 |

---

## 1. 模块拆分

| 子模块 | 描述 | 验证方式 |
|--------|------|---------|
| **2.1 API Gateway** | Nginx 反向代理、路由分发 | curl 测试路由转发 |
| **2.2 鉴权中心** | JWT 签发、验证、刷新 | 测试 Token 生命周期 |
| **2.3 限流熔断** | 基于 Sentinel 的限流熔断 | 压力测试验证限流效果 |
| **2.4 WebSocket Gateway** | 长连接管理、消息广播 | WebSocket 客户端连接测试 |
| **2.5 统一响应格式** | 标准化 API 响应 | 单元测试覆盖 |

---

## 2. 技术方案

### 2.1 API Gateway

**路由规则：**

| 前缀 | 目标服务 | 示例 |
|------|---------|------|
| `/api/v1/market` | 行情聚合服务 | `/api/v1/market/price` |
| `/api/v1/order` | 订单撮合引擎 | `/api/v1/order/place` |
| `/api/v1/nft` | NFT 业务服务 | `/api/v1/nft/assets` |
| `/api/v1/buyback` | 回购统计服务 | `/api/v1/buyback/stats` |
| `/api/v1/risk` | 风控服务 | `/api/v1/risk/events` |
| `/ws` | WebSocket Gateway | `ws://localhost/ws` |

**Nginx 配置：**

```nginx
server {
    listen 80;
    location /api/v1/market {
        proxy_pass http://market-data-service:8080;
    }
    # ... 其他路由
}
```

### 2.2 鉴权中心

**JWT 设计：**

```go
type Claims struct {
    UserID    string    `json:"user_id"`
    Role      string    `json:"role"`
    ExpiresAt time.Time `json:"exp"`
    IssuedAt  time.Time `json:"iat"`
}
```

**接口定义：**

| 接口 | 方法 | 描述 |
|------|------|------|
| `/auth/login` | POST | 用户名密码登录，返回 JWT |
| `/auth/refresh` | POST | 刷新 Token |
| `/auth/logout` | POST | 登出，黑名单 Token |
| `/auth/verify` | GET | 验证 Token 有效性 |

### 2.3 限流熔断

**限流策略：**

| 维度 | 规则 | 阈值 |
|------|------|------|
| IP 限流 | 每分钟请求数 | 60 次/分钟 |
| 用户限流 | 每分钟请求数 | 100 次/分钟 |
| 接口限流 | 每秒请求数 | 1000 QPS |

**熔断规则：**

| 指标 | 阈值 | 持续时间 |
|------|------|---------|
| 错误率 | > 50% | 30 秒 |
| 响应时间 | > 2s | 60 秒 |
| 降级恢复 | 每 10 秒允许 1 个请求 | 直至成功 |

### 2.4 WebSocket Gateway

**消息格式：**

```json
{
  "type": "subscribe|unsubscribe|heartbeat|data",
  "channel": "market.price|order.filled|nft.heat",
  "payload": {}
}
```

**连接管理：**
- 心跳机制：30 秒间隔 PING/PONG
- 连接上限：单节点 10 万连接
- 消息广播：基于 Redis Pub/Sub 多节点同步

---

## 3. 验证清单

| 检查项 | 验证命令 | 通过标准 |
|--------|---------|---------|
| 路由转发 | `curl /api/v1/market/price` | 正确转发到目标服务 |
| JWT 签发 | `curl -X POST /auth/login` | 返回有效 Token |
| JWT 验证 | `curl -H "Authorization: Bearer <token>"` | 验证通过 |
| 限流生效 | `ab -n 1000 -c 100` | 超限时返回 429 |
| 熔断生效 | 模拟服务故障 | 返回 503，触发降级 |
| WebSocket 连接 | `wscat -c ws://localhost/ws` | 连接成功，心跳正常 |
| 消息广播 | 多客户端订阅同一频道 | 消息同步推送 |

---

## 4. 风险与应对

| 风险 | 影响 | 应对措施 |
|------|------|---------|
| JWT 密钥泄露 | 安全风险 | 使用 RSA 非对称加密，密钥定期轮换 |
| WebSocket 连接泄漏 | 内存泄漏 | 设置连接超时、心跳检测、自动清理 |
| 限流误伤 | 正常用户被拒绝 | 提供白名单机制，动态调整阈值 |

---

## 5. 交付物

- [ ] `cmd/api-gateway/` — API Gateway 实现
- [ ] `cmd/auth-service/` — 鉴权服务实现
- [ ] `cmd/ws-gateway/` — WebSocket Gateway 实现
- [ ] `internal/middleware/` — 限流、熔断中间件
- [ ] `test/integration/gateway_test.go` — 集成测试
- [ ] `deployments/nginx.conf` — Nginx 配置
