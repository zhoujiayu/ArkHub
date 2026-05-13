// ============================================================
// internal/matching/orderbook.go
// 内存订单簿
// 职责：基于价格优先 + 时间优先的买卖盘管理
// ============================================================

package matching

import (
	"sort"
	"sync"
	"time"
)

// OrderBook 内存订单簿
// 基于两个有序价格列表实现：买单从高到低，卖单从低到高
// 同一价格内的订单按时间先后排序（FIFO）
//
// 核心设计：
//   - 使用 map[float64][]*Order 按价格分组存储订单
//   - 使用 []float64 维护排序后的价格列表
//   - 买单按价格从高到低排序（优先成交高价买单）
//   - 卖单按价格从低到高排序（优先成交低价卖单）
//   - 同一价格内按时间先后排序（先来的先成交）
type OrderBook struct {
	buys       map[float64][]*Order // 价格 -> 该价格的买单队列（按时间排序）
	sells      map[float64][]*Order // 价格 -> 该价格的卖单队列（按时间排序）
	buyPrices  []float64            // 排序后的买单价格（从高到低）
	sellPrices []float64            // 排序后的卖单价格（从低到高）
	mu         sync.RWMutex         // 读写锁保护订单簿操作
}

// NewOrderBook 创建一个新的内存订单簿
func NewOrderBook() *OrderBook {
	return &OrderBook{
		buys:       make(map[float64][]*Order),
		sells:      make(map[float64][]*Order),
		buyPrices:  make([]float64, 0),
		sellPrices: make([]float64, 0),
	}
}

// AddOrder 向订单簿添加订单
// 根据订单方向添加到对应的队列中
// 同一价格内的订单按时间先后排序
func (ob *OrderBook) AddOrder(order *Order) {
	ob.mu.Lock()
	defer ob.mu.Unlock()

	if order.Side == Buy {
		ob.addBuyOrder(order)
	} else {
		ob.addSellOrder(order)
	}
}

// addBuyOrder 添加买单到订单簿
func (ob *OrderBook) addBuyOrder(order *Order) {
	// 初始化该价格的订单列表（如果不存在）
	if _, ok := ob.buys[order.Price]; !ok {
		ob.buys[order.Price] = make([]*Order, 0)
		// 将价格插入到排序列表中（保持从高到低）
		ob.insertBuyPrice(order.Price)
	}
	ob.buys[order.Price] = append(ob.buys[order.Price], order)
}

// addSellOrder 添加卖单到订单簿
func (ob *OrderBook) addSellOrder(order *Order) {
	// 初始化该价格的订单列表（如果不存在）
	if _, ok := ob.sells[order.Price]; !ok {
		ob.sells[order.Price] = make([]*Order, 0)
		// 将价格插入到排序列表中（保持从低到高）
		ob.insertSellPrice(order.Price)
	}
	ob.sells[order.Price] = append(ob.sells[order.Price], order)
}

// insertBuyPrice 将价格插入到买单价格列表中（从高到低排序）
func (ob *OrderBook) insertBuyPrice(price float64) {
	// 二分查找插入位置
	idx := sort.Search(len(ob.buyPrices), func(i int) bool {
		return ob.buyPrices[i] <= price
	})
	ob.buyPrices = append(ob.buyPrices, 0)
	copy(ob.buyPrices[idx+1:], ob.buyPrices[idx:])
	ob.buyPrices[idx] = price
}

// insertSellPrice 将价格插入到卖单价格列表中（从低到高排序）
func (ob *OrderBook) insertSellPrice(price float64) {
	// 二分查找插入位置
	idx := sort.Search(len(ob.sellPrices), func(i int) bool {
		return ob.sellPrices[i] >= price
	})
	ob.sellPrices = append(ob.sellPrices, 0)
	copy(ob.sellPrices[idx+1:], ob.sellPrices[idx:])
	ob.sellPrices[idx] = price
}

// RemoveOrder 从订单簿中移除订单
// 返回 true 表示成功移除，false 表示订单不存在
func (ob *OrderBook) RemoveOrder(orderID string, side Side, price float64) bool {
	ob.mu.Lock()
	defer ob.mu.Unlock()

	if side == Buy {
		return ob.removeFromBook(orderID, price, ob.buys, &ob.buyPrices)
	}
	return ob.removeFromBook(orderID, price, ob.sells, &ob.sellPrices)
}

// removeFromBook 从指定价格队列中移除订单
func (ob *OrderBook) removeFromBook(orderID string, price float64, book map[float64][]*Order, prices *[]float64) bool {
	orders, ok := book[price]
	if !ok {
		return false
	}

	// 查找并移除订单
	for i, o := range orders {
		if o.ID == orderID {
			// 从列表中移除
			orders = append(orders[:i], orders[i+1:]...)
			if len(orders) == 0 {
				// 该价格下没有订单了，删除价格层级
				delete(book, price)
				ob.removePrice(price, prices)
			} else {
				book[price] = orders
			}
			return true
		}
	}
	return false
}

// removePrice 从价格列表中移除指定价格
func (ob *OrderBook) removePrice(price float64, prices *[]float64) {
	for i, p := range *prices {
		if p == price {
			*prices = append((*prices)[:i], (*prices)[i+1:]...)
			return
		}
	}
}

// GetBestBuy 获取最优买单价格（最高买价）
// 如果无买单，返回 0
func (ob *OrderBook) GetBestBuy() float64 {
	ob.mu.RLock()
	defer ob.mu.RUnlock()

	if len(ob.buyPrices) == 0 {
		return 0
	}
	return ob.buyPrices[0]
}

// GetBestSell 获取最优卖单价格（最低卖价）
// 如果无卖单，返回 0
func (ob *OrderBook) GetBestSell() float64 {
	ob.mu.RLock()
	defer ob.mu.RUnlock()

	if len(ob.sellPrices) == 0 {
		return 0
	}
	return ob.sellPrices[0]
}

// PeekBestBuy 查看最优买单（不移除）
func (ob *OrderBook) PeekBestBuy() *Order {
	ob.mu.RLock()
	defer ob.mu.RUnlock()

	if len(ob.buyPrices) == 0 {
		return nil
	}
	orders := ob.buys[ob.buyPrices[0]]
	if len(orders) == 0 {
		return nil
	}
	return orders[0]
}

// PeekBestSell 查看最优卖单（不移除）
func (ob *OrderBook) PeekBestSell() *Order {
	ob.mu.RLock()
	defer ob.mu.RUnlock()

	if len(ob.sellPrices) == 0 {
		return nil
	}
	orders := ob.sells[ob.sellPrices[0]]
	if len(orders) == 0 {
		return nil
	}
	return orders[0]
}

// PopBestBuy 取出并移除最优买单
func (ob *OrderBook) PopBestBuy() *Order {
	ob.mu.Lock()
	defer ob.mu.Unlock()

	if len(ob.buyPrices) == 0 {
		return nil
	}

	price := ob.buyPrices[0]
	orders := ob.buys[price]
	if len(orders) == 0 {
		return nil
	}

	order := orders[0]
	orders = orders[1:]
	if len(orders) == 0 {
		delete(ob.buys, price)
		ob.buyPrices = ob.buyPrices[1:]
	} else {
		ob.buys[price] = orders
	}
	return order
}

// PopBestSell 取出并移除最优卖单
func (ob *OrderBook) PopBestSell() *Order {
	ob.mu.Lock()
	defer ob.mu.Unlock()

	if len(ob.sellPrices) == 0 {
		return nil
	}

	price := ob.sellPrices[0]
	orders := ob.sells[price]
	if len(orders) == 0 {
		return nil
	}

	order := orders[0]
	orders = orders[1:]
	if len(orders) == 0 {
		delete(ob.sells, price)
		ob.sellPrices = ob.sellPrices[1:]
	} else {
		ob.sells[price] = orders
	}
	return order
}

// GetBuyDepth 获取买单深度（价格层级数量）
func (ob *OrderBook) GetBuyDepth() int {
	ob.mu.RLock()
	defer ob.mu.RUnlock()
	return len(ob.buyPrices)
}

// GetSellDepth 获取卖单深度（价格层级数量）
func (ob *OrderBook) GetSellDepth() int {
	ob.mu.RLock()
	defer ob.mu.RUnlock()
	return len(ob.sellPrices)
}

// GetBuyOrders 获取指定价格的所有买单
func (ob *OrderBook) GetBuyOrders(price float64) []*Order {
	ob.mu.RLock()
	defer ob.mu.RUnlock()
	orders := ob.buys[price]
	result := make([]*Order, len(orders))
	copy(result, orders)
	return result
}

// GetSellOrders 获取指定价格的所有卖单
func (ob *OrderBook) GetSellOrders(price float64) []*Order {
	ob.mu.RLock()
	defer ob.mu.RUnlock()
	orders := ob.sells[price]
	result := make([]*Order, len(orders))
	copy(result, orders)
	return result
}

// Snapshot 获取订单簿快照
func (ob *OrderBook) Snapshot() (buys map[float64][]*Order, sells map[float64][]*Order) {
	ob.mu.RLock()
	defer ob.mu.RUnlock()

	buys = make(map[float64][]*Order)
	sells = make(map[float64][]*Order)
	for k, v := range ob.buys {
		orders := make([]*Order, len(v))
		copy(orders, v)
		buys[k] = orders
	}
	for k, v := range ob.sells {
		orders := make([]*Order, len(v))
		copy(orders, v)
		sells[k] = orders
	}
	return
}

// NewOrder 创建一个新的订单
func NewOrder(id, userID, symbol string, side Side, price, quantity float64) *Order {
	return &Order{
		ID:        id,
		UserID:    userID,
		Symbol:    symbol,
		Side:      side,
		Price:     price,
		Quantity:  quantity,
		Status:    Pending,
		CreatedAt: time.Now().UnixMilli(),
	}
}
