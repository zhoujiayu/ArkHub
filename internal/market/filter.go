// ============================================================
// internal/market/filter.go
// 中位数滤波器与异常剔除算法
// 职责：平滑多源价格数据，识别并剔除异常值
// ============================================================

package market

import (
	"math"
	"sort"
)

// MedianFilter 对价格序列进行中位数滤波
// 对异常值不敏感，极端价格不会影响最终结果
func MedianFilter(prices []float64) float64 {
	if len(prices) == 0 {
		return 0
	}

	// 复制切片，避免修改原始数据
	sorted := make([]float64, len(prices))
	copy(sorted, prices)
	sort.Float64s(sorted)

	n := len(sorted)
	if n%2 == 1 {
		return sorted[n/2]
	}
	return (sorted[n/2-1] + sorted[n/2]) / 2
}

// RemoveOutliers 使用 Z-Score 方法剔除异常价格
// 阈值默认为 3.0，即剔除偏离均值超过 3 个标准差的价格
func RemoveOutliers(prices []float64, threshold ...float64) []float64 {
	if len(prices) <= 2 {
		return prices
	}

	th := 3.0
	if len(threshold) > 0 && threshold[0] > 0 {
		th = threshold[0]
	}

	mean, std := calcMeanStd(prices)
	if std == 0 {
		return prices
	}

	var result []float64
	for _, p := range prices {
		if math.Abs((p-mean)/std) < th {
			result = append(result, p)
		}
	}
	return result
}

// FilterTicksByZScore 对 PriceTick 切片进行 Z-Score 异常剔除
// 返回 Z-Score 过滤后的 tick 列表，以及基于保留价格计算的中位数
func FilterTicksByZScore(ticks []*PriceTick, threshold ...float64) (filtered []*PriceTick, median float64) {
	if len(ticks) <= 2 {
		return ticks, 0
	}

	th := 3.0
	if len(threshold) > 0 && threshold[0] > 0 {
		th = threshold[0]
	}

	// 提取价格用于 Z-Score 计算
	prices := make([]float64, 0, len(ticks))
	for _, t := range ticks {
		prices = append(prices, t.Price)
	}

	mean, std := calcMeanStd(prices)
	if std == 0 {
		return ticks, mean
	}

	// Z-Score 过滤，保留正常的 tick
	for i, t := range ticks {
		if math.Abs((prices[i]-mean)/std) < th {
			filtered = append(filtered, t)
		}
	}

	// 基于保留的价格计算中位数
	if len(filtered) > 0 {
		keepPrices := make([]float64, len(filtered))
		for i, t := range filtered {
			keepPrices[i] = t.Price
		}
		median = MedianFilter(keepPrices)
	}

	return filtered, median
}

// FilterTicksByMedian 用中位数过滤 tick，只保留偏离中位数 <= maxDeviation 的 tick
// maxDeviation 为 0.1 表示偏离 <= 10%
func FilterTicksByMedian(ticks []*PriceTick, median float64, maxDeviation float64) []*PriceTick {
	if median == 0 || len(ticks) == 0 {
		return ticks
	}

	var result []*PriceTick
	for _, t := range ticks {
		if t.Price == 0 {
			continue
		}
		deviation := (t.Price - median) / median
		if math.Abs(deviation) <= maxDeviation {
			result = append(result, t)
		}
	}
	return result
}

// RemoveOutliersIQR 使用 IQR（四分位距）方法剔除异常价格
// 对极端异常值鲁棒，适合作为 Z-Score 的辅助验证手段
func RemoveOutliersIQR(prices []float64) []float64 {
	if len(prices) <= 4 {
		return prices
	}

	sorted := make([]float64, len(prices))
	copy(sorted, prices)
	sort.Float64s(sorted)

	q1 := percentile(sorted, 0.25)
	q3 := percentile(sorted, 0.75)
	iqr := q3 - q1
	lower := q1 - 1.5*iqr
	upper := q3 + 1.5*iqr

	var result []float64
	for _, p := range prices {
		if p >= lower && p <= upper {
			result = append(result, p)
		}
	}
	return result
}

// calcMeanStd 计算均值和标准差
func calcMeanStd(values []float64) (mean, std float64) {
	n := float64(len(values))
	var sum float64
	for _, v := range values {
		sum += v
	}
	mean = sum / n

	var variance float64
	for _, v := range values {
		diff := v - mean
		variance += diff * diff
	}
	std = math.Sqrt(variance / n)
	return
}

// percentile 计算有序切片的百分位数
func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	if len(sorted) == 1 {
		return sorted[0]
	}
	idx := p * float64(len(sorted)-1)
	lower := int(idx)
	upper := lower + 1
	if upper >= len(sorted) {
		return sorted[lower]
	}
	weight := idx - float64(lower)
	return sorted[lower]*(1-weight) + sorted[upper]*weight
}
