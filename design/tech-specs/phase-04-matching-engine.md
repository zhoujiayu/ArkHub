# 阶段四：订单撮合引擎技术方案

> 阶段目标：实现 Disruptor 无锁队列、内存订单簿、撮合算法、成交记录生成、自动熔断。
> 预计工期：10 天
> 依赖阶段：阶段一、阶段二

---

## 0. 设计原因

### 0.1 为什么用内存订单簿而非数据库订单簿

**业务背景**：
- 订单簿是撮合引擎的核心数据结构，每秒可能被读写数万次
- PostgreSQL 单次查询 5-10ms，无法满足撮合延迟 <10ms 的目标
- 内存操作比数据库快 1000 倍，是高性能撮合的必然选择

**设计决策**：

| 方案 | 延迟 | 吞吐量 | 适用场景 | 选择原因 |
|------|------|--------|---------|---------|
| **内存订单簿** | <1μs | 百万级 TPS | 核心撮合 | 极致性能 |
| **Redis 订单簿** | 1-5ms | 十万级 TPS | 辅助查询 | 持久化备份 |
| **PostgreSQL 订单簿** | 5-10ms | 千级 TPS | 历史归档 | 最终一致性 |

**关键设计**：
- 买单按价格从高到低排序（优先成交高价买单）
- 卖单按价格从低到高排序（优先成交低价卖单）
- 红黑树实现 O(log n) 级别的插入、删除、查询
- 异步持久化成交记录至 PostgreSQL，内存仅保留活跃订单

### 0.2 为什么选 Disruptor 而非 Kafka 或自研队列

| 队列方案 | 延迟 | 吞吐量 | 锁机制 | 选择原因 |
|---------|------|--------|--------|---------|
| **Disruptor** | <1μs | 600万+ TPS | 无锁 CAS | LMAX 架构，金融领域验证 |
| **Kafka** | 1-10ms | 百万级 TPS | 分布式锁 | 高吞吐，但延迟不满足 |
| **Go channel** | 10-100μs | 十万级 TPS | 有锁 | 简单易用，但性能不足 |
| **自研环形队列** | 可变 | 可变 | 需精细设计 | 维护成本高 |

**Disruptor 核心优势**：
- 无锁设计：通过 CAS 原子操作避免锁竞争，CPU 缓存友好
- 预分配内存：环形缓冲区预分配，避免 GC 压力
- 批量消费：消费者批量处理，减少上下文切换

### 0.3 为什么需要自动熔断

**业务背景**：
- 极端行情（如 2021 年 5·19、2022 年 Luna 崩盘）下，订单量暴增 10 倍+
- 若不做熔断，系统可能因资源耗尽而整体宕机
- 竞品在极端行情中多次出现宕机，用户资产安全受损

**设计决策**：
- 错误率 >50% 持续 30 秒 → 自动熔断
- 响应时间 >2s 持续 60 秒 → 自动熔断
- 熔断后每 10 秒允许 1 个探测请求，成功后逐步恢复
- 熔断期间返回缓存价格，保证查询可用

**业务价值**：
- 极端行情下系统 0 崩溃，保障用户资产安全
- 熔断期间仍可查询行情，用户不会完全失联
- 相比竞品宕机数小时，ArkHub 仅需数秒恢复

---

## 1. 模块拆分

| 子模块 | 描述 | 验证方式 |
|--------|------|---------|
| **4.1 Disruptor 队列** | 无锁环形队列，高并发订单接收 | Benchmark 测试吞吐 |
| **4.2 内存订单簿** | 价格优先 + 时间优先的买卖盘 | 单元测试验证订单簿操作 |
| **4.3 撮合算法** | 价格优先、时间优先的撮合逻辑 | 多场景撮合测试 |
| **4.4 成交记录** | 生成成交记录、异步落库 | 验证成交记录正确性 |
| **4.5 自动熔断** | 极端行情自动限流/降级 | 模拟极端行情，验证熔断 |
| **4.6 资产扣减** | 用户余额校验与扣减 | 并发扣减测试 |

---

## 2. 技术方案

### 2.1 Disruptor 队列

```go
type RingBuffer struct {
    buffer []Order
    size   int64
    mask   int64
    write  atomic.Int64
    read   atomic.Int64
}

func (rb *RingBuffer) Put(order Order) error {
    // CAS 无锁写入
}

func (rb *RingBuffer) Get() (Order, error) {
    // CAS 无锁读取
}
```

### 2.2 内存订单簿

```go
type OrderBook struct {
    buys  *redblacktree.Tree // 价格从高到低
    sells *redblacktree.Tree // 价格从低到高
    mutex sync.RWMutex
}

type Order struct {
    ID        string
    UserID    string
    Symbol    string
    Side      Side // Buy / Sell
    Price     float64
    Quantity  float64
    Status    Status // Pending / Partial / Filled / Cancelled
    CreatedAt time.Time
}
```

### 2.3 撮合算法

```go
func (ob *OrderBook) Match(order Order) []Trade {
    var trades []Trade
    if order.Side == Buy {
        // 从 sell 队列取最低价匹配
        for ob.sells.Len() > 0 && order.Quantity > 0 {
            bestSell := ob.sells.Min()
            if bestSell.Price > order.Price {
                break
            }
            trade := ob.executeMatch(order, bestSell)
            trades = append(trades, trade)
        }
    }
    return trades
}
```

### 2.4 成交记录

```go
type Trade struct {
    ID           string
    BuyOrderID   string
    SellOrderID  string
    Symbol       string
    Price        float64
    Quantity     float64
    BuyUserID    string
    SellUserID   string
    CreatedAt    time.Time
}
```

### 2.5 自动熔断

```go
type CircuitBreaker struct {
    state       State // Closed / Open / HalfOpen
    failCount   atomic.Int32
    threshold   int32
    timeout     time.Duration
    lastFail    time.Time
}

func (cb *CircuitBreaker) Call(fn func() error) error {
    if cb.state == Open {
        if time.Since(cb.lastFail) < cb.timeout {
            return errors.New("circuit breaker open")
        }
        cb.state = HalfOpen
    }
    err := fn()
    if err != nil {
        cb.recordFail()
        return err
    }
    cb.reset()
    return nil
}
```

---

## 3. 验证清单

| 检查项 | 验证方法 | 通过标准 |
|--------|---------|---------|
| Disruptor 吞吐 | `go test -bench` | ≥ 100万 TPS |
| 订单簿操作 | 单元测试 | 增删改查正确 |
| 撮合逻辑 | 场景测试 | 价格优先、时间优先 |
| 成交记录 | 验证生成记录 | 记录完整准确 |
| 熔断机制 | 模拟连续失败 | 正确触发熔断 |
| 资产扣减 | 并发测试 | 余额一致性 |
| 性能测试 | 压测 | 撮合延迟 < 10ms |

---

## 4. 风险与应对

| 风险 | 影响 | 应对措施 |
|------|------|---------|
| 内存订单簿数据丢失 | 订单丢失 | 异步持久化 + 快照机制 |
| 撮合死锁 | 系统卡死 | 无锁队列 + 超时机制 |
| 极端行情 | 系统过载 | 熔断降级 + 队列限流 |

---

## 5. 交付物

- [ ] `cmd/matching-engine/` — 撮合引擎服务
- [ ] `internal/matching/disruptor.go` — Disruptor 队列
- [ ] `internal/matching/orderbook.go` — 内存订单簿
- [ ] `internal/matching/matcher.go` — 撮合算法
- [ ] `internal/matching/trade.go` — 成交记录
- [ ] `internal/matching/circuit.go` — 熔断机制
- [ ] `test/integration/matching_test.go` — 集成测试
- [ ] `docs/matching-api.md` — API 文档
