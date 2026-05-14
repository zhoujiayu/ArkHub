// ============================================================
// internal/middleware/ratelimit.go
// 限流熔断中间件
// 职责：基于 Sentinel-Go 实现 IP 限流、用户限流、接口限流和按服务隔离的熔断
// ============================================================

package middleware

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	sentinel "github.com/alibaba/sentinel-golang/api"
	"github.com/alibaba/sentinel-golang/core/base"
	"github.com/alibaba/sentinel-golang/core/circuitbreaker"
	"github.com/alibaba/sentinel-golang/core/flow"

	"arhub/internal/response"
)

// 服务名称映射：从 URL 路径提取服务名
// 例如：/api/v1/market/price → market
var serviceMap = map[string]string{
	"market":  "market-data",
	"order":   "matching-engine",
	"nft":     "nft-service",
	"buyback": "buyback-service",
	"risk":    "risk-service",
	"chain":   "chain-sync",
}

// InitSentinel 初始化 Sentinel 规则
func InitSentinel() error {
	// 初始化 Sentinel
	err := sentinel.InitDefault()
	if err != nil {
		return err
	}

	// IP 限流：60 次/分钟
	_, err = flow.LoadRules([]*flow.Rule{
		{
			Resource:               "ip_limit",
			Threshold:              60,
			TokenCalculateStrategy: flow.Direct,
			ControlBehavior:        flow.Reject,
			StatIntervalInMs:       60000,
		},
	})
	if err != nil {
		return err
	}

	// 用户限流：100 次/分钟
	_, err = flow.LoadRules([]*flow.Rule{
		{
			Resource:               "user_limit",
			Threshold:              100,
			TokenCalculateStrategy: flow.Direct,
			ControlBehavior:        flow.Reject,
			StatIntervalInMs:       60000,
		},
	})
	if err != nil {
		return err
	}

	// 接口限流：1000 QPS
	_, err = flow.LoadRules([]*flow.Rule{
		{
			Resource:               "api_limit",
			Threshold:              1000,
			TokenCalculateStrategy: flow.Direct,
			ControlBehavior:        flow.Reject,
			StatIntervalInMs:       1000,
		},
	})
	if err != nil {
		return err
	}

	// 按服务隔离的熔断规则：每个下游服务独立熔断
	// 当某个服务错误率过高时，只熔断该服务，不影响其他服务
	for _, serviceName := range serviceMap {
		err := initCircuitBreakerRule(serviceName)
		if err != nil {
			return fmt.Errorf("初始化 %s 熔断规则失败: %w", serviceName, err)
		}
	}

	return nil
}

// initCircuitBreakerRule 为单个服务初始化熔断规则
func initCircuitBreakerRule(serviceName string) error {
	resourceName := fmt.Sprintf("circuit:%s", serviceName)
	_, err := circuitbreaker.LoadRules([]*circuitbreaker.Rule{
		{
			Resource:         resourceName,
			Strategy:         circuitbreaker.ErrorRatio,
			Threshold:        0.5,     // 错误率 50%
			RetryTimeoutMs:   30000,   // 30秒后尝试恢复
			StatIntervalMs:   30000,   // 统计窗口
			MinRequestAmount: 10,      // 最小请求数
		},
	})
	return err
}

// RateLimitMiddleware 返回限流中间件
func RateLimitMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// IP 限流
		ip := c.ClientIP()
		if e, b := sentinel.Entry("ip_limit", sentinel.WithArgs(ip)); b != nil {
			response.Error(c, http.StatusTooManyRequests, response.CodeRateLimited, "请求过于频繁，请稍后再试")
			c.Abort()
			return
		} else {
			defer e.Exit()
		}

		// 用户限流
		userID := c.GetString("user_id")
		if userID != "" {
			if e, b := sentinel.Entry("user_limit", sentinel.WithArgs(userID)); b != nil {
				response.Error(c, http.StatusTooManyRequests, response.CodeRateLimited, "请求过于频繁，请稍后再试")
				c.Abort()
				return
			} else {
				defer e.Exit()
			}
		}

		// 接口限流
		if e, b := sentinel.Entry("api_limit", sentinel.WithArgs(c.Request.URL.Path)); b != nil {
			response.Error(c, http.StatusTooManyRequests, response.CodeRateLimited, "请求过于频繁，请稍后再试")
			c.Abort()
			return
		} else {
			defer e.Exit()
		}

		c.Next()
	}
}

// CircuitBreakerMiddleware 返回按服务隔离的熔断中间件
// 根据请求路径识别目标服务，每个服务独立熔断
func CircuitBreakerMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 从路径提取服务名，例如 /api/v1/market/price → market
		serviceName := extractServiceName(c.Request.URL.Path)
		if serviceName == "" {
			// 无法识别服务，放行
			c.Next()
			return
		}

		resourceName := fmt.Sprintf("circuit:%s", serviceName)
		e, b := sentinel.Entry(resourceName, sentinel.WithTrafficType(base.Inbound))
		if b != nil {
			response.Error(c, http.StatusServiceUnavailable, response.CodeServiceUnavailable,
				fmt.Sprintf("服务 %s 暂不可用，请稍后再试", serviceName))
			c.Abort()
			return
		}
		defer e.Exit()

		c.Next()

		// 根据响应状态码记录错误（下游服务返回 500/502/503/504）
		if c.Writer.Status() >= 500 {
			sentinel.TraceError(e, errors.New("server error"))
		}
	}
}

// extractServiceName 从请求路径提取服务名
// 例如：/api/v1/market/price → market
//      /api/v1/order/submit → order
func extractServiceName(path string) string {
	// 去掉 /api/v1/ 前缀
	prefix := "/api/v1/"
	if !strings.HasPrefix(path, prefix) {
		return ""
	}

	// 提取服务名（第一个路径段）
	afterPrefix := strings.TrimPrefix(path, prefix)
	parts := strings.SplitN(afterPrefix, "/", 2)
	if len(parts) == 0 || parts[0] == "" {
		return ""
	}

	service := parts[0]
	// 验证是否为有效的服务名
	if _, ok := serviceMap[service]; ok {
		return service
	}
	return ""
}
