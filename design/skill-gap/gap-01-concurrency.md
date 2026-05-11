# 差距分析一：Go 高并发编程

> 对比范围：web3-go Week 1/Week 3/Week 4 vs ArkHub 订单撮合引擎

---

## 1. 技能对比

| 技能点 | web3-go 覆盖 | ArkHub 需求 | 差距 |
|--------|-------------|------------|------|
| goroutine + channel | ✅ 生产者-消费者、事件总线 | ✅ 必需 | 无差距 |
| sync.WaitGroup | ✅ 等待 goroutine 完成 | ✅ 必需 | 无差距 |
| sync.Mutex/RWMutex | ✅ 缓存客户端锁保护 | ✅ 必需 | 无差距 |
| errgroup | ✅ 并发扫块 Worker Pool | ✅ 必需 | 无差距 |
| context.Context | ✅ 超时控制、取消信号 | ✅ 必需 | 无差距 |
| **原子操作 (atomic)** | ❌ 未涉及 | ❌ CAS、原子读写 | 🔴 大差距 |
| **无锁队列 (Disruptor)** | ❌ 未涉及 | ❌ 百万级 TPS 订单接收 | 🔴 大差距 |
| **内存屏障/ happens-before** | ❌ 未涉及 | ❌ 无锁并发正确性保证 | 🔴 大差距 |
| **CPU 缓存优化** | ❌ 未涉及 | ❌ 伪共享避免、缓存行填充 | 🔴 大差距 |

---

## 2. web3-go 已覆盖内容

### 2.1 生产者-消费者模式

```go
// Week 1 Day 3
func producer(ch chan<- int, wg *sync.WaitGroup) {
    defer wg.Done()
    for i := 0; i < 100; i++ {
        ch <- i
    }
    close(ch)
}

func consumer(ch <-chan int, wg *sync.WaitGroup) {
    defer wg.Done()
    for v := range ch {
        fmt.Println(v)
    }
}
```

### 2.2 errgroup 并发扫块

```go
// Week 4
func (idx *Indexer) scanBlocks(ctx context.Context, start, end uint64) error {
    g, ctx := errgroup.WithContext(ctx)
    for i := start; i <= end; i++ {
        blockNum := i
        g.Go(func() error {
            return idx.processBlock(ctx, blockNum)
        })
    }
    return g.Wait()
}
```

---

## 3. ArkHub 需要补充的内容

### 3.1 原子操作 (atomic)

```go
// CAS 原子操作：无锁计数器
import "sync/atomic"

type AtomicCounter struct {
    value int64
}

func (c *AtomicCounter) Increment() int64 {
    return atomic.AddInt64(&c.value, 1)
}

func (c *AtomicCounter) Get() int64 {
    return atomic.LoadInt64(&c.value)
}

// CAS 循环实现原子更新
func (c *AtomicCounter) CompareAndSwap(old, new int64) bool {
    return atomic.CompareAndSwapInt64(&c.value, old, new)
}
```

**学习资源：**
- [Go 官方 atomic 文档](https://pkg.go.dev/sync/atomic)
- [Go 内存模型](https://go.dev/ref/mem)
- [Disruptor 论文](https://lmax-exchange.github.io/disruptor/)

### 3.2 无锁环形队列 (Disruptor)

```go
// Disruptor 风格的无锁环形队列
type RingBuffer[T any] struct {
    buffer   []T
    size     int64
    mask     int64
    _pad1    [56]byte                    // 缓存行填充，避免伪共享
    writeSeq atomic.Int64               // 写入序列号
    _pad2    [56]byte
    readSeq  atomic.Int64               // 读取序列号
}

func NewRingBuffer[T any](size int) *RingBuffer[T] {
    // size 必须是 2 的幂次方
    return &RingBuffer[T]{
        buffer: make([]T, size),
        size:   int64(size),
        mask:   int64(size - 1),
    }
}

// Put 无锁写入
func (rb *RingBuffer[T]) Put(value T) error {
    for {
        writeSeq := rb.writeSeq.Load()
        readSeq := rb.readSeq.Load()
        
        // 检查队列是否已满
        if writeSeq-readSeq >= rb.size {
            return errors.New("ring buffer full")
        }
        
        // CAS 更新写入序列号
        if rb.writeSeq.CompareAndSwap(writeSeq, writeSeq+1) {
            idx := writeSeq & rb.mask
            rb.buffer[idx] = value
            return nil
        }
        // CAS 失败，重试
    }
}

// Get 无锁读取
func (rb *RingBuffer[T]) Get() (T, error) {
    for {
        readSeq := rb.readSeq.Load()
        writeSeq := rb.writeSeq.Load()
        
        // 检查队列是否为空
        if readSeq >= writeSeq {
            var zero T
            return zero, errors.New("ring buffer empty")
        }
        
        // CAS 更新读取序列号
        if rb.readSeq.CompareAndSwap(readSeq, readSeq+1) {
            idx := readSeq & rb.mask
            return rb.buffer[idx], nil
        }
        // CAS 失败，重试
    }
}
```

**学习要点：**
1. **缓存行对齐**：使用 `_pad` 避免伪共享
2. **CAS 循环**：CompareAndSwap 失败时重试
3. **序列号替代锁**：用原子序列号替代互斥锁
4. **2 的幂次方大小**：用位运算替代取模

### 3.3 内存屏障与 happens-before

```go
// Go 中的 happens-before 关系
// 1. 同一个 goroutine 中，前面的操作 happens-before 后面的操作
// 2. channel 的 send happens-before 对应的 receive
// 3. sync.Mutex 的 Unlock happens-before 后续的 Lock
// 4. atomic.Store happens-before atomic.Load（同一个变量）

// 正确使用 atomic 保证可见性
var ready atomic.Bool
var data int

func writer() {
    data = 42              // 写数据
    ready.Store(true)       // 设置标志
}

func reader() {
    for !ready.Load() {    // 等待标志
        runtime.Gosched()
    }
    fmt.Println(data)       // 保证看到 42
}
```

### 3.4 CPU 缓存优化

```go
// 伪共享 (False Sharing) 示例与解决
type SharedCounter struct {
    // 错误：两个计数器在同一个缓存行，会导致缓存行频繁失效
    counter1 int64
    counter2 int64
}

// 正确：使用缓存行填充，确保每个计数器独占一个缓存行
type PaddedCounter struct {
    counter int64
    _pad    [56]byte  // 64 字节缓存行 - 8 字节 int64 = 56 字节填充
}

type OptimizedCounters struct {
    c1 PaddedCounter
    c2 PaddedCounter
}
```

---

## 4. 实践建议

### 4.1 学习计划

| 天数 | 主题 | 产出 |
|------|------|------|
| Day 1 | atomic 基础操作 | 实现无锁计数器、无锁栈 |
| Day 2 | CAS 循环模式 | 实现无锁队列、无锁链表 |
| Day 3 | Disruptor 原理 | 阅读 Disruptor 论文 |
| Day 4 | Disruptor 实现 | 手写简化版 Disruptor |
| Day 5 | CPU 缓存优化 | 测试伪共享影响、缓存行对齐 |

### 4.2 验证标准

- [ ] 无锁队列 Benchmark 达到 100万+ ops/sec
- [ ] 对比 sync.Mutex 实现，性能提升 5 倍以上
- [ ] 理解并能解释 Go 内存模型中的 happens-before 关系

---

## 5. 参考资源

| 资源 | 链接 | 说明 |
|------|------|------|
| Disruptor 论文 | https://lmax-exchange.github.io/disruptor/ | 核心原理 |
| Go 内存模型 | https://go.dev/ref/mem | 官方文档 |
| Go atomic 包 | https://pkg.go.dev/sync/atomic | API 参考 |
| 《Go 并发编程实战》 | 书籍 | 深入学习 |
