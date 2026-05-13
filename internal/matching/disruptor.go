// ============================================================
// internal/matching/disruptor.go
// 无锁环形队列（Disruptor 模式简化版）
// 职责：高并发无锁订单接收，避免锁竞争导致的性能瓶颈
// ============================================================

package matching

import (
	"errors"
	"sync/atomic"
)

// Order 表示一个交易订单
type Order struct {
	ID        string
	UserID    string
	Symbol    string
	Side      Side // Buy / Sell
	Price     float64
	Quantity  float64
	Status    Status // Pending / Partial / Filled / Cancelled
	CreatedAt int64  // 毫秒时间戳，用于时间优先排序
}

// Side 订单方向
type Side int

const (
	Buy Side = iota
	Sell
)

// Status 订单状态
type Status int

const (
	Pending   Status = iota // 待撮合：刚提交，尚未开始匹配
	Partial                 // 部分成交：已撮合一部分，还有剩余未成交
	Filled                  // 完全成交：全部数量已撮合完成
	Cancelled               // 已取消：用户主动取消或系统超时取消
)

// String 返回方向的字符串表示
func (s Side) String() string {
	if s == Buy {
		return "Buy"
	}
	return "Sell"
}

// StatusString 返回状态的字符串表示
func (s Status) StatusString() string {
	switch s {
	case Pending:
		return "Pending"
	case Partial:
		return "Partial"
	case Filled:
		return "Filled"
	case Cancelled:
		return "Cancelled"
	default:
		return "Unknown"
	}
}

// RingBuffer 无锁环形队列（Disruptor 简化版）
// 基于 CAS 原子操作实现生产者-消费者模型，避免锁竞争
// 适用于高并发订单入队的场景
//
// 核心设计：
//   - 预分配固定大小的环形缓冲区，size 必须是 2 的幂次方
//   - 通过 CAS 原子操作更新 write 和 read 指针
//   - 当队列满时，Put 返回错误，由调用方决定重试或丢弃
//   - 当队列空时，Get 返回错误，由调用方决定轮询或阻塞
//
type RingBuffer struct {
	buffer []Order // 预分配的订单缓冲区
	size   int64   // 缓冲区大小（必须是 2 的幂次方）
	mask   int64   // 用于快速取模：index & mask
	_      [56]byte // 防止 false sharing（缓存行对齐）
	write  atomic.Int64 // 生产者的写入位置
	_      [56]byte // 防止 false sharing（缓存行对齐）
	read   atomic.Int64 // 消费者的读取位置
}

// NewRingBuffer 创建指定容量的无锁环形队列
// capacity 必须是 2 的幂次方，若不是则向上取整到最近的 2 的幂次方
func NewRingBuffer(capacity int64) *RingBuffer {
	if capacity <= 0 {
		capacity = 1024
	}
	// 向上取整到最近的 2 的幂次方
	capVal := capacity
	if capVal&(capVal-1) != 0 {
		capVal = nextPowerOfTwo(capVal)
	}
	return &RingBuffer{
		buffer: make([]Order, capVal),
		size:   capVal,
		mask:   capVal - 1,
	}
}

// nextPowerOfTwo 向上取整到最近的 2 的幂次方
func nextPowerOfTwo(n int64) int64 {
	n--
	n |= n >> 1
	n |= n >> 2
	n |= n >> 4
	n |= n >> 8
	n |= n >> 16
	n |= n >> 32
	n++
	return n
}

// Put 无锁写入订单
// 使用 CAS 原子操作尝试写入，队列满时返回 ErrRingBufferFull
// 非阻塞设计：调用方可选择重试或降级
func (rb *RingBuffer) Put(order Order) error {
	for {
		writePos := rb.write.Load()
		readPos := rb.read.Load()

		// 检查队列是否已满（保留一个槽位作为区分空/满的标记）
		if writePos-readPos >= rb.size {
			return ErrRingBufferFull
		}

		// 尝试 CAS 原子更新 write 指针
		if rb.write.CompareAndSwap(writePos, writePos+1) {
			// CAS 成功，写入数据
			rb.buffer[writePos&rb.mask] = order
			return nil
		}
		// CAS 失败，其他 goroutine 已修改，重试
	}
}

// Get 无锁读取订单
// 使用 CAS 原子操作尝试读取，队列空时返回 ErrRingBufferEmpty
// 非阻塞设计：调用方可选择重试或阻塞等待
func (rb *RingBuffer) Get() (Order, error) {
	for {
		readPos := rb.read.Load()
		writePos := rb.write.Load()

		// 检查队列是否为空
		if readPos >= writePos {
			return Order{}, ErrRingBufferEmpty
		}

		// 尝试 CAS 原子更新 read 指针
		if rb.read.CompareAndSwap(readPos, readPos+1) {
			// CAS 成功，读取数据
			return rb.buffer[readPos&rb.mask], nil
		}
		// CAS 失败，其他 goroutine 已修改，重试
	}
}

// Len 返回当前队列中的元素数量
func (rb *RingBuffer) Len() int64 {
	return rb.write.Load() - rb.read.Load()
}

// Cap 返回队列总容量
func (rb *RingBuffer) Cap() int64 {
	return rb.size
}

var (
	// ErrRingBufferFull 当环形队列满时返回
	ErrRingBufferFull = errors.New("ring buffer full")
	// ErrRingBufferEmpty 当环形队列空时返回
	ErrRingBufferEmpty = errors.New("ring buffer empty")
)
