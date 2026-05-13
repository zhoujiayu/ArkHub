// ============================================================
// internal/buyback/cache.go
// 缓存层 — Redis 预聚合结果缓存
// 职责：读写 Redis 缓存，TTL 自动管理过期
// ============================================================

package buyback

import (
	"context"
	"encoding/json"
	"time"

	"github.com/go-redis/redis/v8"
)

// CacheLayer 缓存层接口
type CacheLayer interface {
	SetStats(ctx context.Context, key string, stats *BuybackStats) error
	GetStats(ctx context.Context, key string) (*BuybackStats, error)
	Invalidate(ctx context.Context, pattern string) error
}

// RedisCacheLayer Redis 缓存实现
type RedisCacheLayer struct {
	client *redis.Client
	ttl    time.Duration
}

// NewRedisCacheLayer 创建 Redis 缓存层
func NewRedisCacheLayer(addr string, ttl time.Duration) CacheLayer {
	client := redis.NewClient(&redis.Options{Addr: addr})
	return &RedisCacheLayer{client: client, ttl: ttl}
}

// SetStats 写入缓存
func (c *RedisCacheLayer) SetStats(ctx context.Context, key string, stats *BuybackStats) error {
	data, err := json.Marshal(stats)
	if err != nil {
		return err
	}
	return c.client.Set(ctx, key, data, c.ttl).Err()
}

// GetStats 读取缓存
func (c *RedisCacheLayer) GetStats(ctx context.Context, key string) (*BuybackStats, error) {
	data, err := c.client.Get(ctx, key).Bytes()
	if err != nil {
		return nil, err
	}
	var stats BuybackStats
	if err := json.Unmarshal(data, &stats); err != nil {
		return nil, err
	}
	return &stats, nil
}

// Invalidate 按模式清除缓存
func (c *RedisCacheLayer) Invalidate(ctx context.Context, pattern string) error {
keys, err := c.client.Keys(ctx, pattern).Result()
	if err != nil {
		return err
	}
	if len(keys) > 0 {
		return c.client.Del(ctx, keys...).Err()
	}
	return nil
}

// InMemoryCacheLayer 内存缓存（测试用）
type InMemoryCacheLayer struct {
	data map[string][]byte
}

// NewInMemoryCacheLayer 创建内存缓存
func NewInMemoryCacheLayer() CacheLayer {
	return &InMemoryCacheLayer{data: make(map[string][]byte)}
}

func (c *InMemoryCacheLayer) SetStats(ctx context.Context, key string, stats *BuybackStats) error {
	data, err := json.Marshal(stats)
	if err != nil {
		return err
	}
	c.data[key] = data
	return nil
}

func (c *InMemoryCacheLayer) GetStats(ctx context.Context, key string) (*BuybackStats, error) {
	data, ok := c.data[key]
	if !ok {
		return nil, redis.Nil
	}
	var stats BuybackStats
	if err := json.Unmarshal(data, &stats); err != nil {
		return nil, err
	}
	return &stats, nil
}

func (c *InMemoryCacheLayer) Invalidate(ctx context.Context, pattern string) error {
	for key := range c.data {
		delete(c.data, key)
	}
	return nil
}
