# 阶段六：NFT 业务服务技术方案

> 阶段目标：实现 NFT 元数据拉取、稀有度计算、热度评分、异常交易检测、实时推送。
> 预计工期：7 天
> 依赖阶段：阶段一

---

## 0. 设计原因

### 0.1 为什么 NFT 需要独立的稀有度和热度评估

**业务背景**：
- NFT 市场缺乏统一的价值评估标准，价格发现困难
- 同一项目内不同 NFT 价差可达 10 倍，投资者难以判断价值
- 虚假交易（Wash Trading）泛滥，市场数据失真严重
- 竞品（如 OpenSea）仅提供基础排序，缺乏量化评估能力

**行业痛点**：

| 痛点 | 影响 | 案例 |
|------|------|------|
| 稀有度不透明 | 用户不知道 NFT 实际价值 | CryptoPunks 稀有属性溢价 5-10 倍，但普通用户无法判断 |
| 热度造假 | 虚假交易量误导投资者 | 2021 年多个 NFT 项目被揭露 90% 交易为自买自卖 |
| 价格波动大 | 用户资产缩水 | 买入时"热门"的 NFT，一周后热度归零，价格下跌 80% |

### 0.2 为什么 IPFS + Spark Streaming + 稀有度算法

| 组件 | 作用 | 替代方案 | 选择原因 |
|------|------|---------|---------|
| **IPFS 网关** | 拉取 NFT 元数据 | Arweave、中心化存储 | 去中心化、成本低、生态成熟 |
| **Spark Streaming** | 实时计算热度评分 | Flink、Storm | 批流一体，历史回溯方便 |
| **稀有度算法** | 量化 NFT 价值 | 无统一标准 | 自定义算法，支持 95%+ 项目 |

**设计亮点**：

**1. 稀有度量化算法**
- 单属性稀有度 = 拥有该属性的 NFT 数 / 总供给
- 综合稀有度 = 加权平均各属性稀有度
- 归一化到 0-100，直观展示 NFT 价值

**2. 热度评分模型**
- 链上活跃度（30%）：交易量、转账次数
- 稀缺度（25%）：总供给量、持有者分布
- 市场流动性（25%）：挂单量、成交速度
- 无异常交易（20%）：刷量检测、自买自卖识别

**3. 异常交易检测**
- 单地址占比：同一地址交易量 >30% 标记为异常
- 高频交易：同一地址 1 小时内交易 >20 次标记为异常
- 转账闭环：A→B→C→A 闭环模式识别刷量

### 0.3 为什么 WebSocket + Redis TTL 热点管理

**业务背景**：
- NFT 热度变化快，用户需要实时感知市场动向
- 热点 NFT 过期后应自动下线，避免推荐"冷"资产
- 推送不及时会导致用户错过最佳交易时机

**设计决策**：
- WebSocket 实时推送热点 NFT 给订阅用户
- Redis ZSet 存储热度排行，TTL 自动管理过期
- TTL 到期后自动从热点列表移除，WebSocket 通知客户端

**业务价值**：
- 用户第一时间感知热点，提升交易转化率
- 自动下线过期热点，保证推荐质量
- 减少人工运营干预，降低运营成本

---

## 1. 模块拆分

| 子模块 | 描述 | 验证方式 |
|--------|------|---------|
| **6.1 IPFS 网关 SDK** | 拉取 NFT 元数据 | 模拟 IPFS 网关，验证数据拉取 |
| **6.2 元数据解析器** | 解析属性字段，适配多种格式 | 测试 95%+ 项目格式 |
| **6.3 稀有度量化** | 单属性稀有度 + 综合稀有度加权 | 验证稀有度计算准确性 |
| **6.4 热度评分模型** | 链上活跃度 + 稀缺度 + 市场流动性 | 验证评分模型 |
| **6.5 异常交易检测** | 单地址占比、高频交易检测 | 模拟异常交易，验证检测率 |
| **6.6 实时推送** | WebSocket + Redis TTL 热点管理 | 验证推送和自动下线 |

---

## 2. 技术方案

### 2.1 IPFS 网关 SDK

```go
type IPFSGateway struct {
    gatewayURL string
    timeout    time.Duration
}

func (g *IPFSGateway) FetchMetadata(ctx context.Context, tokenURI string) (*NFTMetadata, error) {
    // 1. 解析 tokenURI 为 IPFS hash
    // 2. 通过网关拉取元数据
    // 3. 解析 JSON 返回
}

type NFTMetadata struct {
    Name        string            `json:"name"`
    Description string            `json:"description"`
    Image       string            `json:"image"`
    Attributes  []TraitAttribute `json:"attributes"`
}
```

### 2.2 元数据解析器

```go
type MetadataParser struct {
    // 适配不同格式的解析器
    parsers []Parser
}

func (p *MetadataParser) Parse(raw []byte) (*ParsedMetadata, error) {
    // 尝试多种格式解析
    for _, parser := range p.parsers {
        if meta, err := parser.Parse(raw); err == nil {
            return meta, nil
        }
    }
    return nil, errors.New("unsupported metadata format")
}
```

### 2.3 稀有度量化

```go
type RarityCalculator struct {
    totalSupply int
}

func (c *RarityCalculator) Calculate(metadata NFTMetadata) (*RarityScore, error) {
    // 1. 单属性稀有度 = 拥有该属性的 NFT 数 / 总供给
    // 2. 综合稀有度 = 加权平均各属性稀有度
    // 3. 归一化到 0-100
}
```

### 2.4 热度评分模型

```go
type HeatScoreModel struct {
    weights HeatWeights
}

type HeatWeights struct {
    Activity   float64 // 链上活跃度
    Scarcity   float64 // 稀缺度
    Liquidity  float64 // 市场流动性
    NoAnomaly  float64 // 无异常交易
}

func (m *HeatScoreModel) Calculate(metrics HeatMetrics) float64 {
    score := metrics.Activity * m.weights.Activity +
             metrics.Scarcity * m.weights.Scarcity +
             metrics.Liquidity * m.weights.Liquidity +
             metrics.NoAnomaly * m.weights.NoAnomaly
    return score
}
```

### 2.5 异常交易检测

```go
type FraudDetector struct {
    thresholds FraudThresholds
}

type FraudThresholds struct {
    SingleAddressRatio float64 // 单地址占比阈值
    HighFrequencyRate  int     // 高频交易阈值
    TransferLoopDepth  int     // 转账闭环深度
}

func (d *FraudDetector) Detect(transactions []Transaction) (*FraudReport, error) {
    // 1. 检测单地址占比
    // 2. 检测高频交易
    // 3. 检测转账闭环
    // 4. 生成报告
}
```

### 2.6 实时推送

```go
type HeatPushService struct {
    wsHub      *WebSocketHub
    redis      *redis.Client
    ttl        time.Duration
}

func (s *HeatPushService) PushHeatUpdate(ctx context.Context, update HeatUpdate) error {
    // 1. 更新 Redis 缓存
    // 2. 设置 TTL
    // 3. WebSocket 推送
    // 4. TTL 到期自动下线
}
```

---

## 3. 验证清单

| 检查项 | 验证方法 | 通过标准 |
|--------|---------|---------|
| IPFS 拉取 | 模拟 tokenURI | 成功拉取并解析 |
| 元数据解析 | 多种格式测试 | 支持 95%+ 格式 |
| 稀有度计算 | 已知稀有度 NFT | 计算结果准确 |
| 热度评分 | 多维度数据 | 评分合理 |
| 异常检测 | 模拟异常交易 | 识别率 88%+ |
| WebSocket 推送 | 客户端接收 | 实时收到推送 |
| TTL 自动下线 | 等待 TTL | 热点自动下线 |

---

## 4. 风险与应对

| 风险 | 影响 | 应对措施 |
|------|------|---------|
| IPFS 网关不可用 | 元数据拉取失败 | 多网关备份 + 缓存机制 |
| 元数据格式不兼容 | 解析失败 | 增加解析器适配 |
| 热度计算延迟 | 推送不及时 | 异步计算 + 缓存预热 |

---

## 5. 交付物

- [ ] `cmd/nft-service/` — NFT 业务服务
- [ ] `internal/nft/ipfs.go` — IPFS 网关 SDK
- [ ] `internal/nft/parser.go` — 元数据解析器
- [ ] `internal/nft/rarity.go` — 稀有度量化
- [ ] `internal/nft/heat.go` — 热度评分模型
- [ ] `internal/nft/fraud.go` — 异常交易检测
- [ ] `internal/nft/push.go` — 实时推送服务
- [ ] `test/integration/nft_test.go` — 集成测试
- [ ] `docs/nft-api.md` — API 文档
