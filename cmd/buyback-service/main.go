// ============================================================
// cmd/buyback-service/main.go
// 回购统计服务入口
// 职责：定时聚合回购数据、Redis 缓存、降级策略、统计 API
// ============================================================

package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"arhub/internal/buyback"
	"arhub/internal/pkg/db"
)

func main() {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())

	// 初始化 PostgreSQL
	dbConn, err := db.NewDB(db.DefaultConfig())
	if err != nil {
		log.Fatalf("PostgreSQL 初始化失败: %v", err)
	}
	defer dbConn.Close()

	// 初始化 Redis 缓存
	cache := buyback.NewInMemoryCacheLayer()

	// 初始化聚合任务
	job := buyback.NewAggregationJob(dbConn, cache)

	// 初始化处理器
	handler := buyback.NewStatsHandler(cache, job)
	handler.RegisterRoutes(r)

	// 注册健康检查
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "time": time.Now().Format(time.RFC3339)})
	})

	// 启动定时聚合任务
	go startAggregationJob(job)

	// 启动 HTTP 服务
	port := ":8086"
	srv := &http.Server{Addr: port, Handler: r}

	go func() {
		fmt.Printf("回购统计服务启动成功，监听端口 %s\n", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("服务启动失败: %v", err)
		}
	}()

	// 优雅关闭
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	srv.Shutdown(ctx)
}

func startAggregationJob(job *buyback.AggregationJob) {
	// 立即执行一次
	if err := job.Run(context.Background()); err != nil {
		log.Printf("首次聚合失败: %v", err)
	}

	// 定时每5分钟执行
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		if err := job.Run(context.Background()); err != nil {
			log.Printf("聚合失败: %v", err)
		}
	}
}
