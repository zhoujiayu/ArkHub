# 差距分析二：数据结构与算法

> 对比范围：web3-go 基础数据结构 vs ArkHub 内存订单簿、索引结构

---

## 1. 技能对比

| 数据结构/算法 | web3-go 覆盖 | ArkHub 需求 | 差距 |
|-------------|-------------|------------|------|
| map (哈希表) | ✅ 基础使用 | ✅ 用户余额、订单索引 | 无差距 |
| slice (动态数组) | ✅ 基础使用 | ✅ 价格列表、日志存储 | 无差距 |
| 链表 | ⚠️ 基础实现 | ⚠️ 订单链表、事件链 | 中等 |
| **红黑树** | ❌ 未涉及 | ❌ 订单簿价格排序 | 🔴 大差距 |
| **跳表 (Skip List)** | ❌ 未涉及 | ❌ 有序数据索引 | 🔴 大差距 |
| **环形队列** | ❌ 未涉及 | ❌ Disruptor 核心结构 | 🔴 大差距 |
| **优先队列 (堆)** | ❌ 未涉及 | ⚠️ 订单优先级排序 | 中等 |
| **Bloom Filter** | ❌ 未涉及 | ❌ 防止缓存穿透 | 🔴 大差距 |
| **一致性哈希** | ❌ 未涉及 | ❌ 分布式缓存分片 | 🔴 大差距 |
| **LRU 缓存** | ❌ 未涉及 | ❌ 热点数据缓存 | 🔴 大差距 |

---

## 2. 红黑树 (Red-Black Tree)

### 为什么需要红黑树

订单簿需要**按价格排序**的买卖盘，要求：
- 快速插入新订单（O(log n)）
- 快速删除成交订单（O(log n)）
- 快速查找最优价格（O(log n)）

Go 标准库没有内置红黑树，需要自研或使用第三方库。

### Go 实现示例

```go
package orderbook

import (
    "fmt"
    "github.com/emirpasic/gods/trees/redblacktree"
)

// Order 订单结构
type Order struct {
    ID        string
    Price     float64
    Quantity  float64
    Timestamp int64
    UserID    string
}

// OrderBook 内存订单簿
type OrderBook struct {
    // 买单：价格从高到低
    bids *redblacktree.Tree
    // 卖单：价格从低到高
    asks *redblacktree.Tree
}

// 比较函数：价格优先，时间次之
func bidComparator(a, b interface{}) int {
    o1, o2 := a.(*Order), b.(*Order)
    // 价格从高到低
    if o1.Price > o2.Price { return -1 }
    if o1.Price < o2.Price { return 1 }
    // 时间从早到晚
    if o1.Timestamp < o2.Timestamp { return -1 }
    if o1.Timestamp > o2.Timestamp { return 1 }
    return 0
}

func askComparator(a, b interface{}) int {
    o1, o2 := a.(*Order), b.(*Order)
    // 价格从低到高
    if o1.Price < o2.Price { return -1 }
    if o1.Price > o2.Price { return 1 }
    // 时间从早到晚
    if o1.Timestamp < o2.Timestamp { return -1 }
    if o1.Timestamp > o2.Timestamp { return 1 }
    return 0
}

func NewOrderBook() *OrderBook {
    return &OrderBook{
        bids: redblacktree.NewWith(bidComparator),
        asks: redblacktree.NewWith(askComparator),
    }
}

// AddBid 添加买单
func (ob *OrderBook) AddBid(order *Order) {
    ob.bids.Put(order.ID, order)
}

// AddAsk 添加卖单
func (ob *OrderBook) AddAsk(order *Order) {
    ob.asks.Put(order.ID, order)
}

// GetBestBid 获取最高买单价格
func (ob *OrderBook) GetBestBid() *Order {
    if ob.bids.Empty() {
        return nil
    }
    node := ob.bids.Left()  // 红黑树最左节点
    return node.Value.(*Order)
}

// GetBestAsk 获取最低卖单价格
func (ob *OrderBook) GetBestAsk() *Order {
    if ob.asks.Empty() {
        return nil
    }
    node := ob.asks.Left()
    return node.Value.(*Order)
}
```

### 替代方案

| 方案 | 优点 | 缺点 |
|------|------|------|
| `github.com/emirpasic/gods` | 成熟、功能全 | 引入外部依赖 |
| 手写红黑树 | 无依赖、完全可控 | 开发成本高 |
| 有序 map 模拟 | 简单 | 性能差 (O(n)) |
| 跳表替代 | 实现简单 | 查询稍慢 |

---

## 3. 跳表 (Skip List)

### 适用场景

- 需要有序存储且支持范围查询
- 实现比红黑树简单
- 并发友好（支持无锁实现）

```go
package skiplist

import (
    "math/rand"
    "sync"
)

type Node struct {
    key     float64
    value   interface{}
    forward []*Node
}

type SkipList struct {
    head   *Node
    maxLevel int
    level    int
    mutex    sync.RWMutex
}

func NewSkipList(maxLevel int) *SkipList {
    return &SkipList{
        head:     &Node{forward: make([]*Node, maxLevel)},
        maxLevel: maxLevel,
        level:    1,
    }
}

func (sl *SkipList) randomLevel() int {
    level := 1
    for level < sl.maxLevel && rand.Float32() < 0.5 {
        level++
    }
    return level
}

func (sl *SkipList) Insert(key float64, value interface{}) {
    sl.mutex.Lock()
    defer sl.mutex.Unlock()
    
    update := make([]*Node, sl.maxLevel)
    current := sl.head
    
    // 查找插入位置
    for i := sl.level - 1; i >= 0; i-- {
        for current.forward[i] != nil && current.forward[i].key < key {
            current = current.forward[i]
        }
        update[i] = current
    }
    
    // 生成随机层数
    level := sl.randomLevel()
    if level > sl.level {
        for i := sl.level; i < level; i++ {
            update[i] = sl.head
        }
        sl.level = level
    }
    
    // 创建新节点
    newNode := &Node{
        key:     key,
        value:   value,
        forward: make([]*Node, level),
    }
    
    // 插入节点
    for i := 0; i < level; i++ {
        newNode.forward[i] = update[i].forward[i]
        update[i].forward[i] = newNode
    }
}
```

---

## 4. Bloom Filter

### 适用场景

防止缓存穿透：快速判断一个元素**一定不存在**于集合中。

```go
package bloom

import (
    "hash/fnv"
    "math"
)

type BloomFilter struct {
    bits   []uint64
    size   uint64
    hashes int
}

func NewBloomFilter(expectedItems int, falsePositiveRate float64) *BloomFilter {
    size := uint64(-float64(expectedItems) * math.Log(falsePositiveRate) / math.Pow(math.Ln2, 2))
    hashes := int(math.Ceil(float64(size) / float64(expectedItems) * math.Ln2))
    
    return &BloomFilter{
        bits:   make([]uint64, (size+63)/64),
        size:   size,
        hashes: hashes,
    }
}

func (bf *BloomFilter) hash(data []byte, seed int) uint64 {
    h := fnv.New64a()
    h.Write(data)
    h.Write([]byte{byte(seed)})
    return h.Sum64() % bf.size
}

func (bf *BloomFilter) Add(data []byte) {
    for i := 0; i < bf.hashes; i++ {
        idx := bf.hash(data, i)
        bf.bits[idx/64] |= 1 << (idx % 64)
    }
}

func (bf *BloomFilter) Contains(data []byte) bool {
    for i := 0; i < bf.hashes; i++ {
        idx := bf.hash(data, i)
        if bf.bits[idx/64]&(1<<(idx%64)) == 0 {
            return false
        }
    }
    return true
}
```

---

## 5. LRU 缓存

```go
package lru

import (
    "container/list"
    "sync"
)

type Cache struct {
    mu        sync.RWMutex
    maxSize   int
    ll        *list.List
    cache     map[string]*list.Element
}

type entry struct {
    key   string
    value interface{}
}

func NewCache(maxSize int) *Cache {
    return &Cache{
        maxSize: maxSize,
        ll:      list.New(),
        cache:   make(map[string]*list.Element),
    }
}

func (c *Cache) Get(key string) (interface{}, bool) {
    c.mu.Lock()
    defer c.mu.Unlock()
    
    if ele, ok := c.cache[key]; ok {
        c.ll.MoveToFront(ele)
        return ele.Value.(*entry).value, true
    }
    return nil, false
}

func (c *Cache) Set(key string, value interface{}) {
    c.mu.Lock()
    defer c.mu.Unlock()
    
    if ele, ok := c.cache[key]; ok {
        c.ll.MoveToFront(ele)
        ele.Value.(*entry).value = value
        return
    }
    
    ele := c.ll.PushFront(&entry{key, value})
    c.cache[key] = ele
    
    if c.ll.Len() > c.maxSize {
        c.removeOldest()
    }
}

func (c *Cache) removeOldest() {
    ele := c.ll.Back()
    if ele != nil {
        c.ll.Remove(ele)
        kv := ele.Value.(*entry)
        delete(c.cache, kv.key)
    }
}
```

---

## 6. 学习建议

| 优先级 | 数据结构 | 学习时间 | 验证方式 |
|--------|---------|---------|---------|
| P0 | 红黑树 | 2 天 | 实现订单簿，测试插入/删除/查询 |
| P0 | 跳表 | 1 天 | 实现有序集合，对比性能 |
| P1 | Bloom Filter | 0.5 天 | 实现缓存穿透防护 |
| P1 | LRU 缓存 | 0.5 天 | 实现带淘汰策略的缓存 |
| P2 | 一致性哈希 | 1 天 | 实现分布式缓存分片 |

---

## 7. 参考资源

| 资源 | 说明 |
|------|------|
| `github.com/emirpasic/gods` | Go 数据结构库 |
| `github.com/hashicorp/golang-lru` | LRU 缓存实现 |
| 《算法导论》第 13 章 | 红黑树理论 |
| Redis 源码 skiplist.c | 跳表实现参考 |
