// cmd/api/main.go
package main

import (
	"log"
	"net/http"

	"github.com/oschwald/maxminddb-golang/v2"
	"github.com/sinspired/checkip/api"
	"github.com/sinspired/checkip/internal/assets"
	"github.com/sinspired/checkip/internal/checkip"
	"github.com/sinspired/checkip/internal/config"
)

func main() {
	// 加载配置
	cfg := config.Load()

	// 加载 Cloudflare CIDR 数据
	cidrs, err := assets.LoadCloudflareCIDRs(cfg.CFCIDRPath)
	if err != nil {
		log.Fatalf("load cloudflare cidrs failed: %v", err)
	}

	// 加载 MaxMind 数据库
	var geo *maxminddb.Reader
	if cfg.MaxMindDBPath != "" {
		geo, err = assets.OpenGeoDB(cfg.MaxMindDBPath)
		if err != nil {
			log.Fatalf("open geo db failed: %v", err)
		}
		defer geo.Close()
	}

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
