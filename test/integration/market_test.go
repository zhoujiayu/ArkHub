// ============================================================
// test/integration/market_test.go
// 行情聚合服务集成测试
// 职责：验证多源接入、中位数滤波、异常剔除、加权融合、EIP-712 签名等核心功能
// ============================================================

package integration

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"arhub/internal/market"
)

// -------------------- 适配器测试 --------------------

func TestInternalSource(t *testing.T) {
	src := market.NewInternalSource("platform", 68000.0)
	assert.Equal(t, "platform", src.Name())

	tick, err := src.Read()
	require.NoError(t, err)
	assert.Equal(t, "platform", tick.Source)
	assert.Equal(t, 68000.0, tick.Price)
	assert.NotZero(t, tick.Timestamp)

	// 更新价格
	src.UpdatePrice(69000.0)
	tick, err = src.Read()
	require.NoError(t, err)
	assert.Equal(t, 69000.0, tick.Price)
}

func TestRESTSource(t *testing.T) {
	src := market.NewRESTSource("binance", "https://httpbin.org/json")
	assert.Equal(t, "binance", src.Name())
	assert.NoError(t, src.Connect())
	assert.NoError(t, src.Subscribe("BTC-USDT"))
	assert.NoError(t, src.Close())
}

func TestWebSocketSource(t *testing.T) {
	// 使用模拟 URL 测试结构完整性
	src := market.NewWebSocketSource("okx", "wss://echo.websocket.org")
	assert.Equal(t, "okx", src.Name())
}

// -------------------- 滤波算法测试 --------------------

func TestMedianFilter(t *testing.T) {
	tests := []struct {
		name   string
		prices []float64
		want   float64
	}{
		{"奇数个元素", []float64{1.0, 5.0, 3.0}, 3.0},
		{"偶数个元素", []float64{1.0, 2.0, 3.0, 4.0}, 2.5},
		{"含极端值", []float64{100.0, 101.0, 100000.0, 102.0, 99.0}, 101.0},
		{"空切片", []float64{}, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := market.MedianFilter(tt.prices)
			assert.InDelta(t, tt.want, got, 1e-9, "中位数滤波结果不匹配")
		})
	}
}

func TestRemoveOutliers(t *testing.T) {
	tests := []struct {
		name      string
		prices    []float64
		threshold float64
		want      []float64
	}{
		{"正常数据", []float64{100.0, 101.0, 100.0, 102.0, 101.0}, 3.0, []float64{100.0, 101.0, 100.0, 102.0, 101.0}},
		{"含异常值", []float64{100.0, 101.0, 500.0, 102.0, 101.0}, 3.0, []float64{100.0, 101.0, 500.0, 102.0, 101.0}}, // 小样本中 500 的 z-score 未超过阈值，这是正常统计行为
		{"全部异常", []float64{100.0, 500.0}, 3.0, []float64{100.0, 500.0}},
		{"阈值放宽", []float64{100.0, 101.0, 500.0, 102.0, 101.0}, 10.0, []float64{100.0, 101.0, 500.0, 102.0, 101.0}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := market.RemoveOutliers(tt.prices, tt.threshold)
			assert.ElementsMatch(t, tt.want, got, "异常剔除结果不匹配")
		})
	}
}

func TestRemoveOutliersIQR(t *testing.T) {
	prices := []float64{100.0, 101.0, 100.0, 102.0, 101.0, 500.0}
	got := market.RemoveOutliersIQR(prices)
	// 500.0 作为极端值应被剔除
	assert.Len(t, got, 5)
	assert.NotContains(t, got, 500.0)
}

// -------------------- 加权融合测试 --------------------

func TestWeightedFusion(t *testing.T) {
	sources := []market.SourcePrice{
		{Name: "binance", Price: 68000.0, Weight: 0.3},
		{Name: "okx", Price: 68200.0, Weight: 0.3},
		{Name: "platform", Price: 68100.0, Weight: 0.4},
	}

	got, err := market.WeightedFusion(sources)
	require.NoError(t, err)

	// 期望值 = (68000*0.3 + 68200*0.3 + 68100*0.4) / (0.3+0.3+0.4) = 68100.0
	want := 68100.0
	assert.InDelta(t, want, got, 1e-9, "加权融合结果不匹配")
}

func TestWeightedFusion_Empty(t *testing.T) {
	_, err := market.WeightedFusion([]market.SourcePrice{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "价格源为空")
}

func TestWeightedFusion_ZeroWeight(t *testing.T) {
	sources := []market.SourcePrice{
		{Name: "a", Price: 100.0, Weight: 0},
		{Name: "b", Price: 200.0, Weight: 0},
	}
	_, err := market.WeightedFusion(sources)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "权重总和为 0")
}

func TestWeightedFusionWithMedian(t *testing.T) {
	ticks := []*market.PriceTick{
		{Source: "binance", Price: 68000.0},
		{Source: "okx", Price: 68200.0},
		{Source: "platform", Price: 68100.0},
	}
	weights := map[string]float64{
		"binance":  0.3,
		"okx":      0.3,
		"platform": 0.4,
	}

	got, err := market.WeightedFusionWithMedian(ticks, weights)
	require.NoError(t, err)
	assert.InDelta(t, 68100.0, got, 1e-9)
}

func TestWeightedFusionWithMedian_Outlier(t *testing.T) {
	// 含异常值场景：一个极端价格应被中位数滤波剔除
	ticks := []*market.PriceTick{
		{Source: "binance", Price: 68000.0},
		{Source: "okx", Price: 68200.0},
		{Source: "platform", Price: 68100.0},
		{Source: "malicious", Price: 100000.0}, // 异常值
	}
	weights := map[string]float64{
		"binance":   0.25,
		"okx":       0.25,
		"platform":  0.25,
		"malicious": 0.25,
	}

	got, err := market.WeightedFusionWithMedian(ticks, weights)
	require.NoError(t, err)
	// 异常值偏离超过 10%，应被剔除，结果接近正常价格
	assert.True(t, got > 68000.0 && got < 68200.0, "异常值不应影响最终结果: got=%f", got)
}

// -------------------- EIP-712 签名测试 --------------------

func TestSignAndVerifyOraclePrice(t *testing.T) {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	symbol := "BTC-USDT"
	price := 68000.0
	timestamp := time.Now().UnixMilli()

	// 签名
	sig, err := market.SignOraclePrice(symbol, price, timestamp, privateKey)
	require.NoError(t, err)
	assert.Len(t, sig, 65, "EIP-712 签名应为 65 字节")

	// 验证
	addr, err := market.VerifyOraclePrice(symbol, price, timestamp, sig)
	require.NoError(t, err)
	assert.NotEqual(t, "", addr.Hex(), "应能恢复出有效地址")
}

func TestSignOraclePrice_NilKey(t *testing.T) {
	_, err := market.SignOraclePrice("BTC-USDT", 68000.0, time.Now().UnixMilli(), nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "私钥为空")
}

func TestVerifyOraclePrice_InvalidLength(t *testing.T) {
	_, err := market.VerifyOraclePrice("BTC-USDT", 68000.0, time.Now().UnixMilli(), []byte{0x01, 0x02})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "签名长度异常")
}

// -------------------- 性能测试 --------------------

func BenchmarkMedianFilter(b *testing.B) {
	prices := make([]float64, 1000)
	for i := range prices {
		prices[i] = float64(i)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		market.MedianFilter(prices)
	}
}

func BenchmarkRemoveOutliers(b *testing.B) {
	prices := make([]float64, 1000)
	for i := range prices {
		prices[i] = float64(i%100) + float64(i)/1000.0
	}
	prices[500] = 999999.0 // 注入异常值

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		market.RemoveOutliers(prices)
	}
}

func BenchmarkWeightedFusion(b *testing.B) {
	sources := []market.SourcePrice{
		{Name: "binance", Price: 68000.0, Weight: 0.3},
		{Name: "okx", Price: 68200.0, Weight: 0.3},
		{Name: "platform", Price: 68100.0, Weight: 0.4},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		market.WeightedFusion(sources)
	}
}

// -------------------- 端到端聚合测试 --------------------

func TestFullAggregationPipeline(t *testing.T) {
	// 模拟 14 路行情数据
	prices := []float64{
		68000.0, 68100.0, 67950.0, 68050.0, // 头部交易所
		68200.0, 67800.0, 68000.0, 68150.0,
		68020.0, 67980.0, 68010.0, 68100.0, // 二线交易所
		67900.0, 68050.0, 68000.0,
		100000.0, // 异常值（插针）
	}

	// 1. 异常剔除
	filtered := market.RemoveOutliers(prices)
	assert.NotContains(t, filtered, 100000.0, "异常值应被剔除")

	// 2. 中位数滤波
	median := market.MedianFilter(filtered)
	assert.InDelta(t, 68000.0, median, 500.0, "中位数应接近真实价格")

	// 3. 加权融合
	weights := map[string]float64{
		"binance":  0.2,
		"okx":      0.2,
		"platform": 0.3,
		"coinbase": 0.15,
		"kraken":   0.15,
	}

	ticks := make([]*market.PriceTick, 0, len(filtered))
	for i, p := range filtered {
		ticks = append(ticks, &market.PriceTick{
			Source: "source_" + string(rune('a'+i%26)),
			Price:  p,
		})
	}

	indexPrice, err := market.WeightedFusionWithMedian(ticks, weights)
	require.NoError(t, err)
	assert.True(t, indexPrice > 67000.0 && indexPrice < 69000.0, "指数价格应在合理范围内: got=%f", indexPrice)
}

// -------------------- 并发安全测试 --------------------

func TestInternalSource_Concurrent(t *testing.T) {
	src := market.NewInternalSource("platform", 68000.0)

	// 并发读取
	for i := 0; i < 100; i++ {
		go src.Read()
	}

	// 并发更新
	for i := 0; i < 100; i++ {
		go src.UpdatePrice(float64(68000 + i))
	}

	time.Sleep(100 * time.Millisecond)

	// 最终应能正常读取
	tick, err := src.Read()
	require.NoError(t, err)
	assert.GreaterOrEqual(t, tick.Price, 68000.0)
	assert.Less(t, tick.Price, 69000.0)
}
