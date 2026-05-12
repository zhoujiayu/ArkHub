// ============================================================
// internal/market/adapter.go
// 多协议行情适配器
// 职责：抽象 REST / WebSocket / FIX / Internal 协议，统一行情数据接入
// ============================================================

package market

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// PriceTick 表示单条行情数据
// 用于各协议适配器之间统一的数据结构
type PriceTick struct {
	Source    string    `json:"source"`    // 数据源名称，如 binance、okx
	Symbol    string    `json:"symbol"`    // 交易对，如 BTC-USDT
	Price     float64   `json:"price"`     // 最新成交价
	Timestamp int64     `json:"timestamp"` // 数据产生时间戳（毫秒）
}

// MarketDataSource 定义行情数据源接口
// 所有数据源适配器必须实现此接口，以便聚合器统一调度
type MarketDataSource interface {
	Name() string
	Connect() error
	Subscribe(symbol string) error
	Read() (*PriceTick, error)
	Close() error
}

// -------------------- REST 数据源 --------------------

// RESTSource 基于 HTTP REST API 的行情适配器
// 通过轮询方式获取最新价格
type RESTSource struct {
	name   string
	url    string
	client *http.Client
}

// NewRESTSource 创建 REST 数据源适配器
func NewRESTSource(name, url string) *RESTSource {
	return &RESTSource{
		name:   name,
		url:    url,
		client: &http.Client{Timeout: 5 * time.Second},
	}
}

func (r *RESTSource) Name() string { return r.name }

func (r *RESTSource) Connect() error { return nil }

func (r *RESTSource) Subscribe(symbol string) error { return nil }

// Read 从 REST API 拉取最新价格
func (r *RESTSource) Read() (*PriceTick, error) {
	resp, err := r.client.Get(r.url)
	if err != nil {
		return nil, fmt.Errorf("REST 请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("REST 状态码异常: %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	// 简化解析：实际场景应根据交易所 API 结构解析
	var result struct {
		Price float64 `json:"price,string"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("REST 响应解析失败: %w", err)
	}

	return &PriceTick{
		Source:    r.name,
		Symbol:    "BTC-USDT",
		Price:     result.Price,
		Timestamp: time.Now().UnixMilli(),
	}, nil
}

func (r *RESTSource) Close() error { return nil }

// -------------------- WebSocket 数据源 --------------------

// WebSocketSource 基于 WebSocket 的行情适配器
// 通过长连接接收实时推送数据
type WebSocketSource struct {
	name string
	url  string
	ws   *websocket.Conn
	mu   sync.Mutex
}

// NewWebSocketSource 创建 WebSocket 数据源适配器
func NewWebSocketSource(name, url string) *WebSocketSource {
	return &WebSocketSource{name: name, url: url}
}

func (w *WebSocketSource) Name() string { return w.name }

func (w *WebSocketSource) Connect() error {
	conn, _, err := websocket.DefaultDialer.Dial(w.url, nil)
	if err != nil {
		return fmt.Errorf("WebSocket 连接失败: %w", err)
	}
	w.ws = conn
	return nil
}

func (w *WebSocketSource) Subscribe(symbol string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.ws == nil {
		return fmt.Errorf("WebSocket 未连接")
	}
	// 简化：实际场景应发送交易所订阅报文
	return w.ws.WriteJSON(map[string]interface{}{
		"op": "subscribe",
		"args": []map[string]string{{"channel": "tickers", "instId": symbol}},
	})
}

func (w *WebSocketSource) Read() (*PriceTick, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.ws == nil {
		return nil, fmt.Errorf("WebSocket 未连接")
	}
	_, message, err := w.ws.ReadMessage()
	if err != nil {
		return nil, fmt.Errorf("WebSocket 读取失败: %w", err)
	}
	var result struct {
		Symbol string  `json:"symbol"`
		Price  float64 `json:"price,string"`
	}
	if err := json.Unmarshal(message, &result); err != nil {
		return nil, fmt.Errorf("WebSocket 消息解析失败: %w", err)
	}
	return &PriceTick{
		Source:    w.name,
		Symbol:    result.Symbol,
		Price:     result.Price,
		Timestamp: time.Now().UnixMilli(),
	}, nil
}

func (w *WebSocketSource) Close() error {
	if w.ws != nil {
		return w.ws.Close()
	}
	return nil
}

// -------------------- FIX 数据源 --------------------

// FIXSource 基于 FIX 协议的行情适配器
// 用于对接传统金融机构的专业交易接口
type FIXSource struct {
	name string
	addr string
}

// NewFIXSource 创建 FIX 数据源适配器
func NewFIXSource(name, addr string) *FIXSource {
	return &FIXSource{name: name, addr: addr}
}

func (f *FIXSource) Name() string { return f.name }

func (f *FIXSource) Connect() error {
	// FIX 协议连接逻辑（占位）
	return nil
}

func (f *FIXSource) Subscribe(symbol string) error { return nil }

func (f *FIXSource) Read() (*PriceTick, error) {
	// FIX 协议解析逻辑（占位）
	return &PriceTick{Source: f.name, Symbol: "BTC-USDT", Price: 0, Timestamp: time.Now().UnixMilli()}, nil
}

func (f *FIXSource) Close() error { return nil }

// -------------------- 内部数据源 --------------------

// InternalSource 平台自有现货成交数据源
// 权重最高，作为价格聚合的内部锚点
type InternalSource struct {
	name  string
	price float64
	mu    sync.RWMutex
}

// NewInternalSource 创建内部数据源适配器
func NewInternalSource(name string, initialPrice float64) *InternalSource {
	return &InternalSource{name: name, price: initialPrice}
}

func (i *InternalSource) Name() string { return i.name }

func (i *InternalSource) Connect() error { return nil }

func (i *InternalSource) Subscribe(symbol string) error { return nil }

// Read 返回平台自有现货最新成交价
func (i *InternalSource) Read() (*PriceTick, error) {
	i.mu.RLock()
	p := i.price
	i.mu.RUnlock()
	return &PriceTick{
		Source:    i.name,
		Symbol:    "BTC-USDT",
		Price:     p,
		Timestamp: time.Now().UnixMilli(),
	}, nil
}

func (i *InternalSource) Close() error { return nil }

// UpdatePrice 更新内部数据源价格（供外部调用）
func (i *InternalSource) UpdatePrice(price float64) {
	i.mu.Lock()
	i.price = price
	i.mu.Unlock()
}
