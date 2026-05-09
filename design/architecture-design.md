# 加密货币/NFT 综合交易平台 — 系统架构设计文档

> 版本：v1.0  
> 日期：2026-05-09  
> 作者：系统架构组

---

## 1. 项目概述

本项目为面向现货、合约、NFT 交易的综合性数字资产金融平台。核心诉求包括：

- **高并发低延迟**：Disruptor 无锁队列 + 内存订单簿，支撑百万级订单吞吐，撮合延迟 10 ms。
- **抗干扰行情聚合**：14 路异构行情源通过中位数滤波 + 异常剔除算法融合，指数价格精准贴合现货波动。
- **链上链下强一致**：双源校验 + 异步补偿 + 事件监听三重保障，数据一致性达 100%。
- **轻量多链扩展**：统一区块链交互 SDK + 策略模式适配，新增公链仅需 2 周。
- **实时风控与运营**：Spark Streaming 实时交易风控模型 + WebSocket 热点推送，自动识别异常交易并动态管理热点生命周期。

---

## 2. 整体架构分层总览

```mermaid
flowchart TB
    subgraph L1["客户端层 Client Layer"]
        C1[Web 前端]
        C2[移动端 App]
        C3[第三方 API 接入]
        C4[合约/链上预言机]
    end

    subgraph L2["网关层 Gateway Layer"]
        G1[API Gateway / Nginx]
        G2[WebSocket Gateway]
        G3[鉴权中心 Auth]
        G4[限流熔断 Rate Limit]
    end

    subgraph L3["核心业务层 Core Service Layer"]
        S1[行情聚合服务<br/>Market Data Aggregation]
        S2[订单撮合引擎<br/>Order Matching Engine]
        S3[链上链下一致性服务<br/>On-Off-Chain Sync]
        S4[NFT 业务服务<br/>NFT Business Service]
        S5[回购统计服务<br/>Buyback Statistics]
        S6[风控服务<br/>Risk Control Engine]
    end

    subgraph L4["数据层 Data Layer"]
        D1[(PostgreSQL<br/>主从集群)]
        D2[(Redis<br/>缓存/会话)]
        D3[(RocketMQ<br/>消息队列)]
        D4[(Spark Streaming<br/>实时计算)]
        D5[(IPFS Gateway<br/>元数据存储)]
    end

    subgraph L5["基础设施层 Infrastructure Layer"]
        I1[Docker / K8s]
        I2[Nacos<br/>配置/注册中心]
        I3[Prometheus + Grafana<br/>监控告警]
        I4[ELK / Loki<br/>日志聚合]
    end

    subgraph L6["区块链层 Blockchain Layer"]
        B1[Ethereum]
        B2[Polygon]
        B3[Arbitrum]
        B4[统一区块链交互 SDK]
    end

    C1 --> G1
    C2 --> G1
    C2 --> G2
    C3 --> G1
    C4 --> G1

    G1 --> G3
    G2 --> G3
    G3 --> S1
    G3 --> S2
    G3 --> S3
    G3 --> S4
    G3 --> S5
    G3 --> S6

    S1 --> D2
    S1 --> D3
    S2 --> D1
    S2 --> D2
    S3 --> D1
    S3 --> D2
    S3 --> B4
    S4 --> D1
    S4 --> D2
    S4 --> D5
    S5 --> D1
    S5 --> D2
    S6 --> D4
    S6 --> D2

    S2 --> D3
    S3 --> D3
    S4 --> D3
    S5 --> D3

    B4 --> B1
    B4 --> B2
    B4 --> B3

    I2 --> S1
    I2 --> S2
    I2 --> S3
    I2 --> S4
    I2 --> S5
    I2 --> S6
```

---

## 3. 子系统详细架构

### 3.1 行情聚合服务

```mermaid
flowchart LR
    subgraph SRC["外部行情源 (14+)"]
        E1[交易所 A REST]
        E2[交易所 B WebSocket]
        E3[交易所 C 私有协议]
        E4[平台现货价格]
    end

    subgraph AGG["行情聚合引擎"]
        AD1[多协议适配器<br/>REST / WS / FIX]
        AD2[中位数滤波器<br/>Median Filter]
        AD3[异常剔除算法<br/>Outlier Detection]
        AD4[加权融合引擎<br/>Weighted Fusion]
        AD5[EIP-712 签名服务]
    end

    subgraph CFG["配置中心"]
        N1[Nacos<br/>ChannelConfig 热更新]
        N2[数据源权重配置]
        N3[价格限幅规则]
    end

    subgraph OUT["输出通道"]
        O1[WebSocket 推送<br/>实时行情]
        O2[RocketMQ<br/>指数/标记价格]
        O3[RocketMQ<br/>预言机价格]
        O4[Redis 缓存<br/>热点价格]
    end

    E1 --> AD1
    E2 --> AD1
    E3 --> AD1
    E4 --> AD1

    AD1 --> AD2
    AD2 --> AD3
    AD3 --> AD4
    AD4 --> AD5

    N1 --> AD1
    N2 --> AD4
    N3 --> AD3

    AD4 --> O1
    AD4 --> O2
    AD5 --> O3
    AD4 --> O4
```

**关键组件说明：**

| 组件 | 技术选型 | 职责 |
|------|---------|------|
| 多协议适配器 | Go + net/http + gorilla/websocket | 适配 REST / WebSocket / FIX 等异构协议 |
| 中位数滤波器 | 自定义算法 | 对 14+ 源价格排序取中位数，降低异常干扰 |
| 异常剔除算法 | Z-Score / IQR | 检测并剔除偏离正常区间的异常价格 |
| 加权融合引擎 | 自定义权重模型 | 结合平台现货权重与交易所权重计算指数价格 |
| EIP-712 签名服务 | go-ethereum/crypto | 对预言机价格进行链上可信签名 |

---

### 3.2 订单撮合引擎

```mermaid
flowchart TB
    subgraph IN["订单输入"]
        I1[用户下单 REST API]
        I2[批量订单导入]
    end

    subgraph QUEUE["队列层 (Disruptor)"]
        Q1[无锁环形队列<br/>Ring Buffer]
        Q2[订单预处理<br/>参数校验/风控拦截]
    end

    subgraph MATCH["撮合核心"]
        M1[内存订单簿<br/>OrderBook in Memory]
        M2[价格优先 + 时间优先<br/>Matching Algorithm]
        M3[成交记录生成<br/>Trade Record]
        M4[自动熔断机制<br/>Circuit Breaker]
    end

    subgraph POST["后置处理"]
        P1[用户资产扣减<br/>Balance Update]
        P2[成交通知推送<br/>WebSocket / MQ]
        P3[订单持久化<br/>异步落库]
        P4[清算对账<br/>Settlement]
    end

    I1 --> Q1
    I2 --> Q1
    Q1 --> Q2
    Q2 --> M1
    M1 --> M2
    M2 --> M3
    M2 --> M4
    M3 --> P1
    M3 --> P2
    M3 --> P3
    P3 --> P4
```

**关键组件说明：**

| 组件 | 技术选型 | 职责 |
|------|---------|------|
| Disruptor 无锁环形队列 | Go 版 Disruptor 实现 | 高并发订单接收，避免锁竞争 |
| 内存订单簿 | Go map + 红黑树 | 按价格和时间排序的买卖盘 |
| 撮合算法 | 自定义匹配引擎 | 价格优先、时间优先的撮合逻辑 |
| 自动熔断 | Sentinel / 自研 | 极端行情下自动限流或降级 |

---

### 3.3 链上链下一致性保障

```mermaid
flowchart TB
    subgraph CHAIN["链上数据"]
        C1[公链节点 API]
        C2[合约事件监听<br/>Event Listener]
        C3[链上交易状态]
    end

    subgraph CORE["一致性服务"]
        R1[双源校验器<br/>Real-time Validator]
        R2[异步补偿任务<br/>Compensation Job]
        R3[事件同步引擎<br/>Event Sync Engine]
        R4[数据校准器<br/>Data Reconciler]
    end

    subgraph OFF["链下数据"]
        O1[(PostgreSQL<br/>业务数据)]
        O2[(Redis<br/>缓存状态)]
        O3[(RocketMQ<br/>消息队列)]
    end

    C1 --> R1
    C2 --> R3
    C3 --> R1

    R1 --> O1
    R1 --> O2

    R2 --> C1
    R2 --> O1
    R2 --> R4

    R3 --> O3
    R3 --> O1
    R3 --> O2

    R4 --> O1
    R4 --> O2
```

**关键组件说明：**

| 组件 | 技术选型 | 职责 |
|------|---------|------|
| 双源校验器 | Go + RPC | 业务操作前实时调用链上 API 校验状态 |
| 异步补偿任务 | Cron / 定时调度 | 周期性对比链上链下差异，自动触发补偿 |
| 事件同步引擎 | Go + ethclient | 毫秒级监听链上合约事件，同步至链下 |
| 数据校准器 | 自定义规则引擎 | 自动修正不一致数据，生成校准报告 |

---

### 3.4 NFT 业务系统

```mermaid
flowchart TB
    subgraph META["元数据处理"]
        M1[IPFS 网关 SDK]
        M2[元数据解析器<br/>Metadata Parser]
        M3[稀有度量化算法<br/>Rarity Algorithm]
    end

    subgraph COMPUTE["实时计算"]
        S1[Spark Streaming<br/>交易数据流]
        S2[热度评分模型<br/>Heat Score Model]
        S3[异常交易检测<br/>Fraud Detection]
    end

    subgraph PUSH["推送与运营"]
        P1[WebSocket 实时推送]
        P2[Redis 过期键管理<br/>TTL-based]
        P3[热点动态上下线]
    end

    subgraph DB["数据存储"]
        D1[(PostgreSQL<br/>NFT 资产/交易)]
        D2[(Redis<br/>热度缓存)]
        D3[(IPFS<br/>原始元数据)]
    end

    M1 --> M2
    M2 --> M3
    M3 --> D2

    S1 --> S2
    S1 --> S3
    S2 --> D2
    S3 --> D2

    P1 --> P2
    P2 --> P3
    P3 --> D2

    D2 --> D1
    D3 --> M1
```

**关键组件说明：**

| 组件 | 技术选型 | 职责 |
|------|---------|------|
| IPFS 网关 SDK | ipfs/go-ipfs-api | 拉取 NFT 元数据 |
| 元数据解析器 | 自定义 JSON 解析器 | 提取属性字段，适配 95%+ 项目格式 |
| 稀有度量化算法 | 自定义算法 | 单属性稀有度 + 综合稀有度加权计算 |
| 热度评分模型 | Spark ML / 自定义 | 链上活跃度 + 稀缺度 + 市场流动性 + 无异常交易 |
| 异常交易检测 | Spark Streaming | 单地址占比、高频交易、转账闭环检测 |

---

### 3.5 回购统计服务

```mermaid
flowchart LR
    subgraph SOURCE["数据源"]
        S1[(PostgreSQL<br/>回购交易表)]
    end

    subgraph AGG["预聚合引擎"]
        A1[定时任务<br/>Cron Scheduler]
        A2[全量聚合计算<br/>Aggregation Logic]
        A3[数据清洗与格式化]
    end

    subgraph CACHE["缓存层"]
        C1[(Redis<br/>预聚合结果)]
        C2[缓存失效策略<br/>TTL / 手动刷新]
    end

    subgraph API["接口层"]
        I1[回购统计 API]
        I2[降级策略<br/>Cache Miss → DB Fallback]
    end

    S1 --> A1
    A1 --> A2
    A2 --> A3
    A3 --> C1
    C1 --> C2
    C1 --> I1
    I1 --> I2
    I2 -.fallback.-> S1
```

**关键组件说明：**

| 组件 | 技术选型 | 职责 |
|------|---------|------|
| 定时任务 | Cron / Go timer | 按配置频率触发全量聚合 |
| 全量聚合计算 | Go + SQL | 统计回购量、回购金额、回购趋势 |
| 缓存失效策略 | Redis TTL | 自动过期旧数据，支持手动刷新 |
| 降级策略 | 自定义逻辑 | Redis 不可用时回查数据库 |

---

## 4. 数据流向总览

```mermaid
sequenceDiagram
    autonumber
    participant U as 用户/客户端
    participant G as API Gateway
    participant S1 as 行情聚合
    participant S2 as 撮合引擎
    participant S3 as 链上链下
    participant MQ as RocketMQ
    participant R as Redis
    participant DB as PostgreSQL
    participant B as 区块链

    U->>G: 查询行情 / 下单
    G->>S1: 转发请求
    S1->>S1: 14+ 源聚合计算
    S1->>R: 缓存指数价格
    S1->>MQ: 推送预言机价格
    S1-->>U: 返回实时行情

    U->>G: 提交订单
    G->>S2: 转发订单
    S2->>S2: Disruptor 队列 → 撮合
    S2->>R: 更新订单簿缓存
    S2->>DB: 异步持久化成交记录
    S2->>MQ: 推送成交通知
    S2-->>U: 返回成交结果

    S3->>B: 实时校验链上状态
    B-->>S3: 返回链上数据
    S3->>S3: 双源校验 / 异步补偿
    S3->>DB: 同步校准数据

    MQ->>U: WebSocket 推送通知
```

---

## 5. 技术组件映射表

| 业务模块 | 核心组件 | 技术选型 | 性能指标 |
|---------|---------|---------|---------|
| **行情聚合** | 多协议适配器 | Go + gorilla/websocket | 300 ms 响应延迟 |
| | 滤波融合引擎 | 自定义算法 | 抗干扰性提升 90% |
| | EIP-712 签名 | go-ethereum/crypto | 100% 可信率 |
| **订单撮合** | Disruptor 队列 | Go 版 Ring Buffer | 百万级 TPS |
| | 内存订单簿 | Go map + 红黑树 | 10 ms 撮合延迟 |
| | 熔断降级 | Sentinel / 自研 | 极端行情 0 崩溃 |
| **链上链下** | 双源校验器 | Go + RPC | 一致性 100% |
| | 事件同步引擎 | Go + ethclient | 毫秒级同步 |
| **NFT 业务** | 元数据解析器 | 自定义 JSON 解析器 | 支持 95%+ 项目 |
| | 稀有度算法 | 自定义算法 | 准确率 92% |
| | 热度评分模型 | Spark Streaming | 异常识别率 88% |
| **回购统计** | 预聚合引擎 | Go + Cron + SQL | P99 < 10 ms |
| | 缓存降级 | Redis + 回查逻辑 | 读限制失败率 0 |

---

## 6. 部署拓扑建议

```mermaid
flowchart TB
    subgraph K8S["Kubernetes 集群"]
        subgraph NS1["ns: gateway"]
            NG[Nginx Ingress]
            WG[WebSocket Gateway]
        end

        subgraph NS2["ns: core"]
            S1[行情聚合 Pod x3]
            S2[撮合引擎 Pod x5]
            S3[链上链下 Pod x3]
            S4[NFT 服务 Pod x3]
            S5[回购统计 Pod x2]
            S6[风控服务 Pod x2]
        end

        subgraph NS3["ns: data"]
            PG[(PostgreSQL HA)]
            RD[(Redis Cluster)]
            MQ[(RocketMQ Cluster)]
            SP[(Spark Cluster)]
        end

        subgraph NS4["ns: infra"]
            NA[Nacos]
            PM[Prometheus + Grafana]
            ELK[ELK Stack]
        end
    end

    NG --> WG
    WG --> S1
    WG --> S2
    WG --> S3
    WG --> S4
    WG --> S5
    WG --> S6

    S1 --> NS3
    S2 --> NS3
    S3 --> NS3
    S4 --> NS3
    S5 --> NS3
    S6 --> NS3

    S1 --> NA
    S2 --> NA
    S3 --> NA
    S4 --> NA
    S5 --> NA
    S6 --> NA
```

---

## 7. 总结

本架构设计文档覆盖了加密货币/NFT 综合交易平台的完整技术架构，包括：

- **宏观分层**：客户端 → 网关 → 核心业务 → 数据 → 基础设施 → 区块链
- **子系统详图**：行情聚合、订单撮合、链上链下一致性、NFT 业务、回购统计五大核心模块
- **数据流向**：从用户请求到链上交互的完整链路
- **技术映射**：各模块关键组件、技术选型与性能指标
- **部署拓扑**：基于 Kubernetes 的容器化部署建议

各子系统之间通过 **RocketMQ** 解耦，状态通过 **Redis** 缓存加速，持久化通过 **PostgreSQL** 保障，配置通过 **Nacos** 热更新，监控通过 **Prometheus + Grafana** 覆盖，形成高可用、高性能、可扩展的技术底座。
