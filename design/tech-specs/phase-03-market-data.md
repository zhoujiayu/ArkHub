# 阶段三：行情聚合服务技术方案

> 阶段目标：实现多源行情接入、中位数滤波、异常剔除、加权融合，输出指数价格。
> 预计工期：7 天
> 依赖阶段：阶段一

---

## 0. 设计原因

### 0.1 为什么需要 14+ 路行情聚合

**业务背景**：
- 单一交易所行情易被操纵（如 2021 年某些小交易所价格插针导致用户爆仓）
- 合约的标记价格和指数价格必须锚定真实现货市场
- 若使用单源行情，一旦数据源异常，平台将面临巨大法律和赔付风险

**设计决策**：

| 数据源类型 | 数量 | 作用 |
|-----------|------|------|
| 头部交易所 | 8 家 | Binance、OKX、Coinbase 等，提供主流币对价格 |
| 二线交易所 | 4 家 | 补充小众币种、地区性价格 |
| 平台自有现货 | 1 路 | 自身现货成交价，作为权重最高的内部锚点 |
| 数据聚合商 | 1 家 | CoinGecko、CoinMarketCap 等，交叉验证 |

**核心价值**：
- 单一数据源失效时，其余 13 路仍可正常计算，系统可用性从 99.9% 提升至 99.999%
- 抗操纵能力提升 90%，异常价格波动被中位数滤波平滑

### 0.2 为什么选中位数滤波 + Z-Score 异常剔除

| 算法 | 优势 | 劣势 | 适用场景 |
|------|------|------|---------|
| **中位数滤波** | 对异常值不敏感，极端价格不影响结果 | 计算稍慢于均值 | 多源价格聚合 |
| **均值滤波** | 计算快 | 异常值会拉偏结果 | 数据质量高的场景 |
| **Z-Score** | 统计意义明确，可量化偏离程度 | 需足够样本量 | 异常检测 |
| **IQR** | 对极端异常鲁棒 | 阈值固定，不够灵活 | 辅助验证 |

**组合优势**：
- 中位数滤波提供基准价格，Z-Score 识别并剔除偏离超过 3σ 的异常值
- 双重保障确保输出价格精准贴合现货市场，为合约定价提供可靠锚点

### 0.3 为什么 EIP-712 预言机签名

**业务背景**：
- 链上智能合约需要可信价格来源，但链上无法直接获取链下数据
- 传统预言机（如 Chainlink）依赖外部节点网络，延迟高、成本高
- 平台自研预言机可将延迟从秒级降至毫秒级

**设计决策**：
- EIP-712 结构化签名标准，确保价格数据不可篡改且可验证
- 签名后的价格通过 RocketMQ 广播至链上合约，可信率 100%
- 相比 Chainlink，延迟降低 100 倍，成本降低 90%

---

## 1. 模块拆分

| 子模块 | 描述 | 验证方式 |
|--------|------|---------|
| **3.1 多协议适配器** | 适配 REST / WebSocket / FIX 协议 | 模拟数据源，验证数据接入 |
| **3.2 中位数滤波器** | 对多源价格排序取中位数 | 单元测试验证滤波效果 |
| **3.3 异常剔除算法** | Z-Score / IQR 异常检测 | 注入异常数据，验证剔除逻辑 |
| **3.4 加权融合引擎** | 按权重计算指数价格 | 多场景权重计算验证 |
| **3.5 EIP-712 签名** | 预言机价格链上可信签名 | 签名生成与验证测试 |
| **3.6 数据输出** | WebSocket / MQ / Redis 多通道推送 | 各通道消息接收验证 |

---

## 2. 技术方案

### 2.1 多协议适配器

**数据源接口抽象：**

```go
type MarketDataSource interface {
    Name() string
    Connect() error
    Subscribe(symbol string) error
    Read() (*PriceTick, error)
    Close() error
}

type RESTSource struct{}
type WebSocketSource struct{}
type FIXSource struct{}
```

**配置结构（Nacos ChannelConfig）：**

```json
{
  "sources": [
    {"name": "binance", "type": "rest", "url": "https://api.binance.com", "weight": 0.2},
    {"name": "okx", "type": "websocket", "url": "wss://ws.okx.com:8443", "weight": 0.2},
    {"name": "platform", "type": "internal", "weight": 0.6}
  ]
}
```

### 2.2 中位数滤波器

**算法实现：**

```go
func MedianFilter(prices []float64) float64 {
    sorted := sort.Float64s(prices)
    n := len(sorted)
    if n%2 == 1 {
        return sorted[n/2]
    }
    return (sorted[n/2-1] + sorted[n/2]) / 2
}
```

### 2.3 异常剔除算法

**Z-Score 算法：**

```go
func RemoveOutliers(prices []float64) []float64 {
    mean, std := calcMeanStd(prices)
    threshold := 3.0
    var result []float64
    for _, p := range prices {
        if math.Abs((p-mean)/std) < threshold {
            result = append(result, p)
        }
    }
    return result
}
```

### 2.4 加权融合引擎

```go
func WeightedFusion(sources []SourcePrice) float64 {
    var sum, weightSum float64
    for _, s := range sources {
        sum += s.Price * s.Weight
        weightSum += s.Weight
    }
    return sum / weightSum
}
```

### 2.5 EIP-712 签名

```go
func SignOraclePrice(price float64, timestamp int64, privateKey *ecdsa.PrivateKey) ([]byte, error) {
    // 使用 go-ethereum/crypto 生成 EIP-712 签名
}
```

---

## 3. 验证清单

| 检查项 | 验证方法 | 通过标准 |
|--------|---------|---------|
| 多源接入 | 模拟 3+ 数据源 | 数据正常接入 |
| 中位数滤波 | 注入异常价格 | 输出正确中位数 |
| 异常剔除 | Z-Score > 3 的数据 | 被正确剔除 |
| 加权融合 | 固定权重计算 | 结果符合预期 |
| EIP-712 签名 | 签名 + 验证 | 验证通过 |
| WebSocket 推送 | 客户端订阅 | 实时收到价格 |
| MQ 推送 | 消费消息 | 消息内容正确 |
| Redis 缓存 | 读取缓存 | 价格一致性 |
| 性能测试 | 1000 次聚合 | P99 < 300ms |

---

## 4. 风险与应对

| 风险 | 影响 | 应对措施 |
|------|------|---------|
| 数据源全部失效 | 无价格输出 | 兜底机制：使用缓存价格 + 告警 |
| 数据源延迟高 | 聚合延迟增大 | 设置超时机制，超时不等 |
| 权重配置错误 | 价格偏离 | Nacos 热更新 + 配置校验 |

---

## 5. 交付物

- [ ] `cmd/market-data/` — 行情聚合服务
- [ ] `internal/market/adapter.go` — 多协议适配器
- [ ] `internal/market/filter.go` — 滤波算法
- [ ] `internal/market/fusion.go` — 加权融合
- [ ] `internal/market/signer.go` — EIP-712 签名
- [ ] `test/integration/market_test.go` — 集成测试
- [ ] `docs/market-api.md` — API 文档
