// api/checkip.go
package api

import (
	"encoding/json"
	"net"
	"net/http"
	"strings"

	"github.com/sinspired/checkip/internal/checkip"
)

type Handler struct {
	Checker *checkip.Checker
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	// 解析路径
	path := strings.TrimPrefix(r.URL.Path, "/api")
	path = strings.TrimPrefix(path, "/")

	// 处理不同的路由
	switch {
	case path == "" || path == "ip":
		// /api 或 /api/ip - 获取当前 IP
		if path == "" {
			// /api - 获取当前 IP 的完整信息
			res, err := h.Checker.GetCurrentIPInfo()
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			json.NewEncoder(w).Encode(res)
		} else {
			// /api/ip - 仅返回 IP 地址
			ip, err := h.Checker.GetCurrentIP()
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			json.NewEncoder(w).Encode(map[string]string{"ip": ip})
		}
	default:
		// /api?ip=x.x.x.x 或 /api/x.x.x.x - 检查指定 IP
		var targetIP string

		// 首先检查查询参数
		if queryIP := r.URL.Query().Get("ip"); queryIP != "" {
			targetIP = queryIP
		} else {
			// 如果没有查询参数，尝试从路径中解析 IP
			targetIP = path
		}

		if targetIP == "" {
			http.Error(w, "missing ip parameter", http.StatusBadRequest)
			return
		}

		// 验证 IP 格式
		if net.ParseIP(targetIP) == nil {
			http.Error(w, "invalid IP address", http.StatusBadRequest)
			return
		}

		res, err := h.Checker.Check(targetIP)
		if err != nil {
			// 所有错误都返回 400 Bad Request，避免 500
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if res == nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}

		json.NewEncoder(w).Encode(res)
	}
}
