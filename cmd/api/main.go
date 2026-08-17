// cmd/api/main.go

package main

import (
	"fmt"
	"github.com/joho/godotenv"
	"log"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/sinspired/checkip/internal/config"
	"github.com/sinspired/checkip/internal/data"
	"github.com/sinspired/checkip/internal/resolver"
	"github.com/sinspired/checkip/internal/server"
)

const (
	envFile        = ".env"
	dbFileName     = "GeoLite2-City.mmdb"
	updateInterval = 7 * 24 * time.Hour // 每周更新一次
)

func ensureEnvFile() {
	if _, err := os.Stat(envFile); os.IsNotExist(err) {
		defaultEnv := `# 服务器配置
ADDR=:8099
PORT=8099

# MaxMind 数据库路径（留空则使用内置库并自动更新）
MAXMIND_DB_PATH=

# Cloudflare CIDR 数据路径（可选）
CF_CIDR_PATH=

# HTTP 客户端配置
HTTP_TIMEOUT=10s
MAX_RETRIES=3

# 日志配置
LOG_LEVEL=info

# GitHub 代理（可选）
GITHUB_PROXY=https://ghproxy.net/

# ISP 检查开关
ISP_CHECK=true

# ISP 渠道 apikey（留空则跳过该渠道）
# ipapi.is 免费额度：每天 1000 次
ISP_APIKEY_IPAPI=

# proxycheck.io 免费额度：每天 1000 次
ISP_APIKEY_PROXYCHECK=

# iplocate.io 免费额度：每天 1000 次
ISP_APIKEY_IPLOCATE=

# ipdata.co 免费额度：每天 1500 次
ISP_APIKEY_IPDATA=
`
		_ = os.WriteFile(envFile, []byte(defaultEnv), 0644)
	}
}

func UpdateCronJob(dbPath string) {
	if dbPath == "" {
		return
	}
	go func() {
		for {
			now := time.Now()
			// 计算下一个周日的 00:00（本地时区）
			daysUntil := (int(time.Sunday) - int(now.Weekday()) + 7) % 7
			target := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, daysUntil)
			if !target.After(now) {
				// 如果计算得到的时刻不在将来，向后推一周
				target = target.AddDate(0, 0, 7)
			}
			wait := time.Until(target)
			// 等待到计划时间
			time.Sleep(wait)

			// 在计划时间执行更新
			if err := data.UpdateGeoLite2DB(dbPath); err != nil {
				slog.Warn("MaxMind 更新失败", "error", err)
			} else {
				slog.Info("MaxMind 数据库已更新", "path", dbPath)
			}
			// 循环继续，下一次会重新计算（处理 DST 及其它时间变化）
		}
	}()
}

func main() {
	// 自动创建 .env 文件
	ensureEnvFile()

	// 加载 .env 到环境变量
	if err := godotenv.Load(".env"); err != nil {
		log.Printf("未找到 .env 文件，使用系统环境变量: %v", err)
	}

	// 加载配置
	cfg := config.Load()

	// 加载 Cloudflare CIDR 数据
	cidrs := data.GetCfCdnIPRanges()

	// 仅当未指定外部路径且文件存在时才检查更新
	if cfg.MaxMindDBPath == "" {
		dataPath := data.ResolveDataPath()
		dbPath := filepath.Join(dataPath, dbFileName)
		// 添加一个定时更新任务
		UpdateCronJob(dbPath)
		// 如果文件存在,检查是否过期
		if fi, err := os.Stat(dbPath); err == nil {
			if time.Since(fi.ModTime()) > updateInterval {
				// 立即更新数据库
				_ = data.UpdateGeoLite2DB(dbPath)
			}
		}
	}

	// 打开 MaxMind 数据库（为空时自动解压内置库）
	geo, err := data.OpenMaxMindDB(cfg.MaxMindDBPath)
	if err != nil {
		log.Fatalf("打开 MaxMind 数据库失败: %v", err)
	}
	defer geo.Close()

	// 创建检查器
	ck := resolver.NewResolver(cidrs, geo,cfg)
	h := &server.Handler{Resolver: ck}

	// 设置路由
	mux := http.NewServeMux()
	mux.Handle("/api/", h)

	// 启动服务器
	slog.Info(fmt.Sprintf("listening on http://0.0.0.0%s/api ...", cfg.Addr))
	if err := http.ListenAndServe(cfg.Addr, mux); err != nil {
		log.Fatal(err)
	}
}
