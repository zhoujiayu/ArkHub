// ============================================================
// internal/nft/parser.go
// 元数据解析器 — 适配多种 NFT 元数据格式
// 职责：解析 OpenSea、ERC-721、ERC-1155 等标准格式的元数据
// ============================================================

package nft

import (
	"encoding/json"
	"fmt"
)

// MetadataParser 元数据解析器
type MetadataParser struct {
	parsers []Parser
}

// Parser 解析器接口
type Parser interface {
	Parse(raw []byte) (*ParsedMetadata, error)
	CanParse(raw []byte) bool
}

// ParsedMetadata 解析后的元数据
type ParsedMetadata struct {
	Name        string
	Description string
	Image       string
	Attributes  map[string]interface{}
	ExternalURL string
}

// NewMetadataParser 创建解析器
func NewMetadataParser() *MetadataParser {
	return &MetadataParser{
		parsers: []Parser{
			&OpenSeaParser{},
			&ERC721Parser{},
		},
	}
}

// Parse 尝试多种格式解析
func (p *MetadataParser) Parse(raw []byte) (*ParsedMetadata, error) {
	for _, parser := range p.parsers {
		if parser.CanParse(raw) {
			return parser.Parse(raw)
		}
	}
	return nil, fmt.Errorf("不支持的元数据格式")
}

// OpenSeaParser OpenSea 格式解析器
type OpenSeaParser struct{}

func (p *OpenSeaParser) CanParse(raw []byte) bool {
	var meta map[string]interface{}
	if err := json.Unmarshal(raw, &meta); err != nil {
		return false
	}
	_, ok := meta["attributes"]
	return ok
}

func (p *OpenSeaParser) Parse(raw []byte) (*ParsedMetadata, error) {
	var meta NFTMetadata
	if err := json.Unmarshal(raw, &meta); err != nil {
		return nil, err
	}

	attrs := make(map[string]interface{})
	for _, attr := range meta.Attributes {
		attrs[attr.TraitType] = attr.Value
	}

	return &ParsedMetadata{
		Name:        meta.Name,
		Description: meta.Description,
		Image:       meta.Image,
		Attributes:  attrs,
		ExternalURL: meta.ExternalURL,
	}, nil
}

// ERC721Parser ERC-721 标准解析器
type ERC721Parser struct{}

func (p *ERC721Parser) CanParse(raw []byte) bool {
	var meta map[string]interface{}
	if err := json.Unmarshal(raw, &meta); err != nil {
		return false
	}
	_, ok := meta["name"]
	return ok
}

func (p *ERC721Parser) Parse(raw []byte) (*ParsedMetadata, error) {
	var meta NFTMetadata
	if err := json.Unmarshal(raw, &meta); err != nil {
		return nil, err
	}

	attrs := make(map[string]interface{})
	for _, attr := range meta.Attributes {
		attrs[attr.TraitType] = attr.Value
	}

	return &ParsedMetadata{
		Name:        meta.Name,
		Description: meta.Description,
		Image:       meta.Image,
		Attributes:  attrs,
	}, nil
}
