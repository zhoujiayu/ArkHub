// ============================================================
// internal/matching/trade.go
// 成交记录与异步持久化
// 职责：生成成交记录、异步写入数据库、提供查询接口
// ============================================================

package matching

import (
	"fmt"
	"sync"
	"time"
)

// Trade 成交记录
// 记录一次撮合的完整信息，包括买卖双方订单信息
type Trade struct {
	ID          string  // 成交记录唯一 ID
	BuyOrderID  string  // 买单 ID
	SellOrderID string  // 卖单 ID
	Symbol      string  // 交易对，如 BTC-USDT
	Price       float64 // 成交价格
	Quantity    float64 // 成交数量
	BuyUserID   string  // 买方用户 ID
	SellUserID  string  // 卖方用户 ID
	CreatedAt   int64   // 成交时间戳（毫秒）
}

// TradeStore 成交记录存储接口
// 支持内存缓存和异步持久化到数据库
type TradeStore interface {
	// Save 保存成交记录
	Save(trade *Trade) error
	// GetByID 根据 ID 查询成交记录
	GetByID(id string) (*Trade, error)
	// GetByOrderID 根据订单 ID 查询相关成交记录
	GetByOrderID(orderID string) ([]*Trade, error)
	// GetBySymbol 查询指定交易对的成交记录
	GetBySymbol(symbol string, limit int) ([]*Trade, error)
	// Close 关闭存储
	Close() error
}

// MemoryTradeStore 内存成交记录存储
// 用于测试和低延迟场景，数据不持久化
type MemoryTradeStore struct {
	trades map[string]*Trade   // ID -> Trade
	byBuy  map[string][]string // BuyOrderID -> []TradeID
	bySell map[string][]string // SellOrderID -> []TradeID
	bySym  map[string][]string // Symbol -> []TradeID
	mu     sync.RWMutex
}

// NewMemoryTradeStore 创建内存成交记录存储
func NewMemoryTradeStore() *MemoryTradeStore {
	return &MemoryTradeStore{
		trades: make(map[string]*Trade),
		byBuy:  make(map[string][]string),
		bySell: make(map[string][]string),
		bySym:  make(map[string][]string),
	}
}

// Save 保存成交记录到内存
func (s *MemoryTradeStore) Save(trade *Trade) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.trades[trade.ID] = trade
	s.byBuy[trade.BuyOrderID] = append(s.byBuy[trade.BuyOrderID], trade.ID)
	s.bySell[trade.SellOrderID] = append(s.bySell[trade.SellOrderID], trade.ID)
	s.bySym[trade.Symbol] = append(s.bySym[trade.Symbol], trade.ID)
	return nil
}

// GetByID 根据 ID 查询成交记录
func (s *MemoryTradeStore) GetByID(id string) (*Trade, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	trade, ok := s.trades[id]
	if !ok {
		return nil, fmt.Errorf("trade not found: %s", id)
	}
	return trade, nil
}

// GetByOrderID 根据订单 ID 查询相关成交记录
func (s *MemoryTradeStore) GetByOrderID(orderID string) ([]*Trade, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*Trade
	if ids, ok := s.byBuy[orderID]; ok {
		for _, id := range ids {
			if trade, ok := s.trades[id]; ok {
				result = append(result, trade)
			}
		}
	}
	if ids, ok := s.bySell[orderID]; ok {
		for _, id := range ids {
			if trade, ok := s.trades[id]; ok {
				result = append(result, trade)
			}
		}
	}
	return result, nil
}

// GetBySymbol 查询指定交易对的成交记录
func (s *MemoryTradeStore) GetBySymbol(symbol string, limit int) ([]*Trade, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ids, ok := s.bySym[symbol]
	if !ok {
		return []*Trade{}, nil
	}

	var result []*Trade
	for _, id := range ids {
		if trade, ok := s.trades[id]; ok {
			result = append(result, trade)
		}
	}

	// 限制返回数量
	if limit > 0 && len(result) > limit {
		result = result[len(result)-limit:]
	}
	return result, nil
}

// Close 关闭内存存储
func (s *MemoryTradeStore) Close() error {
	return nil
}

// AsyncTradeWriter 异步成交记录写入器
// 负责将成交记录从通道批量写入存储
type AsyncTradeWriter struct {
	store     TradeStore
	tradeCh   <-chan *Trade
	batchSize int
	interval  time.Duration
	stopCh    chan struct{} // 用于停止写入 goroutine
	wg        sync.WaitGroup
}

// NewAsyncTradeWriter 创建异步成交记录写入器
// batchSize: 每批写入的最大记录数
// interval: 批量写入的最大间隔
func NewAsyncTradeWriter(store TradeStore, tradeCh <-chan *Trade, batchSize int, interval time.Duration) *AsyncTradeWriter {
	return &AsyncTradeWriter{
		store:     store,
		tradeCh:   tradeCh,
		batchSize: batchSize,
		interval:  interval,
		stopCh:    make(chan struct{}),
	}
}

// Start 启动异步写入 goroutine
func (w *AsyncTradeWriter) Start() {
	w.wg.Add(1)
	go w.run()
}

// Stop 停止异步写入
func (w *AsyncTradeWriter) Stop() {
	close(w.stopCh)
	w.wg.Wait()
}

// run 异步写入主循环
func (w *AsyncTradeWriter) run() {
	defer w.wg.Done()

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	batch := make([]*Trade, 0, w.batchSize)

	for {
		select {
		case trade, ok := <-w.tradeCh:
			if !ok {
				// 通道关闭，写入剩余数据
				w.flush(batch)
				return
			}
			batch = append(batch, trade)
			if len(batch) >= w.batchSize {
				w.flush(batch)
				batch = batch[:0]
			}

		case <-ticker.C:
			if len(batch) > 0 {
				w.flush(batch)
				batch = batch[:0]
			}

		case <-w.stopCh:
			// 收到停止信号，写入剩余数据
			w.flush(batch)
			return
		}
	}
}

// flush 批量写入成交记录
func (w *AsyncTradeWriter) flush(trades []*Trade) {
	if len(trades) == 0 {
		return
	}
	for _, trade := range trades {
		if err := w.store.Save(trade); err != nil {
			// 实际生产环境应记录日志或重试
			fmt.Printf("保存成交记录失败: %v\n", err)
		}
	}
}
