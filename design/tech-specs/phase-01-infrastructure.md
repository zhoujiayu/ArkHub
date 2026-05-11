# 阶段一：基础设施层技术方案

> 阶段目标：搭建可独立运行的基础设施，提供 PostgreSQL、Redis、RocketMQ、Nacos、Prometheus、Grafana 全套环境，健康检查全部通过。
> 预计工期：3 天

---

## 0. 设计原因

### 0.1 为什么选 Docker Compose 作为本地环境

| 对比维度 | Docker Compose | Kubernetes | 选择原因 |
|---------|---------------|-----------|---------|
| **学习成本** | 低，一条命令启动 | 高，需掌握 Pod/Service/Ingress 等概念 | 团队快速上手，3 天完成搭建 |
| **资源占用** | 低，8GB 内存可运行 | 高，Minikube 需 16GB+ | 开发机器配置要求低 |
| **调试便利性** | 直接访问容器，日志直观 | 需 kubectl 转发，层级多 | 开发阶段故障定位快 |
| **迁移路径** | docker-compose.yml → kompose → K8s yaml | 直接可用 | 平滑过渡，生产环境再用 K8s |

**业务价值**：
- 开发环境一键启动，新人 onboarding 时间从 1 天缩短至 1 小时
- 与 CI/CD 流程集成，自动化测试环境快速创建
- 后续阶段可平滑迁移至 K8s，无技术债务

### 0.2 为什么选这套中间件组合

| 组件 | 替代方案 | 选择原因 | 业务价值 |
|------|---------|---------|---------|
| **PostgreSQL** | MySQL | JSONB 支持灵活元数据；ACID 完整性更强；主从复制成熟 | 金融级数据安全 |
| **Redis** | Memcached | 支持 ZSet（订单簿排序）、持久化、Lua 原子操作 | 支撑订单簿、热点缓存 |
| **RocketMQ** | Kafka、RabbitMQ | 事务消息保障金融级可靠；广播模式适配价格推送 | 价格推送不丢失 |
| **Nacos** | Consul、Etcd | 配置 + 注册中心一体；国产生态；支持热更新 | 行情权重实时生效 |
| **Prometheus + Grafana** | ELK、InfluxDB | 云原生标准方案；社区成熟；与 Go 生态无缝集成 | 系统/业务指标全覆盖 |

---

## 1. 模块拆分

本阶段细分为以下子模块，每个子模块独立验证：

| 子模块 | 描述 | 验证方式 |
|--------|------|---------|
| **1.1 Docker 编排** | docker-compose.yml 编排所有服务 | `docker-compose up` 无报错 |
| **1.2 PostgreSQL 主从集群** | 初始化库表结构、用户权限 | 连接测试 + 表结构校验 |
| **1.3 Redis 集群** | 缓存节点部署、持久化配置 | Redis CLI 连接 + SET/GET 测试 |
| **1.4 RocketMQ 集群** | NameServer + Broker 部署 | 发送/消费测试消息 |
| **1.5 Nacos 配置中心** | 服务注册、配置热更新 | Web UI 登录 + 配置发布 |
| **1.6 监控告警** | Prometheus + Grafana | 仪表盘展示、告警规则验证 |
| **1.7 项目脚手架** | Go 项目目录结构、Makefile | `make build` 编译通过 |

---

## 2. 技术方案

### 2.1 Docker 编排

```yaml
# deployments/docker-compose.yml
# 包含：PostgreSQL、Redis、RocketMQ、Nacos、Prometheus、Grafana
```

**关键配置：**
- 使用 `docker-compose.yml` 单文件编排
- 各服务通过 Docker Network 内网通信
- 数据卷持久化（PostgreSQL 数据、Redis 数据、Grafana 配置）
- 健康检查配置（healthcheck），确保依赖服务就绪后才启动

### 2.2 PostgreSQL 主从集群

**库表规划：**

| 数据库 | 表名 | 用途 |
|--------|------|------|
| `arhub` | `orders` | 订单数据 |
| `arhub` | `trades` | 成交记录 |
| `arhub` | `user_balances` | 用户资产 |
| `arhub` | `buyback_records` | 回购记录 |
| `arhub` | `nft_assets` | NFT 资产 |
| `arhub` | `sync_checkpoint` | 链上同步断点 |
| `arhub` | `risk_events` | 风控事件 |

**初始化脚本：**
```sql
-- migrations/001_init_schema.sql
-- 包含：建库、建表、建索引、初始化用户
```

### 2.3 Redis 集群

**数据分片规划：**

| DB | 用途 |
|----|------|
| 0 | 订单簿缓存 |
| 1 | 用户会话 |
| 2 | 行情价格 |
| 3 | NFT 热度 |
| 4 | 回购统计 |
| 5 | 风控计数 |

### 2.4 RocketMQ 集群

**Topic 规划：**

| Topic | 用途 | 消息类型 |
|-------|------|---------|
| `market-price` | 指数/标记价格推送 | 广播 |
| `oracle-price` | 预言机价格签名 | 广播 |
| `order-filled` | 成交通知 | 集群 |
| `trade-notification` | 交易通知 | 集群 |
| `chain-event` | 链上事件 | 集群 |
| `nft-heat` | NFT 热度推送 | 广播 |
| `risk-alert` | 风控告警 | 集群 |

### 2.5 Nacos 配置中心

**命名空间规划：**

| 命名空间 | 用途 |
|---------|------|
| `dev` | 开发环境配置 |
| `test` | 测试环境配置 |
| `prod` | 生产环境配置 |

### 2.6 监控告警

**监控指标：**

| 指标 | 来源 | 告警阈值 |
|------|------|---------|
| CPU 使用率 | Node Exporter | > 80% |
| 内存使用率 | Node Exporter | > 80% |
| PostgreSQL 连接数 | Postgres Exporter | > 80% |
| Redis 内存使用 | Redis Exporter | > 80% |
| RocketMQ 消息积压 | RocketMQ Exporter | > 10000 |

### 2.7 项目脚手架

**目录结构：**

```
ArkHub/
├── cmd/
│   └── api-gateway/          # 阶段二实现
├── internal/
│   └── pkg/                  # 公共包（数据库、Redis、MQ 客户端）
├── pkg/
│   └── utils/                # 工具函数
├── api/
│   └── openapi/              # OpenAPI 规范
├── configs/
│   ├── app.yml               # 应用配置
│   └── nacos/                # Nacos 配置模板
├── deployments/
│   ├── docker-compose.yml    # 基础设施编排
│   └── prometheus/             # 监控配置
├── test/
│   └── integration/          # 集成测试脚本
└── Makefile                  # 构建脚本
```

---

## 3. 验证清单

| 检查项 | 验证命令 | 通过标准 |
|--------|---------|---------|
| Docker Compose 启动 | `docker-compose up -d` | 所有容器状态 `healthy` |
| PostgreSQL 连接 | `psql -h localhost -U arhub` | 连接成功 |
| Redis 连接 | `redis-cli ping` | 返回 `PONG` |
| RocketMQ 消息 | 发送/消费测试消息 | 消息收发成功 |
| Nacos 访问 | `curl http://localhost:8848/nacos` | 返回 200 |
| Prometheus 查询 | `curl http://localhost:9090/api/v1/query?query=up` | 返回 JSON |
| Grafana 登录 | `curl http://localhost:3000/login` | 返回 200 |
| Go 项目编译 | `make build` | 无报错 |

---

## 4. 风险与应对

| 风险 | 影响 | 应对措施 |
|------|------|---------|
| Docker 内存不足 | 容器启动失败 | 增加 Docker 内存限制至 8GB |
| 端口冲突 | 服务启动失败 | 使用非标准端口，文档标注 |
| RocketMQ 启动慢 | 依赖服务超时 | 增加 healthcheck 等待时间 |

---

## 5. 交付物

- [ ] `deployments/docker-compose.yml` — Docker 编排文件
- [ ] `migrations/*.sql` — 数据库初始化脚本
- [ ] `configs/nacos/*.json` — Nacos 初始配置
- [ ] `deployments/prometheus/*.yml` — 监控配置
- [ ] `Makefile` — 构建脚本
- [ ] `README.md` — 启动与验证说明
