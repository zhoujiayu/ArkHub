// ============================================================
// test/integration/matching_test.go
// 撮合引擎集成测试
// 验证 Disruptor 队列、订单簿、撮合算法、成交记录、熔断机制的完整流程
// ============================================================

package integration

import (
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"arhub/internal/matching"
)

// ==================== Disruptor 测试 ====================

func TestDisruptorQueue(t *testing.T) {
	t.Run("基本读写", func(t *testing.T) {
		rb := matching.NewRingBuffer(16)

		// 写入 5 个订单
		for i := 0; i < 5; i++ {
			order := matching.NewOrder("O"+string(rune('0'+i)), "user1", "BTC-USDT", matching.Buy, 100.0, 1.0)
			err := rb.Put(*order)
			assert.NoError(t, err)
		}

		// 读取 5 个订单
		for i := 0; i < 5; i++ {
			order, err := rb.Get()
			assert.NoError(t, err)
			assert.Equal(t, "user1", order.UserID)
			assert.Equal(t, matching.Buy, order.Side)
		}
	})

	t.Run("队列满时拒绝写入", func(t *testing.T) {
		rb := matching.NewRingBuffer(4)

		// 写入 4 个订单（填满队列）
		for i := 0; i < 4; i++ {
			order := matching.NewOrder("O"+string(rune('0'+i)), "user1", "BTC-USDT", matching.Buy, 100.0, 1.0)
			err := rb.Put(*order)
			assert.NoError(t, err)
		}

		// 再写入一个，应该失败
		order := matching.NewOrder("O5", "user1", "BTC-USDT", matching.Buy, 100.0, 1.0)
		err := rb.Put(*order)
		assert.Equal(t, matching.ErrRingBufferFull, err)
	})

	t.Run("并发读写", func(t *testing.T) {
		rb := matching.NewRingBuffer(1024)
		const num = 1000

		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < num; i++ {
				order := matching.NewOrder("O"+string(rune('0'+(i%10))), "user1", "BTC-USDT", matching.Buy, 100.0, 1.0)
				for {
					err := rb.Put(*order)
					if err == nil {
						break
					}
					time.Sleep(1 * time.Millisecond)
				}
			}
		}()

		var count int
		for count < num {
			_, err := rb.Get()
			if err == nil {
				count++
			} else {
				time.Sleep(1 * time.Millisecond)
			}
		}
		wg.Wait()
		assert.Equal(t, num, count)
	})
}

// ==================== 订单簿测试 ====================

func TestOrderBook(t *testing.T) {
	t.Run("添加和移除订单", func(t *testing.T) {
		ob := matching.NewOrderBook()

		// 添加买单
		buyOrder := matching.NewOrder("B1", "user1", "BTC-USDT", matching.Buy, 100.0, 1.0)
		ob.AddOrder(buyOrder)

		assert.Equal(t, 1, ob.GetBuyDepth())
		assert.Equal(t, 0, ob.GetSellDepth())

		// 添加卖单
		sellOrder := matching.NewOrder("S1", "user2", "BTC-USDT", matching.Sell, 101.0, 1.0)
		ob.AddOrder(sellOrder)

		assert.Equal(t, 1, ob.GetSellDepth())

		// 移除买单
		removed := ob.RemoveOrder("B1", matching.Buy, 100.0)
		assert.True(t, removed)
		assert.Equal(t, 0, ob.GetBuyDepth())

		// 移除不存在的订单
		removed = ob.RemoveOrder("B999", matching.Buy, 100.0)
		assert.False(t, removed)
	})

	t.Run("价格排序", func(t *testing.T) {
		ob := matching.NewOrderBook()

		// 添加三个买单，价格分别为 90, 100, 95
		ob.AddOrder(matching.NewOrder("B1", "user1", "BTC-USDT", matching.Buy, 90.0, 1.0))
		ob.AddOrder(matching.NewOrder("B2", "user1", "BTC-USDT", matching.Buy, 100.0, 1.0))
		ob.AddOrder(matching.NewOrder("B3", "user1", "BTC-USDT", matching.Buy, 95.0, 1.0))

		// 最优买价应该是 100（最高）
		bestBuy := ob.GetBestBuy()
		assert.Equal(t, 100.0, bestBuy)

		// 添加三个卖单，价格分别为 105, 95, 100
		ob.AddOrder(matching.NewOrder("S1", "user2", "BTC-USDT", matching.Sell, 105.0, 1.0))
		ob.AddOrder(matching.NewOrder("S2", "user2", "BTC-USDT", matching.Sell, 95.0, 1.0))
		ob.AddOrder(matching.NewOrder("S3", "user2", "BTC-USDT", matching.Sell, 100.0, 1.0))

		// 最优卖价应该是 95（最低）
		bestSell := ob.GetBestSell()
		assert.Equal(t, 95.0, bestSell)
	})

	t.Run("同一价格多个订单", func(t *testing.T) {
		ob := matching.NewOrderBook()

		// 添加三个同价格的买单
		ob.AddOrder(matching.NewOrder("B1", "user1", "BTC-USDT", matching.Buy, 100.0, 1.0))
		ob.AddOrder(matching.NewOrder("B2", "user2", "BTC-USDT", matching.Buy, 100.0, 2.0))
		ob.AddOrder(matching.NewOrder("B3", "user3", "BTC-USDT", matching.Buy, 100.0, 3.0))

		orders := ob.GetBuyOrders(100.0)
		assert.Equal(t, 3, len(orders))

		// 移除第一个
		removed := ob.RemoveOrder("B1", matching.Buy, 100.0)
		assert.True(t, removed)

		orders = ob.GetBuyOrders(100.0)
		assert.Equal(t, 2, len(orders))
	})
}

// ==================== 撮合测试 ====================

func TestMatcher(t *testing.T) {
	t.Run("简单撮合", func(t *testing.T) {
		ob := matching.NewOrderBook()
		tradeCh := make(chan *matching.Trade, 100)
		matcher := matching.NewMatcher(ob, tradeCh)

		// 先挂一个买单
		buyOrder := matching.NewOrder("B1", "user1", "BTC-USDT", matching.Buy, 100.0, 1.0)
		matcher.Match(buyOrder)

		// 再挂一个价格更低的卖单
		sellOrder := matching.NewOrder("S1", "user2", "BTC-USDT", matching.Sell, 95.0, 1.0)
		trades := matcher.Match(sellOrder)

		// 应该生成一条成交记录
		assert.Equal(t, 1, len(trades))
		assert.Equal(t, "B1", trades[0].BuyOrderID)
		assert.Equal(t, "S1", trades[0].SellOrderID)
		assert.Equal(t, 95.0, trades[0].Price) // 成交价格为被动单价格（卖单价格）
		assert.Equal(t, 1.0, trades[0].Quantity)
	})

	t.Run("部分成交", func(t *testing.T) {
		ob := matching.NewOrderBook()
		tradeCh := make(chan *matching.Trade, 100)
		matcher := matching.NewMatcher(ob, tradeCh)

		// 挂一个买单 100.0 x 5.0
		buyOrder := matching.NewOrder("B1", "user1", "BTC-USDT", matching.Buy, 100.0, 5.0)
		matcher.Match(buyOrder)

		// 挂一个卖单 95.0 x 2.0
		sellOrder := matching.NewOrder("S1", "user2", "BTC-USDT", matching.Sell, 95.0, 2.0)
		trades := matcher.Match(sellOrder)

		// 应该生成一条成交记录，数量 2.0
		assert.Equal(t, 1, len(trades))
		assert.Equal(t, 2.0, trades[0].Quantity)

		// 订单簿中应该还有未成交的买单 3.0
		assert.Equal(t, 1, ob.GetBuyDepth())
	})

	t.Run("无法撮合", func(t *testing.T) {
		ob := matching.NewOrderBook()
		tradeCh := make(chan *matching.Trade, 100)
		matcher := matching.NewMatcher(ob, tradeCh)

		// 挂一个买单 100.0
		buyOrder := matching.NewOrder("B1", "user1", "BTC-USDT", matching.Buy, 100.0, 1.0)
		matcher.Match(buyOrder)

		// 挂一个卖单 105.0（价格高于买价，无法成交）
		sellOrder := matching.NewOrder("S1", "user2", "BTC-USDT", matching.Sell, 105.0, 1.0)
		trades := matcher.Match(sellOrder)

		// 无法成交
		assert.Equal(t, 0, len(trades))

		// 订单簿中应该有买单和卖单各一个
		assert.Equal(t, 1, ob.GetBuyDepth())
		assert.Equal(t, 1, ob.GetSellDepth())
	})

	t.Run("多笔撮合", func(t *testing.T) {
		ob := matching.NewOrderBook()
		tradeCh := make(chan *matching.Trade, 100)
		matcher := matching.NewMatcher(ob, tradeCh)

		// 挂两个卖单
		ob.AddOrder(matching.NewOrder("S1", "user2", "BTC-USDT", matching.Sell, 95.0, 1.0))
		ob.AddOrder(matching.NewOrder("S2", "user2", "BTC-USDT", matching.Sell, 96.0, 1.0))

		// 挂一个大买单，可以吃掉两个卖单
		buyOrder := matching.NewOrder("B1", "user1", "BTC-USDT", matching.Buy, 100.0, 3.0)
		trades := matcher.Match(buyOrder)

		// 应该生成两条成交记录
		assert.Equal(t, 2, len(trades))
	})

	t.Run("取消订单", func(t *testing.T) {
		ob := matching.NewOrderBook()
		tradeCh := make(chan *matching.Trade, 100)
		matcher := matching.NewMatcher(ob, tradeCh)

		// 挂一个买单
		buyOrder := matching.NewOrder("B1", "user1", "BTC-USDT", matching.Buy, 100.0, 1.0)
		ob.AddOrder(buyOrder)

		// 取消订单
		cancelled := matcher.CancelOrder("B1", matching.Buy, 100.0)
		assert.True(t, cancelled)

		// 订单簿应该为空
		assert.Equal(t, 0, ob.GetBuyDepth())
	})
}

// ==================== 成交记录测试 ====================

func TestTradeStore(t *testing.T) {
	t.Run("保存和查询", func(t *testing.T) {
		store := matching.NewMemoryTradeStore()
		trade := &matching.Trade{
			ID:         "T1",
			BuyOrderID:  "B1",
			SellOrderID: "S1",
			Symbol:      "BTC-USDT",
			Price:       100.0,
			Quantity:    1.0,
			BuyUserID:   "user1",
			SellUserID:  "user2",
			CreatedAt:   time.Now().UnixMilli(),
		}

		err := store.Save(trade)
		assert.NoError(t, err)

		// 根据 ID 查询
		found, err := store.GetByID("T1")
		assert.NoError(t, err)
		assert.Equal(t, "B1", found.BuyOrderID)

		// 根据订单 ID 查询
		trades, err := store.GetByOrderID("B1")
		assert.NoError(t, err)
		assert.Equal(t, 1, len(trades))

		// 根据交易对查询
		trades, err = store.GetBySymbol("BTC-USDT", 10)
		assert.NoError(t, err)
		assert.Equal(t, 1, len(trades))
	})

	t.Run("多笔成交记录", func(t *testing.T) {
		store := matching.NewMemoryTradeStore()

		for i := 0; i < 5; i++ {
			trade := &matching.Trade{
				ID:         string(rune('T' + i)),
				BuyOrderID:  "B1",
				SellOrderID: "S1",
				Symbol:      "BTC-USDT",
				Price:       100.0 + float64(i),
				Quantity:    1.0,
				BuyUserID:   "user1",
				SellUserID:  "user2",
				CreatedAt:   time.Now().UnixMilli(),
			}
			store.Save(trade)
		}

		trades, err := store.GetBySymbol("BTC-USDT", 3)
		assert.NoError(t, err)
		assert.Equal(t, 3, len(trades))
	})
}

// ==================== 熔断测试 ====================

func TestCircuitBreaker(t *testing.T) {
	t.Run("熔断和恢复", func(t *testing.T) {
		cb := matching.NewCircuitBreaker(3, 100*time.Millisecond, 10)

		// 连续失败 3 次
		for i := 0; i < 3; i++ {
			err := cb.Call(func() error {
				return assert.AnError
			})
			assert.Error(t, err)
		}

		// 熔断器应该已经打开
		assert.True(t, cb.IsOpen())

		// 再次调用应该被熔断
		err := cb.Call(func() error {
			return nil
		})
		assert.Equal(t, matching.ErrCircuitOpen, err)

		// 等待熔断超时
		time.Sleep(150 * time.Millisecond)

		// 进入半开状态，允许探测请求
		for i := 0; i < 3; i++ {
			err := cb.Call(func() error {
				return nil
			})
			assert.NoError(t, err)
		}

		// 熔断器应该已经恢复
		assert.True(t, cb.IsClosed())
	})

	t.Run("重置", func(t *testing.T) {
		cb := matching.NewCircuitBreaker(3, 100*time.Millisecond, 10)

		// 触发熔断
		for i := 0; i < 3; i++ {
			cb.Call(func() error {
				return assert.AnError
			})
		}

		assert.True(t, cb.IsOpen())

		// 重置
		cb.Reset()
		assert.True(t, cb.IsClosed())
	})
}

// ==================== 性能测试 ====================

func BenchmarkRingBuffer(b *testing.B) {
	rb := matching.NewRingBuffer(1024 * 1024)
	order := matching.NewOrder("B1", "user1", "BTC-USDT", matching.Buy, 100.0, 1.0)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			rb.Put(*order)
			rb.Get()
		}
	})
}

func BenchmarkOrderBookAdd(b *testing.B) {
	ob := matching.NewOrderBook()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		order := matching.NewOrder(
			string(rune('O'+i%26)),
			"user1",
			"BTC-USDT",
			matching.Buy,
			float64(rand.Intn(1000)),
			1.0,
		)
		ob.AddOrder(order)
	}
}

func BenchmarkMatcher(b *testing.B) {
	ob := matching.NewOrderBook()
	tradeCh := make(chan *matching.Trade, 10000)
	matcher := matching.NewMatcher(ob, tradeCh)

	// 预挂一些卖单
	for i := 0; i < 100; i++ {
		ob.AddOrder(matching.NewOrder(
			"S"+string(rune('0'+i%10)),
			"user2",
			"BTC-USDT",
			matching.Sell,
			float64(90+i),
			1.0,
		))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		order := matching.NewOrder(
			"B"+string(rune('0'+i%10)),
			"user1",
			"BTC-USDT",
			matching.Buy,
			200.0,
			1.0,
		)
		matcher.Match(order)
	}
}
