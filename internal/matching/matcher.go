// ============================================================
// internal/matching/matcher.go
// 撮合算法
// 职责：价格优先、时间优先的撮合逻辑实现
// ============================================================

package matching

import (
	"fmt"
	"sync"
	"time"
)

// Matcher 撮合引擎
// 负责将订单与订单簿中的最优价格进行匹配，生成成交记录
//
// 核心设计：
//   - 价格优先：买单优先匹配最低卖价，卖单优先匹配最高买价
//   - 时间优先：同一价格内，先来的订单先成交
//   - 完全成交：当撮合数量 >= 订单数量时，订单状态变为 Filled
//   - 部分成交：当撮合数量 < 订单数量时，订单状态变为 Partial，剩余未成交部分留在订单簿
type Matcher struct {
	orderBook *OrderBook
	trades    chan *Trade // 成交记录通道，异步输出
	mu        sync.Mutex
}

// NewMatcher 创建一个新的撮合引擎
func NewMatcher(orderBook *OrderBook, tradeChan chan *Trade) *Matcher {
	return &Matcher{
		orderBook: orderBook,
		trades:    tradeChan,
	}
}

// Match 对传入的订单进行撮合
// 返回生成的成交记录列表
// 核心逻辑：
//   - 买单：从卖单队列中找最低价格匹配
//   - 卖单：从买单队列中找最高价格匹配
//   - 价格交叉条件：买价 >= 卖价（买单）或 卖价 <= 买价（卖单）
func (m *Matcher) Match(order *Order) []*Trade {
	m.mu.Lock()
	defer m.mu.Unlock()

	var trades []*Trade

	if order.Side == Buy {
		trades = m.matchBuy(order)
	} else {
		trades = m.matchSell(order)
	}

	// 将成交记录发送到通道
	for _, trade := range trades {
		select {
		case m.trades <- trade:
		default:
			// 通道满时丢弃（实际生产环境应有更完善的处理）
		}
	}

	return trades
}

// matchBuy 撮合买单
// 从卖单队列中查找最低价格的卖单进行匹配
// 直到订单完全成交或无法继续匹配为止
func (m *Matcher) matchBuy(order *Order) []*Trade {
	var trades []*Trade

	for order.Quantity > 0 {
		// 获取最优卖单
		bestSell := m.orderBook.PeekBestSell()
		if bestSell == nil {
			// 没有卖单可匹配，将剩余买单加入订单簿
			m.orderBook.AddOrder(order)
			break
		}

		// 价格交叉条件：买价 >= 最低卖价
		if order.Price < bestSell.Price {
			// 无法匹配，将买单加入订单簿
			m.orderBook.AddOrder(order)
			break
		}

		// 取出最优卖单
		bestSell = m.orderBook.PopBestSell()

		// 执行撮合
		trade := m.execute(order, bestSell)
		trades = append(trades, trade)

		// 更新订单状态
		if bestSell.Quantity <= 0 {
			bestSell.Status = Filled
		} else {
			bestSell.Status = Partial
			// 部分成交的卖单放回订单簿
			m.orderBook.AddOrder(bestSell)
		}

		if order.Quantity <= 0 {
			order.Status = Filled
		} else {
			order.Status = Partial
		}
	}

	return trades
}

// matchSell 撮合卖单
// 从买单队列中查找最高价格的买单进行匹配
// 直到订单完全成交或无法继续匹配为止
func (m *Matcher) matchSell(order *Order) []*Trade {
	var trades []*Trade

	for order.Quantity > 0 {
		// 获取最优买单
		bestBuy := m.orderBook.PeekBestBuy()
		if bestBuy == nil {
			// 没有买单可匹配，将剩余卖单加入订单簿
			m.orderBook.AddOrder(order)
			break
		}

		// 价格交叉条件：卖价 <= 最高买价
		if order.Price > bestBuy.Price {
			// 无法匹配，将卖单加入订单簿
			m.orderBook.AddOrder(order)
			break
		}

		// 取出最优买单
		bestBuy = m.orderBook.PopBestBuy()

		// 执行撮合
		trade := m.execute(bestBuy, order)
		trades = append(trades, trade)

		// 更新订单状态
		if bestBuy.Quantity <= 0 {
			bestBuy.Status = Filled
		} else {
			bestBuy.Status = Partial
			// 部分成交的买单放回订单簿
			m.orderBook.AddOrder(bestBuy)
		}

		if order.Quantity <= 0 {
			order.Status = Filled
		} else {
			order.Status = Partial
		}
	}

	return trades
}

// execute 执行单次撮合
// buyOrder 是买单，sellOrder 是卖单
// 返回生成的成交记录
// 撮合规则：
//   - 成交数量 = min(买单剩余数量, 卖单剩余数量)
//   - 成交价格 = 被动单（订单簿中已有的订单）的价格
func (m *Matcher) execute(buyOrder, sellOrder *Order) *Trade {
	// 计算成交数量
	matchQty := buyOrder.Quantity
	if sellOrder.Quantity < matchQty {
		matchQty = sellOrder.Quantity
	}

	// 更新订单数量
	buyOrder.Quantity -= matchQty
	sellOrder.Quantity -= matchQty

	// 生成成交记录（成交价格为被动单价格，即订单簿中已有订单的价格）
	trade := &Trade{
		ID:          generateTradeID(),
		BuyOrderID:  buyOrder.ID,
		SellOrderID: sellOrder.ID,
		Symbol:      buyOrder.Symbol,
		Price:       sellOrder.Price, // 被动单价格（卖单价格）
		Quantity:    matchQty,
		BuyUserID:   buyOrder.UserID,
		SellUserID:  sellOrder.UserID,
		CreatedAt:   time.Now().UnixMilli(),
	}

	return trade
}

// generateTradeID 生成唯一的成交记录 ID
func generateTradeID() string {
	return fmt.Sprintf("T%d", time.Now().UnixNano())
}

// CancelOrder 取消订单
// 从订单簿中移除指定订单
// 返回 true 表示成功取消，false 表示订单不存在或已成交
func (m *Matcher) CancelOrder(orderID string, side Side, price float64) bool {
	return m.orderBook.RemoveOrder(orderID, side, price)
}

// GetOrderBook 获取当前订单簿
func (m *Matcher) GetOrderBook() *OrderBook {
	return m.orderBook
}
