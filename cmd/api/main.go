// cmd/api/main.go

package main

import (
	"log"
	"net/http"
	"os"

	"github.com/sinspired/checkip/api"
	"github.com/sinspired/checkip/internal/assets"
	"github.com/sinspired/checkip/internal/checkip"
	"github.com/sinspired/checkip/internal/config"
)

func main() {
	// 自动创建 .env 文件（如不存在）
	envPath := ".env"
	if _, err := os.Stat(envPath); os.IsNotExist(err) {
		defaultEnv := `ADDR=:8099
	PORT=8099
	MAXMIND_DB_PATH=
	CF_CIDR_PATH=
	HTTP_TIMEOUT=10s
	MAX_RETRIES=3
	LOG_LEVEL=info
	`
		_ = os.WriteFile(envPath, []byte(defaultEnv), 0644)
	}
	// 加载配置
	cfg := config.Load()

	// 加载 Cloudflare CIDR 数据
	cidrs, err := assets.LoadCloudflareCIDRs(cfg.CFCIDRPath)
	if err != nil {
		log.Fatalf("load cloudflare cidrs failed: %v", err)
	}

	// 加载 MaxMind 数据库（自动处理路径为空时解压内置数据库）
	geo, err := assets.OpenGeoDB(cfg.MaxMindDBPath)
	if err != nil {
		log.Fatalf("open geo db failed: %v", err)
	}
	defer geo.Close()

	// 创建检查器
	ck := checkip.NewChecker(cidrs, geo)
	h := &api.Handler{Checker: ck}

	// 设置路由
	mux := http.NewServeMux()
	mux.Handle("/api/", h)

	// 启动服务器
	log.Printf("listening on %s ...", cfg.Addr)
	if err := http.ListenAndServe(cfg.Addr, mux); err != nil {
		log.Fatal(err)
	}
}
