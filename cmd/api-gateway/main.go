// ============================================================
// cmd/api-gateway/main.go
// API 网关服务入口
// 职责：接收所有 HTTP 请求，进行路由分发、鉴权、限流，然后转发到后端服务
// ============================================================

package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"arhub/internal/middleware"
	"arhub/internal/response"
)

// Prometheus 指标定义
var (
	httpRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "HTTP 请求总数",
		},
		[]string{"method", "path", "status"},
	)

	httpRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP 请求耗时（秒）",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path"},
	)
)

func init() {
	prometheus.MustRegister(httpRequestsTotal)
	prometheus.MustRegister(httpRequestDuration)
}

// 后端服务地址映射表
var serviceMap = map[string]string{
	"market":  "http://localhost:8082",
	"order":   "http://localhost:8081",
	"nft":     "http://localhost:8084",
	"buyback": "http://localhost:8085",
	"risk":    "http://localhost:8086",
	"chain":   "http://localhost:8083",
}

func main() {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()

	// 注册全局中间件
	r.Use(gin.Recovery())
	r.Use(prometheusMiddleware())

	// 初始化 Sentinel
	if err := middleware.InitSentinel(); err != nil {
		log.Fatalf("初始化 Sentinel 失败: %v", err)
	}

	// 注册 /metrics 端点
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))

	// 注册健康检查端点
	r.GET("/health", func(c *gin.Context) {
		response.JSON(c, gin.H{
			"status": "ok",
			"time":   time.Now().Format(time.RFC3339),
		})
	})

	// API 路由组（带鉴权和限流）
	api := r.Group("/api/v1")
	api.Use(middleware.AuthMiddleware("configs/public.pem"))
	api.Use(middleware.RateLimitMiddleware())
	api.Use(middleware.CircuitBreakerMiddleware())
	{
		api.Any("/market/*path", forwardTo("market"))
		api.Any("/order/*path", forwardTo("order"))
		api.Any("/nft/*path", forwardTo("nft"))
		api.Any("/buyback/*path", forwardTo("buyback"))
		api.Any("/risk/*path", forwardTo("risk"))
		api.Any("/chain/*path", forwardTo("chain"))
	}

	// 启动 HTTP 服务
	port := ":8080"
	srv := &http.Server{
		Addr:    port,
		Handler: r,
	}

	go func() {
		fmt.Printf("🚀 API Gateway 启动成功，监听端口 %s\n", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("API Gateway 启动失败: %v", err)
		}
	}()

	// 优雅关闭
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("API Gateway 关闭失败: %v", err)
	}
}

func prometheusMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		duration := time.Since(start).Seconds()
		method := c.Request.Method
		path := c.Request.URL.Path
		status := fmt.Sprintf("%d", c.Writer.Status())

		httpRequestsTotal.WithLabelValues(method, path, status).Inc()
		httpRequestDuration.WithLabelValues(method, path).Observe(duration)
	}
}

func forwardTo(serviceName string) gin.HandlerFunc {
	targetURL, ok := serviceMap[serviceName]
	if !ok {
		return func(c *gin.Context) {
			response.Error(c, http.StatusServiceUnavailable, response.CodeServiceUnavailable, fmt.Sprintf("服务 %s 不可用", serviceName))
		}
	}

	target, err := url.Parse(targetURL)
	if err != nil {
		return func(c *gin.Context) {
			response.Error(c, http.StatusInternalServerError, response.CodeInternal, "内部错误")
		}
	}

	proxy := httputil.NewSingleHostReverseProxy(target)
	return func(c *gin.Context) {
		// 修改请求路径，去掉 /api/v1/<service> 前缀
		path := c.Request.URL.Path
		path = strings.TrimPrefix(path, fmt.Sprintf("/api/v1/%s", serviceName))
		if path == "" {
			path = "/"
		}
		c.Request.URL.Path = path

		proxy.ServeHTTP(c.Writer, c.Request)
	}
}
