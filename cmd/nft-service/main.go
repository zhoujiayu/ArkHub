// ============================================================
// cmd/nft-service/main.go
// NFT 业务服务入口
// 职责：启动元数据拉取、稀有度计算、热度评分、异常检测、实时推送
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

	"arhub/internal/nft"
)

func main() {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())

	// 初始化组件
	ipfsGateway := nft.NewIPFSGateway("https://ipfs.io")
	parser := nft.NewMetadataParser()
	fraudDetector := nft.NewFraudDetector(nft.DefaultFraudThresholds())
	heatModel := nft.NewHeatScoreModel(nft.DefaultHeatWeights())
	heatPush := nft.NewHeatPushService()

	// 注册路由
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "time": time.Now().Format(time.RFC3339)})
	})
	r.GET("/api/v1/nft/metadata", metadataHandler(ipfsGateway, parser))
	r.POST("/api/v1/nft/rarity", rarityHandler())
	r.POST("/api/v1/nft/heat", heatHandler(heatModel))
	r.POST("/api/v1/nft/fraud-detect", fraudDetectHandler(fraudDetector))
	r.GET("/api/v1/nft/heat-updates", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"updates": []nft.HeatUpdate{}})
	})

	// 启动热度推送 goroutine
	go startHeatPusher(heatPush)

	// 启动 HTTP 服务
	port := ":8085"
	srv := &http.Server{Addr: port, Handler: r}

	go func() {
		fmt.Printf("NFT 业务服务启动成功，监听端口 %s\n", port)
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

func metadataHandler(gateway *nft.IPFSGateway, parser *nft.MetadataParser) gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenURI := c.Query("token_uri")
		if tokenURI == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "token_uri is required"})
			return
		}

		meta, err := gateway.FetchMetadata(c.Request.Context(), tokenURI)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, meta)
	}
}

func rarityHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			TokenID     string `json:"token_id" binding:"required"`
			TotalSupply int    `json:"total_supply" binding:"required,gt=0"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		calculator := nft.NewRarityCalculator(req.TotalSupply)
		score := calculator.Calculate(&nft.ParsedMetadata{}, make(map[string]map[interface{}]int))

		c.JSON(http.StatusOK, score)
	}
}

func heatHandler(model *nft.HeatScoreModel) gin.HandlerFunc {
	return func(c *gin.Context) {
		var metrics nft.HeatMetrics
		if err := c.ShouldBindJSON(&metrics); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		score := model.Calculate(metrics)
		c.JSON(http.StatusOK, gin.H{"heat_score": score})
	}
}

func fraudDetectHandler(detector *nft.FraudDetector) gin.HandlerFunc {
	return func(c *gin.Context) {
		var transactions []nft.Transaction
		if err := c.ShouldBindJSON(&transactions); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		report := detector.Detect(transactions)
		c.JSON(http.StatusOK, report)
	}
}

func startHeatPusher(service *nft.HeatPushService) {
	for update := range service.Updates() {
		log.Printf("热度更新: Token=%s, Score=%.2f", update.TokenID, update.Score)
	}
}
