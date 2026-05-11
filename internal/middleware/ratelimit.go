// ============================================================
// internal/middleware/ratelimit.go
// 限流熔断中间件
// 职责：基于 Sentinel-Go 实现 IP 限流、用户限流、接口限流和熔断
// ============================================================

package middleware

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	sentinel "github.com/alibaba/sentinel-golang/api"
	"github.com/alibaba/sentinel-golang/core/base"
	"github.com/alibaba/sentinel-golang/core/circuitbreaker"
	"github.com/alibaba/sentinel-golang/core/flow"

	"arhub/internal/response"
)

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

	// 熔断：错误率 > 50% 持续 30 秒
	_, err = circuitbreaker.LoadRules([]*circuitbreaker.Rule{
		{
			Resource:         "api_circuit_breaker",
			Strategy:         circuitbreaker.ErrorRatio,
			Threshold:        0.5,
			RetryTimeoutMs:   30000,
			StatIntervalMs:   30000,
			MinRequestAmount: 10,
		},
	})
	if err != nil {
		return err
	}

	return nil
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

// CircuitBreakerMiddleware 返回熔断中间件
func CircuitBreakerMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		e, b := sentinel.Entry("api_circuit_breaker", sentinel.WithTrafficType(base.Inbound))
		if b != nil {
			response.Error(c, http.StatusServiceUnavailable, response.CodeServiceUnavailable, "服务暂不可用，请稍后再试")
			c.Abort()
			return
		}
		defer e.Exit()

		c.Next()

		// 根据响应状态码记录错误
		if c.Writer.Status() >= 500 {
				sentinel.TraceError(e, errors.New("server error"))
		}
	}
}
