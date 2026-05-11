// ============================================================
// internal/pkg/db/postgres.go
// PostgreSQL 数据库客户端封装
// 职责：提供数据库连接池初始化、查询封装、事务管理
// ============================================================

package db

import (
	"database/sql"
	"fmt"
	"log"
	"time"

	_ "github.com/lib/pq" // PostgreSQL 驱动
)

// Config 定义 PostgreSQL 连接配置
// 字段说明：
//   Host     - 数据库主机地址
//   Port     - 数据库端口号
//   User     - 数据库用户名
//   Password - 数据库密码
//   DBName   - 数据库名称
//   SSLMode  - SSL 连接模式（disable/require）
type Config struct {
	Host     string
	Port     int
	User     string
	Password string
	DBName   string
	SSLMode  string
}

// DefaultConfig 返回默认配置（本地开发环境）
func DefaultConfig() Config {
	return Config{
		Host:     "localhost",
		Port:     5432,
		User:     "arhub",
		Password: "arhub123",
		DBName:   "arhub",
		SSLMode:  "disable",
	}
}

// NewDB 初始化 PostgreSQL 连接池
// 参数：
//   cfg - 数据库配置
// 返回：
//   *sql.DB - 数据库连接池实例
//   error   - 初始化过程中的错误
// 注意：调用方需要在使用完毕后调用 db.Close()
func NewDB(cfg Config) (*sql.DB, error) {
	// 构建连接字符串
	// 格式：host=xxx port=xxx user=xxx password=xxx dbname=xxx sslmode=xxx
	dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.DBName, cfg.SSLMode)

	// 打开数据库连接
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("打开数据库连接失败: %w", err)
	}

	// 配置连接池参数
	// MaxOpenConns: 最大活跃连接数（并发请求多时需要调大）
	// MaxIdleConns: 最大空闲连接数（保持一定数量可减少连接开销）
	// ConnMaxLifetime: 连接最大生命周期（防止连接长时间不释放）
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	// 验证连接是否可用
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("数据库连接验证失败: %w", err)
	}

	log.Println("✅ PostgreSQL 连接池初始化成功")
	return db, nil
}

// HealthCheck 检查数据库连接是否健康
// 用途：健康检查端点调用，判断服务是否可用
func HealthCheck(db *sql.DB) error {
	return db.Ping()
}
