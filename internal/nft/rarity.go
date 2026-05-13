// ============================================================
// internal/nft/rarity.go
// 稀有度量化 — 计算 NFT 稀有度评分
// 职责：单属性稀有度 + 综合稀有度加权，归一化到 0-100
// ============================================================

package nft

import (
	"math"
)

// RarityCalculator 稀有度计算器
type RarityCalculator struct {
	totalSupply int
}

// RarityScore 稀有度评分
type RarityScore struct {
	Overall     float64            // 综合稀有度（0-100）
	Traits      map[string]float64 // 各属性稀有度
	Rank        int              // 排名
}

// NewRarityCalculator 创建计算器
func NewRarityCalculator(totalSupply int) *RarityCalculator {
	return &RarityCalculator{totalSupply: totalSupply}
}

// Calculate 计算稀有度
// 1. 单属性稀有度 = 拥有该属性的 NFT 数 / 总供给
// 2. 综合稀有度 = 加权平均各属性稀有度
// 3. 归一化到 0-100
func (c *RarityCalculator) Calculate(metadata *ParsedMetadata, traitCounts map[string]map[interface{}]int) *RarityScore {
	if c.totalSupply == 0 {
		return &RarityScore{Overall: 0, Traits: make(map[string]float64)}
	}

	traitRarities := make(map[string]float64)
	var totalScore float64

	for traitType, value := range metadata.Attributes {
		// 拥有该属性值的数量
		count := 0
		if tc, ok := traitCounts[traitType]; ok {
			if c, ok := tc[value]; ok {
				count = c
			}
		}

		// 单属性稀有度（值越小越稀有）
		rarity := float64(count) / float64(c.totalSupply)
		traitRarities[traitType] = rarity
		totalScore += rarity
	}

	// 综合稀有度（越低越稀有，取反归一化）
	numTraits := float64(len(metadata.Attributes))
	var overall float64
	if numTraits > 0 {
		avgScore := totalScore / numTraits
		overall = (1 - avgScore) * 100
	}

	return &RarityScore{
		Overall: math.Max(0, math.Min(100, overall)),
		Traits:  traitRarities,
	}
}

// CalculateTraitFrequency 计算属性频率分布
func CalculateTraitFrequency(nfts []*ParsedMetadata) map[string]map[interface{}]int {
	freq := make(map[string]map[interface{}]int)
	for _, nft := range nfts {
		for traitType, value := range nft.Attributes {
			if _, ok := freq[traitType]; !ok {
				freq[traitType] = make(map[interface{}]int)
			}
			freq[traitType][value]++
		}
	}
	return freq
}
