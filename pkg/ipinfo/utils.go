package ipinfo

import (
	"encoding/json"
	"net"
	"strings"
	"time"
)

// shuffleStringSlice 对字符串切片进行乱序
func shuffleStringSlice(src []string) []string {
	dst := make([]string, len(src))
	copy(dst, src)
	for i := len(dst) - 1; i > 0; i-- {
		j := int(time.Now().UnixNano() % int64(i+1))
		dst[i], dst[j] = dst[j], dst[i]
	}
	return dst
}

// getIPFromJSON 尝试从JSON响应中解析出IP地址
func getIPFromJSON(bodyBytes []byte) (ipv4, ipv6 string) {
	var data map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &data); err != nil {
		return "", ""
	}

	candidates := []string{}
	for _, k := range []string{"ip", "query", "origin"} {
		if v, ok := data[k].(string); ok {
			candidates = append(candidates, v)
		}
	}
	if query, ok := data["query"].(map[string]interface{}); ok {
		if ip, ok := query["ip"].(string); ok {
			candidates = append(candidates, ip)
		}
	}
	for _, ip := range candidates {
		if strings.Contains(ip, ":") && net.ParseIP(ip) != nil {
			return "", ip
		}
		if net.ParseIP(ip) != nil {
			return ip, ""
		}
	}
	return "", ""
}
