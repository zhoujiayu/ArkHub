// ============================================================
// internal/market/fusion.go
// 加权融合引擎
// 职责：按权重融合多源价格，输出指数价格
// ============================================================

package market

import (
	"fmt"
	"math"
)

// SourcePrice 带权重的价格源
type SourcePrice struct {
	Name   string  // 数据源名称
	Price  float64 // 价格
	Weight float64 // 权重（归一化后 0~1）
}

// WeightedFusion 按权重计算加权平均价格
// 权重总和必须大于 0，否则返回错误
func WeightedFusion(sources []SourcePrice) (float64, error) {
	if len(sources) == 0 {
		return 0, fmt.Errorf("价格源为空")
	}

	var sum, weightSum float64
	for _, s := range sources {
		sum += s.Price * s.Weight
		weightSum += s.Weight
	}

	if weightSum == 0 {
		return 0, fmt.Errorf("权重总和为 0")
	}

	return sum / weightSum, nil
}

// WeightedFusionFromTicks 从 PriceTick 切片和权重映射直接计算加权融合
// 不做任何过滤，适合过滤后的数据直接传入
func WeightedFusionFromTicks(ticks []*PriceTick, weights map[string]float64) (float64, error) {
	if len(ticks) == 0 {
		return 0, fmt.Errorf("价格源为空")
	}

	var sources []SourcePrice
	for _, t := range ticks {
		weight := weights[t.Source]
		if weight > 0 {
			sources = append(sources, SourcePrice{
				Name:   t.Source,
				Price:  t.Price,
				Weight: weight,
			})
		}
	}

	return WeightedFusion(sources)
}

// WeightedFusionWithMedian 先中位数滤波，再按权重融合
// 适合多源数据质量参差不齐的场景，双重保障确保输出价格精准
func WeightedFusionWithMedian(ticks []*PriceTick, weights map[string]float64) (float64, error) {
	if len(ticks) == 0 {
		return 0, fmt.Errorf("价格源为空")
	}

	// 提取价格进行中位数滤波
	prices := make([]float64, 0, len(ticks))
	for _, t := range ticks {
		prices = append(prices, t.Price)
	}

	median := MedianFilter(prices)

	// 用中位数作为基准，剔除偏离过大的价格
	var validSources []SourcePrice
	for _, t := range ticks {
		// 偏离中位数 10% 以内的价格才参与加权
		if math.Abs(t.Price-median)/median <= 0.1 {
			weight := weights[t.Source]
			if weight > 0 {
				validSources = append(validSources, SourcePrice{
					Name:   t.Source,
					Price:  t.Price,
					Weight: weight,
				})
			}
		}
	}

	if len(validSources) == 0 {
		// 若全部偏离，退化为中位数
		return median, nil
	}

	return WeightedFusion(validSources)
}
