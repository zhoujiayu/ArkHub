// ============================================================
// internal/buyback/fallback.go
// 降级策略 — Redis 不可用时回查数据库
// 职责：先查缓存，缓存失败时回查 PostgreSQL
// ============================================================

package buyback

import (
	"context"
	"log"
	"time"
)

// FallbackStrategy 降级策略
type FallbackStrategy struct {
	cache CacheLayer
	job   *AggregationJob
}

// NewFallbackStrategy 创建降级策略
func NewFallbackStrategy(cache CacheLayer, job *AggregationJob) *FallbackStrategy {
	return &FallbackStrategy{cache: cache, job: job}
}

// GetStats 获取统计（带降级）
func (s *FallbackStrategy) GetStats(ctx context.Context, key string) (*BuybackStats, error) {
	// 1. 先尝试从缓存获取
	stats, err := s.cache.GetStats(ctx, key)
	if err == nil {
		return stats, nil
	}

	// 2. 缓存失败，回查数据库（默认最近24小时）
	log.Printf("缓存失败，降级查询数据库: %v", err)
	start := time.Now().Add(-24 * time.Hour)
	end := time.Now()
	return s.job.Aggregate(ctx, start, end)
}
