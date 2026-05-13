# ArkHub 项目开发规则

> 本文件记录 AI 辅助开发时的项目规则、经验教训和检查清单。
> 每次修改代码前，必须先阅读本文件。

---

## 1. 项目背景

ArkHub 是一个面向现货、合约、NFT 交易的综合性数字资产金融平台。

- **技术栈**：Go 1.23+, PostgreSQL, Redis, RocketMQ, Nacos
- **核心模块**：API Gateway, Auth Service, Market Data, Matching Engine 等
- **开发原则**：中间件优先、单机模式优先、详细记录变更

---

## 2. 数据流思维规则（核心）

### 2.1 数据必须流动

**规则**：代码中的每个处理步骤，输入必须来自上一步的输出，输出必须传递给下一步。

```go
// ❌ 错误：步骤之间没有数据流动
filtered := market.RemoveOutliers(prices)
_ = market.MedianFilter(filtered)  // 结果丢弃，数据断流
indexPrice, err := market.WeightedFusionWithMedian(ticks, weights)  // 用的还是原始数据

// ✅ 正确：数据在步骤间流动
filtered := market.RemoveOutliers(prices)
median := market.MedianFilter(filtered)  // 保留结果
validTicks := filterByMedian(ticks, median)  // 用上一步的结果
indexPrice, err := market.WeightedFusion(validTicks, weights)  // 用过滤后的数据
```

### 2.2 禁止丢弃中间结果

**规则**：如果一个函数有返回值，必须确保结果被使用或传递给下一步。

```go
// ❌ 错误：计算了但没有使用
_ = market.MedianFilter(prices)

// ✅ 正确：结果用于后续逻辑
median := market.MedianFilter(prices)
if diff/median <= 0.1 {
    // 使用 median 进行判断
}
```

### 2.3 输入数据必须经过验证

**规则**：每个处理函数的输入数据，必须确认已经过前置步骤的处理。

```go
// ❌ 错误：用的还是原始数据
ticks := collectPrices(sources)
// ... 经过 RemoveOutliers 和 MedianFilter ...
indexPrice, err := market.WeightedFusionWithMedian(ticks, weights)  // ❌ 还是原始 ticks

// ✅ 正确：使用过滤后的数据
ticks := collectPrices(sources)
validTicks := filterAndValidate(ticks)  // 先处理
indexPrice, err := market.WeightedFusionWithMedian(validTicks, weights)  // ✅ 用处理后的数据
```

---

## 3. 接口设计规则

### 3.1 函数命名必须准确反映行为

**规则**：函数名应该让人一眼看出它做了什么，以及它的输入和输出。

```go
// ❌ 错误：名字有误导性
func WeightedFusionWithMedian(ticks []*PriceTick, weights map[string]float64) (float64, error)
// 这个名字暗示"内部会处理中位数"，但实际上中位数过滤应该在调用前完成

// ✅ 正确：名字准确
func WeightedFusion(sources []SourcePrice) (float64, error)
// 调用者明确知道：传入的数据必须先经过过滤
```

### 3.2 接口职责单一

**规则**：每个函数只做一件事，不要在函数内部做多种不相关的操作。

```go
// ❌ 错误：一个函数既过滤又融合
func FilterAndFusion(prices []float64, weights map[string]float64) float64

// ✅ 正确：拆分为独立的步骤
func RemoveOutliers(prices []float64) []float64
func MedianFilter(prices []float64) float64
func WeightedFusion(sources []SourcePrice) (float64, error)
```

---

## 4. 测试规则

### 4.1 必须覆盖异常场景

**规则**：每个功能必须测试正常场景和异常场景，不能只测试"happy path"。

```go
func TestRemoveOutliers(t *testing.T) {
    // ✅ 正常场景
    t.Run("正常数据", func(t *testing.T) {
        // ...
    })
    
    // ✅ 异常场景
    t.Run("含异常值", func(t *testing.T) {
        // ...
    })
    
    // ✅ 边界场景
    t.Run("空切片", func(t *testing.T) {
        // ...
    })
}
```

### 4.2 测试必须验证数据流

**规则**：测试不仅要验证函数输出，还要验证数据在流程中的传递是否正确。

```go
func TestAggregationPipeline(t *testing.T) {
    // 1. 准备包含异常值的输入
    prices := []float64{68000, 68100, 100000, 67950, 68050}
    
    // 2. 执行完整流程
    filtered := RemoveOutliers(prices)
    median := MedianFilter(filtered)
    
    // 3. 验证：异常值被剔除
    assert.NotContains(t, filtered, 100000)
    
    // 4. 验证：中位数基于过滤后的数据
    assert.InDelta(t, 68050, median, 1)
    
    // 5. 验证：加权融合使用过滤后的数据
    // ...
}
```

---

## 5. 代码审查检查清单

在提交代码前，必须检查以下项目：

### 5.1 数据流检查

- [ ] 每个步骤的输出是否被下一步使用？
- [ ] 是否有中间结果被丢弃（`_`）？
- [ ] 最终使用的数据是否经过前置处理？
- [ ] 是否有"幽灵数据"（看似被处理但实际未被使用）？

### 5.2 接口检查

- [ ] 函数名是否准确反映了行为？
- [ ] 函数参数是否清晰明确？
- [ ] 返回值是否被调用者正确使用？
- [ ] 是否有副作用（修改全局状态、IO 操作）？如果有，是否明确文档化？

### 5.3 测试检查

- [ ] 是否覆盖了正常场景？
- [ ] 是否覆盖了异常场景？
- [ ] 是否覆盖了边界条件？
- [ ] 测试是否验证了数据流？

---

## 6. 经验教训

### 6.1 中位数滤波 bug

**问题**：`WeightedFusionWithMedian` 中使用了原始的 `ticks` 数据，而不是经过异常剔除后的数据。

**原因**：
1. 步骤之间没有数据流动
2. 函数名有误导性，误以为"内部会处理中位数"
3. 测试覆盖不足，没有验证异常值场景

**解决方案**：
1. 明确数据流：prices → RemoveOutliers → MedianFilter → validTicks → WeightedFusion
2. 函数职责单一，过滤和融合分离
3. 测试覆盖异常场景

---

## 7. 项目结构约定

```
ArkHub/
├── cmd/                    # 各阶段可执行入口
├── internal/               # 私有代码
│   ├── market/            # 行情聚合模块
│   ├── middleware/        # 中间件
│   ├── pkg/               # 公共包
│   └── response/          # 统一响应格式
├── pkg/                    # 公共代码库
├── api/                    # API 定义
├── configs/               # 配置文件
├── deployments/           # Docker / K8s 部署脚本
├── migrations/            # 数据库迁移脚本
├── test/                  # 集成测试
├── docs/                  # 文档
│   ├── highlights/       # 技术亮点文档
│   └── setup/            # 部署说明
├── Makefile              # 构建脚本
├── go.mod                 # Go 模块管理
├── CHANGELOG.md           # 变更记录
├── README.md              # 项目说明
└── CLAUDE.md              # 本文件
```

---

## 8. 常用命令

```bash
# 编译
go build ./cmd/market-data/

# 测试
go test ./test/integration/...

# 格式化
go fmt ./...

# 静态检查
golangci-lint run
```

---

## 9. 方案实现后自动更新规则

### 9.1 每次方案实现完后必须执行

**规则**：每个阶段（Phase）的技术方案实现完成后，必须自动检查并更新以下文件：

1. **CHANGELOG.md** — 将对应 Phase 状态从 "待实现 ⏳" 改为 "已完成 ✅"
2. **CHANGELOG.md** — 补充该阶段所有变更文件的明细（格式参照 Phase 1~3）
3. **CLAUDE.md** — 如有新的项目规则或经验教训，补充到对应章节

### 9.2 更新 CHANGELOG.md 的格式要求

```markdown
### Phase N：XXX（已完成 ✅）

**变更文件：**

#### 1. 模块名称
- `文件路径` — 一句话描述
  - 函数/结构体说明（带中文注释）
  - 核心功能点
  - 基准测试数据（如有）

#### 2. 模块名称
- ...

**实现说明：**
- 设计决策说明
- 性能指标
- 注意事项
```

### 9.3 更新检查清单

在每次方案实现完成后，提交前必须检查：

- [ ] CHANGELOG.md 中对应 Phase 状态已更新为 "已完成 ✅"
- [ ] 所有新增/修改的文件已在 CHANGELOG.md 中记录
- [ ] 每个文件的功能描述、接口说明、测试覆盖情况已补充
- [ ] 性能基准测试数据已记录（如有）
- [ ] CLAUDE.md 是否需要新增规则或经验教训

---

*本文件会随着项目发展持续更新，每次遇到问题或学到经验，都应该补充到这里。*
