// ============================================================
// cmd/chain-sync/main.go
// 链上链下一致性服务入口
// 职责：启动双源校验器、异步补偿任务、事件同步引擎、数据校准器
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

	"arhub/internal/chain"
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

	// 初始化链上客户端
	chainClient := chain.NewEthereumClient("https://mainnet.infura.io/v3/YOUR_KEY")

	// 初始化各组件
	validator := chain.NewDualSourceValidator(chainClient, dbConn)
	compensation := chain.NewCompensationJob(dbConn, chainClient)
	syncEngine := chain.NewEventSyncEngine(chainClient)
	reconciler := chain.NewDataReconciler(dbConn)

	// 注册路由
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "time": time.Now().Format(time.RFC3339)})
	})
	r.POST("/api/v1/validate", validateHandler(validator))
	r.POST("/api/v1/compensate", compensateHandler(compensation))
	r.GET("/api/v1/reconcile", reconcileHandler(reconciler))

	// 启动 HTTP 服务
	port := ":8084"
	srv := &http.Server{Addr: port, Handler: r}

	// 启动定时补偿任务
	go startCompensationJob(compensation)
	go startEventSync(syncEngine)

	go func() {
		fmt.Printf("链上链下一致性服务启动成功，监听端口 %s\n", port)
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

func validateHandler(v *chain.DualSourceValidator) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Address string `json:"address" binding:"required"`
			Amount  string `json:"amount" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		err := v.ValidateBeforeAction(c.Request.Context(), chain.Action{
			Type:    "transfer",
			Address: req.Address,
			Amount:  req.Amount,
		})
		if err != nil {
			c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "valid"})
	}
}

func compensateHandler(job *chain.CompensationJob) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := job.Run(c.Request.Context()); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "compensation triggered"})
	}
}

func reconcileHandler(r *chain.DataReconciler) gin.HandlerFunc {
	return func(c *gin.Context) {
		report := chain.DiscrepancyReport{Items: make([]chain.ReportItem, 0)}
		if err := r.Reconcile(c.Request.Context(), report); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "reconciled"})
	}
}

func startCompensationJob(job *chain.CompensationJob) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		if err := job.Run(context.Background()); err != nil {
			log.Printf("补偿任务失败: %v", err)
		}
	}
}

func startEventSync(engine *chain.EventSyncEngine) {
	ctx := context.Background()
	engine.Start(ctx)
}
