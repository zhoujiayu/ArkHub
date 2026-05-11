// ============================================================
// internal/middleware/ratelimit.go
// 限流中间件
// 职责：控制单位时间内的请求数量，防止系统被流量冲垮
// 算法：令牌桶（Token Bucket）
// ============================================================

package middleware

import (
	"net/http"
	"time"
)

// RateLimiter 定义限流器接口
// 不同限流策略（固定窗口、滑动窗口、令牌桶）实现此接口
type RateLimiter interface {
	// Allow 判断是否允许请求通过
	// 参数：
	//   key - 限流的标识（如 IP 地址、用户ID）
	// 返回：
	//   bool - true 允许通过，false 拒绝
	Allow(key string) bool
}

// TokenBucket 实现令牌桶限流算法
// 原理：
//   1. 系统以固定速率向桶中放入令牌
//   2. 每个请求需要消耗一个令牌
//   3. 桶满后不再放入令牌，桶空后请求被拒绝
// 优点：允许突发流量，同时限制平均速率
type TokenBucket struct {
	capacity   int           // 桶的容量（最多可容纳的令牌数）
	tokens     int           // 当前桶中的令牌数
	refillRate time.Duration // 令牌补充间隔（如每 100ms 补充一个）
	lastRefill time.Time     // 上次补充令牌的时间
}

// NewTokenBucket 创建令牌桶限流器
// 参数：
//   capacity   - 桶容量（如 100）
//   refillRate - 补充速率（如 100ms 补充一个）
// 返回：
//   *TokenBucket - 限流器实例
func NewTokenBucket(capacity int, refillRate time.Duration) *TokenBucket {
	return &TokenBucket{
		capacity:   capacity,
		tokens:     capacity, // 初始时桶是满的
		refillRate: refillRate,
		lastRefill: time.Now(),
	}
}

// Allow 判断是否允许请求通过
// 实现逻辑：
//   1. 计算距离上次补充的时间，补充相应数量的令牌
//   2. 如果桶中有令牌，消耗一个并返回 true
//   3. 如果桶中没有令牌，返回 false
func (tb *TokenBucket) Allow(key string) bool {
	// TODO: 实现线程安全的令牌桶逻辑
	// 当前为骨架，实际开发需使用 sync.Mutex 保证并发安全
	return true
}

// RateLimitMiddleware 返回一个 HTTP 限流中间件
// 使用方式：
//   rateLimiter := NewTokenBucket(100, 100*time.Millisecond)
//   http.Handle("/api/", RateLimitMiddleware(rateLimiter)(handler))
//
// 参数：
//   limiter - 限流器实例
// 返回：
//   func(http.Handler) http.Handler - 包装后的中间件
func RateLimitMiddleware(limiter RateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// 获取限流标识（优先使用用户ID， fallback 到 IP 地址）
			key := r.Header.Get("X-User-ID")
			if key == "" {
				key = r.RemoteAddr // 如果没有用户ID，使用 IP 地址
			}

			// 检查是否允许通过
			if !limiter.Allow(key) {
				// 请求被限流，返回 429 Too Many Requests
				w.WriteHeader(http.StatusTooManyRequests)
				w.Write([]byte("请求过于频繁，请稍后再试"))
				return
			}

			// 请求通过，继续处理
			next.ServeHTTP(w, r)
		})
	}
}
