// internal/config/config.go
package config

import (
	"os"
	"strconv"
	"time"
)

// Config 应用程序配置
type Config struct {
	// 服务器配置
	Addr string
	Port int

	// 数据库配置
	MaxMindDBPath string
	CFCIDRPath    string

	// HTTP 客户端配置
	HTTPTimeout time.Duration
	MaxRetries  int

	// 日志配置
	LogLevel string
}

// Load 从环境变量加载配置
func Load() *Config {
	cfg := &Config{
		Addr:          getEnv("ADDR", ":8099"),
		Port:          getEnvAsInt("PORT", 8099),
		MaxMindDBPath: getEnv("MAXMIND_DB_PATH", ""),
		CFCIDRPath:    getEnv("CF_CIDR_PATH", ""),
		HTTPTimeout:   getEnvAsDuration("HTTP_TIMEOUT", 10*time.Second),
		MaxRetries:    getEnvAsInt("MAX_RETRIES", 3),
		LogLevel:      getEnv("LOG_LEVEL", "info"),
	}

	return cfg
}

// getEnv 获取环境变量，如果不存在则返回默认值
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvAsInt 获取环境变量并转换为整数
func getEnvAsInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

// getEnvAsDuration 获取环境变量并转换为时间间隔
func getEnvAsDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}
