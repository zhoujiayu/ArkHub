// ============================================================
// cmd/risk-service/main.go
// 风控服务入口（占位实现）
// 职责：实时风控、异常检测（待实现）
// ============================================================

package main

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

func main() {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())

	// 健康检查
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status": "ok",
			"time":   time.Now().Format(time.RFC3339),
		})
	})

	port := ":8089"
	fmt.Printf("🚀 Risk Service（占位）启动成功，监听端口 %s\n", port)
	if err := r.Run(port); err != nil {
		log.Fatalf("Risk Service 启动失败: %v", err)
	}
}
