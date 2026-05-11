// ============================================================
// internal/response/response.go
// 统一响应格式
// 职责：为所有 HTTP API 提供标准化的响应结构
// ============================================================

package response

import (
	"net/http"

	"github.com/gin-gonic/gin"
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
	CodeSuccess            = 0
	CodeBadReq             = 400
	CodeUnauthorized       = 401
	CodeForbidden          = 403
	CodeNotFound           = 404
	CodeRateLimited        = 429
	CodeInternal           = 500
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
