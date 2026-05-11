# 差距分析四：缓存与消息队列

> 对比范围：web3-go 无缓存/MQ vs ArkHub Redis、RocketMQ

---

## 1. 技能对比

| 技能点 | web3-go 覆盖 | ArkHub 需求 | 差距 |
|--------|-------------|------------|------|
| **Redis 基础** | ❌ 未涉及 | ❌ 缓存、会话、热点数据 | 🔴 大差距 |
| **Redis 分布式锁** | ❌ 未涉及 | ❌ 并发订单处理 | 🔴 大差距 |
| **缓存策略** | ❌ 未涉及 | ❌ 穿透/击穿/雪崩防护 | 🔴 大差距 |
| **RocketMQ** | ❌ 未涉及 | ❌ 消息队列、异步通信 | 🔴 大差距 |
| **消息可靠性** | ❌ 未涉及 | ❌ 发送确认、消费幂等 | 🔴 大差距 |
| **WebSocket** | ⚠️ 节点订阅 | ❌ Gateway、心跳、广播 | 🔴 大差距 |

---

## 2. Redis 分布式锁

### RedLock 算法

```go
package lock

import (
    "context"
    "crypto/rand"
    "encoding/hex"
    "fmt"
    "time"
    
    "github.com/redis/go-redis/v9"
)

// DistributedLock Redis 分布式锁
type DistributedLock struct {
    client *redis.Client
    key    string
    value  string
    ttl    time.Duration
}

// NewDistributedLock 创建分布式锁
func NewDistributedLock(client *redis.Client, key string, ttl time.Duration) *DistributedLock {
    return &DistributedLock{
        client: client,
        key:    key,
        value:  generateUniqueID(),
        ttl:    ttl,
    }
}

// Lock 获取锁
func (dl *DistributedLock) Lock(ctx context.Context) (bool, error) {
    // 使用 SET key value NX EX ttl 原子操作
    result, err := dl.client.SetNX(ctx, dl.key, dl.value, dl.ttl).Result()
    if err != nil {
        return false, err
    }
    return result, nil
}

// Unlock 释放锁
func (dl *DistributedLock) Unlock(ctx context.Context) error {
    // 使用 Lua 脚本保证原子性：只有 value 匹配时才删除
    script := `
        if redis.call("get", KEYS[1]) == ARGV[1] then
            return redis.call("del", KEYS[1])
        else
            return 0
        end
    `
    _, err := dl.client.Eval(ctx, script, []string{dl.key}, dl.value).Result()
    return err
}

// Renew 续期锁
func (dl *DistributedLock) Renew(ctx context.Context) (bool, error) {
    script := `
        if redis.call("get", KEYS[1]) == ARGV[1] then
            return redis.call("pexpire", KEYS[1], ARGV[2])
        else
            return 0
        end
    `
    result, err := dl.client.Eval(ctx, script, []string{dl.key}, dl.value, int64(dl.ttl.Milliseconds())).Result()
    if err != nil {
        return false, err
    }
    return result.(int64) == 1, nil
}

func generateUniqueID() string {
    b := make([]byte, 16)
    rand.Read(b)
    return hex.EncodeToString(b)
}
```

---

## 3. 缓存策略

### 缓存穿透防护

```go
package cache

import (
    "context"
    "time"
    
    "github.com/redis/go-redis/v9"
)

// Cache 缓存封装
type Cache struct {
    client      *redis.Client
    nullTTL     time.Duration
    bloomFilter *BloomFilter
}

// Get 获取缓存
func (c *Cache) Get(ctx context.Context, key string) (string, error) {
    // 1. Bloom Filter 检查
    if !c.bloomFilter.Contains([]byte(key)) {
        return "", fmt.Errorf("key not exist")
    }
    
    // 2. 从 Redis 获取
    val, err := c.client.Get(ctx, key).Result()
    if err == redis.Nil {
        return "", fmt.Errorf("key not exist")
    }
    if err != nil {
        return "", err
    }
    
    // 3. 空值缓存检查
    if val == "null" {
        return "", fmt.Errorf("key not exist")
    }
    
    return val, nil
}

// Set 设置缓存
func (c *Cache) Set(ctx context.Context, key string, value string, ttl time.Duration) error {
    // 添加到 Bloom Filter
    c.bloomFilter.Add([]byte(key))
    return c.client.Set(ctx, key, value, ttl).Err()
}

// SetNull 设置空值缓存
func (c *Cache) SetNull(ctx context.Context, key string) error {
    return c.client.Set(ctx, key, "null", c.nullTTL).Err()
}
```

### 缓存击穿防护

```go
package cache

import (
    "context"
    "sync"
    "time"
)

// SingleFlight 防止缓存击穿
type SingleFlight struct {
    mu    sync.Mutex
    calls map[string]*call
}

type call struct {
    wg  sync.WaitGroup
    val interface{}
    err error
}

func NewSingleFlight() *SingleFlight {
    return &SingleFlight{
        calls: make(map[string]*call),
    }
}

// Do 执行函数，保证同一时刻只有一个 goroutine 执行
func (sf *SingleFlight) Do(key string, fn func() (interface{}, error)) (interface{}, error) {
    sf.mu.Lock()
    if c, ok := sf.calls[key]; ok {
        sf.mu.Unlock()
        c.wg.Wait()
        return c.val, c.err
    }
    
    c := &call{}
    c.wg.Add(1)
    sf.calls[key] = c
    sf.mu.Unlock()
    
    // 执行函数
    c.val, c.err = fn()
    c.wg.Done()
    
    // 清理
    sf.mu.Lock()
    delete(sf.calls, key)
    sf.mu.Unlock()
    
    return c.val, c.err
}
```

---

## 4. RocketMQ 消息队列

### 生产者封装

```go
package mq

import (
    "context"
    "fmt"
    
    "github.com/apache/rocketmq-client-go/v2/producer"
    "github.com/apache/rocketmq-client-go/v2/primitive"
)

// RocketMQProducer 消息生产者
type RocketMQProducer struct {
    producer producer.Producer
}

// NewRocketMQProducer 创建生产者
func NewRocketMQProducer(nameServer []string, group string) (*RocketMQProducer, error) {
    p, err := producer.NewProducer(
        producer.WithNameServer(nameServer),
        producer.WithGroupName(group),
        producer.WithRetry(3),
    )
    if err != nil {
        return nil, err
    }
    
    err = p.Start()
    if err != nil {
        return nil, err
    }
    
    return &RocketMQProducer{producer: p}, nil
}

// Send 发送消息
func (p *RocketMQProducer) Send(ctx context.Context, topic string, body []byte, tags ...string) error {
    tag := ""
    if len(tags) > 0 {
        tag = tags[0]
    }
    
    msg := &primitive.Message{
        Topic: topic,
        Body:  body,
        Tag:   tag,
    }
    
    result, err := p.producer.SendSync(ctx, msg)
    if err != nil {
        return err
    }
    
    if result.Status != primitive.SendOK {
        return fmt.Errorf("send failed: %s", result.Status)
    }
    
    return nil
}

// SendAsync 异步发送
func (p *RocketMQProducer) SendAsync(ctx context.Context, topic string, body []byte, callback func(result *primitive.SendResult, err error)) error {
    msg := &primitive.Message{
        Topic: topic,
        Body:  body,
    }
    
    p.producer.SendAsync(ctx, msg, callback)
    return nil
}

// Close 关闭生产者
func (p *RocketMQProducer) Close() error {
    return p.producer.Shutdown()
}
```

### 消费者封装

```go
package mq

import (
    "context"
    "fmt"
    
    "github.com/apache/rocketmq-client-go/v2/consumer"
    "github.com/apache/rocketmq-client-go/v2/primitive"
)

// MessageHandler 消息处理器
type MessageHandler func(ctx context.Context, msg *primitive.MessageExt) error

// RocketMQConsumer 消息消费者
type RocketMQConsumer struct {
    consumer consumer.PushConsumer
    handlers map[string]MessageHandler
}

// NewRocketMQConsumer 创建消费者
func NewRocketMQConsumer(nameServer []string, group string) (*RocketMQConsumer, error) {
    c, err := consumer.NewPushConsumer(
        consumer.WithNameServer(nameServer),
        consumer.WithGroupName(group),
        consumer.WithConsumeMessageBatchMaxSize(10),
    )
    if err != nil {
        return nil, err
    }
    
    return &RocketMQConsumer{
        consumer: c,
        handlers: make(map[string]MessageHandler),
    }, nil
}

// Subscribe 订阅主题
func (c *RocketMQConsumer) Subscribe(topic string, handler MessageHandler) error {
    c.handlers[topic] = handler
    
    return c.consumer.Subscribe(topic, consumer.MessageSelector{}, func(ctx context.Context,
        msgs ...*primitive.MessageExt) (consumer.ConsumeResult, error) {
        for _, msg := range msgs {
            if handler, ok := c.handlers[msg.Topic]; ok {
                if err := handler(ctx, msg); err != nil {
                    // 消费失败，返回重试
                    return consumer.ConsumeRetryLater, err
                }
            }
        }
        return consumer.ConsumeSuccess, nil
    })
}

// Start 启动消费
func (c *RocketMQConsumer) Start() error {
    return c.consumer.Start()
}

// Close 关闭消费者
func (c *RocketMQConsumer) Close() error {
    return c.consumer.Shutdown()
}
```

---

## 5. WebSocket Gateway

```go
package ws

import (
    "context"
    "fmt"
    "net/http"
    "sync"
    "time"
    
    "github.com/gorilla/websocket"
)

// WebSocketGateway WebSocket 网关
type WebSocketGateway struct {
    upgrader    websocket.Upgrader
    clients     map[string]*Client
    mu          sync.RWMutex
    broadcast   chan Message
    heartbeat   time.Duration
}

// Client WebSocket 客户端
type Client struct {
    ID       string
    Conn     *websocket.Conn
    Send     chan []byte
    Topics   map[string]bool
    mu       sync.RWMutex
}

// Message 消息结构
type Message struct {
    Type    string      `json:"type"`
    Topic   string      `json:"topic"`
    Payload interface{} `json:"payload"`
}

// NewWebSocketGateway 创建网关
func NewWebSocketGateway() *WebSocketGateway {
    return &WebSocketGateway{
        upgrader: websocket.Upgrader{
            CheckOrigin: func(r *http.Request) bool {
                return true
            },
        },
        clients:   make(map[string]*Client),
        broadcast: make(chan Message, 1000),
        heartbeat: 30 * time.Second,
    }
}

// HandleConnection 处理连接
func (g *WebSocketGateway) HandleConnection(w http.ResponseWriter, r *http.Request) {
    conn, err := g.upgrader.Upgrade(w, r, nil)
    if err != nil {
        return
    }
    
    client := &Client{
        ID:     generateClientID(),
        Conn:   conn,
        Send:   make(chan []byte, 256),
        Topics: make(map[string]bool),
    }
    
    g.addClient(client)
    
    // 启动 goroutine
    go g.readPump(client)
    go g.writePump(client)
    
    // 发送欢迎消息
    g.sendToClient(client, Message{
        Type:    "connected",
        Topic:   "system",
        Payload: map[string]string{"client_id": client.ID},
    })
}

// readPump 读取消息
func (g *WebSocketGateway) readPump(client *Client) {
    defer g.removeClient(client)
    
    client.Conn.SetReadDeadline(time.Now().Add(g.heartbeat + 10*time.Second))
    client.Conn.SetPongHandler(func(string) error {
        client.Conn.SetReadDeadline(time.Now().Add(g.heartbeat + 10*time.Second))
        return nil
    })
    
    for {
        _, message, err := client.Conn.ReadMessage()
        if err != nil {
            break
        }
        
        // 处理消息
        g.handleMessage(client, message)
    }
}

// writePump 发送消息
func (g *WebSocketGateway) writePump(client *Client) {
    ticker := time.NewTicker(g.heartbeat)
    defer ticker.Stop()
    
    for {
        select {
        case message, ok := <-client.Send:
            if !ok {
                client.Conn.WriteMessage(websocket.CloseMessage, []byte{})
                return
            }
            
            client.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
            client.Conn.WriteMessage(websocket.TextMessage, message)
            
        case <-ticker.C:
            client.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
            client.Conn.WriteMessage(websocket.PingMessage, nil)
        }
    }
}

// Broadcast 广播消息
func (g *WebSocketGateway) Broadcast(msg Message) {
    g.broadcast <- msg
}

// Subscribe 订阅主题
func (g *WebSocketGateway) Subscribe(client *Client, topic string) {
    client.mu.Lock()
    client.Topics[topic] = true
    client.mu.Unlock()
}

// Publish 发布消息到主题
func (g *WebSocketGateway) Publish(topic string, payload interface{}) {
    g.broadcast <- Message{
        Type:    "data",
        Topic:   topic,
        Payload: payload,
    }
}

func (g *WebSocketGateway) addClient(client *Client) {
    g.mu.Lock()
    g.clients[client.ID] = client
    g.mu.Unlock()
}

func (g *WebSocketGateway) removeClient(client *Client) {
    g.mu.Lock()
    delete(g.clients, client.ID)
    g.mu.Unlock()
    close(client.Send)
    client.Conn.Close()
}

func (g *WebSocketGateway) sendToClient(client *Client, msg Message) {
    data, _ := json.Marshal(msg)
    select {
    case client.Send <- data:
    default:
    }
}

func (g *WebSocketGateway) handleMessage(client *Client, data []byte) {
    // 解析消息并处理
}

func generateClientID() string {
    return fmt.Sprintf("client_%d", time.Now().UnixNano())
}
```

---

## 6. 学习建议

| 优先级 | 主题 | 学习时间 | 产出 |
|--------|------|---------|------|
| P0 | Redis 分布式锁 | 1 天 | 实现 RedLock |
| P0 | 缓存策略 | 1 天 | 实现防护机制 |
| P0 | RocketMQ 基础 | 1 天 | 实现收发消息 |
| P0 | WebSocket Gateway | 1 天 | 实现心跳/广播 |
| P1 | 消息可靠性 | 1 天 | 实现事务消息 |
| P1 | 消息幂等 | 0.5 天 | 实现幂等消费 |

---

## 7. 参考资源

| 资源 | 链接 | 说明 |
|------|------|------|
| Redis 官方文档 | https://redis.io/docs/ | 官方文档 |
| go-redis | https://github.com/redis/go-redis | Go 客户端 |
| RocketMQ 文档 | https://rocketmq.apache.org/docs/ | 官方文档 |
| gorilla/websocket | https://github.com/gorilla/websocket | WebSocket 库 |
