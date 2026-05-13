// ============================================================
// internal/buyback/aggregator.go
// 回购聚合计算 — 定时统计回购数据
// 职责：按时间维度聚合回购量、金额、趋势
// ============================================================

package buyback

import (
	"context"
	"database/sql"
	"time"
)

// AggregationJob 定时聚合任务
type AggregationJob struct {
	db    *sql.DB
	cache CacheLayer
}

// BuybackStats 回购统计
type BuybackStats struct {
	TotalCount int64       `json:"total_count"`
	TotalAmount float64    `json:"total_amount"`
	AvgAmount   float64     `json:"avg_amount"`
	MaxAmount   float64     `json:"max_amount"`
	MinAmount   float64     `json:"min_amount"`
	DailyTrend  []DailyStat `json:"daily_trend"`
}

// DailyStat 每日统计
type DailyStat struct {
	Date   string  `json:"date"`
	Count  int64   `json:"count"`
	Amount float64 `json:"amount"`
}

// NewAggregationJob 创建聚合任务
func NewAggregationJob(db *sql.DB, cache CacheLayer) *AggregationJob {
	return &AggregationJob{db: db, cache: cache}
}

// Run 执行聚合
func (j *AggregationJob) Run(ctx context.Context) error {
	start := time.Now().Add(-24 * time.Hour)
	end := time.Now()

	stats, err := j.Aggregate(ctx, start, end)
	if err != nil {
		return err
	}

	// 写入缓存
	return j.cache.SetStats(ctx, "buyback:stats:latest", stats)
}

// Aggregate 全量聚合计算
func (j *AggregationJob) Aggregate(ctx context.Context, start, end time.Time) (*BuybackStats, error) {
	query := `
		SELECT
			COUNT(*) as total_count,
			COALESCE(SUM(amount), 0) as total_amount,
			COALESCE(AVG(amount), 0) as avg_amount,
			COALESCE(MAX(amount), 0) as max_amount,
			COALESCE(MIN(amount), 0) as min_amount
		FROM buyback_records
		WHERE created_at BETWEEN $1 AND $2
	`

	var stats BuybackStats
	err := j.db.QueryRowContext(ctx, query, start, end).Scan(
		&stats.TotalCount, &stats.TotalAmount, &stats.AvgAmount, &stats.MaxAmount, &stats.MinAmount,
	)
	if err != nil {
		return nil, err
	}

	// 每日趋势
	trendQuery := `
		SELECT DATE(created_at), COUNT(*), SUM(amount)
		FROM buyback_records
		WHERE created_at BETWEEN $1 AND $2
		GROUP BY DATE(created_at)
		ORDER BY DATE(created_at) DESC
	`
	rows, err := j.db.QueryContext(ctx, trendQuery, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var stat DailyStat
		if err := rows.Scan(&stat.Date, &stat.Count, &stat.Amount); err != nil {
			continue
		}
		stats.DailyTrend = append(stats.DailyTrend, stat)
	}

	return &stats, nil
}
