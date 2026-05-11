# ArkHub Phase 01 & Phase 02 验证文档

> 验证范围：基础设施层（Phase 01）+ 网关层与基础服务（Phase 02）
> 验证环境：Docker Compose
> 验证日期：2026-05-11

---

## 目录

- [Phase 01：基础设施层验证](#phase-01基础设施层验证)
  - [1.1 Docker Compose 启动验证](#11-docker-compose-启动验证)
  - [1.2 PostgreSQL 验证](#12-postgresql-验证)
  - [1.3 Redis 验证](#13-redis-验证)
  - [1.4 RocketMQ 验证](#14-rocketmq-验证)
  - [1.5 Nacos 验证](#15-nacos-验证)
  - [1.6 Prometheus 验证](#16-prometheus-验证)
  - [1.7 Grafana 验证](#17-grafana-验证)
- [Phase 02：网关层与基础服务验证](#phase-02网关层与基础服务验证)
  - [2.1 中间件启动](#21-中间件启动)
  - [2.2 API Gateway 验证](#22-api-gateway-验证)
  - [2.3 鉴权中心（Auth Service）验证](#23-鉴权中心auth-service验证)
  - [2.4 WebSocket Gateway 验证](#24-websocket-gateway-验证)
  - [2.5 JWT 鉴权中间件验证](#25-jwt-鉴权中间件验证)
  - [2.6 Sentinel 限流熔断验证](#26-sentinel-限流熔断验证)
  - [2.7 Nginx 反向代理验证](#27-nginx-反向代理验证)
- [一键验证脚本](#一键验证脚本)
- [常见问题排查](#常见问题排查)

---

## Phase 01：基础设施层验证

### 前置条件

```bash
# 确保 Docker Desktop 已启动
docker info

# 确保在 ArkHub 项目根目录
cd /path/to/ArkHub
```

### 1.1 Docker Compose 启动验证

**目标**：验证所有中间件服务能正常启动

**步骤**：

```bash
# 1. 启动所有中间件
make infra-up

# 2. 查看容器状态
docker-compose -f deployments/docker-compose.yml ps
```

**预期结果**：

```
NAME                IMAGE                          STATUS
arhub-postgres      postgres:15-alpine             Up (healthy)
arhub-redis         redis:7-alpine                 Up (healthy)
arhub-rocketmq      apache/rocketmq:5.1.4          Up
arhub-nacos         nacos/nacos-server:v2.2.3      Up
arhub-prometheus    prom/prometheus:v2.54.0        Up
arhub-grafana       grafana/grafana:10.2.0         Up
```

**通过标准**：所有容器状态为 `Up (healthy)` 或 `Up`

---

### 1.2 PostgreSQL 验证

**目标**：验证 PostgreSQL 数据库可连接，表结构正确

**步骤**：

```bash
# 1. 命令行连接测试
docker exec -it arhub-postgres psql -U arhub -d arhub -c "SELECT version();"

# 2. 验证表结构
docker exec -it arhub-postgres psql -U arhub -d arhub -c "\dt"

# 3. 验证初始化数据
docker exec -it arhub-postgres psql -U arhub -d arhub -c "SELECT COUNT(*) FROM user_balances;"
```

**预期结果**：

```
                                   version
-------------------------------------------------------------------------
 PostgreSQL 15.x on ...
(1 row)

          List of relations
 Schema |     Name      | Type  | Owner
--------+---------------+-------+-------
 public | orders        | table | arhub
 public | trades        | table | arhub
 public | user_balances | table | arhub
 ...

 count
-------
     0
(1 row)
```

**通过标准**：
- 连接成功返回 PostgreSQL 版本
- `\dt` 显示所有预期表（orders, trades, user_balances, nft_assets, buyback_records, sync_checkpoint, risk_events）

---

### 1.3 Redis 验证

**目标**：验证 Redis 缓存服务可连接

**步骤**：

```bash
# 1. PING 测试
docker exec -it arhub-redis redis-cli ping

# 2. SET/GET 测试
docker exec -it arhub-redis redis-cli SET test_key "hello"
docker exec -it arhub-redis redis-cli GET test_key

# 3. 查看配置
docker exec -it arhub-redis redis-cli INFO persistence
```

**预期结果**：

```
PONG

OK
"hello"

# Redis 持久化信息
rdb_last_save_time:xxx
aof_enabled:1
```

**通过标准**：
- `PING` 返回 `PONG`
- `SET/GET` 读写正常
- AOF 持久化已启用

---

### 1.4 RocketMQ 验证

**目标**：验证 RocketMQ NameServer 和 Broker 可访问

**步骤**：

```bash
# 1. 验证 NameServer 端口监听
docker exec arhub-rocketmq netstat -tlnp | grep 9876

# 2. 测试消息发送（在容器内）
docker exec arhub-rocketmq sh -c "
  cd /home/rocketmq/rocketmq-5.1.4/bin &&
  sh tools.sh org.apache.rocketmq.example.quickstart.Producer
"
```

**预期结果**：

```
tcp        0      0 :::9876                 :::*                    LISTEN

SendResult [sendStatus=SEND_OK, msgId=...]
```

**通过标准**：
- NameServer 端口 9876 在监听
- 消息发送成功（SendResult sendStatus=SEND_OK）

---

### 1.5 Nacos 验证

**目标**：验证 Nacos 配置中心 Web UI 可访问

**步骤**：

```bash
# 1. HTTP 接口测试
curl -s http://localhost:8848/nacos/v1/ns/operator/metrics | head -5

# 2. 浏览器访问
# 打开 http://localhost:8848/nacos
# 用户名：nacos，密码：nacos
```

**预期结果**：

```json
{
  "status": "UP",
  "data": {
    "members": [...]
  }
}
```

**通过标准**：
- HTTP 接口返回 JSON，status 为 UP
- Web UI 登录成功

---

### 1.6 Prometheus 验证

**目标**：验证 Prometheus 监控采集服务正常

**步骤**：

```bash
# 1. 健康检查接口
curl -s http://localhost:9090/-/healthy

# 2. 查询接口
curl -s 'http://localhost:9090/api/v1/query?query=up' | jq .

# 3. 浏览器访问 http://localhost:9090
```

**预期结果**：

```
Prometheus Server is Healthy.

{
  "status": "success",
  "data": {
    "resultType": "vector",
    "result": [...]
  }
}
```

**通过标准**：
- `/-/healthy` 返回 `Prometheus Server is Healthy.`
- `/api/v1/query` 返回有效 JSON

---

### 1.7 Grafana 验证

**目标**：验证 Grafana 监控仪表盘可访问

**步骤**：

```bash
# 1. HTTP 测试
curl -s http://localhost:3000/api/health

# 2. 浏览器访问 http://localhost:3000
# 用户名：admin，密码：admin
```

**预期结果**：

```json
{
  "database": "ok",
  "version": "10.2.0"
}
```

**通过标准**：
- `/api/health` 返回 `database: ok`
- Web UI 登录成功

---

## Phase 02：网关层与基础服务验证

### 前置条件

```bash
# 确保 Phase 01 的中间件已启动
make infra-up

# 确保在 ArkHub 项目根目录
cd /path/to/ArkHub
```

### 2.1 中间件启动

**目标**：启动 PostgreSQL、Redis、RocketMQ、Nacos、Prometheus、Grafana

**步骤**：

```bash
# 启动中间件
make infra-up

# 查看状态
make infra-status
```

**预期结果**：所有中间件服务状态为 `Up`

---

### 2.2 API Gateway 验证

**目标**：验证 API Gateway 路由、健康检查、Prometheus 指标

**步骤**：

```bash
# 1. 编译 API Gateway
go build -o bin/api-gateway ./cmd/api-gateway

# 2. 启动 API Gateway（在后台）
./bin/api-gateway &
API_GATEWAY_PID=$!

# 3. 健康检查
curl -s http://localhost:8080/health | jq .

# 4. Prometheus 指标
curl -s http://localhost:8080/metrics | grep "http_requests_total" | head -3

# 5. 未授权访问（测试 JWT 中间件）
curl -s http://localhost:8080/api/v1/market/price

# 6. 停止 API Gateway
kill $API_GATEWAY_PID
```

**预期结果**：

```json
// 步骤 3：健康检查
{
  "code": 0,
  "message": "success",
  "data": {
    "status": "ok",
    "time": "2026-05-11T10:00:00Z"
  }
}
```

```
# 步骤 4：Prometheus 指标
http_requests_total{method="GET",path="/health",status="200"} 1
```

```json
// 步骤 5：未授权访问
{
  "code": 401,
  "message": "未提供认证信息",
  "trace_id": ""
}
```

**通过标准**：
- `/health` 返回 200，状态为 ok
- `/metrics` 暴露 Prometheus 指标
- `/api/v1/market/price` 未带 Token 返回 401

---

### 2.3 鉴权中心（Auth Service）验证

**目标**：验证 JWT 签发、验证、刷新、登出接口

**步骤**：

```bash
# 1. 编译 Auth Service
go build -o bin/auth-service ./cmd/auth-service

# 2. 启动 Auth Service（在后台）
./bin/auth-service &
AUTH_PID=$!

# 3. 登录 - 签发 JWT
curl -s -X POST http://localhost:8088/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"test_user","password":"123456"}' | jq .

# 4. 验证 Token（使用步骤 3 返回的 token）
curl -s -X POST http://localhost:8088/auth/verify \
  -H "Content-Type: application/json" \
  -d '{"token":"eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJ1c2VyX2lkIjoidGVzdF91c2VyIiwicm9sZSI6InVzZXIiLCJleHAiOjE3Nzg1OTI1MjcsImlhdCI6MTc3ODUwNjEyN30.jc1ObZnGhgBd5FU3W3LAB1uDQis4WUwvidLt8xOAbYq0kFLALd96FlH2agKKL2rpXwyzFZH0wQqejnMd_pqjdsZrhyn5SS14n2Jm0wAxWghWiIu8gjSBTZ8H4AkLPs79p_mEVqclKeOw-1R8AObHWCYxjiRpAzru_m-_k5BWhuOUO6d1KeLTpMWypV_J292UORFfw9jn5HoO-yHLm0AEWuWSkHpNAG6E7FxitLW3giFW9ElmyZ1oxE992w0T4xJbThKcoqJTQr1LGbKqhx3j-b3BYEb_khVQ_FoGouHdsxuxtDj93FgzA_eZIlr7NZruParg4xVoVleO1XJizsRiBQ"}' | jq .

# 5. 刷新 Token
curl -s -X POST http://localhost:8088/auth/refresh \
  -H "Content-Type: application/json" \
  -d '{"token":"eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJ1c2VyX2lkIjoidGVzdF91c2VyIiwicm9sZSI6InVzZXIiLCJleHAiOjE3Nzg1OTEwMDIsImlhdCI6MTc3ODUwNDYwMn0.N8B0-DfA4nKsT4J152YKuWU30A1oeaX5qbQasoyNylsjMgQbPtQqKU2PPh2NaaI7a0AKisySW0ZaC0XSfpf3c4v-dcWIuhLACVudrBbZ7agsfAk1aco6cPGdp9dw978-ohUIC9RFvtdVqOaub5mBbIqaglupzbZbEQ5WGbUyZT3zXNsA7p_1Zpc2qWBGNkX187-Z_rvJxf1TH1hI0j_d2agcOWl9kBGiPyft3_RSM0uOeUQge4HKrlKvZXq6hkeX3hRh5nLGnEbPfUd19APXdoj96poiBqfUe2u6l3K4sepnr7ZPM29Gl7J5PLG1M-fiNI1NjQUkUnitqZFoFQDiIQ"}' | jq .

# 6. 登出
curl -s -X POST http://localhost:8088/auth/logout | jq .

# 7. 停止 Auth Service
kill $AUTH_PID
```

**预期结果**：

```json
// 步骤 3：登录
{
  "code": 0,
  "message": "success",
  "data": {
    "access_token": "eyJhbGciOiJSUzI1NiIs...",
    "refresh_token": "eyJhbGciOiJSUzI1NiIs...",
    "expires_in": 86400
  }
}
```

```json
// 步骤 4：验证 Token
{
  "code": 0,
  "message": "success",
  "data": {
    "user_id": "test_user",
    "role": "user",
    "valid": true
  }
}
```

```json
// 步骤 5：刷新 Token
{
  "code": 0,
  "message": "success",
  "data": {
    "access_token": "eyJhbGciOiJSUzI1NiIs...",
    "expires_in": 86400
  }
}
```

**通过标准**：
- `/auth/login` 返回有效的 access_token 和 refresh_token
- `/auth/verify` 返回用户信息和 valid: true
- `/auth/refresh` 返回新的 access_token
- `/auth/logout` 返回成功消息

---

### 2.4 WebSocket Gateway 验证

**目标**：验证 WebSocket 连接、心跳、消息广播

**步骤**：

```bash
# 1. 编译 WS Gateway
go build -o bin/ws-gateway ./cmd/ws-gateway

# 2. 启动 WS Gateway（在后台）
./bin/ws-gateway &
WS_PID=$!

# 3. 使用 wscat 测试（需安装 wscat: npm install -g wscat）
# 如果没有 wscat，可以用 curl 测试 HTTP 升级

# 3.1 测试 HTTP 端点（WebSocket 升级前）
curl -s http://localhost:8087/ws -i

# 3.2 使用 websocat 测试（需安装 websocat）
# websocat ws://localhost:8087/ws

# 4. 停止 WS Gateway
kill $WS_PID
```

**预期结果**：

```
# 步骤 3.1：HTTP 端点
HTTP/1.1 400 Bad Request
# （因为不是 WebSocket 升级请求，返回 400 是正常的）
```

```
# 步骤 3.2：使用 websocat（如果有安装）
> {"type":"subscribe","channel":"market.price"}
< {"type":"pong"}
```

**通过标准**：
- HTTP 端点返回 400（非 WebSocket 请求）
- WebSocket 客户端可以连接并收发消息
- 心跳 PING/PONG 正常

---

### 2.5 JWT 鉴权中间件验证

**目标**：验证 JWT 中间件能正确拦截未授权请求

**步骤**：

```bash
# 1. 启动 API Gateway 和 Auth Service
go build -o bin/api-gateway ./cmd/api-gateway
./bin/api-gateway &
sleep 2

# 2. 未授权访问（不带 Token）
curl -s http://localhost:8080/api/v1/market/price | jq .

# 3. 错误 Token
 curl -s http://localhost:8080/api/v1/market/price \
  -H "Authorization: Bearer invalid_token" | jq .

# 4. 正确 Token
# 4.1 先登录获取 Token（复制返回的 access_token）
curl -s -X POST http://localhost:8088/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"test","password":"123"}' | jq -r '.data.access_token'

# 4.2 带正确 Token 访问
curl -s http://localhost:8080/api/v1/market/price \
  -H "Authorization: Bearer eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9..." | jq .

# 5. 停止服务
pkill -f "bin/api-gateway"
```

**预期结果**：

```json
// 步骤 2：未授权
{
  "code": 401,
  "message": "未提供认证信息"
}
```

```json
// 步骤 3：错误 Token
{
  "code": 401,
  "message": "无效的 Token"
}
```

```json
// 步骤 4.2：正确 Token（会转发到 market-data 服务）
{
  "code": 0,
  "message": "success",
  "data": {
    "service": "market",
    "message": "请求已收到（转发功能待实现）"
  }
}
```

**通过标准**：
- 未带 Token → 401
- 错误 Token → 401
- 正确 Token → 200，成功转发

---

### 2.6 Sentinel 限流熔断验证

**目标**：验证限流和熔断规则生效

**步骤**：

```bash
# 1. 启动 API Gateway
./bin/api-gateway &
sleep 2

# 2. 先登录获取 Token（复制返回的 access_token）
curl -s -X POST http://localhost:8088/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"test","password":"123"}' | jq -r '.data.access_token'

# 3. 正常请求（单条）
curl -s http://localhost:8080/api/v1/market/price \
  -H "Authorization: Bearer eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9..." | jq .

# 4. 压力测试 - 触发限流
# 使用 ab (Apache Bench) 或 curl 循环
curl -s http://localhost:8080/api/v1/market/price \
  -H "Authorization: Bearer eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9..." \
  -H "X-Forwarded-For: 1.2.3.4" \
  -H "X-User-ID: test_user" &

# 重复发送 70 次（触发 IP 限流阈值 60/min）
for i in {1..70}; do
  curl -s -o /dev/null -w "%{http_code}" http://localhost:8080/api/v1/market/price \
    -H "Authorization: Bearer $TOKEN" \
    -H "X-Forwarded-For: 1.2.3.4"
done

# 5. 停止服务
pkill -f "bin/api-gateway"
```

**预期结果**：

```json
// 步骤 3：正常请求
{
  "code": 0,
  "message": "success"
}
```

```json
// 步骤 4：触发限流后
{
  "code": 429,
  "message": "请求过于频繁，请稍后再试"
}
```

**通过标准**：
- 正常请求返回 200
- 触发限流后返回 429
- 熔断触发后返回 503

---

### 2.7 Nginx 反向代理验证

**目标**：验证 Nginx 能正确转发请求到 API Gateway 和 WebSocket Gateway

**步骤**：

```bash
# 1. 安装 Nginx（macOS）
# brew install nginx

# 2. 启动 Nginx（使用项目配置）
nginx -c $(pwd)/deployments/nginx.conf

# 3. 测试 HTTP 代理
curl -s http://localhost/api/health | jq .

# 4. 测试 API 代理
curl -s http://localhost/api/api/v1/market/price | jq .

# 5. 停止 Nginx
nginx -s stop
```

**预期结果**：

```json
// 步骤 3：健康检查
{
  "status": "ok"
}
```

```json
// 步骤 4：API 代理
{
  "code": 401,
  "message": "未提供认证信息"
}
```

**通过标准**：
- `/api/health` 正确转发到 API Gateway
- `/api/*` 正确转发到 API Gateway

---

## 验证用 Curl 命令

以下命令可直接复制到终端执行。每个命令包含：请求 → 预期返回。

### Phase 01：基础设施层

```bash
# PostgreSQL 连接测试
curl -s http://localhost:5432 2>/dev/null || echo "❌ PostgreSQL 未就绪"

# Redis PING
curl -s telnet://localhost:6379 2>/dev/null || echo "❌ Redis 未就绪"

# Nacos 健康
curl -s http://localhost:8848/nacos/v1/ns/operator/metrics | head -20

# Prometheus 健康
curl -s http://localhost:9090/-/healthy

# Grafana 健康
curl -s http://localhost:3000/api/health
```

---

### Phase 02：网关层（分步执行）

**Step 0 — 启动服务**

```bash
# 1. 启动中间件（如果未启动）
make infra-up

# 2. 编译并启动 Go 服务
make build
./bin/auth-service &
./bin/api-gateway &
./bin/ws-gateway &
sleep 3
```

**Step 1 — Auth Service：登录（签发 JWT）**

```bash
curl -X POST http://localhost:8088/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"test_user","password":"123456"}'
```

```json
// 预期返回
{
  "code": 0,
  "message": "success",
  "data": {
    "access_token": "eyJhbGciOiJSUzI1NiIs...",
    "refresh_token": "eyJhbGciOiJSUzI1NiIs...",
    "expires_in": 86400
  }
}
```

**Step 2 — Auth Service：验证 Token**

```bash
# 将上一步返回的 access_token 替换到这里
curl -X GET http://localhost:8088/auth/verify \
  -H "Authorization: Bearer eyJhbGciOiJSUzI1NiIs..."
```

```json
// 预期返回
{
  "code": 0,
  "message": "success",
  "data": {
    "user_id": "test_user",
    "role": "user",
    "valid": true
  }
}
```

**Step 3 — Auth Service：刷新 Token**

```bash
curl -X POST http://localhost:8088/auth/refresh \
  -H "Content-Type: application/json" \
  -d '{"refresh_token":"eyJhbGciOiJSUzI1NiIs..."}'
```

```json
// 预期返回
{
  "code": 0,
  "message": "success",
  "data": {
    "access_token": "eyJhbGciOiJSUzI1NiIs...",
    "expires_in": 86400
  }
}
```

**Step 4 — Auth Service：登出**

```bash
curl -X POST http://localhost:8088/auth/logout \
  -H "Authorization: Bearer eyJhbGciOiJSUzI1NiIs..."
```

```json
// 预期返回
{
  "code": 0,
  "message": "success",
  "data": {
    "message": "登出成功"
  }
}
```

**Step 5 — API Gateway：健康检查**

```bash
curl -X GET http://localhost:8080/health
```

```json
// 预期返回
{
  "code": 0,
  "message": "success",
  "data": {
    "status": "ok",
    "time": "2026-05-11T10:00:00Z"
  }
}
```

**Step 6 — API Gateway：Prometheus 指标**

```bash
curl -X GET http://localhost:8080/metrics | grep "http_requests_total" | head -3
```

```
// 预期返回
http_requests_total{method="GET",path="/health",status="200"} 1
```

**Step 7 — API Gateway：未授权访问（测试 JWT 中间件拦截）**

```bash
curl -X GET http://localhost:8080/api/v1/market/price
```

```json
// 预期返回
{
  "code": 401,
  "message": "未提供认证信息",
  "trace_id": ""
}
```

**Step 8 — API Gateway：错误 Token**

```bash
curl -X GET http://localhost:8080/api/v1/market/price \
  -H "Authorization: Bearer invalid_token"
```

```json
// 预期返回
{
  "code": 401,
  "message": "无效的 Token"
}
```

**Step 9 — API Gateway：正确 Token（路由转发）**

```bash
curl -X GET http://localhost:8080/api/v1/market/price \
  -H "Authorization: Bearer $TOKEN"
```

```json
// 预期返回（转发到 market-data 服务）
{
  "code": 0,
  "message": "success",
  "data": {
    "service": "market",
    "message": "请求已收到（转发功能待实现）"
  }
}
```

**Step 10 — API Gateway：Sentinel 限流测试**

```bash
# 先快速发送 70 次请求（触发 IP 限流阈值 60/min）
for i in {1..70}; do
  curl -s -o /dev/null \
    -H "Authorization: Bearer $TOKEN" \
    -H "X-Forwarded-For: 1.2.3.4" \
    http://localhost:8080/api/v1/market/price
done

# 再发送一次，应该被限流
curl -X GET http://localhost:8080/api/v1/market/price \
  -H "Authorization: Bearer $TOKEN" \
  -H "X-Forwarded-For: 1.2.3.4"
```

```json
// 预期返回
{
  "code": 429,
  "message": "请求过于频繁，请稍后再试"
}
```

**Step 11 — WebSocket Gateway：连接测试**

```bash
# HTTP 端点（非 WebSocket 请求，返回 400）
curl -X GET http://localhost:8087/ws -i
```

```
// 预期返回
HTTP/1.1 400 Bad Request
// （正常，因为不是 WebSocket 升级请求）
```

**Step 12 — Nginx 反向代理（如果已启动 Nginx）**

```bash
# 测试 Nginx 转发到 API Gateway
curl -X GET http://localhost/api/health
```

```json
// 预期返回
{
  "code": 0,
  "message": "success",
  "data": {
    "status": "ok"
  }
}
```

---

### 完整链路验证（顺序执行）

```bash
#!/bin/bash
# 完整链路：登录 → 获取 Token → 验证 → 带 Token 访问 API → 限流测试

echo "=== Step 1: 登录 ==="
TOKEN_RES=$(curl -s -X POST http://localhost:8088/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"test_user","password":"123456"}')
echo "$TOKEN_RES" | jq .
TOKEN=$(echo "$TOKEN_RES" | jq -r '.data.access_token')

echo ""
echo "=== Step 2: 验证 Token ==="
curl -s http://localhost:8088/auth/verify \
  -H "Authorization: Bearer $TOKEN" | jq .

echo ""
echo "=== Step 3: 带 Token 访问 API Gateway ==="
curl -s http://localhost:8080/api/v1/market/price \
  -H "Authorization: Bearer $TOKEN" | jq .

echo ""
echo "=== Step 4: 限流测试（70次请求）==="
for i in {1..70}; do
  curl -s -o /dev/null \
    -H "Authorization: Bearer $TOKEN" \
    -H "X-Forwarded-For: 1.2.3.4" \
    http://localhost:8080/api/v1/market/price
  echo -n "."
done
echo ""

echo ""
echo "=== Step 5: 验证限流生效 ==="
curl -s http://localhost:8080/api/v1/market/price \
  -H "Authorization: Bearer $TOKEN" \
  -H "X-Forwarded-For: 1.2.3.4" | jq .

echo ""
echo "=== 验证完成 ==="
```

---

## 常见问题排查

### Q1: 端口被占用

```bash
# 查找占用端口的进程
lsof -i :8080  # API Gateway
lsof -i :8081  # Matching Engine
lsof -i :8082  # Market Data
lsof -i :8087  # WS Gateway
lsof -i :8088  # Auth Service
lsof -i :5432  # PostgreSQL
lsof -i :6379  # Redis

# 杀死进程
kill -9 <PID>
```

### Q2: Docker 容器启动失败

```bash
# 查看容器日志
docker logs arhub-postgres
docker logs arhub-redis

# 重启容器
docker restart arhub-postgres
```

### Q3: JWT 验证失败

```bash
# 检查 RSA 密钥是否存在
ls -la configs/private.pem configs/public.pem

# 重新生成
openssl genrsa -out configs/private.pem 2048
openssl rsa -in configs/private.pem -pubout -out configs/public.pem
```

### Q4: Sentinel 初始化失败

```bash
# 检查日志
pkill -f "bin/api-gateway"
./bin/api-gateway  # 前台启动，查看错误日志
```

### Q5: 编译失败

```bash
# 更新依赖
go mod tidy

# 重新编译
make build
```

---

## 验证记录表

| 阶段 | 检查项 | 命令 | 通过标准 | 结果 | 备注 |
|------|--------|------|---------|------|------|
| Phase 01 | Docker Compose | `make infra-up` | 6 个服务全部 Up | ⬜ | |
| Phase 01 | PostgreSQL | `docker exec ... psql` | 连接成功 | ⬜ | |
| Phase 01 | Redis | `redis-cli ping` | 返回 PONG | ⬜ | |
| Phase 01 | RocketMQ | `docker exec ... netstat` | 端口 9876 监听 | ⬜ | |
| Phase 01 | Nacos | `curl localhost:8848` | 返回 UP | ⬜ | |
| Phase 01 | Prometheus | `curl localhost:9090` | 返回 Healthy | ⬜ | |
| Phase 01 | Grafana | `curl localhost:3000` | 返回 ok | ⬜ | |
| Phase 02 | API Gateway 健康 | `curl :8080/health` | 返回 ok | ⬜ | |
| Phase 02 | JWT 签发 | `curl :8088/auth/login` | 返回 Token | ⬜ | |
| Phase 02 | JWT 验证 | `curl :8088/auth/verify` | 返回用户信息 | ⬜ | |
| Phase 02 | 鉴权中间件 | `curl :8080/api/...` | 返回 401 | ⬜ | |
| Phase 02 | 限流 | 70 次请求 | 返回 429 | ⬜ | |
| Phase 02 | WebSocket | `curl :8087/ws` | 返回 400 | ⬜ | |
| Phase 02 | Nginx | `curl localhost/api/health` | 返回 ok | ⬜ | |

---

> 验证完成后，请填写上表并签名确认。
