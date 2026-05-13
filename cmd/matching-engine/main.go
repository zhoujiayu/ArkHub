// ============================================================
// cmd/matching-engine/main.go
// 撮合引擎服务入口
// 职责：启动 Disruptor 队列、内存订单簿、撮合引擎、成交记录异步写入、HTTP API
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

	"arhub/internal/matching"
)

func main() {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())

	// 初始化撮合引擎核心组件
	orderBook := matching.NewOrderBook()
	tradeCh := make(chan *matching.Trade, 10000)
	matcher := matching.NewMatcher(orderBook, tradeCh)

	// 初始化成交记录存储（内存存储，生产环境可替换为数据库存储）
	tradeStore := matching.NewMemoryTradeStore()
	tradeWriter := matching.NewAsyncTradeWriter(tradeStore, tradeCh, 100, 100*time.Millisecond)
	tradeWriter.Start()

	// 初始化熔断器
	circuitBreaker := matching.NewCircuitBreaker(5, 30*time.Second, 100)

	// 初始化 Disruptor 队列
	ringBuffer := matching.NewRingBuffer(1024)

	// 启动订单消费 goroutine
	go consumeOrders(ringBuffer, matcher, circuitBreaker)

	// 注册 HTTP API
	r.GET("/health", healthHandler(orderBook, circuitBreaker))
	r.POST("/api/v1/order", submitOrderHandler(ringBuffer, circuitBreaker))
	r.POST("/api/v1/order/cancel", cancelOrderHandler(matcher))
	r.GET("/api/v1/orderbook", getOrderBookHandler(orderBook))
	r.GET("/api/v1/trades", getTradesHandler(tradeStore))
	r.GET("/api/v1/circuit/status", circuitStatusHandler(circuitBreaker))

	// 启动 HTTP 服务
	port := ":8083"
	srv := &http.Server{
		Addr:    port,
		Handler: r,
	}

	go func() {
		fmt.Printf("撮合引擎服务启动成功，监听端口 %s\n", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("撮合引擎服务启动失败: %v", err)
		}
	}()

	// 优雅关闭
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("撮合引擎服务关闭失败: %v", err)
	}

	tradeWriter.Stop()
	close(tradeCh)
	tradeStore.Close()
}

// consumeOrders 从 Disruptor 队列消费订单并进行撮合
func consumeOrders(rb *matching.RingBuffer, matcher *matching.Matcher, cb *matching.CircuitBreaker) {
	for {
		order, err := rb.Get()
		if err != nil {
			if err == matching.ErrRingBufferEmpty {
				// 队列为空，短暂休眠后重试
				time.Sleep(1 * time.Millisecond)
				continue
			}
			log.Printf("从队列读取订单失败: %v", err)
			continue
		}

		// 使用熔断器保护撮合操作
		err = cb.Call(func() error {
			trades := matcher.Match(&order)
			if len(trades) > 0 {
				log.Printf("订单 %s 撮合完成，生成 %d 条成交记录", order.ID, len(trades))
			}
			return nil
		})

		if err != nil {
			log.Printf("撮合操作被熔断: %v", err)
		}
	}
}

// --- HTTP 处理器 ---

// submitOrderRequest 提交订单请求
type submitOrderRequest struct {
	UserID   string  `json:"user_id" binding:"required"`
	Symbol   string  `json:"symbol" binding:"required"`
	Side     string  `json:"side" binding:"required"` // "buy" 或 "sell"
	Price    float64 `json:"price" binding:"required,gt=0"`
	Quantity float64 `json:"quantity" binding:"required,gt=0"`
}

// submitOrderHandler 提交订单
func submitOrderHandler(rb *matching.RingBuffer, cb *matching.CircuitBreaker) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req submitOrderRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		side := matching.Buy
		if req.Side == "sell" {
			side = matching.Sell
		}

		order := matching.NewOrder(
			generateOrderID(),
			req.UserID,
			req.Symbol,
			side,
			req.Price,
			req.Quantity,
		)

		// 使用熔断器保护写入操作
		err := cb.Call(func() error {
			return rb.Put(*order)
		})

		if err != nil {
			if err == matching.ErrRingBufferFull {
				c.JSON(http.StatusServiceUnavailable, gin.H{"error": "订单队列已满，请稍后重试"})
				return
			}
			if err == matching.ErrCircuitOpen {
				c.JSON(http.StatusServiceUnavailable, gin.H{"error": "系统熔断中，请稍后重试"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"order_id": order.ID,
			"status":   "accepted",
		})
	}
}

// cancelOrderRequest 取消订单请求
type cancelOrderRequest struct {
	OrderID string        `json:"order_id" binding:"required"`
	Side    matching.Side `json:"side" binding:"required"`
	Price   float64       `json:"price" binding:"required,gt=0"`
}

// cancelOrderHandler 取消订单
func cancelOrderHandler(matcher *matching.Matcher) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req cancelOrderRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if matcher.CancelOrder(req.OrderID, req.Side, req.Price) {
			c.JSON(http.StatusOK, gin.H{"status": "cancelled"})
		} else {
			c.JSON(http.StatusNotFound, gin.H{"error": "订单不存在或已成交"})
		}
	}
}

// getOrderBookHandler 获取订单簿快照
func getOrderBookHandler(ob *matching.OrderBook) gin.HandlerFunc {
	return func(c *gin.Context) {
		buys, sells := ob.Snapshot()
		c.JSON(http.StatusOK, gin.H{
			"buys":  formatOrderBook(buys),
			"sells": formatOrderBook(sells),
		})
	}
}

// getTradesHandler 获取成交记录
func getTradesHandler(store matching.TradeStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		symbol := c.Query("symbol")
		limit := 0
		if l := c.Query("limit"); l != "" {
			fmt.Sscanf(l, "%d", &limit)
		}
		if limit <= 0 {
			limit = 10
		}

		trades, err := store.GetBySymbol(symbol, limit)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"trades": trades})
	}
}

// circuitStatusHandler 获取熔断器状态
func circuitStatusHandler(cb *matching.CircuitBreaker) gin.HandlerFunc {
	return func(c *gin.Context) {
		var state string
		if cb.IsOpen() {
			state = "open"
		} else if cb.IsHalfOpen() {
			state = "half_open"
		} else {
			state = "closed"
		}
		c.JSON(http.StatusOK, gin.H{
			"state":           state,
			"average_latency": cb.GetAverageLatency(),
		})
	}
}

// healthHandler 健康检查
func healthHandler(ob *matching.OrderBook, cb *matching.CircuitBreaker) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":     "ok",
			"time":       time.Now().Format(time.RFC3339),
			"buy_depth":  ob.GetBuyDepth(),
			"sell_depth": ob.GetSellDepth(),
			"circuit":    cb.IsOpen(),
		})
	}
}

// formatOrderBook 格式化订单簿数据
func formatOrderBook(orders map[float64][]*matching.Order) []gin.H {
	var result []gin.H
	for price, orderList := range orders {
		var totalQty float64
		for _, o := range orderList {
			totalQty += o.Quantity
		}
		result = append(result, gin.H{
			"price":    price,
			"quantity": totalQty,
			"orders":   len(orderList),
		})
	}
	return result
}

// generateOrderID 生成唯一的订单 ID
func generateOrderID() string {
	return fmt.Sprintf("O%d", time.Now().UnixNano())
}
