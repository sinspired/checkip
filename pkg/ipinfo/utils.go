package ipinfo

import (
	"encoding/json"
	"math/rand"
	"net"
	"net/netip"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/metacubex/mihomo/common/convert"
)

var (
	reIPv4 = regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}\b`)
	reIPv6 = regexp.MustCompile(`\b(?:[a-fA-F0-9]{1,4}:){2,7}[a-fA-F0-9]{1,4}\b`)
	// reIPv4      = regexp.MustCompile(`\b(?:(?:2(?:5[0-5]|[0-4]\d))|1?\d{1,2})(?:\.(?:(?:2(?:5[0-5]|[0-4]\d))|1?\d{1,2})){3}\b`)
	// reIPv6      = regexp.MustCompile(`\b([0-9a-fA-F]{1,4}:){1,7}[0-9a-fA-F]{0,4}\b`)
	// reCurrentIP = regexp.MustCompile(`(?i)Current\s*IP(?:\s*Address)?:\s*([0-9.]+)`)
	// reOriginIP  = regexp.MustCompile(`(?i)(?:X-Forwarded-For|CF-Connecting-IP|origin\s*ip)\s*[:=]\s*([0-9a-fA-F:.\[\]]+)`)
)

// IPData 存储 IP 地址信息到结构体
func CreateIPDataFromIP(ip string) *IPData {
	info := &IPData{}
	if parsedIP := net.ParseIP(ip); parsedIP != nil {
		if strings.Contains(ip, ":") {
			info.IPv6 = ip
		} else {
			info.IPv4 = ip
		}
	}
	return info
}

// getIPFromJSON 从 JSON 字符串中提取 IPv4 和 IPv6 地址
func getIPFromJSON(b []byte) (ipv4 string, ipv6 string) {
	var obj map[string]any
	if err := json.Unmarshal(b, &obj); err != nil {
		return "", ""
	}
	// 常见字段
	candidates := []string{"ip", "ipv4", "ipv6", "query", "origin", "your_ip", "ip_addr", "address"}
	for _, k := range candidates {
		if v, ok := obj[k]; ok {
			if s, ok := v.(string); ok && s != "" {
				if strings.Contains(s, ":") {
					if net.ParseIP(s) != nil {
						ipv6 = s
					}
				} else {
					if net.ParseIP(s) != nil {
						ipv4 = s
					}
				}
			}
			if ipv4 != "" && ipv6 != "" {
				return ipv4, ipv6
			}
		}
	}
	return ipv4, ipv6
}

// apiCommonHeaders 返回通用的 API 请求头
func apiCommonHeaders() map[string]string {
	return map[string]string{
		"Accept":          "application/json, text/plain, */*",
		"Accept-Language": "en-US,en;q=0.9",
		"User-Agent":      "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
		"Cache-Control":   "no-cache",
		"Pragma":          "no-cache",
	}
}

// Cloudflare CDN 请求头
func cfCommonHeaders() map[string]string {
	return map[string]string{
		"User-Agent":      convert.RandUserAgent(),
		"Accept-Language": "en-US,en;q=0.5",
		// "Accept":             "*/*",
		"Origin":             "https://www.cloudflare.com",
		"Sec-Ch-Ua":          "\"Chromium\";v=\"122\", \"Google Chrome\";v=\"122\", \"Not A(Brand\";v=\"99\"",
		"Sec-Ch-Ua-Mobile":   "?0",
		"Sec-Ch-Ua-Platform": "\"Windows\"",
		"Accept":             "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8",
		"Connection":         "close",
	}
}

// shuffle 随机打乱字符串切片
func shuffle(in []string) []string {
	if len(in) <= 1 {
		return slices.Clone(in)
	}
	out := slices.Clone(in)
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	r.Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
	return out
}

// ExtractIPRegex 使用正则匹配从文本中提取所有 IPv4 和 IPv6 地址
func ExtractIPRegex(text string) (ipv4s, ipv6s []string) {
	ipv4s = reIPv4.FindAllString(text, -1)
	ipv6s = reIPv6.FindAllString(text, -1)
	return
}

// ExtractIPStrings 通过扫描文本提取 IP 地址，结合文本扫描和过滤来检测潜在的 IPv4 和 IPv6 地址
func ExtractIPStrings(text string) (ipv4, ipv6 string) {
	isHex := func(b byte) bool { return (b >= '0' && b <= '9') || (b|0x20 >= 'a' && b|0x20 <= 'f') }
	isDigit := func(b byte) bool { return b >= '0' && b <= '9' }

	n := len(text)
	for i := 0; i < n && (ipv4 == "" || ipv6 == ""); {
		c := text[i]
		// 只从可能是 IP 的起始字符开始（数字/冒号/点）
		if !(isDigit(c) || c == ':' || c == '.') {
			i++
			continue
		}
		j := i
		dots, colons := 0, 0
		// valid := true

		for j < n {
			ch := text[j]
			if ch == '.' {
				dots++
			} else if ch == ':' {
				colons++
			} else if !(isHex(ch)) {
				break
			}
			j++
		}

		// 只处理包含至少一个分隔符的 token
		if dots > 0 && colons == 0 {
			// IPv4 快速筛：最多 3 个点、长度不超过 15、仅数字和点
			if dots <= 3 && (j-i) <= 15 {
				token := text[i:j] // 零拷贝子串
				if addr, err := netip.ParseAddr(token); err == nil && addr.Is4() && ipv4 == "" {
					ipv4 = addr.String()
				}
			}
		} else if colons > 0 && dots == 0 {
			// IPv6 快速筛：长度不超过 39（典型上限）
			if (j - i) <= 39 {
				token := text[i:j]
				if addr, err := netip.ParseAddr(token); err == nil && addr.Is6() && ipv6 == "" {
					ipv6 = addr.String()
				}
			}
		}

		// 跳过当前 token；+1 跳过分隔符，防止卡在同一位置
		if j == i {
			i++
		} else {
			i = j + 1
		}
	}
	return
}

// ExtractGeoIPStrings 在返回数据中查找 IP 和 国家代码,兼容不同格式的返回数据
func ExtractGeoIPStrings(bodyBytes []byte) (ip, countryCode string) {
    var data map[string]any
    if err := json.Unmarshal(bodyBytes, &data); err != nil {
        return "", ""
    }

    codeFields := []string{"countryCode", "country_code", "cc", "country"}
    ipFields := []string{"ip", "query"}

    // 通用提取函数
    extractField := func(m map[string]any, keys []string, validate func(string) bool) string {
        for _, key := range keys {
            if v, ok := m[key]; ok {
                if s, ok := v.(string); ok && validate(s) {
                    return s
                }
            }
        }
        return ""
    }

    // 优先从顶层获取
    ip = extractField(data, ipFields, func(s string) bool { return s != "" })
    countryCode = extractField(data, codeFields, func(s string) bool { return len(s) == 2 })

    // location 优先处理嵌套字段
    if loc, ok := data["location"].(map[string]any); ok {
        if ip == "" {
            ip = extractField(loc, ipFields, func(s string) bool { return s != "" })
        }
        if countryCode == "" {
            countryCode = extractField(loc, codeFields, func(s string) bool { return len(s) == 2 })
        }
    }

    // 其次尝试 datacenter 中查找
    if dc, ok := data["datacenter"].(map[string]any); ok && countryCode == "" {
        countryCode = extractField(dc, codeFields, func(s string) bool { return len(s) == 2 })
    }

    // 最后尝试 fallback
    if ip == "" && data["status"] == "success" {
        ip = extractField(data, ipFields, func(s string) bool { return s != "" })
    }

    return ip, countryCode
}

