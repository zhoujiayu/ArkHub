# 差距分析六：算法与风控

> 对比范围：web3-go 无算法 vs ArkHub 行情聚合、风控、NFT 评分

---

## 1. 技能对比

| 技能点 | web3-go 覆盖 | ArkHub 需求 | 差距 |
|--------|-------------|------------|------|
| **中位数滤波** | ❌ 未涉及 | ❌ 14 路行情源聚合 | 🔴 大差距 |
| **异常检测 (Z-Score/IQR)** | ❌ 未涉及 | ❌ 价格异常剔除 | 🔴 大差距 |
| **加权融合** | ❌ 未涉及 | ❌ 指数价格计算 | 🔴 大差距 |
| **异常交易检测** | ❌ 未涉及 | ❌ 刷量、关联交易识别 | 🔴 大差距 |
| **评分模型** | ❌ 未涉及 | ❌ NFT 热度评分 | 🔴 大差距 |
| **稀有度计算** | ❌ 未涉及 | ❌ 单属性/综合稀有度 | 🔴 大差距 |

---

## 2. 行情聚合算法

### 中位数滤波器

```go
package filter

import (
    "math"
    "sort"
)

// MedianFilter 中位数滤波器
type MedianFilter struct {
    windowSize int
    prices     []float64
}

func NewMedianFilter(windowSize int) *MedianFilter {
    return &MedianFilter{
        windowSize: windowSize,
        prices:     make([]float64, 0, windowSize),
    }
}

// AddPrice 添加价格
func (mf *MedianFilter) AddPrice(price float64) {
    mf.prices = append(mf.prices, price)
    if len(mf.prices) > mf.windowSize {
        mf.prices = mf.prices[1:]
    }
}

// Median 计算中位数
func (mf *MedianFilter) Median() float64 {
    if len(mf.prices) == 0 {
        return 0
    }
    
    sorted := make([]float64, len(mf.prices))
    copy(sorted, mf.prices)
    sort.Float64s(sorted)
    
    n := len(sorted)
    if n%2 == 1 {
        return sorted[n/2]
    }
    return (sorted[n/2-1] + sorted[n/2]) / 2
}

// Filter 滤波：返回中位数
func (mf *MedianFilter) Filter(prices []float64) float64 {
    for _, p := range prices {
        mf.AddPrice(p)
    }
    return mf.Median()
}
```

### 异常剔除 (Z-Score)

```go
package filter

import (
    "math"
)

// ZScoreDetector Z-Score 异常检测
type ZScoreDetector struct {
    threshold float64
}

func NewZScoreDetector(threshold float64) *ZScoreDetector {
    return &ZScoreDetector{threshold: threshold}
}

// Detect 检测异常值
func (z *ZScoreDetector) Detect(prices []float64) (normal []float64, outliers []float64) {
    if len(prices) == 0 {
        return prices, nil
    }
    
    // 计算均值和标准差
    mean := calculateMean(prices)
    std := calculateStd(prices, mean)
    
    if std == 0 {
        return prices, nil
    }
    
    for _, price := range prices {
        zScore := math.Abs((price - mean) / std)
        if zScore <= z.threshold {
            normal = append(normal, price)
        } else {
            outliers = append(outliers, price)
        }
    }
    
    return normal, outliers
}

func calculateMean(prices []float64) float64 {
    if len(prices) == 0 {
        return 0
    }
    var sum float64
    for _, p := range prices {
        sum += p
    }
    return sum / float64(len(prices))
}

func calculateStd(prices []float64, mean float64) float64 {
    if len(prices) == 0 {
        return 0
    }
    var sum float64
    for _, p := range prices {
        diff := p - mean
        sum += diff * diff
    }
    return math.Sqrt(sum / float64(len(prices)))
}
```

### 加权融合引擎

```go
package fusion

import (
    "fmt"
)

// SourcePrice 数据源价格
type SourcePrice struct {
    Source string
    Price  float64
    Weight float64
    Valid  bool
}

// WeightedFusion 加权融合引擎
type WeightedFusion struct {
    sources []SourcePrice
}

func NewWeightedFusion() *WeightedFusion {
    return &WeightedFusion{
        sources: make([]SourcePrice, 0),
    }
}

// AddSource 添加数据源
func (wf *WeightedFusion) AddSource(source SourcePrice) {
    wf.sources = append(wf.sources, source)
}

// Calculate 计算加权融合价格
func (wf *WeightedFusion) Calculate() (float64, error) {
    var totalWeight float64
    var weightedSum float64
    
    for _, source := range wf.sources {
        if !source.Valid {
            continue
        }
        totalWeight += source.Weight
        weightedSum += source.Price * source.Weight
    }
    
    if totalWeight == 0 {
        return 0, fmt.Errorf("no valid sources")
    }
    
    return weightedSum / totalWeight, nil
}

// CalculateWithMedian 中位数 + 加权融合
func (wf *WeightedFusion) CalculateWithMedian() (float64, error) {
    // 1. 先过滤异常值
    prices := make([]float64, 0)
    for _, s := range wf.sources {
        if s.Valid {
            prices = append(prices, s.Price)
        }
    }
    
    detector := NewZScoreDetector(3.0)
    normalPrices, _ := detector.Detect(prices)
    
    // 2. 用正常值重新计算权重
    var totalWeight float64
    var weightedSum float64
    
    for _, source := range wf.sources {
        if !source.Valid {
            continue
        }
        
        // 检查是否在正常值范围内
        isNormal := false
        for _, np := range normalPrices {
            if np == source.Price {
                isNormal = true
                break
            }
        }
        
        if !isNormal {
            continue
        }
        
        totalWeight += source.Weight
        weightedSum += source.Price * source.Weight
    }
    
    if totalWeight == 0 {
        return 0, fmt.Errorf("no valid sources after filtering")
    }
    
    return weightedSum / totalWeight, nil
}
```

---

## 3. 风控算法

### 异常交易检测

```go
package risk

import (
    "time"
)

// Transaction 交易记录
type Transaction struct {
    ID        string
    UserID    string
    Amount    float64
    Timestamp time.Time
    Type      string
    IP        string
}

// FraudDetector 异常交易检测器
type FraudDetector struct {
    // 用户交易历史
    userHistory map[string][]Transaction
    // 阈值配置
    thresholds FraudThresholds
}

type FraudThresholds struct {
    MaxSingleTrade     float64       // 单笔最大金额
    MaxDailyVolume     float64       // 单日最大交易量
    MaxFrequency       int           // 最大交易频率（每小时）
    MaxSameIPAccounts  int           // 同一 IP 最大账户数
    MaxRelatedAccounts int           // 最大关联账户数
}

// DetectionResult 检测结果
type DetectionResult struct {
    IsFraud    bool
    Rule       string
    RiskLevel  string
    Score      float64
    Details    string
}

// Detect 检测异常交易
func (fd *FraudDetector) Detect(tx Transaction) *DetectionResult {
    // 1. 大额交易检测
    if tx.Amount > fd.thresholds.MaxSingleTrade {
        return &DetectionResult{
            IsFraud:   true,
            Rule:      "LargeTransaction",
            RiskLevel: "High",
            Score:     0.8,
            Details:   fmt.Sprintf("Transaction amount %.2f exceeds threshold %.2f", tx.Amount, fd.thresholds.MaxSingleTrade),
        }
    }
    
    // 2. 高频交易检测
    if fd.isHighFrequency(tx) {
        return &DetectionResult{
            IsFraud:   true,
            Rule:      "HighFrequency",
            RiskLevel: "Medium",
            Score:     0.6,
            Details:   "High frequency trading detected",
        }
    }
    
    // 3. 关联交易检测
    if fd.isRelatedTransaction(tx) {
        return &DetectionResult{
            IsFraud:   true,
            Rule:      "RelatedTransaction",
            RiskLevel: "High",
            Score:     0.9,
            Details:   "Related transaction detected",
        }
    }
    
    return &DetectionResult{
        IsFraud: false,
        Score:   0.1,
    }
}

func (fd *FraudDetector) isHighFrequency(tx Transaction) bool {
    history := fd.userHistory[tx.UserID]
    recentCount := 0
    oneHourAgo := time.Now().Add(-1 * time.Hour)
    
    for _, h := range history {
        if h.Timestamp.After(oneHourAgo) {
            recentCount++
        }
    }
    
    return recentCount > fd.thresholds.MaxFrequency
}

func (fd *FraudDetector) isRelatedTransaction(tx Transaction) bool {
    // 检查同一 IP 下的账户数
    ipAccounts := make(map[string]int)
    for _, h := range fd.userHistory[tx.UserID] {
        ipAccounts[h.IP]++
    }
    
    for _, count := range ipAccounts {
        if count > fd.thresholds.MaxSameIPAccounts {
            return true
        }
    }
    
    return false
}
```

---

## 4. NFT 稀有度与热度

### 稀有度计算

```go
package nft

import (
    "math"
)

// NFTAttribute NFT 属性
type NFTAttribute struct {
    TraitType string
    Value     string
    Count     int // 拥有该属性的 NFT 数量
}

// RarityCalculator 稀有度计算器
type RarityCalculator struct {
    totalSupply int
}

// RarityScore 稀有度分数
type RarityScore struct {
    Overall  float64 // 综合稀有度 (0-100)
    Traits   map[string]float64 // 单属性稀有度
}

// Calculate 计算稀有度
func (rc *RarityCalculator) Calculate(attributes []NFTAttribute) *RarityScore {
    score := &RarityScore{
        Overall: 0,
        Traits: make(map[string]float64),
    }
    
    var traitScores []float64
    
    for _, attr := range attributes {
        // 单属性稀有度 = 1 - (拥有该属性的 NFT 数 / 总供给)
        traitRarity := 1.0 - float64(attr.Count)/float64(rc.totalSupply)
        score.Traits[attr.TraitType] = traitRarity * 100
        traitScores = append(traitScores, traitRarity)
    }
    
    // 综合稀有度：加权平均各属性稀有度
    if len(traitScores) > 0 {
        var sum float64
        for _, s := range traitScores {
            sum += s
        }
        score.Overall = (sum / float64(len(traitScores))) * 100
    }
    
    return score
}
```

### 热度评分模型

```go
package nft

import (
    "time"
)

// HeatMetrics 热度指标
type HeatMetrics struct {
    Activity       float64 // 链上活跃度 (0-1)
    Scarcity       float64 // 稀缺度 (0-1)
    Liquidity      float64 // 市场流动性 (0-1)
    NoAnomaly      float64 // 无异常交易 (0-1)
    RecentSales    int     // 近期销量
    AveragePrice   float64 // 平均价格
}

// HeatScoreModel 热度评分模型
type HeatScoreModel struct {
    Weights HeatWeights
}

type HeatWeights struct {
    Activity  float64 // 活跃度权重
    Scarcity  float64 // 稀缺度权重
    Liquidity float64 // 流动性权重
    NoAnomaly float64 // 无异常权重
}

func NewDefaultHeatScoreModel() *HeatScoreModel {
    return &HeatScoreModel{
        Weights: HeatWeights{
            Activity:  0.3,
            Scarcity:  0.25,
            Liquidity: 0.25,
            NoAnomaly: 0.2,
        },
    }
}

// Calculate 计算热度评分
func (hsm *HeatScoreModel) Calculate(metrics HeatMetrics) float64 {
    // 归一化各指标
    activityScore := metrics.Activity
    scarcityScore := metrics.Scarcity
    liquidityScore := metrics.Liquidity
    noAnomalyScore := metrics.NoAnomaly
    
    // 加权计算
    score := activityScore * hsm.Weights.Activity +
        scarcityScore * hsm.Weights.Scarcity +
        liquidityScore * hsm.Weights.Liquidity +
        noAnomalyScore * hsm.Weights.NoAnomaly
    
    // 映射到 0-100
    return score * 100
}

// CalculateWithDecay 带时间衰减的热度评分
func (hsm *HeatScoreModel) CalculateWithDecay(metrics HeatMetrics, lastUpdate time.Time) float64 {
    baseScore := hsm.Calculate(metrics)
    
    // 时间衰减因子
    hoursSinceUpdate := time.Since(lastUpdate).Hours()
    decayFactor := math.Exp(-0.01 * hoursSinceUpdate) // 每小时衰减 1%
    
    return baseScore * decayFactor
}
```

---

## 5. 学习建议

| 优先级 | 主题 | 学习时间 | 产出 |
|--------|------|---------|------|
| P0 | 中位数滤波 | 0.5 天 | 实现滤波器 |
| P0 | Z-Score/IQR 异常检测 | 0.5 天 | 实现异常剔除 |
| P0 | 加权融合 | 0.5 天 | 实现指数价格计算 |
| P1 | 异常交易检测 | 1 天 | 实现风控规则引擎 |
| P1 | 稀有度计算 | 0.5 天 | 实现 NFT 稀有度 |
| P1 | 热度评分 | 0.5 天 | 实现评分模型 |
| P2 | 机器学习入门 | 3 天 | 了解基础 ML 算法 |

---

## 6. 参考资源

| 资源 | 链接 | 说明 |
|------|------|------|
| 中位数滤波 | https://en.wikipedia.org/wiki/Median_filter | 维基百科 |
| Z-Score | https://en.wikipedia.org/wiki/Standard_score | 维基百科 |
| NFT 稀有度算法 | https://rarity.tools/ | 参考实现 |
| 风控规则引擎 | https://github.com/RulezKT/truelog | 开源实现 |
