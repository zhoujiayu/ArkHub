# 阶段七：回购统计服务技术方案

> 阶段目标：实现定时聚合回购数据、Redis 缓存、降级策略，提供高性能统计接口。
> 预计工期：5 天
> 依赖阶段：阶段一

---

## 0. 设计原因

### 0.1 为什么回购统计需要预聚合 + 缓存

**业务背景**：
- 回购统计接口是平台运营的核心数据看板，运营人员频繁查询
- 直接查询 PostgreSQL 需全表扫描，单次查询 500ms+
- 高并发场景下（如大促期间），数据库压力剧增，可能拖垮整个系统

**问题分析**：

| 场景 | 直接查询 PostgreSQL | 预聚合 + 缓存 | 对比 |
|------|-------------------|-------------|------|
| 单次查询 | 500ms+ | <10ms | 快 50 倍 |
| 并发 100 QPS | 数据库 CPU 100%，响应骤降 | Redis 轻松支撑 | 差距悬殊 |
| 大促期间 | 可能拖垮数据库，影响交易 | 查询不受影响 | 稳定性差异大 |

**设计决策**：
- 定时任务（每 5 分钟）全量聚合回购数据
- 结果写入 Redis 缓存，TTL 设置 10 分钟
- 接口直接读取 Redis，故障时降级回查数据库

### 0.2 为什么定时聚合而非实时计算

| 方案 | 延迟 | 数据库压力 | 适用场景 | 选择原因 |
|------|------|-----------|---------|---------|
| **定时聚合** | 5 分钟 | 低（离线计算） | 运营看板、统计分析 | 数据变化慢，定时足够 |
| **实时计算** | 秒级 | 高（在线计算） | 实时交易监控 | 回购数据不需要秒级更新 |
| **流式计算** | 毫秒级 | 中（流处理） | 实时风控 | 过度设计 |

**业务价值**：
- 5 分钟聚合一次，运营看板数据足够新鲜
- 数据库压力降低 90%，释放资源给核心交易
- 接口 P99 <10ms，用户体验极佳

### 0.3 为什么降级策略必不可少

**业务背景**：
- Redis 故障时（如网络中断、内存不足），若直接返回错误，运营看板不可用
- 运营看板不可用不影响核心交易，但会影响运营决策
- 竞品曾多次出现"统计页面 502"问题，用户体验差

**设计决策**：
- 先查 Redis，命中则直接返回
- Redis 不可用时，回查 PostgreSQL
- 记录降级日志，运维及时发现 Redis 问题

**业务价值**：
- Redis 故障时，统计接口仍可正常使用
- 降级过程对用户透明，无感知
- 为 Redis 恢复争取时间，避免服务完全不可用

---

## 1. 模块拆分

| 子模块 | 描述 | 验证方式 |
|--------|------|---------|
| **7.1 定时任务** | Cron 调度全量聚合 | 验证定时触发 |
| **7.2 全量聚合计算** | 统计回购量、金额、趋势 | 验证聚合结果正确性 |
| **7.3 缓存层** | Redis 预聚合结果缓存 | 验证缓存读写 |
| **7.4 缓存失效策略** | TTL + 手动刷新 | 验证失效和刷新 |
| **7.5 降级策略** | Redis 不可用时回查 DB | 模拟 Redis 故障 |
| **7.6 统计 API** | 高性能查询接口 | 压力测试 P99 < 10ms |

---

## 2. 技术方案

### 2.1 定时任务

```go
type AggregationJob struct {
    scheduler *cron.Cron
    db        *sql.DB
    redis     *redis.Client
    interval  time.Duration
}

func (j *AggregationJob) Start() {
    // 配置 Cron 表达式，如每 5 分钟执行一次
    j.scheduler.AddFunc("*/5 * * * *", j.run)
    j.scheduler.Start()
}

func (j *AggregationJob) run() {
    // 1. 查询最近聚合时间
    // 2. 增量聚合新数据
    // 3. 更新缓存
    // 4. 记录聚合日志
}
```

### 2.2 全量聚合计算

```go
type BuybackAggregator struct {
    db *sql.DB
}

func (a *BuybackAggregator) Aggregate(ctx context.Context, start, end time.Time) (*BuybackStats, error) {
    query := `
        SELECT 
            COUNT(*) as total_count,
            SUM(amount) as total_amount,
            AVG(amount) as avg_amount,
            MAX(amount) as max_amount,
            MIN(amount) as min_amount
        FROM buyback_records
        WHERE created_at BETWEEN $1 AND $2
        GROUP BY DATE(created_at)
        ORDER BY DATE(created_at) DESC
    `
    // 执行聚合查询
}

type BuybackStats struct {
    TotalCount int64
    TotalAmount float64
    AvgAmount   float64
    MaxAmount   float64
    MinAmount   float64
    DailyTrend  []DailyStat
}
```

### 2.3 缓存层

```go
type CacheLayer struct {
    redis *redis.Client
    ttl   time.Duration
}

func (c *CacheLayer) SetStats(ctx context.Context, key string, stats *BuybackStats) error {
    data, _ := json.Marshal(stats)
    return c.redis.Set(ctx, key, data, c.ttl).Err()
}

func (c *CacheLayer) GetStats(ctx context.Context, key string) (*BuybackStats, error) {
    data, err := c.redis.Get(ctx, key).Bytes()
    if err != nil {
        return nil, err
    }
    var stats BuybackStats
    json.Unmarshal(data, &stats)
    return &stats, nil
}
```

### 2.4 缓存失效策略

```go
type CacheInvalidation struct {
    redis *redis.Client
}

func (c *CacheInvalidation) Invalidate(ctx context.Context, pattern string) error {
    // 1. 按模式匹配 Key
    keys, _ := c.redis.Keys(ctx, pattern).Result()
    
    // 2. 删除匹配的 Key
    if len(keys) > 0 {
        return c.redis.Del(ctx, keys...).Err()
    }
    return nil
}

func (c *CacheInvalidation) ManualRefresh(ctx context.Context) error {
    // 1. 触发全量聚合
    // 2. 更新缓存
    // 3. 返回新数据
}
```

### 2.5 降级策略

```go
type FallbackStrategy struct {
    cache *CacheLayer
    db    *sql.DB
}

func (s *FallbackStrategy) GetStats(ctx context.Context, key string) (*BuybackStats, error) {
    // 1. 先尝试从缓存获取
    stats, err := s.cache.GetStats(ctx, key)
    if err == nil {
        return stats, nil
    }
    
    // 2. 缓存失败，回查数据库
    if err == redis.Nil || err != nil {
        // 记录缓存穿透日志
        // 从数据库查询
        return s.queryFromDB(ctx)
    }
    
    return nil, err
}
```

### 2.6 统计 API

```go
type StatsHandler struct {
    aggregator *BuybackAggregator
    cache      *CacheLayer
    fallback   *FallbackStrategy
}

func (h *StatsHandler) GetStats(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    
    // 1. 尝试从缓存获取
    stats, err := h.cache.GetStats(ctx, "buyback:stats:latest")
    if err != nil {
        // 2. 缓存失败，降级到数据库
        stats, err = h.fallback.GetStats(ctx, "buyback:stats:latest")
        if err != nil {
            http.Error(w, err.Error(), http.StatusInternalServerError)
            return
        }
    }
    
    // 3. 返回统计结果
    json.NewEncoder(w).Encode(stats)
}
```

---

## 3. 验证清单

| 检查项 | 验证方法 | 通过标准 |
|--------|---------|---------|
| 定时聚合 | 等待定时触发 | 数据正确聚合 |
| 缓存写入 | 查询 Redis | 数据正确缓存 |
| 缓存读取 | API 查询 | 数据正确返回 |
| 缓存失效 | 等待 TTL | 数据自动过期 |
| 手动刷新 | 调用刷新接口 | 数据更新 |
| 降级策略 | 停止 Redis | 从数据库返回 |
| 性能测试 | 压测 | P99 < 10ms |

---

## 4. 风险与应对

| 风险 | 影响 | 应对措施 |
|------|------|---------|
| 聚合任务耗时过长 | 影响系统性能 | 增量聚合 + 异步执行 |
| 缓存雪崩 | 大量请求穿透 | 熔断 + 随机 TTL |
| 数据不一致 | 缓存与数据库差异 | 定时刷新 + 事件驱动更新 |

---

## 5. 交付物

- [ ] `cmd/buyback-service/` — 回购统计服务
- [ ] `internal/buyback/aggregator.go` — 聚合计算
- [ ] `internal/buyback/cache.go` — 缓存层
- [ ] `internal/buyback/fallback.go` — 降级策略
- [ ] `internal/buyback/handler.go` — API 接口
- [ ] `test/integration/buyback_test.go` — 集成测试
- [ ] `docs/buyback-api.md` — API 文档
