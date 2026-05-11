# 阶段五：链上链下一致性服务技术方案

> 阶段目标：实现双源校验、异步补偿、事件同步、数据校准，确保链上链下数据 100% 一致。
> 预计工期：7 天
> 依赖阶段：阶段一、阶段二

---

## 0. 设计原因

### 0.1 为什么链上链下一致性是核心难题

**业务背景**：
- 用户充值后，交易所需在链上确认到账才能更新用户余额
- 链上区块可能回滚，导致已确认的充值失效
- 若链下数据库与链上状态不一致，用户可能重复提现或无法提现
- 竞品多次出现"充值不到账""提现卡住"等问题，客诉率高

**行业痛点**：

| 问题 | 影响 | 案例 |
|------|------|------|
| 充值未到账 | 用户无法交易 | 2022 年某交易所因链上监听失败，数千用户充值延迟 24h+ |
| 提现双重支付 | 平台资金损失 | 某交易所因状态同步 bug，同一笔提现被执行两次 |
| 区块回滚 | 已确认交易失效 | 2021 年 Ethereum 分叉期间，多家交易所遭遇回滚 |
| 链上数据延迟 | 用户焦虑、客诉 | 平均到账时间 30 分钟，用户反复询问客服 |

### 0.2 为什么三重保障机制

| 保障机制 | 作用 | 适用场景 | 选择原因 |
|---------|------|---------|---------|
| **实时双源校验** | 业务操作前校验链上状态 | 下单、提现等关键操作 | 拦截不一致操作，从源头避免问题 |
| **异步补偿任务** | 定时对比差异并修复 | 历史数据漂移、漏同步 | 修复已发生的不一致 |
| **事件同步引擎** | 毫秒级监听链上事件 | 充值、提现、合约交互 | 实时感知链上变化 |

**单一机制的局限**：
- 仅实时校验：无法发现历史数据漂移
- 仅异步补偿：发现问题时可能已经造成损失
- 仅事件同步：可能遗漏事件（网络抖动、节点故障）

**三重保障的优势**：
- 实时校验防新、异步补偿修旧、事件同步兜底
- 三层防护网确保数据一致性达 100%
- 竞品多为"最终一致性"，ArkHub 实现"准实时一致性"

### 0.3 为什么统一区块链 SDK

**业务背景**：
- 平台需支持 Ethereum、Polygon、Arbitrum 等多链
- 每条链的 RPC 协议、区块结构、事件格式不同
- 若各服务自行接入链上，代码重复、维护困难

**设计决策**：
- 抽象 `ChainClient` 接口，统一多链交互
- 策略模式适配不同链的 RPC 调用、事件解析
- 新增公链仅需实现接口，2 周内完成适配

**业务价值**：
- 代码复用率提升 80%，维护成本降低
- 新增公链周期从行业平均 1 个月缩短至 2 周
- 统一错误处理和重试机制，提升系统稳定性

---

## 1. 模块拆分

| 子模块 | 描述 | 验证方式 |
|--------|------|---------|
| **5.1 双源校验器** | 业务操作前实时调用链上 API 校验 | 模拟链上状态，验证校验逻辑 |
| **5.2 异步补偿任务** | 定时对比链上链下差异并补偿 | 模拟数据差异，验证补偿触发 |
| **5.3 事件同步引擎** | 毫秒级监听链上合约事件 | 模拟链上事件，验证同步延迟 |
| **5.4 数据校准器** | 自动修正不一致数据 | 模拟不一致数据，验证校准结果 |
| **5.5 统一区块链 SDK** | 适配多链的交互 SDK | 适配 Ethereum/Polygon/Arbitrum |

---

## 2. 技术方案

### 2.1 双源校验器

```go
type DualSourceValidator struct {
    chainClient ChainClient
    db          *sql.DB
}

func (v *DualSourceValidator) ValidateBeforeAction(ctx context.Context, action Action) error {
    // 1. 查询链上状态
    chainState, err := v.chainClient.GetState(ctx, action.Address)
    if err != nil {
        return err
    }
    
    // 2. 查询链下状态
    dbState, err := v.getDBState(ctx, action.Address)
    if err != nil {
        return err
    }
    
    // 3. 对比校验
    if !v.isConsistent(chainState, dbState) {
        return errors.New("chain-db state inconsistent")
    }
    
    return nil
}
```

### 2.2 异步补偿任务

```go
type CompensationJob struct {
    scheduler *cron.Cron
    db        *sql.DB
    chain     ChainClient
}

func (j *CompensationJob) Run() error {
    // 1. 扫描最近 1 小时的不一致记录
    discrepancies := j.findDiscrepancies()
    
    // 2. 触发补偿
    for _, d := range discrepancies {
        switch d.Type {
        case MissingChainRecord:
            j.compensateChainToDB(d)
        case MissingDBRecord:
            j.compensateDBToChain(d)
        case DataMismatch:
            j.reconcileData(d)
        }
    }
}
```

### 2.3 事件同步引擎

```go
type EventSyncEngine struct {
    client    *ethclient.Client
    watchers  []EventWatcher
    outCh     chan ChainEvent
}

type EventWatcher struct {
    ContractAddress common.Address
    Topics          []common.Hash
    FromBlock       *big.Int
}

func (e *EventSyncEngine) Start(ctx context.Context) error {
    // 1. 监听合约事件
    // 2. 解析事件日志
    // 3. 写入消息队列
    // 4. 更新同步断点
}
```

### 2.4 数据校准器

```go
type DataReconciler struct {
    db    *sql.DB
    redis *redis.Client
}

func (r *DataReconciler) Reconcile(ctx context.Context, report DiscrepancyReport) error {
    tx, _ := r.db.BeginTx(ctx, nil)
    defer tx.Rollback()
    
    // 1. 修正数据库记录
    for _, item := range report.Items {
        switch item.Action {
        case Update:
            r.updateRecord(tx, item)
        case Delete:
            r.deleteRecord(tx, item)
        case Insert:
            r.insertRecord(tx, item)
        }
    }
    
    // 2. 清除 Redis 缓存
    r.redis.Del(ctx, report.CacheKeys...)
    
    // 3. 生成校准报告
    r.generateReport(report)
    
    return tx.Commit()
}
```

### 2.5 统一区块链 SDK

```go
type ChainClient interface {
    GetBalance(address string) (*big.Int, error)
    GetTransactionReceipt(txHash string) (*Receipt, error)
    SendTransaction(tx *Transaction) (string, error)
    SubscribeEvents(ctx context.Context, watcher EventWatcher) (<-chan ChainEvent, error)
    GetBlockByNumber(number *big.Int) (*Block, error)
    GetLatestBlock() (*Block, error)
}

type EthereumClient struct{}
type PolygonClient struct{}
type ArbitrumClient struct{}
```

---

## 3. 验证清单

| 检查项 | 验证方法 | 通过标准 |
|--------|---------|---------|
| 双源校验 | 模拟链上状态变化 | 校验不通过时拦截操作 |
| 异步补偿 | 模拟数据不一致 | 补偿任务自动修复 |
| 事件同步 | 模拟链上事件 | 事件同步延迟 < 100ms |
| 数据校准 | 模拟数据差异 | 校准后数据一致 |
| 多链适配 | 切换不同链 | 正确调用对应链 API |
| 断点续传 | 重启服务 | 从断点继续同步 |

---

## 4. 风险与应对

| 风险 | 影响 | 应对措施 |
|------|------|---------|
| 链上节点不可用 | 校验失败 | 多节点备份 + 降级策略 |
| 事件监听遗漏 | 数据不一致 | 断点续传 + 定时补偿 |
| 补偿失败重试 | 无限重试 | 指数退避 + 死信队列 |

---

## 5. 交付物

- [ ] `cmd/chain-sync/` — 链上链下一致性服务
- [ ] `internal/chain/client.go` — 统一区块链 SDK
- [ ] `internal/chain/validator.go` — 双源校验器
- [ ] `internal/chain/compensation.go` — 异步补偿任务
- [ ] `internal/chain/sync.go` — 事件同步引擎
- [ ] `internal/chain/reconciler.go` — 数据校准器
- [ ] `test/integration/chain_sync_test.go` — 集成测试
- [ ] `docs/chain-sync-api.md` — API 文档
