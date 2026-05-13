// ============================================================
// internal/nft/ipfs.go
// IPFS 网关 SDK — 拉取 NFT 元数据
// 职责：通过 IPFS 网关获取 NFT 元数据（图片、属性、描述等）
// ============================================================

package nft

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// IPFSGateway IPFS 网关封装
type IPFSGateway struct {
	gatewayURL string
	timeout    time.Duration
	client     *http.Client
}

// NFTMetadata NFT 元数据结构
type NFTMetadata struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Image       string            `json:"image"`
	Attributes  []TraitAttribute  `json:"attributes"`
	ExternalURL string            `json:"external_url"`
}

// TraitAttribute 属性特征
type TraitAttribute struct {
	TraitType string      `json:"trait_type"`
	Value     interface{} `json:"value"`
	DisplayType string    `json:"display_type,omitempty"`
}

// NewIPFSGateway 创建 IPFS 网关客户端
func NewIPFSGateway(gatewayURL string) *IPFSGateway {
	return &IPFSGateway{
		gatewayURL: gatewayURL,
		timeout:    10 * time.Second,
		client:     &http.Client{Timeout: 10 * time.Second},
	}
}

// FetchMetadata 通过 tokenURI 拉取元数据
func (g *IPFSGateway) FetchMetadata(ctx context.Context, tokenURI string) (*NFTMetadata, error) {
	// 解析 tokenURI 为 IPFS hash
	hash := g.parseIPFSHash(tokenURI)
	if hash == "" {
		return nil, fmt.Errorf("无效的 tokenURI: %s", tokenURI)
	}

	url := fmt.Sprintf("%s/ipfs/%s", g.gatewayURL, hash)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("IPFS 请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("IPFS 返回状态码: %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var meta NFTMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("元数据解析失败: %w", err)
	}

	return &meta, nil
}

// parseIPFSHash 从 tokenURI 提取 IPFS hash
func (g *IPFSGateway) parseIPFSHash(tokenURI string) string {
	// 支持格式: ipfs://Qm... 或 https://ipfs.io/ipfs/Qm...
	if strings.HasPrefix(tokenURI, "ipfs://") {
		return strings.TrimPrefix(tokenURI, "ipfs://")
	}
	if strings.Contains(tokenURI, "/ipfs/") {
		parts := strings.Split(tokenURI, "/ipfs/")
		if len(parts) > 1 {
			return parts[1]
		}
	}
	return tokenURI
}

// FetchImage 拉取图片 URL（转换 IPFS 链接为 HTTP 网关链接）
func (g *IPFSGateway) FetchImage(imageURL string) string {
	if strings.HasPrefix(imageURL, "ipfs://") {
		hash := strings.TrimPrefix(imageURL, "ipfs://")
		return fmt.Sprintf("%s/ipfs/%s", g.gatewayURL, hash)
	}
	return imageURL
}
