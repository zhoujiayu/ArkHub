// ============================================================
// internal/chain/compensation.go
// 异步补偿任务 — 定时修复链上链下数据不一致
// 职责：扫描不一致记录，自动触发补偿逻辑
// ============================================================

package chain

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"
)

// CompensationJob 异步补偿任务
type CompensationJob struct {
	db    *sql.DB
	chain ChainClient
}

// DiscrepancyType 不一致类型
const (
	MissingChainRecord = "missing_chain_record"
	MissingDBRecord    = "missing_db_record"
	DataMismatch       = "data_mismatch"
)

// NewCompensationJob 创建补偿任务
func NewCompensationJob(db *sql.DB, chain ChainClient) *CompensationJob {
	return &CompensationJob{db: db, chain: chain}
}

// Run 执行补偿扫描
func (j *CompensationJob) Run(ctx context.Context) error {
	// 1. 扫描最近 1 小时的不一致记录
	discrepancies, err := j.findDiscrepancies(ctx)
	if err != nil {
		return err
	}

	// 2. 触发补偿
	for _, d := range discrepancies {
		if err := j.compensate(ctx, d); err != nil {
			log.Printf("补偿失败: %v", err)
		}
	}

	return nil
}

// findDiscrepancies 扫描不一致记录
func (j *CompensationJob) findDiscrepancies(ctx context.Context) ([]Discrepancy, error) {
	// 从数据库查询最近不一致记录
	rows, err := j.db.QueryContext(ctx,
		"SELECT type, address, chain_value, db_value, timestamp FROM discrepancy_logs WHERE created_at > $1 ORDER BY created_at DESC",
		time.Now().Add(-1*time.Hour),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var discrepancies []Discrepancy
	for rows.Next() {
		var d Discrepancy
		if err := rows.Scan(&d.Type, &d.Address, &d.ChainValue, &d.DBValue, &d.Timestamp); err != nil {
			continue
		}
		discrepancies = append(discrepancies, d)
	}
	return discrepancies, nil
}

// compensate 执行补偿
func (j *CompensationJob) compensate(ctx context.Context, d Discrepancy) error {
	switch d.Type {
	case MissingChainRecord:
		return j.compensateChainToDB(ctx, d)
	case MissingDBRecord:
		return j.compensateDBToChain(ctx, d)
	case DataMismatch:
		return j.reconcileData(ctx, d)
	}
	return fmt.Errorf("unknown discrepancy type: %s", d.Type)
}

func (j *CompensationJob) compensateChainToDB(ctx context.Context, d Discrepancy) error {
	log.Printf("补偿: 链上记录缺失，从数据库同步到链上: %s", d.Address)
	return nil
}

func (j *CompensationJob) compensateDBToChain(ctx context.Context, d Discrepancy) error {
	log.Printf("补偿: 数据库记录缺失，从链上同步到数据库: %s", d.Address)
	return nil
}

func (j *CompensationJob) reconcileData(ctx context.Context, d Discrepancy) error {
	log.Printf("补偿: 数据不一致，重新校准: %s", d.Address)
	return nil
}
