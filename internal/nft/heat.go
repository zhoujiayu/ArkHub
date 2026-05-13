// ============================================================
// internal/nft/heat.go
// 热度评分模型 — 多维度计算 NFT 热度
// 职责：链上活跃度 + 稀缺度 + 市场流动性 + 无异常交易，综合评分
// ============================================================

package nft

// HeatScoreModel 热度评分模型
type HeatScoreModel struct {
	weights HeatWeights
}

// HeatWeights 热度权重
type HeatWeights struct {
	Activity  float64 // 链上活跃度 30%
	Scarcity  float64 // 稀缺度 25%
	Liquidity float64 // 市场流动性 25%
	NoAnomaly float64 // 无异常交易 20%
}

// HeatMetrics 热度指标
type HeatMetrics struct {
	Activity   float64 // 交易量、转账次数
	Scarcity   float64 // 总供给量、持有者分布
	Liquidity  float64 // 挂单量、成交速度
	NoAnomaly  float64 // 无刷量检测得分
}

// HeatUpdate 热度更新
type HeatUpdate struct {
	TokenID string
	Score   float64
	Metrics HeatMetrics
}

// DefaultHeatWeights 默认权重
func DefaultHeatWeights() HeatWeights {
	return HeatWeights{
		Activity:  0.30,
		Scarcity:  0.25,
		Liquidity: 0.25,
		NoAnomaly: 0.20,
	}
}

// NewHeatScoreModel 创建热度评分模型
func NewHeatScoreModel(weights HeatWeights) *HeatScoreModel {
	return &HeatScoreModel{weights: weights}
}

// Calculate 计算热度评分
func (m *HeatScoreModel) Calculate(metrics HeatMetrics) float64 {
	score := metrics.Activity*m.weights.Activity +
		metrics.Scarcity*m.weights.Scarcity +
		metrics.Liquidity*m.weights.Liquidity +
		metrics.NoAnomaly*m.weights.NoAnomaly
	return score
}

// HeatPushService 热度推送服务
type HeatPushService struct {
	updates chan HeatUpdate
}

// NewHeatPushService 创建推送服务
func NewHeatPushService() *HeatPushService {
	return &HeatPushService{updates: make(chan HeatUpdate, 1000)}
}

// Push 推送热度更新
func (s *HeatPushService) Push(update HeatUpdate) {
	select {
	case s.updates <- update:
	default:
	}
}

// Updates 返回更新通道
func (s *HeatPushService) Updates() <-chan HeatUpdate {
	return s.updates
}
