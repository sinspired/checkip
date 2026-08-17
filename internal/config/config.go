// Package config 配置
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

	// HTTP 客户端配置
	HTTPTimeout time.Duration
	MaxRetries  int

	// 日志配置
	LogLevel string

	// ISP类型配置
	ISPConfig ISPConfig
}

type ISPConfig struct {
	// ISPCheck 是否启用 ISP 检查
	ISPCheck bool

	// ISPTimeout 是 ISP 检查的超时时间，单位为秒
	ISPTimeout time.Duration

	// 以下四个渠道的 apikey 均为可选：留空则该渠道自动跳过，不参与轮询。
	// 建议至少配置 2 个渠道，通过轮询F叠加每日免费额度，并在某一渠道
	// 请求失败（超额 / 网络错误）时自动切换到下一个渠道。

	// ISPCheckAPIKeyIPAPI ipapi.is 的 apikey（https://ipapi.is）
	// 免费额度：注册后每天 1000 次
	ISPCheckAPIKeyIPAPI string
	// ISPCheckAPIKeyProxyCheck proxycheck.io 的 apikey（https://proxycheck.io）
	// 免费额度：每天 1000 次（另有约 5 倍的突发令牌可用）
	ISPCheckAPIKeyProxyCheck string

	// ISPCheckAPIKeyIPLocate iplocate.io 的 apikey（https://iplocate.io）
	// 免费额度：每天 1000 次，免费版与付费版字段完全一致
	ISPCheckAPIKeyIPLocate string

	// ISPCheckAPIKeyIPData ipdata.co 的 apikey（https://ipdata.co）
	// 免费额度：每天 1500 次（或每月 45000 次）
	ISPCheckAPIKeyIPData string
}

// Load 从环境变量加载配置
func Load() *Config {
	cfg := &Config{
		Addr:          getEnv("ADDR", ":8099"),
		Port:          getEnvAsInt("PORT", 8099),
		MaxMindDBPath: getEnv("MAXMIND_DB_PATH", ""),
		HTTPTimeout:   getEnvAsDuration("HTTP_TIMEOUT", 10*time.Second),
		MaxRetries:    getEnvAsInt("MAX_RETRIES", 3),
		LogLevel:      getEnv("LOG_LEVEL", "info"),
		ISPConfig:     LoadISPConfig(),
	}

	return cfg
}

// LoadISPConfig 从环境变量加载 ISP 检查相关配置
func LoadISPConfig() ISPConfig {
	return ISPConfig{
		ISPCheck:                 getEnvAsBool("ISP_CHECK", true),
		ISPTimeout:               getEnvAsDuration("ISP_TIMEOUT", 5*time.Second),
		ISPCheckAPIKeyIPAPI:      getEnv("ISP_APIKEY_IPAPI", ""),
		ISPCheckAPIKeyProxyCheck: getEnv("ISP_APIKEY_PROXYCHECK", ""),
		ISPCheckAPIKeyIPLocate:   getEnv("ISP_APIKEY_IPLOCATE", ""),
		ISPCheckAPIKeyIPData:     getEnv("ISP_APIKEY_IPDATA", ""),
	}
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

// getEnvAsBool 获取布尔环境变量
func getEnvAsBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		switch value {
		case "1", "true", "TRUE", "True", "yes", "YES":
			return true
		case "0", "false", "FALSE", "False", "no", "NO":
			return false
		}
	}
	return defaultValue
}
