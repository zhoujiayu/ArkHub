// ============================================================
// internal/middleware/auth.go
// JWT 鉴权中间件
// 职责：验证请求中的 JWT Token，确保只有合法用户才能访问受保护接口
// ============================================================

package middleware

import (
	"net/http"
	"strings"
)

// Claims 定义 JWT 载荷结构
// 包含用户身份信息，用于鉴权和权限控制
// 字段说明：
//   UserID - 用户唯一标识
//   Role   - 用户角色（如 admin、user、guest）
//   Exp    - 过期时间（Unix 时间戳）
type Claims struct {
	UserID string `json:"user_id"` // 用户唯一标识
	Role   string `json:"role"`      // 用户角色（admin/user/guest）
	Exp    int64  `json:"exp"`       // 过期时间（Unix 时间戳）
}

// AuthMiddleware 返回 JWT 鉴权中间件
// 使用方式：
//   http.Handle("/api/protected", AuthMiddleware("your-secret-key")(handler))
//
// 参数：
//   secret - JWT 签名密钥（用于验证 Token 签名）
// 返回：
//   func(http.Handler) http.Handler - 包装后的中间件
func AuthMiddleware(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// 从请求头中提取 Token
			// 格式：Authorization: Bearer <token>
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				// 未提供 Token，返回 401 Unauthorized
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte("未提供认证信息"))
				return
			}

			// 提取 Token 值（去除 "Bearer " 前缀）
			tokenString := strings.TrimPrefix(authHeader, "Bearer ")

			// TODO: 实现 JWT 验证逻辑
			// 1. 解析 Token 字符串
			// 2. 验证签名是否有效
			// 3. 检查 Token 是否过期
			// 4. 提取用户身份信息

			_ = tokenString
			_ = secret

			// TODO: 将用户信息存入请求上下文，供后续处理使用
			// ctx := context.WithValue(r.Context(), "user_id", claims.UserID)
			// r = r.WithContext(ctx)

			// Token 验证通过，继续处理请求
			next.ServeHTTP(w, r)
		})
	}
}

// GenerateToken 生成 JWT Token（工具函数）
// 参数：
//   userID - 用户唯一标识
//   role   - 用户角色
//   secret - 签名密钥
// 返回：
//   string - 生成的 Token 字符串
//   error  - 生成过程中的错误
func GenerateToken(userID, role, secret string) (string, error) {
	// TODO: 使用 jwt-go 库实现 Token 生成
	// 1. 创建 Claims
	// 2. 使用 HMAC SHA256 签名
	// 3. 返回 Token 字符串
	_ = userID
	_ = role
	_ = secret
	return "mock-token", nil
}

// ParseToken 解析 JWT Token（工具函数）
// 参数：
//   tokenString - Token 字符串
//   secret      - 签名密钥
// 返回：
//   *Claims - 解析出的用户 claims
//   error   - 解析过程中的错误
func ParseToken(tokenString, secret string) (*Claims, error) {
	// TODO: 使用 jwt-go 库实现 Token 解析
	_ = tokenString
	_ = secret
	return &Claims{UserID: "mock-user", Role: "user"}, nil
}
