// ============================================================
// internal/buyback/handler.go
// 统计 API — 高性能查询接口
// 职责：先查 Redis，缓存失败时降级到数据库
// ============================================================

package buyback

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// StatsHandler 统计接口处理器
type StatsHandler struct {
	cache    CacheLayer
	job      *AggregationJob
	fallback *FallbackStrategy
}

// NewStatsHandler 创建处理器
func NewStatsHandler(cache CacheLayer, job *AggregationJob) *StatsHandler {
	return &StatsHandler{
		cache:    cache,
		job:      job,
		fallback: NewFallbackStrategy(cache, job),
	}
}

// RegisterRoutes 注册路由
func (h *StatsHandler) RegisterRoutes(r *gin.Engine) {
	r.GET("/api/v1/buyback/stats", h.GetStats)
	r.POST("/api/v1/buyback/refresh", h.RefreshStats)
}

// GetStats 获取统计
func (h *StatsHandler) GetStats(c *gin.Context) {
	ctx := c.Request.Context()
	key := "buyback:stats:latest"

	// 先尝试从缓存获取
	stats, err := h.cache.GetStats(ctx, key)
	if err != nil {
		// 缓存失败，降级到数据库
		stats, err = h.fallback.GetStats(ctx, key)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	c.JSON(http.StatusOK, stats)
}

// RefreshStats 手动刷新统计
func (h *StatsHandler) RefreshStats(c *gin.Context) {
	ctx := c.Request.Context()

	if err := h.job.Run(ctx); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "refreshed"})
}
