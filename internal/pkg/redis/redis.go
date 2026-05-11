// ============================================================
// internal/pkg/redis/redis.go
// Redis 缓存客户端封装
// 职责：提供 Redis 连接初始化、常用操作封装（缓存读写、分布式锁、限流计数等）
// ============================================================

package redis

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/go-redis/redis/v8"
)

// Config 定义 Redis 连接配置
// 字段说明：
//   Addr     - Redis 服务器地址（格式：host:port）
//   Password - Redis 密码（单机模式未设置则为空）
//   DB       - 数据库编号（0-15，不同业务使用不同 DB 隔离）
type Config struct {
	Addr     string
	Password string
	DB       int
}

// DefaultConfig 返回默认配置（本地开发环境）
func DefaultConfig() Config {
	return Config{
		Addr:     "localhost:6379",
		Password: "",
		DB:       0, // 默认使用 DB 0（订单簿缓存）
	}
}

// Client 封装 Redis 客户端，提供常用方法
// 内嵌 redis.Client，可直接调用原生方法
type Client struct {
	client *redis.Client
}

// NewClient 初始化 Redis 客户端
// 参数：
//   cfg - Redis 配置
// 返回：
//   *Client - Redis 客户端封装实例
//   error    - 初始化过程中的错误
func NewClient(cfg Config) (*Client, error) {
	// 创建 Redis 客户端实例
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,     // Redis 服务器地址
		Password: cfg.Password, // 密码（如未设置则为空）
		DB:       cfg.DB,       // 数据库编号
	})

	// 验证连接是否可用（使用 Ping 命令）
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := client.Ping(ctx).Result(); err != nil {
		return nil, fmt.Errorf("Redis 连接验证失败: %w", err)
	}

	log.Println("✅ Redis 客户端初始化成功")
	return &Client{client: client}, nil
}

// Close 关闭 Redis 连接
// 注意：程序退出时调用，释放资源
func (c *Client) Close() error {
	return c.client.Close()
}

// Get 从 Redis 获取键值
// 参数：
//   key - 键名
// 返回：
//   string - 键值
//   error  - 如果键不存在，返回 redis.Nil
func (c *Client) Get(ctx context.Context, key string) (string, error) {
	return c.client.Get(ctx, key).Result()
}

// Set 向 Redis 写入键值（带过期时间）
// 参数：
//   key    - 键名
//   value  - 键值
//   ttl    - 过期时间（如 10*time.Minute）
// 返回：
//   error - 写入过程中的错误
func (c *Client) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	return c.client.Set(ctx, key, value, ttl).Err()
}

// Delete 删除 Redis 中的键
// 参数：
//   keys - 要删除的键列表
// 返回：
//   int64 - 实际删除的键数量
func (c *Client) Delete(ctx context.Context, keys ...string) (int64, error) {
	return c.client.Del(ctx, keys...).Result()
}
