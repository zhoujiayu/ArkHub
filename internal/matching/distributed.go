// ============================================================
// internal/matching/distributed.go
// 分布式改造 — Redis + PostgreSQL 分布式存储
// 职责：将单机内存存储改为分布式存储（Redis 缓存 + PostgreSQL 持久化）
// ============================================================

package matching

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
)

// DistributedOrderBook 分布式订单簿
// 基于 Redis Sorted Set 实现跨进程共享
type DistributedOrderBook struct {
	redis   *redis.Client
	buyKey  string // Redis key: "orderbook:buys"
	sellKey string // Redis key: "orderbook:sells"
}

// NewDistributedOrderBook 创建分布式订单簿
func NewDistributedOrderBook(redisAddr string) *DistributedOrderBook {
	client := redis.NewClient(&redis.Options{Addr: redisAddr})
	return &DistributedOrderBook{
		redis:   client,
		buyKey:  "orderbook:buys",
		sellKey: "orderbook:sells",
	}
}

// AddBuyOrder 添加买单到 Redis（分数 = -price，从高到低排序）
func (d *DistributedOrderBook) AddBuyOrder(ctx context.Context, order *Order) error {
	data, _ := json.Marshal(order)
	// ZADD 分数 = -price，这样高分在前面
	return d.redis.ZAdd(ctx, d.buyKey, &redis.Z{
		Score:  -order.Price,
		Member: string(data),
	}).Err()
}

// AddSellOrder 添加卖单到 Redis（分数 = price，从低到高排序）
func (d *DistributedOrderBook) AddSellOrder(ctx context.Context, order *Order) error {
	data, _ := json.Marshal(order)
	return d.redis.ZAdd(ctx, d.sellKey, &redis.Z{
		Score:  order.Price,
		Member: string(data),
	}).Err()
}

// GetBestBuy 获取最优买单（最高买价）
func (d *DistributedOrderBook) GetBestBuy(ctx context.Context) (*Order, error) {
	result, err := d.redis.ZRevRangeWithScores(ctx, d.buyKey, 0, 0).Result()
	if err != nil || len(result) == 0 {
		return nil, fmt.Errorf("no buy orders")
	}

	var order Order
	if err := json.Unmarshal([]byte(result[0].Member.(string)), &order); err != nil {
		return nil, err
	}
	return &order, nil
}

// GetBestSell 获取最优卖单（最低卖价）
func (d *DistributedOrderBook) GetBestSell(ctx context.Context) (*Order, error) {
	result, err := d.redis.ZRangeWithScores(ctx, d.sellKey, 0, 0).Result()
	if err != nil || len(result) == 0 {
		return nil, fmt.Errorf("no sell orders")
	}

	var order Order
	if err := json.Unmarshal([]byte(result[0].Member.(string)), &order); err != nil {
		return nil, err
	}
	return &order, nil
}

// RemoveBuyOrder 移除买单
func (d *DistributedOrderBook) RemoveBuyOrder(ctx context.Context, order *Order) error {
	data, _ := json.Marshal(order)
	return d.redis.ZRem(ctx, d.buyKey, string(data)).Err()
}

// RemoveSellOrder 移除卖单
func (d *DistributedOrderBook) RemoveSellOrder(ctx context.Context, order *Order) error {
	data, _ := json.Marshal(order)
	return d.redis.ZRem(ctx, d.sellKey, string(data)).Err()
}

// --- PostgreSQL Trade 持久化 ---

// DBTradeStore 数据库存储
type DBTradeStore struct {
	db *sql.DB
}

// NewDBTradeStore 创建数据库存储
func NewDBTradeStore(db *sql.DB) *DBTradeStore {
	return &DBTradeStore{db: db}
}

// Save 持久化成交记录到 PostgreSQL
func (s *DBTradeStore) Save(trade *Trade) error {
	query := `
		INSERT INTO trades (id, buy_order_id, sell_order_id, symbol, price, quantity, buy_user_id, sell_user_id, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (id) DO NOTHING
	`
	_, err := s.db.Exec(query,
		trade.ID, trade.BuyOrderID, trade.SellOrderID, trade.Symbol,
		trade.Price, trade.Quantity, trade.BuyUserID, trade.SellUserID,
		trade.CreatedAt,
	)
	return err
}

// GetByID 查询成交记录
func (s *DBTradeStore) GetByID(id string) (*Trade, error) {
	var trade Trade
	query := `SELECT id, buy_order_id, sell_order_id, symbol, price, quantity, buy_user_id, sell_user_id, created_at FROM trades WHERE id = $1`
	err := s.db.QueryRow(query, id).Scan(
		&trade.ID, &trade.BuyOrderID, &trade.SellOrderID, &trade.Symbol,
		&trade.Price, &trade.Quantity, &trade.BuyUserID, &trade.SellUserID,
		&trade.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &trade, nil
}

// GetByOrderID 根据订单 ID 查询
func (s *DBTradeStore) GetByOrderID(orderID string) ([]*Trade, error) {
	query := `SELECT id, buy_order_id, sell_order_id, symbol, price, quantity, buy_user_id, sell_user_id, created_at FROM trades WHERE buy_order_id = $1 OR sell_order_id = $1`
	rows, err := s.db.Query(query, orderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var trades []*Trade
	for rows.Next() {
		var t Trade
		rows.Scan(&t.ID, &t.BuyOrderID, &t.SellOrderID, &t.Symbol, &t.Price, &t.Quantity, &t.BuyUserID, &t.SellUserID, &t.CreatedAt)
		trades = append(trades, &t)
	}
	return trades, nil
}

// GetBySymbol 根据交易对查询
func (s *DBTradeStore) GetBySymbol(symbol string, limit int) ([]*Trade, error) {
	if limit <= 0 {
		limit = 10
	}
	query := `SELECT id, buy_order_id, sell_order_id, symbol, price, quantity, buy_user_id, sell_user_id, created_at FROM trades WHERE symbol = $1 ORDER BY created_at DESC LIMIT $2`
	rows, err := s.db.Query(query, symbol, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var trades []*Trade
	for rows.Next() {
		var t Trade
		rows.Scan(&t.ID, &t.BuyOrderID, &t.SellOrderID, &t.Symbol, &t.Price, &t.Quantity, &t.BuyUserID, &t.SellUserID, &t.CreatedAt)
		trades = append(trades, &t)
	}
	return trades, nil
}

// Close 关闭连接
func (s *DBTradeStore) Close() error { return nil }

// --- Redis Circuit Breaker ---

// RedisCircuitBreaker 分布式熔断器
type RedisCircuitBreaker struct {
	redis     *redis.Client
	key       string
	threshold int32
	timeout   time.Duration
}

// NewRedisCircuitBreaker 创建分布式熔断器
func NewRedisCircuitBreaker(redisAddr string, threshold int32, timeout time.Duration) *RedisCircuitBreaker {
	client := redis.NewClient(&redis.Options{Addr: redisAddr})
	return &RedisCircuitBreaker{
		redis:     client,
		key:       "circuit:breaker:state",
		threshold: threshold,
		timeout:   timeout,
	}
}

// IsOpen 判断熔断器是否打开（Redis 共享状态）
func (r *RedisCircuitBreaker) IsOpen() bool {
	val, err := r.redis.Get(context.Background(), r.key+":open").Result()
	if err != nil {
		return false
	}
	return val == "1"
}

// RecordFail 记录失败（分布式计数）
func (r *RedisCircuitBreaker) RecordFail() error {
	key := r.key + ":fail_count"
	ctx := context.Background()
	count, err := r.redis.Incr(ctx, key).Result()
	if err != nil {
		return err
	}

	// 设置过期时间
	r.redis.Expire(ctx, key, r.timeout)

	if count >= int64(r.threshold) {
		r.redis.Set(ctx, r.key+":open", "1", r.timeout)
	}
	return nil
}

// IsHalfOpen 判断熔断器是否处于半开状态（分布式版本暂不支持半开）
func (r *RedisCircuitBreaker) IsHalfOpen() bool {
	return false
}

// GetAverageLatency 获取平均响应时间（分布式版本暂无统计，返回 0）
func (r *RedisCircuitBreaker) GetAverageLatency() float64 {
	return 0
}

// Call 执行被熔断保护的函数
func (r *RedisCircuitBreaker) Call(fn func() error) error {
	if r.IsOpen() {
		return ErrCircuitOpen
	}
	if err := fn(); err != nil {
		_ = r.RecordFail()
		return err
	}
	return nil
}

// --- DistributedOrderBook 辅助方法 ---

// GetBuyDepth 获取买单深度
func (d *DistributedOrderBook) GetBuyDepth() int {
	ctx := context.Background()
	count, err := d.redis.ZCard(ctx, d.buyKey).Result()
	if err != nil {
		return 0
	}
	return int(count)
}

// GetSellDepth 获取卖单深度
func (d *DistributedOrderBook) GetSellDepth() int {
	ctx := context.Background()
	count, err := d.redis.ZCard(ctx, d.sellKey).Result()
	if err != nil {
		return 0
	}
	return int(count)
}

// Snapshot 获取订单簿快照
func (d *DistributedOrderBook) Snapshot() (map[float64][]*Order, map[float64][]*Order) {
	ctx := context.Background()
	buys := make(map[float64][]*Order)
	sells := make(map[float64][]*Order)

	// 读取买单
	buyResults, err := d.redis.ZRevRange(ctx, d.buyKey, 0, -1).Result()
	if err == nil {
		for _, data := range buyResults {
			var order Order
			if err := json.Unmarshal([]byte(data), &order); err == nil {
				buys[order.Price] = append(buys[order.Price], &order)
			}
		}
	}

	// 读取卖单
	sellResults, err := d.redis.ZRange(ctx, d.sellKey, 0, -1).Result()
	if err == nil {
		for _, data := range sellResults {
			var order Order
			if err := json.Unmarshal([]byte(data), &order); err == nil {
				sells[order.Price] = append(sells[order.Price], &order)
			}
		}
	}

	return buys, sells
}
