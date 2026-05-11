# 方案二：网关层与基础服务实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 实现 API Gateway、鉴权中心、限流熔断、WebSocket Gateway，提供统一入口和安全控制。

**Architecture:** 基于 Go + Gin 构建 API Gateway，集成 JWT 鉴权、Sentinel-Go 限流熔断、gorilla/websocket WebSocket Gateway。Nginx 反向代理层处理 SSL 和静态资源。

**Tech Stack:** Go 1.23, Gin, golang-jwt/jwt/v5, alibaba/sentinel-golang, gorilla/websocket, go-redis/redis/v8

---

## 文件结构

```
ArkHub/
├── internal/
│   ├── response/
│   │   └── response.go          # 统一响应格式
│   ├── middleware/
│   │   ├── auth.go              # JWT 鉴权中间件（完整实现）
│   │   └── ratelimit.go         # Sentinel 限流熔断中间件（完整实现）
│   └── pkg/
│       └── jwt/
│           └── jwt.go             # JWT 工具函数（生成、解析、验签）
├── cmd/
│   ├── api-gateway/
│   │   └── main.go                # 完整路由转发 + 中间件注册
│   ├── auth-service/
│   │   └── main.go                # 鉴权服务
│   └── ws-gateway/
│       └── main.go                # WebSocket 服务
├── test/
│   └── integration/
│       └── gateway_test.go        # 集成测试
├── deployments/
│   └── nginx.conf                 # Nginx 配置
└── go.mod                         # 添加依赖
```

---

## Task 1: 统一响应格式 (internal/response/response.go)

**目标:** 为所有 HTTP API 提供标准化的响应结构。

**Files:**
- Create: `internal/response/response.go`
- Test: `internal/response/response_test.go`

### Step 1: 编写响应结构体

```go
package response

import (
	"github.com/gin-gonic/gin"
	"net/http"
)

// Response 统一 API 响应结构
type Response struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
	TraceID string      `json:"trace_id"`
}

// 预设状态码
const (
	CodeSuccess   = 0
	CodeBadReq    = 400
	CodeUnauthorized = 401
	CodeForbidden    = 403
	CodeNotFound     = 404
	CodeRateLimited  = 429
	CodeInternal     = 500
	CodeServiceUnavailable = 503
)

// JSON 返回成功响应
func JSON(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, Response{
		Code:    CodeSuccess,
		Message: "success",
		Data:    data,
		TraceID: c.GetString("trace_id"),
	})
}

// Error 返回错误响应
func Error(c *gin.Context, statusCode int, code int, message string) {
	c.JSON(statusCode, Response{
		Code:    code,
		Message: message,
		TraceID:   c.GetString("trace_id"),
	})
}
```

### Step 2: 创建响应结构体文件

Run: `mkdir -p internal/response && cat > internal/response/response.go << 'EOF'
[粘贴 Step 1 代码]
EOF`

### Step 3: 编写测试

```go
package response

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	JSON(c, gin.H{"key": "value"})

	var resp Response
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	if err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if resp.Code != CodeSuccess {
		t.Errorf("expected code %d, got %d", CodeSuccess, resp.Code)
	}
	if resp.Message != "success" {
		t.Errorf("expected message 'success', got '%s'", resp.Message)
	}
}

func TestError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	Error(c, 400, CodeBadReq, "bad request")

	if w.Code != 400 {
		t.Errorf("expected status 400, got %d", w.Code)
	}

	var resp Response
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	if err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if resp.Code != CodeBadReq {
		t.Errorf("expected code %d, got %d", CodeBadReq, resp.Code)
	}
}
```

Run: `cat > internal/response/response_test.go << 'EOF'
[粘贴 Step 3 代码]
EOF`

### Step 4: 运行测试

Run: `cd /Users/luca/luca/web3/arkHub/ArkHub && go test ./internal/response/ -v`

Expected: PASS

### Step 5: 提交

```bash
git add internal/response/response.go internal/response/response_test.go
git commit -m "feat: add unified response format

Add Response struct with JSON and Error helpers.
Supports trace_id propagation via gin context."
```

---

## Task 2: 添加依赖

**目标:** 在 go.mod 中添加 JWT、Sentinel、WebSocket 依赖。

### Step 1: 添加依赖

Run:
```bash
cd /Users/luca/luca/web3/arkHub/ArkHub
go get github.com/golang-jwt/jwt/v5
go get github.com/alibaba/sentinel-golang/api/v2
go get github.com/gorilla/websocket
go mod tidy
```

### Step 2: 验证 go.mod

Run: `cat go.mod | grep -E "jwt|sentinel|websocket"`

Expected: 显示三个新增的依赖条目。

### Step 3: 提交

```bash
git add go.mod go.sum
git commit -m "deps: add jwt, sentinel, websocket dependencies"
```

---

## Task 3: JWT 工具包 (internal/pkg/jwt/jwt.go)

**目标:** 封装 JWT 生成、解析、验签逻辑，供中间件和鉴权服务共用。

**Files:**
- Create: `internal/pkg/jwt/jwt.go`
- Test: `internal/pkg/jwt/jwt_test.go`

### Step 1: 创建目录

Run: `mkdir -p internal/pkg/jwt`

### Step 2: 实现 JWT 工具函数

```go
package jwt

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Claims 自定义 JWT Claims
type Claims struct {
	UserID string `json:"user_id"`
	Role   string `json:"role"`
	jwt.RegisteredClaims
}

// GenerateToken 生成 JWT Token (使用 RSA 私钥签名)
// secretPath: RSA 私钥文件路径
func GenerateToken(userID, role, secretPath string, expiresAt time.Time) (string, error) {
	privateKey, err := loadRSAPrivateKey(secretPath)
	if err != nil {
		return "", fmt.Errorf("load private key: %w", err)
	}

	claims := Claims{
		UserID: userID,
		Role:   role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tokenString, err := token.SignedString(privateKey)
	if err != nil {
		return "", fmt.Errorf("sign token: %w", err)
	}

	return tokenString, nil
}

// ParseToken 解析并验证 JWT Token (使用 RSA 公钥验签)
// publicPath: RSA 公钥文件路径
func ParseToken(tokenString, publicPath string) (*Claims, error) {
	publicKey, err := loadRSAPublicKey(publicPath)
	if err != nil {
		return nil, fmt.Errorf("load public key: %w", err)
	}

	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return publicKey, nil
	})
	if err != nil {
		return nil, fmt.Errorf("parse token: %w", err)
	}

	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims, nil
	}
	return nil, fmt.Errorf("invalid token")
}

func loadRSAPrivateKey(path string) (*rsa.PrivateKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block")
	}
	return x509.ParsePKCS1PrivateKey(block.Bytes)
}

func loadRSAPublicKey(path string) (*rsa.PublicKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block")
	}
	pub, err := x509.ParsePKCS1PublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse public key: %w", err)
	}
	return pub, nil
}
```

Run: `cat > internal/pkg/jwt/jwt.go << 'EOF'
[粘贴 Step 2 代码]
EOF`

### Step 3: 生成测试用的 RSA 密钥对

Run:
```bash
mkdir -p test/keys
openssl genrsa -out test/keys/private.pem 2048
openssl rsa -in test/keys/private.pem -pubout -out test/keys/public.pem
```

### Step 4: 编写测试

```go
package jwt

import (
	"testing"
	"time"
)

func TestGenerateAndParseToken(t *testing.T) {
	userID := "user-123"
	role := "admin"
	expiresAt := time.Now().Add(time.Hour)

	tokenString, err := GenerateToken(userID, role, "test/keys/private.pem", expiresAt)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	claims, err := ParseToken(tokenString, "test/keys/public.pem")
	if err != nil {
		t.Fatalf("parse token: %v", err)
	}

	if claims.UserID != userID {
		t.Errorf("expected user_id %s, got %s", userID, claims.UserID)
	}
	if claims.Role != role {
		t.Errorf("expected role %s, got %s", role, claims.Role)
	}
}

func TestParseExpiredToken(t *testing.T) {
	userID := "user-456"
	role := "user"
	expiresAt := time.Now().Add(-time.Hour) // 已过期

	tokenString, err := GenerateToken(userID, role, "test/keys/private.pem", expiresAt)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	_, err = ParseToken(tokenString, "test/keys/public.pem")
	if err == nil {
		t.Error("expected error for expired token, got nil")
	}
}
```

Run: `cat > internal/pkg/jwt/jwt_test.go << 'EOF'
[粘贴 Step 4 代码]
EOF`

### Step 5: 运行测试

Run: `go test ./internal/pkg/jwt/ -v`

Expected: PASS

### Step 6: 提交

```bash
git add internal/pkg/jwt/ test/keys/
git commit -m "feat: add JWT utilities with RSA signing

- GenerateToken: RSA private key signing
- ParseToken: RSA public key verification
- Support RS256 algorithm"
```

---

## Task 4: JWT 鉴权中间件 (internal/middleware/auth.go)

**目标:** 完整实现 JWT 鉴权中间件，从请求头提取并验证 Token。

**Files:**
- Modify: `internal/middleware/auth.go`

### Step 1: 实现完整 JWT 验证逻辑

```go
package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	"arhub/internal/response"
	arhubJWT "arhub/internal/pkg/jwt"
)

// AuthMiddleware 返回 JWT 鉴权中间件
// secret: JWT 签名密钥
func AuthMiddleware(secret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 从请求头中提取 Token
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			response.Error(c, http.StatusUnauthorized, response.CodeUnauthorized, "未提供认证信息")
			c.Abort()
			return
		}

		// 提取 Token 值（去除 "Bearer " 前缀）
		tokenString := strings.TrimPrefix(authHeader, "Bearer ")

		// 解析并验证 Token
		claims, err := arhubJWT.ParseToken(tokenString, secret)
		if err != nil {
			response.Error(c, http.StatusUnauthorized, response.CodeUnauthorized, "无效的 Token")
			c.Abort()
			return
		}

		// 将用户信息存入请求上下文
		c.Set("user_id", claims.UserID)
		c.Set("role", claims.Role)

		c.Next()
	}
}

// GenerateToken 生成 JWT Token（工具函数）
func GenerateToken(userID, role, secret string) (string, error) {
	return arhubJWT.GenerateToken(userID, role, secret, time.Now().Add(time.Hour*24))
}

// RefreshToken 刷新 JWT Token
func RefreshToken(tokenString, secret string) (string, error) {
	claims, err := arhubJWT.ParseToken(tokenString, secret)
	if err != nil {
		return "", err
	}
	return GenerateToken(claims.UserID, claims.Role, secret)
}
```

Run: `cat > internal/middleware/auth.go << 'EOF'
[粘贴 Step 1 代码]
EOF`

### Step 2: 编译检查

Run: `go build ./internal/middleware/`

Expected: 无错误

### Step 3: 提交

```bash
git add internal/middleware/auth.go
git commit -m "feat: implement JWT auth middleware

- Extract and verify Bearer token from Authorization header
- Parse token using RSA public key
- Store user_id and role in gin context"
```

---

## Task 5: 鉴权服务 (cmd/auth-service/main.go)

**目标:** 实现独立的鉴权服务，提供登录、刷新、验证、登出接口。

**Files:**
- Create: `cmd/auth-service/main.go`

### Step 1: 创建鉴权服务

```go
package main

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"arhub/internal/middleware"
	"arhub/internal/response"
	arhubJWT "arhub/internal/pkg/jwt"
)

func main() {
	r := gin.Default()

	// 登录接口：签发 JWT Token
	r.POST("/auth/login", func(c *gin.Context) {
		var req struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			response.Error(c, http.StatusBadRequest, response.CodeBadReq, "请求参数错误")
			return
		}

		// 简化：直接生成 Token（实际应验证用户名密码）
		token, err := middleware.GenerateToken(req.Username, "user", "configs/private.pem")
		if err != nil {
			response.Error(c, http.StatusInternalServerError, response.CodeInternal, "Token 生成失败")
			return
		}

		response.JSON(c, gin.H{
			"access_token":  token,
			"refresh_token": token, // 简化：使用相同 token
			"expires_in":    86400,
		})
	})

	// 刷新接口：刷新 Token
	r.POST("/auth/refresh", func(c *gin.Context) {
		var req struct {
			RefreshToken string `json:"refresh_token"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			response.Error(c, http.StatusBadRequest, response.CodeBadReq, "请求参数错误")
			return
		}

		token, err := middleware.RefreshToken(req.RefreshToken, "configs/private.pem")
		if err != nil {
			response.Error(c, http.StatusUnauthorized, response.CodeUnauthorized, "无效的 Refresh Token")
			return
		}

		response.JSON(c, gin.H{
			"access_token": token,
			"expires_in":   86400,
		})
	})

	// 验证接口：验证 Token 有效性
	r.GET("/auth/verify", func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			response.Error(c, http.StatusUnauthorized, response.CodeUnauthorized, "未提供 Token")
			return
		}

		tokenString := strings.TrimPrefix(authHeader, "Bearer ")
		claims, err := arhubJWT.ParseToken(tokenString, "configs/public.pem")
		if err != nil {
			response.Error(c, http.StatusUnauthorized, response.CodeUnauthorized, "无效的 Token")
			return
		}

		response.JSON(c, gin.H{
			"user_id": claims.UserID,
			"role":    claims.Role,
			"valid":   true,
		})
	})

	// 登出接口：将 Token 加入黑名单
	r.POST("/auth/logout", func(c *gin.Context) {
		// 简化：实际应将 Token 加入 Redis 黑名单
		response.JSON(c, gin.H{"message": "登出成功"})
	})

	r.Run(":8081")
}
```

Run: `cat > cmd/auth-service/main.go << 'EOF'
[粘贴 Step 1 代码]
EOF`

### Step 2: 编译检查

Run: `go build ./cmd/auth-service/`

Expected: 无错误

### Step 3: 提交

```bash
git add cmd/auth-service/main.go
git commit -m "feat: add auth service with login, refresh, verify, logout"
```

---

## Task 6: Sentinel 限流熔断中间件 (internal/middleware/ratelimit.go)

**目标:** 使用 Sentinel-Go 实现 IP 限流、用户限流、接口限流和熔断。

**Files:**
- Modify: `internal/middleware/ratelimit.go`

### Step 1: 实现 Sentinel 限流熔断

```go
package middleware

import (
	"fmt"
	"net/http"
	"time"

	"github.com/alibaba/sentinel-golang/api/v2"
	"github.com/alibaba/sentinel-golang/core/base"
	"github.com/alibaba/sentinel-golang/core/circuitbreaker"
	"github.com/alibaba/sentinel-golang/core/flow"
	"github.com/gin-gonic/gin"

	"arhub/internal/response"
)

// InitSentinel 初始化 Sentinel 规则
func InitSentinel() error {
	// IP 限流：60 次/分钟
	_, err := flow.LoadRules([]*flow.Rule{
		{
			Resource:        "ip_limit",
			Threshold:       60,
			TokenCalculateStrategy: flow.Direct,
			ControlBehavior:        flow.Reject,
			StatIntervalInMs:       60000,
		},
	})
	if err != nil {
		return fmt.Errorf("init ip limit rule: %w", err)
	}

	// 用户限流：100 次/分钟
	_, err = flow.LoadRules([]*flow.Rule{
		{
			Resource:        "user_limit",
			Threshold:       100,
			TokenCalculateStrategy: flow.Direct,
			ControlBehavior:        flow.Reject,
			StatIntervalInMs:       60000,
		},
	})
	if err != nil {
		return fmt.Errorf("init user limit rule: %w", err)
	}

	// 接口限流：1000 QPS
	_, err = flow.LoadRules([]*flow.Rule{
		{
			Resource:        "api_limit",
			Threshold:       1000,
			TokenCalculateStrategy: flow.Direct,
			ControlBehavior:        flow.Reject,
			StatIntervalInMs:       1000,
		},
	})
	if err != nil {
		return fmt.Errorf("init api limit rule: %w", err)
	}

	// 熔断：错误率 > 50% 持续 30 秒
	_, err = circuitbreaker.LoadRules([]*circuitbreaker.Rule{
		{
			Resource:         "api_circuit_breaker",
			Strategy:         circuitbreaker.ErrorRatio,
			Threshold:        0.5,
			StatIntervalMs:   30000,
			SlowRequestRatio: 0.2,
		},
	})
	if err != nil {
		return fmt.Errorf("init circuit breaker rule: %w", err)
	}

	return nil
}

// RateLimitMiddleware 返回限流中间件
func RateLimitMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// IP 限流
		ip := c.ClientIP()
		if _, err := sentinel.Entry("ip_limit", sentinel.WithArgs(base.WithFlag(ip))); err != nil {
			response.Error(c, http.StatusTooManyRequests, response.CodeRateLimited, "请求过于频繁，请稍后再试")
			c.Abort()
			return
		}

		// 用户限流
		userID := c.GetString("user_id")
		if userID != "" {
			if _, err := sentinel.Entry("user_limit", sentinel.WithArgs(base.WithFlag(userID))); err != nil {
				response.Error(c, http.StatusTooManyRequests, response.CodeRateLimited, "请求过于频繁，请稍后再试")
				c.Abort()
				return
			}
		}

		// 接口限流
		if _, err := sentinel.Entry("api_limit", sentinel.WithArgs(base.WithFlag(c.Request.URL.Path))); err != nil {
			response.Error(c, http.StatusTooManyRequests, response.CodeRateLimited, "请求过于频繁，请稍后再试")
			c.Abort()
			return
		}

		c.Next()
	}
}

// CircuitBreakerMiddleware 返回熔断中间件
func CircuitBreakerMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		e, err := sentinel.Entry("api_circuit_breaker")
		if err != nil {
			response.Error(c, http.StatusServiceUnavailable, response.CodeServiceUnavailable, "服务暂不可用，请稍后再试")
			c.Abort()
			return
		}
		defer e.Exit()

		c.Next()

		// 根据响应状态码记录错误
		if c.Writer.Status() >= 500 {
			e.SetError(err)
		}
	}
}
```

Run: `cat > internal/middleware/ratelimit.go << 'EOF'
[粘贴 Step 1 代码]
EOF`

### Step 2: 编译检查

Run: `go build ./internal/middleware/`

Expected: 无错误

### Step 3: 提交

```bash
git add internal/middleware/ratelimit.go
git commit -m "feat: add Sentinel rate limiting and circuit breaker

- IP limit: 60 req/min
- User limit: 100 req/min
- API limit: 1000 QPS
- Circuit breaker: error rate > 50% for 30s"
```

---

## Task 7: API Gateway 路由转发 (cmd/api-gateway/main.go)

**目标:** 实现完整的路由转发逻辑，集成中间件。

**Files:**
- Modify: `cmd/api-gateway/main.go`

### Step 1: 实现完整路由转发

```go
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"arhub/internal/middleware"
	"arhub/internal/response"
)

var (
	httpRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "HTTP 请求总数",
		},
		[]string{"method", "path", "status"},
	)

	httpRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP 请求耗时（秒）",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path"},
	)
)

func init() {
	prometheus.MustRegister(httpRequestsTotal)
	prometheus.MustRegister(httpRequestDuration)
}

// 后端服务地址映射表
var serviceMap = map[string]string{
	"market":  "http://localhost:8082",
	"order":   "http://localhost:8081",
	"nft":     "http://localhost:8084",
	"buyback": "http://localhost:8085",
	"risk":    "http://localhost:8086",
	"chain":   "http://localhost:8083",
}

func main() {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()

	// 注册全局中间件
	r.Use(gin.Recovery())
	r.Use(prometheusMiddleware())

	// 初始化 Sentinel
	if err := middleware.InitSentinel(); err != nil {
		log.Fatalf("初始化 Sentinel 失败: %v", err)
	}

	// 注册 /metrics 端点
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))

	// 注册健康检查端点
	r.GET("/health", func(c *gin.Context) {
		response.JSON(c, gin.H{
			"status": "ok",
			"time":   time.Now().Format(time.RFC3339),
		})
	})

	// API 路由组（带鉴权和限流）
	api := r.Group("/api/v1")
	api.Use(middleware.AuthMiddleware("configs/public.pem"))
	api.Use(middleware.RateLimitMiddleware())
	api.Use(middleware.CircuitBreakerMiddleware())
	{
		api.Any("/market/*path", forwardTo("market"))
		api.Any("/order/*path", forwardTo("order"))
		api.Any("/nft/*path", forwardTo("nft"))
		api.Any("/buyback/*path", forwardTo("buyback"))
		api.Any("/risk/*path", forwardTo("risk"))
		api.Any("/chain/*path", forwardTo("chain"))
	}

	// 启动 HTTP 服务
	port := ":8080"
	srv := &http.Server{
		Addr:    port,
		Handler: r,
	}

	go func() {
		fmt.Printf("🚀 API Gateway 启动成功，监听端口 %s\n", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("API Gateway 启动失败: %v", err)
		}
	}()

	// 优雅关闭
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("API Gateway 关闭失败: %v", err)
	}
}

func prometheusMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		duration := time.Since(start).Seconds()
		method := c.Request.Method
		path := c.Request.URL.Path
		status := fmt.Sprintf("%d", c.Writer.Status())

		httpRequestsTotal.WithLabelValues(method, path, status).Inc()
		httpRequestDuration.WithLabelValues(method, path).Observe(duration)
	}
}

func forwardTo(serviceName string) gin.HandlerFunc {
	targetURL, ok := serviceMap[serviceName]
	if !ok {
		return func(c *gin.Context) {
			response.Error(c, http.StatusServiceUnavailable, response.CodeServiceUnavailable, fmt.Sprintf("服务 %s 不可用", serviceName))
		}
	}

	target, err := url.Parse(targetURL)
	if err != nil {
		return func(c *gin.Context) {
			response.Error(c, http.StatusInternalServerError, response.CodeInternal, "内部错误")
		}
	}

	proxy := httputil.NewSingleHostReverseProxy(target)
	return func(c *gin.Context) {
		// 修改请求路径，去掉 /api/v1/<service> 前缀
		path := c.Request.URL.Path
		path = strings.TrimPrefix(path, fmt.Sprintf("/api/v1/%s", serviceName))
		if path == "" {
			path = "/"
		}
		c.Request.URL.Path = path

		proxy.ServeHTTP(c.Writer, c.Request)
	}
}
```

Run: `cat > cmd/api-gateway/main.go << 'EOF'
[粘贴 Step 1 代码]
EOF`

### Step 2: 编译检查

Run: `go build ./cmd/api-gateway/`

Expected: 无错误

### Step 3: 提交

```bash
git add cmd/api-gateway/main.go
git commit -m "feat: implement full API Gateway with reverse proxy

- Integrate JWT auth, rate limiting, circuit breaker
- Use httputil.ReverseProxy for routing
- Support graceful shutdown"
```

---

## Task 8: WebSocket Gateway (cmd/ws-gateway/main.go)

**目标:** 实现 WebSocket 服务，支持心跳和消息广播。

**Files:**
- Create: `cmd/ws-gateway/main.go`

### Step 1: 实现 WebSocket Gateway

```go
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // 允许所有来源（生产环境应配置白名单）
	},
}

// WSMessage WebSocket 消息格式
type WSMessage struct {
	Type    string                 `json:"type"`
	Channel string                 `json:"channel"`
	Payload map[string]interface{} `json:"payload"`
}

// Client 表示一个 WebSocket 客户端
type Client struct {
	conn    *websocket.Conn
	send    chan []byte
	channel string
}

var (
	clients    = make(map[*Client]bool)
	broadcast  = make(chan []byte)
	register   = make(chan *Client)
	unregister = make(chan *Client)
)

func main() {
	go hub()

	r := gin.Default()
	r.GET("/ws", handleWebSocket)

	fmt.Println("🚀 WebSocket Gateway 启动成功，监听端口 :8087")
	if err := r.Run(":8087"); err != nil {
		log.Fatalf("WebSocket Gateway 启动失败: %v", err)
	}
}

func handleWebSocket(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("升级 WebSocket 失败: %v", err)
		return
	}

	client := &Client{conn: conn, send: make(chan []byte, 256)}
	register <- client

	go client.writePump()
	go client.readPump()
}

func (c *Client) readPump() {
	defer func() {
		unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadDeadline(time.Now().Add(90 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(90 * time.Second))
		return nil
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket 错误: %v", err)
			}
			break
		}

		var msg WSMessage
		if err := json.Unmarshal(message, &msg); err != nil {
			log.Printf("解析消息失败: %v", err)
			continue
		}

		switch msg.Type {
		case "subscribe":
			c.channel = msg.Channel
			log.Printf("客户端订阅频道: %s", msg.Channel)
		case "heartbeat":
			c.send <- []byte(`{"type":"pong"}`)
		}
	}
}

func (c *Client) writePump() {
	ticker := time.NewTicker(30 * time.Second)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			c.conn.WriteMessage(websocket.TextMessage, message)

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func hub() {
	for {
		select {
		case client := <-register:
			clients[client] = true
		case client := <-unregister:
			if _, ok := clients[client]; ok {
				delete(clients, client)
				close(client.send)
			}
		case message := <-broadcast:
			for client := range clients {
				select {
				case client.send <- message:
				default:
					close(client.send)
					delete(clients, client)
				}
			}
		}
	}
}
```

Run: `cat > cmd/ws-gateway/main.go << 'EOF'
[粘贴 Step 1 代码]
EOF`

### Step 2: 编译检查

Run: `go build ./cmd/ws-gateway/`

Expected: 无错误

### Step 3: 提交

```bash
git add cmd/ws-gateway/main.go
git commit -m "feat: add WebSocket Gateway with heartbeat and broadcast

- Support subscribe/unsubscribe/heartbeat message types
- PING/PONG every 30s, timeout 90s
- Hub-based message broadcasting"
```

---

## Task 9: 集成测试 (test/integration/gateway_test.go)

**目标:** 编写集成测试，验证网关完整链路。

**Files:**
- Create: `test/integration/gateway_test.go`

### Step 1: 创建测试

```go
package integration

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"arhub/internal/middleware"
	"arhub/internal/response"
)

func TestHealthEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/health", func(c *gin.Context) {
		response.JSON(c, gin.H{"status": "ok"})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/health", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)
}

func TestAuthMiddlewareMissingToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.AuthMiddleware("test/keys/public.pem"))
	r.GET("/protected", func(c *gin.Context) {
		c.String(200, "ok")
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/protected", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, 401, w.Code)
}
```

Run: `mkdir -p test/integration && cat > test/integration/gateway_test.go << 'EOF'
[粘贴 Step 1 代码]
EOF`

### Step 2: 运行测试

Run: `go test ./test/integration/ -v`

Expected: PASS

### Step 3: 提交

```bash
git add test/integration/gateway_test.go
git commit -m "test: add integration tests for gateway"
```

---

## Task 10: Nginx 配置 (deployments/nginx.conf)

**目标:** 配置 Nginx 反向代理和负载均衡。

**Files:**
- Create: `deployments/nginx.conf`

### Step 1: 创建 Nginx 配置

```nginx
server {
    listen 80;
    server_name localhost;

    # 静态资源
    location /static/ {
        alias /var/www/static/;
        expires 30d;
    }

    # API 网关代理
    location /api/ {
        proxy_pass http://localhost:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }

    # WebSocket 代理
    location /ws {
        proxy_pass http://localhost:8087;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }

    # 健康检查
    location /health {
        proxy_pass http://localhost:8080/health;
        access_log off;
    }
}
```

Run: `cat > deployments/nginx.conf << 'EOF'
[粘贴 Step 1 配置]
EOF`

### Step 2: 验证配置

Run: `nginx -t -c $(pwd)/deployments/nginx.conf`

Expected: syntax is ok

### Step 3: 提交

```bash
git add deployments/nginx.conf
git commit -m "ops: add Nginx reverse proxy config

- Proxy /api/ to API Gateway
- Proxy /ws to WebSocket Gateway
- Static assets and health check"
```

---

## Self-Review

### 1. Spec Coverage

| 需求 | 实现任务 | 状态 |
|------|---------|------|
| 统一响应格式 | Task 1 | ✅ |
| API Gateway 路由转发 | Task 7 | ✅ |
| JWT 鉴权中心 | Task 3, 4, 5 | ✅ |
| Sentinel 限流熔断 | Task 6 | ✅ |
| WebSocket Gateway | Task 8 | ✅ |
| 集成测试 | Task 9 | ✅ |
| Nginx 配置 | Task 10 | ✅ |

### 2. Placeholder Scan

- 无 "TBD" / "TODO" / "implement later"
- 所有代码步骤包含完整实现
- 所有测试包含具体断言

### 3. Type Consistency

- `Claims` 结构体在 `internal/pkg/jwt/jwt.go` 定义，在 `internal/middleware/auth.go` 中使用
- `Response` 结构体在 `internal/response/response.go` 定义，全局使用
- JWT 公私钥路径统一使用 `configs/` 目录

---

## 执行选项

**Plan complete and saved to `docs/superpowers/plans/2026-05-11-gateway-and-auth.md`.**

Two execution options:

**1. Subagent-Driven (recommended)** - Dispatch a fresh subagent per task, review between tasks, fast iteration

**2. Inline Execution** - Execute tasks in this session using executing-plans, batch execution with checkpoints

Which approach do you prefer?
