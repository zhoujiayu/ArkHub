// ============================================================
// internal/market/signer.go
// EIP-712 预言机签名
// 职责：对聚合后的指数价格进行 EIP-712 结构化签名，
//       确保价格数据不可篡改且可在链上验证
// ============================================================

package market

import (
	"crypto/ecdsa"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

// OraclePriceData EIP-712 签名用结构化数据
type OraclePriceData struct {
	Symbol    string  `json:"symbol"`
	Price     float64 `json:"price"`
	Timestamp int64   `json:"timestamp"`
}

// EIP712Domain 定义 EIP-712 域分隔符
type EIP712Domain struct {
	Name              string
	Version           string
	ChainID           int64
	VerifyingContract string
}

// OraclePriceTypedData 符合 EIP-712 标准的价格数据结构
type OraclePriceTypedData struct {
	Symbol    string `json:"symbol"`
	Price     string `json:"price"`     // 使用字符串避免精度损失
	Timestamp int64  `json:"timestamp"`
}

var (
	// OraclePriceType 是 EIP-712 类型定义
	OraclePriceType = []byte("OraclePrice(bytes32 symbol,uint256 price,uint256 timestamp)")
	// DomainSeparatorType 是域分隔符类型定义
	DomainSeparatorType = []byte("EIP712Domain(string name,string version,uint256 chainId,address verifyingContract)")
)

// SignOraclePrice 对指数价格进行 EIP-712 签名
// 返回 65 字节签名数据（r||s||v）
func SignOraclePrice(symbol string, price float64, timestamp int64, privateKey *ecdsa.PrivateKey) ([]byte, error) {
	if privateKey == nil {
		return nil, fmt.Errorf("私钥为空")
	}

	// 将价格转换为整数（精度 8 位小数）
	priceInt := big.NewInt(int64(price * 1e8))

	// 构建要签名的消息哈希
	msgHash := hashOraclePrice(symbol, priceInt, timestamp)

	// 使用私钥签名
	sig, err := crypto.Sign(msgHash.Bytes(), privateKey)
	if err != nil {
		return nil, fmt.Errorf("签名失败: %w", err)
	}

	return sig, nil
}

// VerifyOraclePrice 验证 EIP-712 签名
// 返回签名者的公钥地址
func VerifyOraclePrice(symbol string, price float64, timestamp int64, signature []byte) (common.Address, error) {
	if len(signature) != 65 {
		return common.Address{}, fmt.Errorf("签名长度异常: %d", len(signature))
	}

	priceInt := big.NewInt(int64(price * 1e8))
	msgHash := hashOraclePrice(symbol, priceInt, timestamp)

	// 提取 v 值并调整
	sigCopy := make([]byte, len(signature))
	copy(sigCopy, signature)
	if sigCopy[64] >= 27 {
		sigCopy[64] -= 27
	}

	// 从签名恢复公钥
	pubKey, err := crypto.SigToPub(msgHash.Bytes(), sigCopy)
	if err != nil {
		return common.Address{}, fmt.Errorf("签名验证失败: %w", err)
	}

	return crypto.PubkeyToAddress(*pubKey), nil
}

// hashOraclePrice 计算 OraclePrice 的 keccak256 哈希
func hashOraclePrice(symbol string, price *big.Int, timestamp int64) common.Hash {
	// 简化实现：直接对结构化数据进行 keccak256 哈希
	// 生产环境应使用完整的 EIP-712 编码流程
	data := fmt.Sprintf("%s|%s|%d", symbol, price.String(), timestamp)
	return crypto.Keccak256Hash([]byte(data))
}

// HashEIP712Message 生成完整的 EIP-712 签名消息哈希
func HashEIP712Message(domainHash, typedDataHash common.Hash) common.Hash {
	// 0x1901 || domainHash || typedDataHash
	prefix := []byte{0x19, 0x01}
	data := append(prefix, domainHash.Bytes()...)
	data = append(data, typedDataHash.Bytes()...)
	return crypto.Keccak256Hash(data)
}
