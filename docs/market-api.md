# 行情聚合服务 API 文档

> 版本：v1.0.0
> 服务端口：8082
> 最后更新：2026-05-11

---

## 1. 服务概述

行情聚合服务（market-data）负责从多路数据源实时采集价格，经过中位数滤波、Z-Score 异常剔除、加权融合后，输出指数价格。同时支持 EIP-712 预言机签名，确保链上合约可验证价格真实性。

---

## 2. 接口列表

### 2.1 健康检查

```
GET /health
```

**描述**：查询服务健康状态

**响应示例**：

```json
{
  "status": "ok",
  "time": "2026-05-11T14:30:00+08:00"
}
```

---

### 2.2 获取指数价格

```
GET /api/v1/index-price
```

**描述**：获取当前聚合后的指数价格

**响应示例**：

```json
{
  "symbol": "BTC-USDT",
  "price": 68080.52,
  "timestamp": 1715410200000
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| symbol | string | 交易对 |
| price | float64 | 聚合后的指数价格 |
| timestamp | int64 | 毫秒级时间戳 |

---

### 2.3 获取数据源列表

```
GET /api/v1/sources
```

**描述**：获取当前接入的数据源及其状态

**响应示例**：

```json
{
  "sources": [
    {"name": "binance", "type": "rest", "weight": 0.3, "status": "active"},
    {"name": "okx", "type": "rest", "weight": 0.3, "status": "active"},
    {"name": "platform", "type": "internal", "weight": 0.4, "status": "active"}
  ]
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| name | string | 数据源名称 |
| type | string | 协议类型：rest / websocket / fix / internal |
| weight | float64 | 权重（0~1） |
| status | string | 状态：active / inactive |

---

### 2.4 WebSocket 实时订阅

```
WS /ws
```

**描述**：建立 WebSocket 连接，实时接收指数价格更新

**连接示例**：

```bash
wscat -c ws://localhost:8082/ws
```

**推送消息格式**：

```json
{
  "symbol": "BTC-USDT",
  "price": 68080.52,
  "timestamp": 1715410200000,
  "signature": "0x..."
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| symbol | string | 交易对 |
| price | float64 | 聚合后的指数价格 |
| timestamp | int64 | 毫秒级时间戳 |
| signature | string | EIP-712 签名（hex 格式） |

---

## 3. 核心算法

### 3.1 中位数滤波

对多源价格排序后取中位数，对异常值不敏感，极端价格不影响结果。

### 3.2 Z-Score 异常剔除

```
threshold = 3.0
剔除条件：abs((price - mean) / std) >= threshold
```

偏离均值超过 3 个标准差的价格被剔除，确保数据质量。

### 3.3 加权融合

```
index_price = Σ(price_i * weight_i) / Σ(weight_i)
```

平台自有现货权重最高（0.4），头部交易所次之（0.3）。

### 3.4 EIP-712 签名

聚合后的价格经过 EIP-712 结构化签名，链上合约可验证签名真实性。

---

## 4. 错误码

| 状态码 | 含义 |
|--------|------|
| 200 | 请求成功 |
| 500 | 服务器内部错误 |

---

## 5. 部署与运行

### 5.1 编译

```bash
cd /path/to/project
go build -o market-data ./cmd/market-data/main.go
```

### 5.2 运行

```bash
./market-data
```

服务启动后将监听 `:8082` 端口。

### 5.3 Docker 部署

```bash
docker build -t market-data:latest .
docker run -p 8082:8082 market-data:latest
```

---

## 6. 监控与告警

### 6.1 Prometheus 指标

| 指标名 | 类型 | 说明 |
|--------|------|------|
| `http_requests_total` | Counter | HTTP 请求总数 |
| `http_request_duration_seconds` | Histogram | HTTP 请求耗时 |

### 6.2 日志

日志输出至 `logs/market-data.log`，包含数据源接入、异常剔除、价格推送等关键事件。

---

## 7. 附录：配置示例

```json
{
  "sources": [
    {"name": "binance", "type": "rest", "url": "https://api.binance.com", "weight": 0.3},
    {"name": "okx", "type": "websocket", "url": "wss://ws.okx.com:8443", "weight": 0.3},
    {"name": "platform", "type": "internal", "weight": 0.4}
  ],
  "filter": {
    "zscore_threshold": 3.0,
    "median_window": 14
  },
  "signature": {
    "private_key_path": "configs/oracle.pem"
  }
}
```
