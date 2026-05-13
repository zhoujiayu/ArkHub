// ============================================================
// internal/chain/validator.go
// 双源校验器 — 链上链下数据一致性实时校验
// 职责：业务操作前实时比对链上状态与链下数据库，拦截不一致操作
// ============================================================

package chain

import (
	"context"
	"database/sql"
	"fmt"
)

// DualSourceValidator 双源校验器
type DualSourceValidator struct {
	chainClient ChainClient
	db          *sql.DB
}

// Action 业务操作上下文
type Action struct {
	Type      string // deposit / withdraw / transfer
	Address   string
	Amount    string
	Asset     string
	ChainName string
}

// Discrepancy 不一致记录
type Discrepancy struct {
	Type      string
	Address   string
	ChainValue string
	DBValue    string
	Timestamp  int64
}

// NewDualSourceValidator 创建双源校验器
func NewDualSourceValidator(chainClient ChainClient, db *sql.DB) *DualSourceValidator {
	return &DualSourceValidator{chainClient: chainClient, db: db}
}

// ValidateBeforeAction 业务操作前校验
// 1. 查询链上状态 2. 查询链下状态 3. 对比一致性
func (v *DualSourceValidator) ValidateBeforeAction(ctx context.Context, action Action) error {
	// 1. 查询链上状态
	chainState, err := v.chainClient.GetState(ctx, action.Address)
	if err != nil {
		return fmt.Errorf("链上查询失败: %w", err)
	}

	// 2. 查询链下状态
	dbState, err := v.getDBState(ctx, action.Address)
	if err != nil {
		return fmt.Errorf("链下查询失败: %w", err)
	}

	// 3. 对比校验
	if !v.isConsistent(chainState, dbState) {
		return fmt.Errorf("链上链下数据不一致: 链上=%s, 链下=%s", chainState.Balance, dbState.Balance)
	}

	return nil
}

// getDBState 从数据库查询地址状态
func (v *DualSourceValidator) getDBState(ctx context.Context, address string) (*State, error) {
	var balance string
	err := v.db.QueryRowContext(ctx,
		"SELECT balance FROM user_balances WHERE address = $1", address,
	).Scan(&balance)
	if err != nil {
		return nil, err
	}
	return &State{Address: address, Balance: balance}, nil
}

// isConsistent 判断链上链下状态是否一致
func (v *DualSourceValidator) isConsistent(chainState, dbState *State) bool {
	return chainState.Balance == dbState.Balance
}

// ValidateTransaction 校验交易状态
func (v *DualSourceValidator) ValidateTransaction(ctx context.Context, txHash string) (*Receipt, error) {
	receipt, err := v.chainClient.GetTransactionReceipt(ctx, txHash)
	if err != nil {
		return nil, err
	}
	return receipt, nil
}
