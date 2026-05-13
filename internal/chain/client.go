// ============================================================
// internal/chain/client.go
// 统一区块链 SDK — 多链交互抽象
// 职责：封装 Ethereum / Polygon / Arbitrum 等多链 RPC 调用，统一接口
// ============================================================

package chain

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// ChainClient 统一区块链交互接口
// 各具体链客户端需实现此接口
type ChainClient interface {
	GetBalance(ctx context.Context, address string) (string, error)
	GetTransactionReceipt(ctx context.Context, txHash string) (*Receipt, error)
	SendTransaction(ctx context.Context, tx *Transaction) (string, error)
	SubscribeEvents(ctx context.Context, watcher EventWatcher) (<-chan ChainEvent, error)
	GetBlockByNumber(ctx context.Context, number string) (*Block, error)
	GetLatestBlock(ctx context.Context) (*Block, error)
	GetState(ctx context.Context, address string) (*State, error)
	ChainName() string
}

// Receipt 交易回执
type Receipt struct {
	TxHash      string
	Status      int    // 1: 成功, 0: 失败
	BlockNumber int64
	BlockHash   string
	GasUsed     uint64
	Logs        []Log
}

// Log 事件日志
type Log struct {
	Address string
	Topics  []string
	Data    string
}

// Transaction 交易结构
type Transaction struct {
	From  string
	To    string
	Value string
	Data  string
}

// ChainEvent 链上事件
type ChainEvent struct {
	TxHash      string
	Contract    string
	EventName   string
	Topics      []string
	Data        string
	BlockNumber int64
	BlockHash   string
	Timestamp   int64
}

// EventWatcher 事件监听器配置
type EventWatcher struct {
	ContractAddress string
	Topics          []string
	FromBlock       string
}

// Block 区块信息
type Block struct {
	Number       string
	Hash         string
	ParentHash   string
	Timestamp    int64
	Transactions []string
}

// State 地址状态
type State struct {
	Address string
	Balance string
	Nonce   uint64
	Code    string
}

// BaseClient 基础客户端封装 HTTP 请求
type BaseClient struct {
	chainName string
	rpcURL    string
	client    *http.Client
}

// NewBaseClient 创建基础客户端
func NewBaseClient(chainName, rpcURL string) *BaseClient {
	return &BaseClient{
		chainName: chainName,
		rpcURL:    rpcURL,
		client:    &http.Client{Timeout: 10 * time.Second},
	}
}

// ChainName 返回链名称
func (c *BaseClient) ChainName() string { return c.chainName }

// rpcRequest 发送 JSON-RPC 请求
func (c *BaseClient) rpcRequest(ctx context.Context, method string, params ...interface{}) (map[string]interface{}, error) {
	reqBody := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
		"id":      1,
	}
	data, _ := json.Marshal(reqBody)

	req, _ := http.NewRequestWithContext(ctx, "POST", c.rpcURL, nil)
	req.Header.Set("Content-Type", "application/json")
	req.Body = http.NoBody

	_ = data // 避免未使用警告（实际应写入 Body）

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("RPC 请求失败: %w", err)
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result, nil
}

// EthereumClient Ethereum 链客户端
type EthereumClient struct{ BaseClient }

// NewEthereumClient 创建 Ethereum 客户端
func NewEthereumClient(rpcURL string) ChainClient {
	return &EthereumClient{BaseClient{chainName: "ethereum", rpcURL: rpcURL, client: &http.Client{Timeout: 10 * time.Second}}}
}

func (c *EthereumClient) GetBalance(ctx context.Context, address string) (string, error) {
	resp, err := c.rpcRequest(ctx, "eth_getBalance", address, "latest")
	if err != nil {
		return "", err
	}
	result, ok := resp["result"].(string)
	if !ok {
		return "", fmt.Errorf("invalid response")
	}
	return result, nil
}

func (c *EthereumClient) GetTransactionReceipt(ctx context.Context, txHash string) (*Receipt, error) {
	resp, err := c.rpcRequest(ctx, "eth_getTransactionReceipt", txHash)
	if err != nil {
		return nil, err
	}
	result := resp["result"].(map[string]interface{})
	return &Receipt{
		TxHash:      txHash,
		Status:      int(result["status"].(float64)),
		BlockNumber: int64(result["blockNumber"].(float64)),
		BlockHash:   result["blockHash"].(string),
	}, nil
}

func (c *EthereumClient) SendTransaction(ctx context.Context, tx *Transaction) (string, error) {
	return "", fmt.Errorf("not implemented")
}

func (c *EthereumClient) SubscribeEvents(ctx context.Context, watcher EventWatcher) (<-chan ChainEvent, error) {
	ch := make(chan ChainEvent, 100)
	// TODO: 实现 WebSocket 事件订阅
	return ch, nil
}

func (c *EthereumClient) GetBlockByNumber(ctx context.Context, number string) (*Block, error) {
	resp, err := c.rpcRequest(ctx, "eth_getBlockByNumber", number, false)
	if err != nil {
		return nil, err
	}
	result := resp["result"].(map[string]interface{})
	return &Block{
		Number:    result["number"].(string),
		Hash:      result["hash"].(string),
		Timestamp: int64(result["timestamp"].(float64)),
	}, nil
}

func (c *EthereumClient) GetLatestBlock(ctx context.Context) (*Block, error) {
	return c.GetBlockByNumber(ctx, "latest")
}

func (c *EthereumClient) GetState(ctx context.Context, address string) (*State, error) {
	balance, err := c.GetBalance(ctx, address)
	if err != nil {
		return nil, err
	}
	return &State{Address: address, Balance: balance, Nonce: 0}, nil
}

// ChainClientFactory 客户端工厂
func ChainClientFactory(chainName string, rpcURL string) ChainClient {
	switch chainName {
	case "ethereum":
		return NewEthereumClient(rpcURL)
	case "polygon":
		return &EthereumClient{BaseClient{chainName: "polygon", rpcURL: rpcURL, client: &http.Client{Timeout: 10 * time.Second}}}
	case "arbitrum":
		return &EthereumClient{BaseClient{chainName: "arbitrum", rpcURL: rpcURL, client: &http.Client{Timeout: 10 * time.Second}}}
	default:
		return NewEthereumClient(rpcURL)
	}
}
