// ============================================================
// cmd/api-gateway/main.go
// API 网关服务入口
// 职责：接收所有 HTTP 请求，进行路由分发、鉴权、限流，然后转发到后端服务
// ============================================================

package main

import (
	"fmt"
	"log"
	"net/http"
)

// main 是 API 网关的入口函数
// 1. 初始化配置（从 Nacos 拉取）
// 2. 初始化中间件（鉴权、限流、日志）
// 3. 注册路由规则
// 4. 启动 HTTP 服务
func main() {
	// 网关监听的端口号
	port := ":8080"

	// 初始化路由
	mux := http.NewServeMux()

	// 注册健康检查端点
	// 用途：K8s 等容器编排平台通过此接口判断服务是否健康
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// 注册 API 路由
	// 格式：/api/v1/<服务名>/<接口>
	// 示例：/api/v1/market/price → 转发到行情聚合服务
	mux.HandleFunc("/api/v1/", handleAPI)

	fmt.Printf("🚀 API Gateway 启动成功，监听端口 %s\n", port)

	// 启动 HTTP 服务
	// 注意：生产环境建议使用 gin 或 echo 框架，此处为骨架示例
	if err := http.ListenAndServe(port, mux); err != nil {
		log.Fatalf("API Gateway 启动失败: %v", err)
	}
}

// handleAPI 处理所有 API 请求
// 流程：1. 鉴权校验 → 2. 限流检查 → 3. 路由转发 → 4. 返回响应
func handleAPI(w http.ResponseWriter, r *http.Request) {
	// TODO: 实现完整的路由转发逻辑
	// 1. 从请求路径中提取服务名和目标路径
	// 2. 根据服务名查找对应的后端服务地址
	// 3. 将请求转发到后端服务
	// 4. 将后端响应返回给客户端

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("API Gateway is working"))
}
