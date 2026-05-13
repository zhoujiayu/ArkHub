// ============================================================
// cmd/auth-service/main.go
// 鉴权中心服务
// 职责：用户注册、登录认证、JWT Token 签发与验证、Token 刷新与注销（含黑名单）
// ============================================================

package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"golang.org/x/crypto/bcrypt"

	"arhub/internal/pkg/db"
	arhubJWT "arhub/internal/pkg/jwt"
	"arhub/internal/response"
)

const (
	privateKeyPath = "configs/private.pem"
	publicKeyPath  = "configs/public.pem"
)

var (
	dbConn   *sql.DB
	redisCli *redis.Client
)

// --- 请求结构体 ---

type RegisterRequest struct {
	Username string `json:"username" binding:"required,min=3,max=32"`
	Password string `json:"password" binding:"required,min=6,max=64"`
}

type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type RefreshRequest struct {
	Token string `json:"token" binding:"required"`
}

// --- 主函数 ---

func main() {
	// 从环境变量读取配置（支持 Docker 部署）
	dbCfg := db.Config{
		Host:     getEnv("DB_HOST", "localhost"),
		Port:     getEnvInt("DB_PORT", 5432),
		User:     getEnv("DB_USER", "arhub"),
		Password: getEnv("DB_PASSWORD", "arhub123"),
		DBName:   getEnv("DB_NAME", "arhub"),
		SSLMode:  "disable",
	}

	// 初始化 PostgreSQL
	var err error
	dbConn, err = db.NewDB(dbCfg)
	if err != nil {
		log.Fatalf("PostgreSQL 初始化失败: %v", err)
	}
	defer dbConn.Close()

	// 初始化 Redis（用于 Token 黑名单）
	redisHost := getEnv("REDIS_HOST", "localhost")
	redisPort := getEnv("REDIS_PORT", "6379")
	redisCli = redis.NewClient(&redis.Options{
		Addr:     redisHost + ":" + redisPort,
		Password: "",
		DB:       1,
	})
	if err := redisCli.Ping(context.Background()).Err(); err != nil {
		log.Fatalf("Redis 连接失败: %v", err)
	}
	defer redisCli.Close()

	// 确保数据表存在
	if err := initTables(); err != nil {
		log.Fatalf("初始化数据表失败: %v", err)
	}

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())

	// 健康检查
	r.GET("/health", func(c *gin.Context) {
		response.JSON(c, gin.H{
			"status": "ok",
			"time":   time.Now().Format(time.RFC3339),
		})
	})

	// 认证相关路由（公开接口，无需鉴权）
	auth := r.Group("/auth")
	{
		auth.POST("/register", handleRegister)
		auth.POST("/login", handleLogin)
		auth.GET("/verify", handleVerify)
		auth.POST("/refresh", handleRefresh)
		auth.POST("/logout", handleLogout)
	}

	port := ":8088"
	fmt.Printf("🚀 Auth Service 启动成功，监听端口 %s\n", port)
	if err := r.Run(port); err != nil {
		log.Fatalf("Auth Service 启动失败: %v", err)
	}
}

// --- 数据表初始化 ---

func initTables() error {
	// users 表
	createUsersSQL := `
		CREATE TABLE IF NOT EXISTS users (
			id BIGSERIAL PRIMARY KEY,
			user_id VARCHAR(64) NOT NULL UNIQUE,
			username VARCHAR(64) NOT NULL UNIQUE,
			password_hash VARCHAR(255) NOT NULL,
			role VARCHAR(20) NOT NULL DEFAULT 'user',
			status VARCHAR(20) NOT NULL DEFAULT 'active',
			created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
		)`
	if _, err := dbConn.Exec(createUsersSQL); err != nil {
		return fmt.Errorf("创建 users 表失败: %w", err)
	}

	// token_blacklist 表
	createBlacklistSQL := `
		CREATE TABLE IF NOT EXISTS token_blacklist (
			id BIGSERIAL PRIMARY KEY,
			jti VARCHAR(64) NOT NULL UNIQUE,
			token_hash VARCHAR(255) NOT NULL,
			expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
		)`
	if _, err := dbConn.Exec(createBlacklistSQL); err != nil {
		return fmt.Errorf("创建 token_blacklist 表失败: %w", err)
	}

	return nil
}

// --- HTTP 处理器 ---

// handleRegister 用户注册
func handleRegister(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, response.CodeBadReq, "请求参数错误: "+err.Error())
		return
	}

	// 检查用户名是否已存在
	var existingUser string
	err := dbConn.QueryRow("SELECT user_id FROM users WHERE username = $1", req.Username).Scan(&existingUser)
	if err != nil && err != sql.ErrNoRows {
		response.Error(c, http.StatusInternalServerError, response.CodeInternal, "数据库查询失败")
		return
	}
	if err != sql.ErrNoRows {
		response.Error(c, http.StatusConflict, response.CodeBadReq, "用户名已存在")
		return
	}

	// bcrypt 加密密码
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, response.CodeInternal, "密码加密失败")
		return
	}

	// 生成 user_id
	userID := fmt.Sprintf("U%d", time.Now().UnixNano())

	// 插入用户
	_, err = dbConn.Exec(
		"INSERT INTO users (user_id, username, password_hash, role, status) VALUES ($1, $2, $3, $4, $5)",
		userID, req.Username, string(hashedPassword), "user", "active",
	)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, response.CodeInternal, "用户创建失败: "+err.Error())
		return
	}

	response.JSON(c, gin.H{
		"user_id":  userID,
		"username": req.Username,
		"message":  "注册成功",
	})
}

// handleLogin 用户登录，签发 JWT Token
func handleLogin(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, response.CodeBadReq, "请求参数错误: "+err.Error())
		return
	}

	// 查询用户
	var userID, passwordHash, role, status string
	err := dbConn.QueryRow(
		"SELECT user_id, password_hash, role, status FROM users WHERE username = $1",
		req.Username,
	).Scan(&userID, &passwordHash, &role, &status)

	if err == sql.ErrNoRows {
		response.Error(c, http.StatusUnauthorized, response.CodeUnauthorized, "用户名或密码错误")
		return
	}
	if err != nil {
		response.Error(c, http.StatusInternalServerError, response.CodeInternal, "数据库查询失败")
		return
	}

	// 检查用户状态
	if status != "active" {
		response.Error(c, http.StatusForbidden, response.CodeUnauthorized, "账户已被禁用或删除")
		return
	}

	// 验证密码
	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(req.Password)); err != nil {
		response.Error(c, http.StatusUnauthorized, response.CodeUnauthorized, "用户名或密码错误")
		return
	}

	// 生成 JWT Token
	token, err := arhubJWT.GenerateToken(userID, role, privateKeyPath, time.Now().Add(24*time.Hour))
	if err != nil {
		response.Error(c, http.StatusInternalServerError, response.CodeInternal, "Token 生成失败: "+err.Error())
		return
	}

	response.JSON(c, gin.H{
		"token":      token,
		"token_type": "Bearer",
		"expires_in": 86400,
	})
}

// handleVerify 验证 JWT Token（从 Authorization Header 读取）
func handleVerify(c *gin.Context) {
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		response.Error(c, http.StatusUnauthorized, response.CodeUnauthorized, "未提供 Authorization 请求头")
		return
	}

	tokenString := strings.TrimPrefix(authHeader, "Bearer ")
	if tokenString == authHeader {
		response.Error(c, http.StatusUnauthorized, response.CodeUnauthorized, "Authorization 请求头格式错误，应为 Bearer <token>")
		return
	}

	// 解析 Token
	claims, err := arhubJWT.ParseToken(tokenString, publicKeyPath)
	if err != nil {
		response.Error(c, http.StatusUnauthorized, response.CodeUnauthorized, "Token 无效或已过期: "+err.Error())
		return
	}

	// 检查 Token 是否在黑名单中
	if isTokenBlacklisted(c.Request.Context(), tokenString) {
		response.Error(c, http.StatusUnauthorized, response.CodeUnauthorized, "Token 已被注销")
		return
	}

	response.JSON(c, gin.H{
		"user_id": claims.UserID,
		"role":    claims.Role,
		"valid":   true,
	})
}

// handleRefresh 刷新 JWT Token
func handleRefresh(c *gin.Context) {
	var req RefreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, response.CodeBadReq, "请求参数错误: "+err.Error())
		return
	}

	// 解析并验证旧 Token
	claims, err := arhubJWT.ParseToken(req.Token, publicKeyPath)
	if err != nil {
		response.Error(c, http.StatusUnauthorized, response.CodeUnauthorized, "Token 无效或已过期: "+err.Error())
		return
	}

	// 检查 Token 是否在黑名单中
	if isTokenBlacklisted(c.Request.Context(), req.Token) {
		response.Error(c, http.StatusUnauthorized, response.CodeUnauthorized, "Token 已被注销，无法刷新")
		return
	}

	// 生成新 Token
	newToken, err := arhubJWT.GenerateToken(claims.UserID, claims.Role, privateKeyPath, time.Now().Add(24*time.Hour))
	if err != nil {
		response.Error(c, http.StatusInternalServerError, response.CodeInternal, "Token 刷新失败: "+err.Error())
		return
	}

	response.JSON(c, gin.H{
		"token":      newToken,
		"token_type": "Bearer",
		"expires_in": 86400,
	})
}

// handleLogout 用户注销（将 Token 加入黑名单）
func handleLogout(c *gin.Context) {
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		response.Error(c, http.StatusUnauthorized, response.CodeUnauthorized, "未提供 Authorization 请求头")
		return
	}

	tokenString := strings.TrimPrefix(authHeader, "Bearer ")
	if tokenString == authHeader {
		response.Error(c, http.StatusUnauthorized, response.CodeUnauthorized, "Authorization 请求头格式错误，应为 Bearer <token>")
		return
	}

	// 解析 Token 获取过期时间
	claims, err := arhubJWT.ParseToken(tokenString, publicKeyPath)
	if err != nil {
		// 即使 Token 解析失败也允许注销（可能是过期 Token）
		response.JSON(c, gin.H{"message": "注销成功"})
		return
	}

	// 检查 Token 是否已在黑名单中
	if isTokenBlacklisted(c.Request.Context(), tokenString) {
		response.JSON(c, gin.H{"message": "注销成功(Token 已在黑名单中）"})
		return
	}

	// 计算 Token 剩余有效期
	var ttl time.Duration
	if claims.ExpiresAt != nil {
		ttl = time.Until(claims.ExpiresAt.Time)
		if ttl <= 0 {
			ttl = time.Second
		}
	} else {
		ttl = 24 * time.Hour
	}

	// 生成 Token 哈希
	tokenHash := hashToken(tokenString)

	// 存入 Redis 黑名单（设置 TTL = Token 剩余有效期）
	key := fmt.Sprintf("token_blacklist:%s", tokenHash)
	if err := redisCli.Set(c.Request.Context(), key, "revoked", ttl).Err(); err != nil {
		// Redis 失败时降级到 PostgreSQL
		var expiresAt time.Time
		if claims.ExpiresAt != nil {
			expiresAt = claims.ExpiresAt.Time
		} else {
			expiresAt = time.Now().Add(24 * time.Hour)
		}
		_ = addTokenToDBBlacklist(tokenHash, expiresAt)
	} else {
		// 同时存入 PostgreSQL（持久化）
		var expiresAt time.Time
		if claims.ExpiresAt != nil {
			expiresAt = claims.ExpiresAt.Time
		} else {
			expiresAt = time.Now().Add(24 * time.Hour)
		}
		_ = addTokenToDBBlacklist(tokenHash, expiresAt)
	}

	response.JSON(c, gin.H{"message": "注销成功"})
}

// --- 辅助函数 ---

// hashToken 计算 Token 的 SHA256 哈希（用于黑名单标识）
func hashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

// isTokenBlacklisted 检查 Token 是否在黑名单中
// 优先查询 Redis，Redis 不存在时查询 PostgreSQL
func isTokenBlacklisted(ctx context.Context, token string) bool {
	tokenHash := hashToken(token)
	key := fmt.Sprintf("token_blacklist:%s", tokenHash)

	// 1. 先查 Redis
	val, err := redisCli.Get(ctx, key).Result()
	if err == nil && val == "revoked" {
		return true
	}

	// 2. Redis 未命中，查 PostgreSQL
	var exists bool
	_ = dbConn.QueryRow(
		"SELECT EXISTS(SELECT 1 FROM token_blacklist WHERE token_hash = $1)",
		tokenHash,
	).Scan(&exists)

	return exists
}

// addTokenToDBBlacklist 将 Token 哈希加入 PostgreSQL 黑名单
func addTokenToDBBlacklist(tokenHash string, expiresAt time.Time) error {
	_, err := dbConn.Exec(
		"INSERT INTO token_blacklist (jti, token_hash, expires_at) VALUES ($1, $2, $3) ON CONFLICT (jti) DO NOTHING",
		tokenHash, tokenHash, expiresAt,
	)
	return err
}

// getEnv 从环境变量读取字符串，若不存在返回默认值
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvInt 从环境变量读取整数，若不存在或解析失败返回默认值
func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		var result int
		if _, err := fmt.Sscanf(value, "%d", &result); err == nil {
			return result
		}
	}
	return defaultValue
}
