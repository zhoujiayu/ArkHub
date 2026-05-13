// ============================================================
// internal/chain/sync.go
// 事件同步引擎 — 毫秒级监听链上合约事件
// 职责：监听链上事件，解析并写入消息队列，更新同步断点
// ============================================================

package chain

import (
	"context"
	"log"
	"time"
)

// EventSyncEngine 事件同步引擎
type EventSyncEngine struct {
	client      ChainClient
	watchers    []EventWatcher
	outCh       chan ChainEvent
	checkpoint  int64 // 同步断点
	db          interface{} // 可选：持久化断点
}

// NewEventSyncEngine 创建事件同步引擎
func NewEventSyncEngine(client ChainClient) *EventSyncEngine {
	return &EventSyncEngine{
		client:   client,
		watchers: make([]EventWatcher, 0),
		outCh:    make(chan ChainEvent, 1000),
		checkpoint: 0,
	}
}

// AddWatcher 添加事件监听器
func (e *EventSyncEngine) AddWatcher(watcher EventWatcher) {
	e.watchers = append(e.watchers, watcher)
}

// Start 启动事件同步
func (e *EventSyncEngine) Start(ctx context.Context) error {
	log.Println("🚀 启动事件同步引擎...")

	for _, watcher := range e.watchers {
		go e.watch(ctx, watcher)
	}

	return nil
}

// watch 监听单个合约事件
func (e *EventSyncEngine) watch(ctx context.Context, watcher EventWatcher) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// 拉取新事件
			// TODO: 实际实现中通过 WebSocket 或轮询获取
			events, err := e.pollEvents(ctx, watcher)
			if err != nil {
				log.Printf("事件拉取失败: %v", err)
				continue
			}
			for _, event := range events {
				select {
				case e.outCh <- event:
				default:
					log.Printf("事件通道已满，丢弃事件")
				}
			}
		}
	}
}

// pollEvents 轮询获取事件（简化版）
func (e *EventSyncEngine) pollEvents(ctx context.Context, watcher EventWatcher) ([]ChainEvent, error) {
	// 实际实现中会调用链上 API 获取日志
	return []ChainEvent{}, nil
}

// Events 返回事件通道
func (e *EventSyncEngine) Events() <-chan ChainEvent {
	return e.outCh
}
