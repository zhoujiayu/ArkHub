# 差距分析三：区块链高级交互

> 对比范围：web3-go 基础 RPC vs ArkHub EIP-712、预言机、多链适配

---

## 1. 技能对比

| 技能点 | web3-go 覆盖 | ArkHub 需求 | 差距 |
|--------|-------------|------------|------|
| 连接节点 | ✅ ethclient.Dial | ✅ 必需 | 无差距 |
| 查询余额 | ✅ BalanceAt | ✅ 必需 | 无差距 |
| 查询交易 | ✅ TransactionByHash | ✅ 必需 | 无差距 |
| 事件监听 | ✅ SubscribeFilterLogs | ✅ 必需 | 无差距 |
| 发送交易 | ✅ SendTransaction | ✅ 必需 | 无差距 |
| **EIP-712 签名** | ❌ 未涉及 | ❌ 预言机价格签名 | 🔴 大差距 |
| **预言机机制** | ❌ 未涉及 | ❌ 链上可信价格推送 | 🔴 大差距 |
| **多链策略模式** | ⚠️ 基础封装 | ❌ 统一 SDK + 策略适配 | 🟡 中等 |
| **合约 ABI 编码** | ⚠️ 基础 Unpack | ❌ 复杂参数编码 | 🟡 中等 |
| **链重组处理** | ✅ 有检测 | ⚠️ 需完善回滚 | 🟡 中等 |

---

## 2. EIP-712 签名

### 为什么需要 EIP-712

ArkHub 需要将聚合后的行情价格以**可信方式**推送到链上，EIP-712 提供结构化的 typed data 签名，确保：
- 签名内容不可篡改
- 链上可验证签名者身份
- 防重放攻击

### EIP-712 核心概念

```
EIP-712 数据结构：
+---------------+
| Domain        |  <-- 定义应用上下文（name, version, chainId）
| Separator     |
+---------------+
| Type Hash     |  <-- 结构化数据类型哈希
+---------------+
| Encoded Data  |  <-- 实际数据 ABI 编码
+---------------+
| Signature     |  <-- secp256k1 签名 (r, s, v)
+---------------+
```

### Go 实现

```go
package signer

import (
    "crypto/ecdsa"
    "encoding/json"
    "math/big"
    
    "github.com/ethereum/go-ethereum/common"
    "github.com/ethereum/go-ethereum/crypto"
    "github.com/ethereum/go-ethereum/signer/core/apitypes"
)

// OraclePrice 预言机价格数据结构
type OraclePrice struct {
    Asset     string  `json:"asset"`
    Price     float64 `json:"price"`
    Timestamp int64   `json:"timestamp"`
    Round     int64   `json:"round"`
}

// EIP712Signer EIP-712 签名器
type EIP712Signer struct {
    privateKey *ecdsa.PrivateKey
    domain     apitypes.TypedDataDomain
}

// NewEIP712Signer 创建签名器
func NewEIP712Signer(privateKeyHex string, chainID int64) (*EIP712Signer, error) {
    privateKey, err := crypto.HexToECDSA(privateKeyHex)
    if err != nil {
        return nil, err
    }
    
    return &EIP712Signer{
        privateKey: privateKey,
        domain: apitypes.TypedDataDomain{
            Name:    "ArkHub Oracle",
            Version: "1",
            ChainId: (*math.HexOrDecimal256)(big.NewInt(chainID)),
        },
    }, nil
}

// SignPrice 对价格数据进行 EIP-712 签名
func (s *EIP712Signer) SignPrice(price OraclePrice) ([]byte, error) {
    // 定义类型
    types := apitypes.Types{
        "EIP712Domain": []apitypes.Type{
            {Name: "name", Type: "string"},
            {Name: "version", Type: "string"},
            {Name: "chainId", Type: "uint256"},
        },
        "OraclePrice": []apitypes.Type{
            {Name: "asset", Type: "string"},
            {Name: "price", Type: "uint256"},
            {Name: "timestamp", Type: "uint64"},
            {Name: "round", Type: "uint64"},
        },
    }
    
    // 构建 TypedData
    message := map[string]interface{}{
        "asset":     price.Asset,
        "price":     big.NewInt(int64(price.Price * 1e8)), // 8 位精度
        "timestamp": price.Timestamp,
        "round":     price.Round,
    }
    
    typedData := apitypes.TypedData{
        Types:       types,
        PrimaryType: "OraclePrice",
        Domain:      s.domain,
        Message:     message,
    }
    
    // 计算签名哈希
    hash, _, err := apitypes.TypedDataAndHash(typedData)
    if err != nil {
        return nil, err
    }
    
    // 签名
    signature, err := crypto.Sign(hash, s.privateKey)
    if err != nil {
        return nil, err
    }
    
    return signature, nil
}

// VerifySignature 验证签名
func (s *EIP712Signer) VerifySignature(price OraclePrice, signature []byte) (bool, error) {
    // 恢复公钥
    pubKey, err := crypto.SigToPub(crypto.Keccak256([]byte("...")), signature)
    if err != nil {
        return false, err
    }
    
    // 验证地址
    address := crypto.PubkeyToAddress(*pubKey)
    expectedAddress := crypto.PubkeyToAddress(s.privateKey.PublicKey)
    
    return address == expectedAddress, nil
}
```

### 智能合约端验证

```solidity
// SPDX-License-Identifier: MIT
pragma solidity ^0.8.0;

interface IArkHubOracle {
    function verifyPrice(
        string memory asset,
        uint256 price,
        uint64 timestamp,
        uint64 round,
        bytes memory signature
    ) external view returns (bool);
}

contract ArkHubOracle is IArkHubOracle {
    address public signer;
    
    constructor(address _signer) {
        signer = _signer;
    }
    
    function verifyPrice(
        string memory asset,
        uint256 price,
        uint64 timestamp,
        uint64 round,
        bytes memory signature
    ) external view override returns (bool) {
        // 1. 构建 EIP-712 哈希
        bytes32 structHash = keccak256(abi.encode(
            keccak256("OraclePrice(string asset,uint256 price,uint64 timestamp,uint64 round)"),
            keccak256(bytes(asset)),
            price,
            timestamp,
            round
        ));
        
        // 2. 恢复签名者地址
        bytes32 digest = keccak256(abi.encodePacked(
            "\x19\x01",
            DOMAIN_SEPARATOR,
            structHash
        ));
        
        address recovered = recoverSigner(digest, signature);
        
        return recovered == signer;
    }
}
```

---

## 3. 预言机机制

### ArkHub 预言机架构

```
+---------------+     +---------------+     +---------------+
|  行情聚合服务  | --> |  EIP-712     | --> |  预言机合约   |
|  计算指数价格 |     |  签名价格    |     |  验证并存储   |
+---------------+     +---------------+     +---------------+
       |                                              |
       |           +---------------+                  |
       +---------->|  RocketMQ     |<-----------------+
                   |  广播价格签名 |
                   +---------------+
```

### 预言机服务实现

```go
package oracle

import (
    "context"
    "time"
)

// OracleService 预言机服务
type OracleService struct {
    marketData  *MarketDataService
    signer      *EIP712Signer
    publisher   *RocketMQPublisher
    round       int64
}

// Start 启动预言机服务
func (os *OracleService) Start(ctx context.Context) {
    ticker := time.NewTicker(5 * time.Second)  // 每 5 秒推送一次
    defer ticker.Stop()
    
    for {
        select {
        case <-ticker.C:
            os.publishPrice(ctx)
        case <-ctx.Done():
            return
        }
    }
}

func (os *OracleService) publishPrice(ctx context.Context) {
    // 1. 获取聚合价格
    price := os.marketData.GetAggregatedPrice("BTC/USDT")
    
    // 2. 构建预言机价格
    oraclePrice := OraclePrice{
        Asset:     "BTC/USDT",
        Price:     price,
        Timestamp: time.Now().Unix(),
        Round:     os.round,
    }
    
    // 3. EIP-712 签名
    signature, err := os.signer.SignPrice(oraclePrice)
    if err != nil {
        return
    }
    
    // 4. 推送至消息队列
    os.publisher.Publish("oracle-price", &OracleMessage{
        Price:     oraclePrice,
        Signature: signature,
    })
    
    os.round++
}
```

---

## 4. 多链策略模式

### 从基础多链到策略模式

```go
package chain

import (
    "context"
    "math/big"
    
    "github.com/ethereum/go-ethereum/common"
    "github.com/ethereum/go-ethereum/core/types"
)

// ChainClient 区块链客户端接口
type ChainClient interface {
    Name() string
    ChainID() *big.Int
    GetBalance(ctx context.Context, address common.Address) (*big.Int, error)
    GetTransactionReceipt(ctx context.Context, txHash common.Hash) (*types.Receipt, error)
    SendTransaction(ctx context.Context, tx *types.Transaction) error
    SubscribeEvents(ctx context.Context, address common.Address, topics []common.Hash) (<-chan types.Log, error)
    GetLatestBlock(ctx context.Context) (uint64, error)
}

// ChainStrategy 链策略接口
type ChainStrategy interface {
    // Gas 价格策略
    GasPriceStrategy(ctx context.Context) (*big.Int, error)
    // 交易确认策略
    ConfirmationStrategy(ctx context.Context, txHash common.Hash) error
    // 重试策略
    RetryStrategy(fn func() error) error
}

// EthereumStrategy Ethereum 链策略
type EthereumStrategy struct{}

func (s *EthereumStrategy) GasPriceStrategy(ctx context.Context) (*big.Int, error) {
    // Ethereum: 使用 EIP-1559 动态 gas
    return nil, nil
}

func (s *EthereumStrategy) ConfirmationStrategy(ctx context.Context, txHash common.Hash) error {
    // Ethereum: 12 个区块确认
    return nil
}

// PolygonStrategy Polygon 链策略
type PolygonStrategy struct{}

func (s *PolygonStrategy) GasPriceStrategy(ctx context.Context) (*big.Int, error) {
    // Polygon: 固定 gas price
    return big.NewInt(50e9), nil  // 50 Gwei
}

func (s *PolygonStrategy) ConfirmationStrategy(ctx context.Context, txHash common.Hash) error {
    // Polygon: 128 个区块确认
    return nil
}

// UnifiedClient 统一区块链客户端
type UnifiedClient struct {
    client   ChainClient
    strategy ChainStrategy
}

func NewUnifiedClient(chain string, rpcURL string) (*UnifiedClient, error) {
    switch chain {
    case "ethereum":
        return &UnifiedClient{
            client:   NewEthereumClient(rpcURL),
            strategy: &EthereumStrategy{},
        }, nil
    case "polygon":
        return &UnifiedClient{
            client:   NewPolygonClient(rpcURL),
            strategy: &PolygonStrategy{},
        }, nil
    default:
        return nil, fmt.Errorf("unsupported chain: %s", chain)
    }
}
```

---

## 5. 学习建议

| 优先级 | 主题 | 学习时间 | 产出 |
|--------|------|---------|------|
| P0 | EIP-712 签名原理 | 1 天 | 实现价格签名 + 验证 |
| P0 | 预言机机制 | 1 天 | 实现定时价格推送服务 |
| P1 | 策略模式多链适配 | 1 天 | 统一 SDK 封装 |
| P1 | 合约 ABI 高级编码 | 0.5 天 | 复杂参数编码/解码 |
| P2 | 链重组深度处理 | 0.5 天 | 完善回滚逻辑 |

---

## 6. 参考资源

| 资源 | 链接 | 说明 |
|------|------|------|
| EIP-712 规范 | https://eips.ethereum.org/EIPS/eip-712 | 官方规范 |
| go-ethereum signer | https://github.com/ethereum/go-ethereum/tree/master/signer | 签名实现 |
| Chainlink 预言机 | https://docs.chain.link/ | 预言机参考架构 |
| OpenZeppelin ECDSA | https://docs.openzeppelin.com/contracts/4.x/api/utils#ECDSA | 合约签名验证 |
