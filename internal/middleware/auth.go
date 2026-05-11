// ============================================================
// internal/middleware/auth.go
// JWT 鉴权中间件
// 职责：验证请求中的 JWT Token，确保只有合法用户才能访问受保护接口
// ============================================================

package middleware

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"arhub/internal/response"
	arhubJWT "arhub/internal/pkg/jwt"
)

// AuthMiddleware 返回 JWT 鉴权中间件
// secret: JWT 签名密钥（用于验证 Token 签名）
func AuthMiddleware(secret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 从请求头中提取 Token
		// 格式：Authorization: Bearer <token>
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

		// 将用户信息存入请求上下文，供后续处理使用
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
