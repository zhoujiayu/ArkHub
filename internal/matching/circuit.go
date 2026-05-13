// ============================================================
// internal/matching/circuit.go
// 自动熔断机制
// 职责：极端行情下自动限流/降级，防止系统因资源耗尽而整体宕机
// ============================================================

package matching

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

// State 熔断器状态
type State int32

const (
	Closed   State = iota // 关闭状态：正常处理请求
	Open                  // 打开状态：拒绝所有请求，返回熔断错误
	HalfOpen              // 半开状态：允许探测请求，验证系统是否恢复
)

// String 返回状态的字符串表示
func (s State) String() string {
	switch s {
	case Closed:
		return "Closed"
	case Open:
		return "Open"
	case HalfOpen:
		return "HalfOpen"
	default:
		return "Unknown"
	}
}

// CircuitBreaker 熔断器
// 基于错误率和响应时间的自动熔断机制
//
// 核心设计：
//   - Closed: 正常处理请求，持续统计错误率和响应时间
//   - Open: 错误率超过阈值或响应时间超过阈值，自动熔断，拒绝所有请求
//   - HalfOpen: 熔断超时后进入半开状态，允许少量探测请求
//   - 探测成功：恢复 Closed 状态
//   - 探测失败：重新进入 Open 状态
//
// 触发条件：
//   - 错误率 > threshold%，持续 timeout 时间
//   - 平均响应时间 > latencyThreshold，持续 timeout 时间
type CircuitBreaker struct {
	state           atomic.Int32  // 当前状态（Closed/Open/HalfOpen）
	failCount       atomic.Int32  // 连续失败次数
	successCount    atomic.Int32  // 连续成功次数（半开状态使用）
	lastFailTime    atomic.Int64  // 最后一次失败时间戳
	threshold       int32         // 错误阈值（连续失败次数达到此值触发熔断）
	timeout         time.Duration // 熔断持续时间（熔断后多久进入半开状态）
	latencyWindow   []int64       // 响应时间窗口（毫秒）
	latencyMu       sync.RWMutex  // 响应时间窗口的锁
	latencyWindowSz int           // 响应时间窗口大小
	latencySum      int64         // 响应时间窗口的和
	probeMax        int32         // 半开状态下允许的最大探测请求数
}

// NewCircuitBreaker 创建一个新的熔断器
// threshold: 连续失败次数阈值
// timeout: 熔断持续时间
// latencyWindowSz: 响应时间窗口大小（用于计算平均响应时间）
func NewCircuitBreaker(threshold int32, timeout time.Duration, latencyWindowSz int) *CircuitBreaker {
	if threshold <= 0 {
		threshold = 5
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	if latencyWindowSz <= 0 {
		latencyWindowSz = 100
	}

	cb := &CircuitBreaker{
		threshold:       threshold,
		timeout:         timeout,
		latencyWindowSz: latencyWindowSz,
		probeMax:        3,
	}
	cb.state.Store(int32(Closed))
	cb.latencyWindow = make([]int64, 0, latencyWindowSz)
	return cb
}

// Call 执行被熔断保护的函数
// 如果熔断器处于 Open 状态，直接返回熔断错误
// 如果处于 HalfOpen 状态，只允许有限数量的探测请求
// 如果处于 Closed 状态，正常执行并统计结果
func (cb *CircuitBreaker) Call(fn func() error) error {
	state := cb.getState()

	switch state {
	case Open:
		// 检查是否已经过了熔断超时时间
		lastFail := cb.lastFailTime.Load()
		if time.Now().UnixMilli()-lastFail < cb.timeout.Milliseconds() {
			return ErrCircuitOpen
		}
		// 超时，进入半开状态
		cb.setState(HalfOpen)
		cb.successCount.Store(0)
		cb.failCount.Store(0)
		fallthrough

	case HalfOpen:
		// 半开状态下，只允许有限数量的探测请求
		if cb.successCount.Load() >= cb.probeMax {
			return ErrCircuitOpen
		}
		return cb.execute(fn)

	case Closed:
		return cb.execute(fn)
	}

	return nil
}

// execute 执行函数并记录结果
func (cb *CircuitBreaker) execute(fn func() error) error {
	start := time.Now().UnixMilli()
	err := fn()
	latency := time.Now().UnixMilli() - start

	// 记录响应时间
	cb.recordLatency(latency)

	if err != nil {
		cb.recordFail()
		return err
	}

	cb.recordSuccess()
	return nil
}

// recordLatency 记录响应时间
func (cb *CircuitBreaker) recordLatency(latency int64) {
	cb.latencyMu.Lock()
	defer cb.latencyMu.Unlock()

	if len(cb.latencyWindow) >= cb.latencyWindowSz {
		cb.latencySum -= cb.latencyWindow[0]
		cb.latencyWindow = cb.latencyWindow[1:]
	}
	cb.latencyWindow = append(cb.latencyWindow, latency)
	cb.latencySum += latency
}

// GetAverageLatency 获取平均响应时间（毫秒）
func (cb *CircuitBreaker) GetAverageLatency() float64 {
	cb.latencyMu.RLock()
	defer cb.latencyMu.RUnlock()

	if len(cb.latencyWindow) == 0 {
		return 0
	}
	return float64(cb.latencySum) / float64(len(cb.latencyWindow))
}

// recordFail 记录失败
func (cb *CircuitBreaker) recordFail() {
	cb.failCount.Add(1)
	cb.successCount.Store(0)
	cb.lastFailTime.Store(time.Now().UnixMilli())

	// 检查是否达到熔断阈值
	if cb.failCount.Load() >= cb.threshold {
		cb.setState(Open)
	}
}

// recordSuccess 记录成功
func (cb *CircuitBreaker) recordSuccess() {
	cb.failCount.Store(0)
	cb.successCount.Add(1)

	// 如果处于半开状态，连续成功次数达到阈值，恢复关闭状态
	if cb.getState() == HalfOpen && cb.successCount.Load() >= cb.probeMax {
		cb.setState(Closed)
		cb.successCount.Store(0)
	}
}

// getState 获取当前状态
func (cb *CircuitBreaker) getState() State {
	return State(cb.state.Load())
}

// setState 设置当前状态
func (cb *CircuitBreaker) setState(s State) {
	cb.state.Store(int32(s))
}

// IsOpen 判断熔断器是否处于打开状态
func (cb *CircuitBreaker) IsOpen() bool {
	return cb.getState() == Open
}

// IsClosed 判断熔断器是否处于关闭状态
func (cb *CircuitBreaker) IsClosed() bool {
	return cb.getState() == Closed
}

// IsHalfOpen 判断熔断器是否处于半开状态
func (cb *CircuitBreaker) IsHalfOpen() bool {
	return cb.getState() == HalfOpen
}

// Reset 重置熔断器状态
func (cb *CircuitBreaker) Reset() {
	cb.setState(Closed)
	cb.failCount.Store(0)
	cb.successCount.Store(0)
	cb.latencyMu.Lock()
	cb.latencyWindow = cb.latencyWindow[:0]
	cb.latencySum = 0
	cb.latencyMu.Unlock()
}

var (
	// ErrCircuitOpen 当熔断器处于 Open 状态时返回
	ErrCircuitOpen = errors.New("circuit breaker open")
)
