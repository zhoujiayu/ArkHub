// ============================================================
// cmd/market-data/main.go
// 行情聚合服务入口
// 职责：多源行情接入、中位数滤波、异常剔除、加权融合、EIP-712 签名、多通道输出
// ============================================================

package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"arhub/internal/market"
)

var (
	// upgrader WebSocket 连接升级器
	upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}

	// clients WebSocket 客户端连接池
	clients   = make(map[*websocket.Conn]bool)
	clientsMu sync.RWMutex

	// latestIndexPrice 缓存最新指数价格
	latestIndexPrice float64
	priceMu          sync.RWMutex

	// privateKey EIP-712 签名用私钥（生产环境应从 KMS 读取）
	privateKey *ecdsa.PrivateKey
)

func main() {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())

	// 生成签名私钥（测试用，生产环境使用 KMS）
	var err error
	privateKey, err = generateTestKey()
	if err != nil {
		log.Fatalf("生成签名私钥失败: %v", err)
	}

	// 启动行情聚合引擎
	go aggregationEngine()

	// 注册路由
	r.GET("/health", healthHandler)
	r.GET("/ws", wsHandler)
	r.GET("/api/v1/index-price", indexPriceHandler)
	r.GET("/api/v1/sources", sourcesHandler)

	// 启动 HTTP 服务
	port := ":8082"
	srv := &http.Server{
		Addr:    port,
		Handler: r,
	}

	go func() {
		fmt.Printf("行情聚合服务启动成功，监听端口 %s\n", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("行情聚合服务启动失败: %v", err)
		}
	}()

	// 优雅关闭
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("行情聚合服务关闭失败: %v", err)
	}
}

// -------------------- 行情聚合引擎 --------------------

// aggregationEngine 定时聚合行情数据
// 每 500ms 执行一次：收集多源价格 → Z-Score 异常剔除 → 中位数滤波 → 加权融合 → EIP-712 签名 → 多通道推送
func aggregationEngine() {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	// 初始化模拟数据源
	sources := []market.MarketDataSource{
		market.NewRESTSource("binance", "https://api.binance.com/api/v3/ticker/price?symbol=BTCUSDT"),
		market.NewRESTSource("okx", "https://www.okx.com/api/v5/market/ticker?instId=BTC-USDT"),
		market.NewInternalSource("platform", 68000.0),
	}

	weights := map[string]float64{
		"binance":  0.3,
		"okx":      0.3,
		"platform": 0.4,
	}

	for range ticker.C {
		// 1. 收集多源价格
		ticks := collectPrices(sources)

		// 2. Z-Score 异常剔除，返回过滤后的 tick 和中位数
		zscoreFiltered, median := market.FilterTicksByZScore(ticks)

		// 3. 中位数二次过滤（偏离中位数 <= 10% 的价格才参与融合）
		validTicks := market.FilterTicksByMedian(zscoreFiltered, median, 0.1)

		// 4. 加权融合
		indexPrice, err := market.WeightedFusionFromTicks(validTicks, weights)
		if err != nil {
			log.Printf("加权融合失败: %v", err)
			continue
		}

		// 5. EIP-712 签名
		timestamp := time.Now().UnixMilli()
		sig, err := market.SignOraclePrice("BTC-USDT", indexPrice, timestamp, privateKey)
		if err != nil {
			log.Printf("签名失败: %v", err)
			continue
		}

		// 6. 缓存最新价格
		priceMu.Lock()
		latestIndexPrice = indexPrice
		priceMu.Unlock()

		// 7. 多通道推送
		data := map[string]interface{}{
			"symbol":    "BTC-USDT",
			"price":     indexPrice,
			"timestamp": timestamp,
			"signature": fmt.Sprintf("0x%x", sig),
		}

		// WebSocket 广播
		broadcastToWS(data)

		// Redis / MQ 推送（占位）
		// pushToRedis(data)
		// pushToMQ(data)
	}
}

// collectPrices 从所有数据源收集价格
func collectPrices(sources []market.MarketDataSource) []*market.PriceTick {
	var wg sync.WaitGroup
	tickCh := make(chan *market.PriceTick, len(sources))

	for _, src := range sources {
		wg.Add(1)
		go func(s market.MarketDataSource) {
			defer wg.Done()
			tick, err := s.Read()
			if err != nil {
				log.Printf("数据源 %s 读取失败: %v", s.Name(), err)
				return
			}
			tickCh <- tick
		}(src)
	}

	go func() {
		wg.Wait()
		close(tickCh)
	}()

	var ticks []*market.PriceTick
	for t := range tickCh {
		ticks = append(ticks, t)
	}
	return ticks
}

// broadcastToWS 向所有 WebSocket 客户端广播数据
func broadcastToWS(data map[string]interface{}) {
	clientsMu.RLock()
	defer clientsMu.RUnlock()

	msg, _ := json.Marshal(data)
	for client := range clients {
		if err := client.WriteMessage(websocket.TextMessage, msg); err != nil {
			log.Printf("WebSocket 写入失败: %v", err)
			client.Close()
		}
	}
}

// -------------------- HTTP 处理器 --------------------

func healthHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": "ok",
		"time":   time.Now().Format(time.RFC3339),
	})
}

func indexPriceHandler(c *gin.Context) {
	priceMu.RLock()
	price := latestIndexPrice
	priceMu.RUnlock()

	c.JSON(http.StatusOK, gin.H{
		"symbol":    "BTC-USDT",
		"price":     price,
		"timestamp": time.Now().UnixMilli(),
	})
}

func sourcesHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"sources": []gin.H{
			{"name": "binance", "type": "rest", "weight": 0.3, "status": "active"},
			{"name": "okx", "type": "rest", "weight": 0.3, "status": "active"},
			{"name": "platform", "type": "internal", "weight": 0.4, "status": "active"},
		},
	})
}

// wsHandler 处理 WebSocket 连接升级
func wsHandler(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("WebSocket 升级失败: %v", err)
		return
	}
	defer conn.Close()

	clientsMu.Lock()
	clients[conn] = true
	clientsMu.Unlock()

	defer func() {
		clientsMu.Lock()
		delete(clients, conn)
		clientsMu.Unlock()
	}()

	// 保持连接活跃，等待客户端断开
	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			break
		}
	}
}

// generateTestKey 生成测试用 ECDSA 私钥
func generateTestKey() (*ecdsa.PrivateKey, error) {
	// 生产环境应从 KMS 或 HSM 读取
	// 这里生成随机密钥仅用于测试
	return ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
}
