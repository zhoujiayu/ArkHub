// ============================================================
// internal/nft/fraud.go
// 异常交易检测 — 识别刷量、自买自卖等欺诈行为
// 职责：单地址占比、高频交易、转账闭环检测
// ============================================================

package nft

import (
	"time"
)

// FraudDetector 欺诈检测器
type FraudDetector struct {
	thresholds FraudThresholds
}

// FraudThresholds 检测阈值
type FraudThresholds struct {
	SingleAddressRatio float64 // 单地址占比阈值
	HighFrequencyRate  int     // 高频交易阈值（次/小时）
	TransferLoopDepth  int     // 转账闭环深度
}

// Transaction 交易记录
type Transaction struct {
	From      string
	To        string
	TokenID   string
	Price     float64
	Timestamp time.Time
}

// FraudReport 欺诈检测报告
type FraudReport struct {
	IsFraud    bool
	Reasons    []string
	Confidence float64 // 置信度 0-1
}

// NewFraudDetector 创建检测器
func NewFraudDetector(thresholds FraudThresholds) *FraudDetector {
	return &FraudDetector{thresholds: thresholds}
}

// Detect 执行检测
func (d *FraudDetector) Detect(transactions []Transaction) *FraudReport {
	report := &FraudReport{
		IsFraud:    false,
		Reasons:    make([]string, 0),
		Confidence: 0,
	}

	// 1. 检测单地址占比
	if d.detectSingleAddressRatio(transactions) {
		report.IsFraud = true
		report.Reasons = append(report.Reasons, "单地址交易占比过高")
		report.Confidence += 0.3
	}

	// 2. 检测高频交易
	if d.detectHighFrequency(transactions) {
		report.IsFraud = true
		report.Reasons = append(report.Reasons, "高频交易异常")
		report.Confidence += 0.3
	}

	// 3. 检测转账闭环
	if d.detectTransferLoop(transactions) {
		report.IsFraud = true
		report.Reasons = append(report.Reasons, "存在转账闭环刷量")
		report.Confidence += 0.4
	}

	return report
}

// detectSingleAddressRatio 检测单地址占比
func (d *FraudDetector) detectSingleAddressRatio(transactions []Transaction) bool {
	if len(transactions) == 0 {
		return false
	}

	addressCount := make(map[string]int)
	for _, tx := range transactions {
		addressCount[tx.From]++
		addressCount[tx.To]++
	}

	maxCount := 0
	for _, count := range addressCount {
		if count > maxCount {
			maxCount = count
		}
	}

	ratio := float64(maxCount) / float64(len(transactions)*2)
	return ratio > d.thresholds.SingleAddressRatio
}

// detectHighFrequency 检测高频交易
func (d *FraudDetector) detectHighFrequency(transactions []Transaction) bool {
	if len(transactions) == 0 {
		return false
	}

	addressFreq := make(map[string]int)
	for _, tx := range transactions {
		addressFreq[tx.From]++
	}

	for _, freq := range addressFreq {
		if freq > d.thresholds.HighFrequencyRate {
			return true
		}
	}
	return false
}

// detectTransferLoop 检测转账闭环
func (d *FraudDetector) detectTransferLoop(transactions []Transaction) bool {
	// 构建转账图
	graph := make(map[string]string)
	for _, tx := range transactions {
		graph[tx.From] = tx.To
	}

	// 检测闭环
	visited := make(map[string]bool)
	for addr := range graph {
		if !visited[addr] {
			if d.hasLoop(graph, addr, make(map[string]bool), 0) {
				return true
			}
			visited[addr] = true
		}
	}
	return false
}

func (d *FraudDetector) hasLoop(graph map[string]string, start string, path map[string]bool, depth int) bool {
	if depth > d.thresholds.TransferLoopDepth {
		return false
	}

	if path[start] {
		return true
	}

	path[start] = true
	defer delete(path, start)

	if next, ok := graph[start]; ok {
		return d.hasLoop(graph, next, path, depth+1)
	}
	return false
}

// DefaultFraudThresholds 默认阈值
func DefaultFraudThresholds() FraudThresholds {
	return FraudThresholds{
		SingleAddressRatio: 0.30,
		HighFrequencyRate:  20,
		TransferLoopDepth:  3,
	}
}
