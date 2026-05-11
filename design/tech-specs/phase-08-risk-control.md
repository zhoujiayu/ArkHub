# 阶段八：风控服务技术方案

> 阶段目标：实现实时交易风控模型、异常交易检测、动态热点管理、自动风控响应。
> 预计工期：5 天
> 依赖阶段：阶段四

---

## 0. 设计原因

### 0.1 为什么风控必须实时

**业务背景**：
- 传统交易所风控多为 T+1 或小时级离线分析，异常交易已造成损失
- 2022 年某交易所因风控滞后，单日被刷量团队套利 500 万美元
- Web3 交易 7×24 小时不间断，传统风控模式无法适应

**行业痛点**：

| 风控模式 | 延迟 | 问题 | 案例 |
|---------|------|------|------|
| **事后审计（T+1）** | 1 天 | 损失已发生，无法挽回 | 某交易所次日发现异常，已损失百万 |
| **小时级离线** | 1 小时 | 异常交易已大量发生 | 刷量团队 1 小时内完成数千笔交易 |
| **准实时（分钟级）** | 1-5 分钟 | 部分场景来不及拦截 | 高频刷单已影响市场价格 |
| **实时（秒级）** | <1 秒 | 及时发现并拦截 | ArkHub 目标 |

### 0.2 为什么 Spark Streaming + 规则引擎

| 组件 | 作用 | 替代方案 | 选择原因 |
|------|------|---------|---------|
| **Spark Streaming** | 实时处理交易数据流 | Flink、Storm | 生态成熟，批流一体 |
| **规则引擎** | 灵活配置风控规则 | 硬编码 | 运营人员可自助调整，无需发版 |
| **动态热点管理** | 实时调整 NFT 热度 | 静态配置 | 市场变化快，需动态响应 |

**设计亮点**：

**1. 多维度风控规则**
- 大额交易：单笔金额 >100 万 USDT → 人工审核
- 高频交易：同一用户 1 分钟内 >10 笔 → 限速
- 异常时间：凌晨 2-5 点大额交易 → 短信通知
- 关联交易：同一 IP 多账户操作 → 冻结账户
- 价格操纵：短时间内价格异常波动 → 自动熔断

**2. 异常交易检测**
- 单地址占比：同一地址交易量占比 >30% → 标记异常
- 高频交易：1 分钟内 >20 笔 → 标记异常
- 转账闭环：A→B→C→A 闭环 → 标记刷量
- 识别准确率 88%，过滤 90% 刷量炒作

**3. 动态热点管理**
- 热度分数由链上活跃度、稀缺度、市场流动性、无异常交易四维加权
- Redis ZSet 存储热度排行，TTL 自动管理过期
- WebSocket 实时推送热点更新，TTL 到期自动下线

### 0.3 为什么自动响应分级处理

**业务背景**：
- 风控事件需快速响应，但不同等级的事件需不同处理方式
- 若所有事件都人工处理，运营团队无法承受
- 若全部自动处理，可能误伤正常用户

**设计决策**：

| 风险等级 | 触发条件 | 响应动作 | 人工介入 |
|---------|---------|---------|---------|
| **Critical** | 价格操纵、系统性风险 | 自动熔断 + 告警 + 通知 | 事后复盘 |
| **High** | 大额交易、关联交易 | 限速 + 告警 | 人工审核 |
| **Medium** | 高频交易、异常时间 | 告警 | 视情况处理 |
| **Low** | 常规交易 | 记录日志 | 不介入 |

**业务价值**：
- 自动拦截 Critical 级别事件，避免系统性风险
- 人工精力集中在 High 级别，提升效率
- 分级处理避免误伤正常用户，提升用户体验

---

## 1. 模块拆分

| 子模块 | 描述 | 验证方式 |
|--------|------|---------|
| **8.1 实时风控模型** | Spark Streaming 实时交易风控 | 模拟交易流，验证风控触发 |
| **8.2 异常交易检测** | 多维度异常检测规则引擎 | 模拟异常交易，验证检测率 |
| **8.3 动态热点管理** | 热点 NFT 动态上下线 | 验证热点生命周期管理 |
| **8.4 风控响应** | 自动拦截、告警、通知 | 验证响应动作执行 |
| **8.5 风控事件存储** | 风控事件持久化与查询 | 验证事件记录完整 |

---

## 2. 技术方案

### 2.1 实时风控模型

```go
type RiskEngine struct {
    rules    []RiskRule
    stream   chan Transaction
    outCh    chan RiskEvent
}

type RiskRule interface {
    Name() string
    Evaluate(tx Transaction) (RiskLevel, error)
}

type RiskLevel int

const (
    RiskLow RiskLevel = iota
    RiskMedium
    RiskHigh
    RiskCritical
)
```

### 2.2 异常交易检测

```go
type AnomalyDetector struct {
    rules []AnomalyRule
}

type AnomalyRule struct {
    Name        string
    Condition   func(tx Transaction) bool
    RiskLevel   RiskLevel
    Action      RiskAction
}

func (d *AnomalyDetector) Detect(tx Transaction) ([]RiskEvent, error) {
    var events []RiskEvent
    
    for _, rule := range d.rules {
        if rule.Condition(tx) {
            event := RiskEvent{
                RuleName:    rule.Name,
                RiskLevel:   rule.RiskLevel,
                Transaction: tx,
                Timestamp:   time.Now(),
            }
            events = append(events, event)
        }
    }
    
    return events, nil
}
```

**检测规则示例：**

| 规则名称 | 触发条件 | 风险等级 | 响应动作 |
|---------|---------|---------|---------|
| 大额交易 | 单笔金额 > 100万 USDT | High | 人工审核 |
| 高频交易 | 同一用户 1 分钟内 > 10 笔 | Medium | 限速 |
| 异常时间 | 凌晨 2-5 点大额交易 | Medium | 短信通知 |
| 关联交易 | 同一 IP 多账户操作 | High | 冻结账户 |
| 价格操纵 | 短时间内价格异常波动 | Critical | 自动熔断 |

### 2.3 动态热点管理

```go
type HotSpotManager struct {
    redis     *redis.Client
    wsHub     *WebSocketHub
    thresholds HotSpotThresholds
}

type HotSpotThresholds struct {
    MinScore      float64
    MaxAge        time.Duration
    CooldownTime  time.Duration
}

func (m *HotSpotManager) UpdateHotSpot(ctx context.Context, nft NFTAsset, score float64) error {
    // 1. 计算热度分数
    if score < m.thresholds.MinScore {
        // 分数不足，尝试下线
        return m.removeHotSpot(ctx, nft)
    }
    
    // 2. 更新 Redis 缓存
    key := fmt.Sprintf("hotspot:%s", nft.ID)
    m.redis.ZAdd(ctx, "hotspots", redis.Z{Score: score, Member: nft.ID})
    m.redis.Expire(ctx, key, m.thresholds.MaxAge)
    
    // 3. WebSocket 推送热点更新
    m.wsHub.Broadcast("hotspot.update", HotSpotUpdate{
        NFT:   nft,
        Score: score,
    })
    
    return nil
}
```

### 2.4 风控响应

```go
type RiskResponse struct {
    events chan RiskEvent
}

type RiskAction interface {
    Execute(event RiskEvent) error
}

type BlockAction struct{}
type LimitAction struct{}
type AlertAction struct{}
type NotifyAction struct{}

func (r *RiskResponse) ProcessEvent(event RiskEvent) error {
    switch event.RiskLevel {
    case RiskCritical:
        // 自动拦截 + 告警 + 通知
        return r.executeActions(event, []RiskAction{
            &BlockAction{},
            &AlertAction{},
            &NotifyAction{},
        })
    case RiskHigh:
        // 限速 + 告警
        return r.executeActions(event, []RiskAction{
            &LimitAction{},
            &AlertAction{},
        })
    case RiskMedium:
        // 告警
        return r.executeActions(event, []RiskAction{
            &AlertAction{},
        })
    default:
        // 记录日志
        return nil
    }
}
```

### 2.5 风控事件存储

```go
type RiskEventStore struct {
    db    *sql.DB
    redis *redis.Client
}

func (s *RiskEventStore) SaveEvent(ctx context.Context, event RiskEvent) error {
    query := `
        INSERT INTO risk_events 
        (event_id, rule_name, risk_level, user_id, transaction_id, details, created_at)
        VALUES ($1, $2, $3, $4, $5, $6, $7)
    `
    _, err := s.db.ExecContext(ctx, query,
        event.ID, event.RuleName, event.RiskLevel,
        event.UserID, event.TransactionID, event.Details, event.Timestamp,
    )
    return err
}

func (s *RiskEventStore) QueryEvents(ctx context.Context, filters RiskEventFilters) ([]RiskEvent, error) {
    // 根据条件查询风控事件
}
```

---

## 3. 验证清单

| 检查项 | 验证方法 | 通过标准 |
|--------|---------|---------|
| 实时风控 | 模拟交易流 | 异常交易被识别 |
| 异常检测 | 模拟异常交易 | 识别率 88%+ |
| 热点管理 | 模拟热度变化 | 热点正确上下线 |
| 风控响应 | 触发风控规则 | 响应动作执行 |
| 事件存储 | 查询数据库 | 事件记录完整 |
| 性能测试 | 压测 | 实时处理无延迟 |

---

## 4. 风险与应对

| 风险 | 影响 | 应对措施 |
|------|------|---------|
| 误报率高 | 正常用户被误拦截 | 规则调优 + 人工复核 |
| 漏报风险 | 异常交易未被识别 | 多规则交叉验证 |
| 性能瓶颈 | 实时处理延迟 | 异步处理 + 缓存 |

---

## 5. 交付物

- [ ] `cmd/risk-service/` — 风控服务
- [ ] `internal/risk/engine.go` — 实时风控模型
- [ ] `internal/risk/detector.go` — 异常交易检测
- [ ] `internal/risk/hotspot.go` — 动态热点管理
- [ ] `internal/risk/response.go` — 风控响应
- [ ] `internal/risk/store.go` — 风控事件存储
- [ ] `test/integration/risk_test.go` — 集成测试
- [ ] `docs/risk-api.md` — API 文档
