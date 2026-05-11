# 差距分析五：微服务与基础设施

> 对比范围：web3-go Docker 基础 vs ArkHub K8s、Nacos、监控告警

---

## 1. 技能对比

| 技能点 | web3-go 覆盖 | ArkHub 需求 | 差距 |
|--------|-------------|------------|------|
| Docker 基础 | ✅ docker-compose | ✅ 必需 | 无差距 |
| **K8s 编排** | ❌ 未涉及 | ❌ Deployment、Service、Ingress | 🔴 大差距 |
| **Nacos 配置中心** | ❌ 未涉及 | ❌ 服务注册、配置热更新 | 🔴 大差距 |
| **Prometheus** | ❌ 未涉及 | ❌ 指标采集、告警规则 | 🔴 大差距 |
| **Grafana** | ❌ 未涉及 | ❌ 仪表盘、可视化 | 🔴 大差距 |
| **ELK 日志** | ❌ 未涉及 | ❌ 日志聚合、检索 | 🔴 大差距 |
| **API Gateway** | ❌ 未涉及 | ❌ 路由、鉴权、限流 | 🔴 大差距 |
| **服务网格** | ❌ 未涉及 | ⚠️ Istio（可选） | 🟡 中等 |

---

## 2. K8s 部署配置

### ArkHub 命名空间规划

```yaml
# k8s/namespaces.yaml
apiVersion: v1
kind: Namespace
metadata:
  name: arhub-gateway
---
apiVersion: v1
kind: Namespace
metadata:
  name: arhub-core
---
apiVersion: v1
kind: Namespace
metadata:
  name: arhub-data
---
apiVersion: v1
kind: Namespace
metadata:
  name: arhub-infra
```

### API Gateway 部署

```yaml
# k8s/gateway-deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: api-gateway
  namespace: arhub-gateway
spec:
  replicas: 3
  selector:
    matchLabels:
      app: api-gateway
  template:
    metadata:
      labels:
        app: api-gateway
    spec:
      containers:
      - name: api-gateway
        image: arhub/api-gateway:latest
        ports:
        - containerPort: 8080
        env:
        - name: REDIS_URL
          valueFrom:
            secretKeyRef:
              name: arhub-secrets
              key: redis-url
        resources:
          requests:
            memory: "256Mi"
            cpu: "250m"
          limits:
            memory: "512Mi"
            cpu: "500m"
        livenessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 10
          periodSeconds: 30
        readinessProbe:
          httpGet:
            path: /ready
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 10
---
apiVersion: v1
kind: Service
metadata:
  name: api-gateway-service
  namespace: arhub-gateway
spec:
  selector:
    app: api-gateway
  ports:
  - port: 80
    targetPort: 8080
  type: ClusterIP
---
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: api-gateway-ingress
  namespace: arhub-gateway
  annotations:
    nginx.ingress.kubernetes.io/rate-limit: "100"
spec:
  rules:
  - host: api.arhub.io
    http:
      paths:
      - path: /api
        pathType: Prefix
        backend:
          service:
            name: api-gateway-service
            port:
              number: 80
```

---

## 3. Nacos 配置中心

### 配置热更新实现

```go
package config

import (
    "encoding/json"
    "fmt"
    "sync"
    
    "github.com/nacos-group/nacos-sdk-go/v2/clients"
    "github.com/nacos-group/nacos-sdk-go/v2/clients/naming_client"
    "github.com/nacos-group/nacos-sdk-go/v2/common/constant"
    "github.com/nacos-group/nacos-sdk-go/v2/vo"
)

// NacosConfig Nacos 配置中心客户端
type NacosConfig struct {
    client    naming_client.INamingClient
    configMap sync.Map
    listeners []func(string, interface{})
}

// NewNacosConfig 创建 Nacos 配置客户端
func NewNacosConfig(serverAddr string, namespace string) (*NacosConfig, error) {
    clientConfig := constant.ClientConfig{
        NamespaceId:         namespace,
        TimeoutMs:           5000,
        NotLoadCacheAtStart: true,
        LogDir:              "/tmp/nacos/log",
        CacheDir:            "/tmp/nacos/cache",
    }
    
    serverConfig := []constant.ServerConfig{
        {
            IpAddr: serverAddr,
            Port:   8848,
        },
    }
    
    client, err := clients.NewNamingClient(
        vo.NacosClientParam{
            ClientConfig:  &clientConfig,
            ServerConfigs: serverConfig,
        },
    )
    if err != nil {
        return nil, err
    }
    
    return &NacosConfig{
        client: client,
    }, nil
}

// RegisterService 注册服务
func (nc *NacosConfig) RegisterService(serviceName string, port int) error {
    param := vo.RegisterInstanceParam{
        Ip:          "127.0.0.1",
        Port:        uint64(port),
        ServiceName: serviceName,
        Weight:      10,
        Enable:      true,
        Healthy:     true,
        Ephemeral:   true,
    }
    
    success, err := nc.client.RegisterInstance(param)
    if err != nil {
        return err
    }
    
    if !success {
        return fmt.Errorf("register service failed")
    }
    
    return nil
}

// DiscoverService 服务发现
func (nc *NacosConfig) DiscoverService(serviceName string) ([]vo.Instance, error) {
    param := vo.SelectInstancesParam{
        ServiceName: serviceName,
        HealthyOnly: true,
    }
    
    return nc.client.SelectInstances(param)
}

// ListenConfig 监听配置变更
func (nc *NacosConfig) ListenConfig(dataId, group string) error {
    // 通过 Nacos 配置服务监听配置变更
    // 简化示例
    return nil
}
```

---

## 4. Prometheus + Grafana 监控

### 指标采集

```go
package metrics

import (
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promhttp"
    "net/http"
)

// Metrics 监控指标
type Metrics struct {
    // HTTP 请求指标
    httpRequestsTotal   *prometheus.CounterVec
    httpRequestDuration *prometheus.HistogramVec
    
    // 业务指标
    ordersTotal         prometheus.Counter
    tradesTotal         prometheus.Counter
    orderBookDepth      prometheus.Gauge
    
    // 系统指标
    goroutines          prometheus.Gauge
    memoryUsage         prometheus.Gauge
}

func NewMetrics() *Metrics {
    m := &Metrics{
        httpRequestsTotal: prometheus.NewCounterVec(
            prometheus.CounterOpts{
                Name: "http_requests_total",
                Help: "Total number of HTTP requests",
            },
            []string{"method", "path", "status"},
        ),
        httpRequestDuration: prometheus.NewHistogramVec(
            prometheus.HistogramOpts{
                Name:    "http_request_duration_seconds",
                Help:    "HTTP request duration",
                Buckets: prometheus.DefBuckets,
            },
            []string{"method", "path"},
        ),
        ordersTotal: prometheus.NewCounter(
            prometheus.CounterOpts{
                Name: "orders_total",
                Help: "Total number of orders",
            },
        ),
        tradesTotal: prometheus.NewCounter(
            prometheus.CounterOpts{
                Name: "trades_total",
                Help: "Total number of trades",
            },
        ),
        orderBookDepth: prometheus.NewGauge(
            prometheus.GaugeOpts{
                Name: "order_book_depth",
                Help: "Current order book depth",
            },
        ),
        goroutines: prometheus.NewGauge(
            prometheus.GaugeOpts{
                Name: "go_goroutines",
                Help: "Number of goroutines",
            },
        ),
        memoryUsage: prometheus.NewGauge(
            prometheus.GaugeOpts{
                Name: "go_memory_usage_bytes",
                Help: "Current memory usage",
            },
        ),
    }
    
    // 注册指标
    prometheus.MustRegister(m.httpRequestsTotal)
    prometheus.MustRegister(m.httpRequestDuration)
    prometheus.MustRegister(m.ordersTotal)
    prometheus.MustRegister(m.tradesTotal)
    prometheus.MustRegister(m.orderBookDepth)
    prometheus.MustRegister(m.goroutines)
    prometheus.MustRegister(m.memoryUsage)
    
    return m
}

// RecordHTTPRequest 记录 HTTP 请求
func (m *Metrics) RecordHTTPRequest(method, path, status string, duration float64) {
    m.httpRequestsTotal.WithLabelValues(method, path, status).Inc()
    m.httpRequestDuration.WithLabelValues(method, path).Observe(duration)
}

// RecordOrder 记录订单
func (m *Metrics) RecordOrder() {
    m.ordersTotal.Inc()
}

// RecordTrade 记录成交
func (m *Metrics) RecordTrade() {
    m.tradesTotal.Inc()
}

// UpdateOrderBookDepth 更新订单簿深度
func (m *Metrics) UpdateOrderBookDepth(depth float64) {
    m.orderBookDepth.Set(depth)
}

// StartMetricsServer 启动指标服务
func StartMetricsServer(addr string) {
    http.Handle("/metrics", promhttp.Handler())
    go http.ListenAndServe(addr, nil)
}
```

### Grafana 仪表盘配置

```json
{
  "dashboard": {
    "title": "ArkHub Platform",
    "panels": [
      {
        "title": "HTTP Requests",
        "targets": [
          {
            "expr": "rate(http_requests_total[5m])",
            "legendFormat": "{{method}} {{path}}"
          }
        ]
      },
      {
        "title": "Order Book Depth",
        "targets": [
          {
            "expr": "order_book_depth",
            "legendFormat": "Depth"
          }
        ]
      },
      {
        "title": "Trade Rate",
        "targets": [
          {
            "expr": "rate(trades_total[1m])",
            "legendFormat": "Trades/sec"
          }
        ]
      }
    ]
  }
}
```

---

## 5. ELK 日志聚合

### 结构化日志

```go
package logger

import (
    "encoding/json"
    "os"
    "time"
)

// LogEntry 结构化日志条目
type LogEntry struct {
    Timestamp string                 `json:"@timestamp"`
    Level     string                 `json:"level"`
    Message   string                 `json:"message"`
    Service   string                 `json:"service"`
    TraceID   string                 `json:"trace_id"`
    SpanID    string                 `json:"span_id"`
    Fields    map[string]interface{} `json:"fields"`
}

type Logger struct {
    service string
    output  *os.File
}

func NewLogger(service string) *Logger {
    return &Logger{
        service: service,
        output:  os.Stdout,
    }
}

func (l *Logger) Info(msg string, fields map[string]interface{}) {
    l.log("INFO", msg, fields)
}

func (l *Logger) Error(msg string, fields map[string]interface{}) {
    l.log("ERROR", msg, fields)
}

func (l *Logger) log(level, msg string, fields map[string]interface{}) {
    entry := LogEntry{
        Timestamp: time.Now().Format(time.RFC3339),
        Level:     level,
        Message:   msg,
        Service:   l.service,
        Fields:    fields,
    }
    
    data, _ := json.Marshal(entry)
    l.output.Write(data)
    l.output.Write([]byte("\n"))
}
```

---

## 6. API Gateway 实现

```go
package gateway

import (
    "context"
    "encoding/json"
    "net/http"
    "strings"
    "time"
    
    "github.com/golang-jwt/jwt/v5"
    "github.com/juju/ratelimit"
)

// Gateway API 网关
type Gateway struct {
    routes      map[string]http.Handler
    jwtSecret   []byte
    rateLimiter *ratelimit.Bucket
}

// NewGateway 创建网关
func NewGateway(jwtSecret string) *Gateway {
    return &Gateway{
        routes:      make(map[string]http.Handler),
        jwtSecret:   []byte(jwtSecret),
        rateLimiter: ratelimit.NewBucket(time.Second, 1000), // 1000 QPS
    }
}

// AddRoute 添加路由
func (g *Gateway) AddRoute(prefix string, handler http.Handler) {
    g.routes[prefix] = handler
}

// ServeHTTP 处理请求
func (g *Gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    // 1. 限流检查
    if g.rateLimiter.TakeAvailable(1) == 0 {
        g.writeError(w, http.StatusTooManyRequests, "rate limit exceeded")
        return
    }
    
    // 2. 鉴权
    token := r.Header.Get("Authorization")
    if token == "" {
        g.writeError(w, http.StatusUnauthorized, "missing token")
        return
    }
    
    if !g.validateToken(token) {
        g.writeError(w, http.StatusUnauthorized, "invalid token")
        return
    }
    
    // 3. 路由分发
    for prefix, handler := range g.routes {
        if strings.HasPrefix(r.URL.Path, prefix) {
            handler.ServeHTTP(w, r)
            return
        }
    }
    
    g.writeError(w, http.StatusNotFound, "route not found")
}

func (g *Gateway) validateToken(tokenString string) bool {
    tokenString = strings.TrimPrefix(tokenString, "Bearer ")
    token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
        return g.jwtSecret, nil
    })
    return err == nil && token.Valid
}

func (g *Gateway) writeError(w http.ResponseWriter, code int, msg string) {
    w.WriteHeader(code)
    json.NewEncoder(w).Encode(map[string]interface{}{
        "error": msg,
        "code":  code,
    })
}
```

---

## 7. 学习建议

| 优先级 | 主题 | 学习时间 | 产出 |
|--------|------|---------|------|
| P0 | K8s 基础 | 3 天 | 部署 ArkHub 到 K8s |
| P0 | Nacos 配置中心 | 2 天 | 服务注册、配置热更新 |
| P0 | Prometheus + Grafana | 2 天 | 监控仪表盘 |
| P1 | ELK 日志 | 1 天 | 日志聚合 |
| P1 | API Gateway | 1 天 | 路由、鉴权、限流 |
| P2 | Istio 服务网格 | 2 天 | 流量管理 |

---

## 8. 参考资源

| 资源 | 链接 | 说明 |
|------|------|------|
| K8s 官方文档 | https://kubernetes.io/docs/home/ | 官方文档 |
| Nacos 文档 | https://nacos.io/zh-cn/docs/what-is-nacos.html | 中文文档 |
| Prometheus | https://prometheus.io/docs/ | 官方文档 |
| Grafana 仪表盘 | https://grafana.com/grafana/dashboards | 社区仪表盘 |
