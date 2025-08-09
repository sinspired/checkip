// internal/checkip/checkip.go
package ipinfo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"os"
	"regexp"
	"strings"

	"github.com/metacubex/mihomo/common/convert"
	"github.com/oschwald/maxminddb-golang/v2"
	"github.com/sinspired/checkip/internal/config"
	"github.com/sinspired/checkip/internal/data"
)

var (
	// 匹配 IPv4 地址格式。
	reIPv4 = regexp.MustCompile(`\b((25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\b`)

	// 匹配 IPv6 地址格式。
	reIPv6 = regexp.MustCompile(`([0-9a-fA-F]{1,4}:){7,7}[0-9a-fA-F]{1,4}|([0-9a-fA-F]{1,4}:){1,7}:|:((:[0-9a-fA-F]{1,4}){1,7}|:)|fe80:(:[0-9a-fA-F]{0,4}){0,4}%[0-9a-zA-Z]{1,}|([0-9a-fA-F]{1,4}:){1,6}:[0-9a-fA-F]{1,4}`)

	// 匹配 "Current IP Address: x.x.x.x" 格式
	reCurrentIP = regexp.MustCompile(`Current IP Address: ([\d\.]+)`)

	// 匹配 httpbin.org 返回的 JSON 格式中的 origin 字段。
	reOriginIP = regexp.MustCompile(`"origin"\s*:\s*"([^"]+)"`)
)

// 请求头，模拟正常访问
func apiCommonHeaders() map[string]string {
	return map[string]string{
		"User-Agent":      convert.RandUserAgent(),
		"Accept-Language": "en-US,en;q=0.5",
		// "Accept":             "*/*",
		"Sec-Ch-Ua":          "\"Chromium\";v=\"122\", \"Google Chrome\";v=\"122\", \"Not A(Brand\";v=\"99\"",
		"Sec-Ch-Ua-Mobile":   "?0",
		"Sec-Ch-Ua-Platform": "\"Windows\"",
		"Accept":             "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8",
		"Connection":         "close",
	}
}

// GetIPInfoData
func GetIPInfoData(ctx context.Context, httpClient *http.Client, db *maxminddb.Reader) (info IPData, err error) {
	// 1. 优先使用 ipAPI 获取 IP 地址
	shuffledAPIs := shuffleStringSlice(config.IP_APIS)
	for _, url := range shuffledAPIs {
		select {
		case <-ctx.Done():
			return IPData{}, ctx.Err()
		default:
		}

		var temp IPData
		err = temp.GetExitIP(ctx, httpClient, url)
		if err == nil && (temp.IPv4 != "" || temp.IPv6 != "") {
			slog.Debug(fmt.Sprintf("%s : IPv4=%s IPv6=%s", url, temp.IPv4, temp.IPv6))
			info = temp
			break
		}
		slog.Debug(fmt.Sprintf("从 ipAPI 获取出口 IP 失败: %s, err: %v", url, err))
	}

	// 2. 使用 MaxMind 数据库查询国家代码
	if info.IPv4 != "" || info.IPv6 != "" {
		if dbErr := info.GetMaxMindData(db); dbErr == nil && info.CountryCode != "" {
			slog.Debug(fmt.Sprintf("MaxMind 获取到 %s 的国家代码: %s", info.IPv4, info.CountryCode))
			return info, nil
		} else {
			slog.Debug(fmt.Sprintf("MaxMind 查询失败: %v", dbErr))
		}
	} else {
		slog.Debug("所有 ipAPI 均未获取到 IP，将尝试 geoAPI（有限额）")
	}

	// 3. 使用 geoAPI 补充
	shuffledGeoAPIs := shuffleStringSlice(config.GEOIP_APIS)
	for _, url := range shuffledGeoAPIs {
		select {
		case <-ctx.Done():
			return IPData{}, ctx.Err()
		default:
		}

		temp, geoErr := FetchGeoIPInfo(httpClient, url)
		if geoErr == nil && temp.CountryCode != "" {
			if temp.CountryCode == "CN" && os.Getenv("SUBS-CHECK-CALL") != "" {
				continue
			}
			slog.Debug(fmt.Sprintf("%s : %s", url, temp.CountryCode))
			info = temp
			return info, nil
		}
		slog.Debug(fmt.Sprintf("geoAPI 查询失败: %s, err: %v", url, geoErr))
	}

	if info.IPv4 == "" && info.IPv6 == "" {
		return IPData{}, errors.New("所有 ipAPI 均未获取有效 IP，可能是网络断开")
	}

	return info, errors.New("未能获取地理位置信息（MaxMind 和 geoAPI 均失败）")
}

// CheckCfRelayIP 检查是否为 cloudflare 中转ip
func CheckCfRelayIP(ctx context.Context, httpClient *http.Client, info *IPData) (isCDN bool, countryCode string, cfRelayLocation string) {
	cfRelayLoc, cfRelayIP := FetchCFProxyWithContext(ctx, httpClient)

	// 若原始 IP 是 CDN
	if info.IsCDN {
		if cfRelayLoc == "" || cfRelayIP == "" {
			return true, info.CountryCode, ""
		}

		// 解析 cfRelayIP 类型
		var ipv4, ipv6 string
		if net.ParseIP(cfRelayIP) != nil {
			if strings.Contains(cfRelayIP, ":") {
				ipv6 = cfRelayIP
			} else {
				ipv4 = cfRelayIP
			}
		}

		// 使用统一结构检测是否为 CDN
		relayInfo := IPData{
			IPv4: ipv4,
			IPv6: ipv6,
		}
		relayInfo.CheckCDN()

		// 若 relay IP 不是 CDN 且来源不同，保留位置信息
		if !relayInfo.IsCDN && info.CountryCode != "" && info.IPv4 != ipv4 {
			return true, info.CountryCode, cfRelayLoc
		}
	}

	return false, info.CountryCode, ""
}

// GetExitIP 获取出口 IP 地址
func (info *IPData) GetExitIP(ctx context.Context, client *http.Client, url string) error {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		slog.Debug(fmt.Sprintf("GetExitIP 创建请求失败: %s, err: %v", url, err))
		return err
	}

	for key, value := range apiCommonHeaders() {
		req.Header.Set(key, value)
	}

	resp, err := client.Do(req)
	if err != nil {
		slog.Debug(fmt.Sprintf("GetExitIP 请求失败: %s, err: %v", url, err))
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		slog.Debug(fmt.Sprintf("GetExitIP 非200状态码: %s, code: %d", url, resp.StatusCode))
		return fmt.Errorf("status: %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		slog.Debug(fmt.Sprintf("GetExitIP 读取响应体失败: %s, err: %v", url, err))
		return err
	}

	body := strings.TrimSpace(string(bodyBytes))
	contentType := resp.Header.Get("Content-Type")

	if strings.Contains(contentType, "application/json") {
		info.IPv4, info.IPv6 = getIPFromJSON(bodyBytes)
	} else {
		info.IPv4 = reIPv4.FindString(body)
		info.IPv6 = reIPv6.FindString(body)
	}

	if info.IPv4 == "" {
		if m := reCurrentIP.FindStringSubmatch(body); len(m) > 1 {
			info.IPv4 = m[1]
		}
	}

	if m := reOriginIP.FindStringSubmatch(body); len(m) > 1 {
		if strings.Contains(m[1], ":") {
			info.IPv6 = m[1]
		} else {
			info.IPv4 = m[1]
		}
	}

	if net.ParseIP(info.IPv4) == nil {
		info.IPv4 = ""
	}
	if net.ParseIP(info.IPv6) == nil {
		info.IPv6 = ""
	}

	if info.IPv4 != "" || info.IPv6 != "" {
		info.CheckCDN()
	}

	return nil
}

// CheckCDN 检查是否属于 CDN ip
func (info *IPData) CheckCDN() bool {
	cfCdnIPRanges := data.GetCfCdnIPRanges()
	if cfCdnIPRanges == nil {
		slog.Debug("Cloudflare CDN IP ranges not loaded")
		info.IsCDN = false
		return false
	}

	check := func(ipStr string, nets []*net.IPNet) bool {
		if ipStr == "" {
			return false
		}
		parsed := net.ParseIP(ipStr)
		if parsed == nil {
			return false
		}
		for _, n := range nets {
			if n != nil && n.Contains(parsed) {
				slog.Debug(fmt.Sprintf("IP %s 属于 Cloudflare CDN IP范围: %s", ipStr, n.String()))
				return true
			}
		}
		return false
	}

	if check(info.IPv4, cfCdnIPRanges["ipv4"]) || check(info.IPv6, cfCdnIPRanges["ipv6"]) {
		info.IsCDN = true
		return true
	}

	info.IsCDN = false
	return false
}

// GetMaxMindData 获取国家码（MaxMind）
func (info *IPData) GetMaxMindData(db *maxminddb.Reader) error {
	if db == nil {
		return fmt.Errorf("MaxMind 数据库未初始化")
	}

	ip := info.IPv4
	if ip == "" {
		ip = info.IPv6
	}
	ipAddr, err := netip.ParseAddr(ip)
	if err != nil {
		return fmt.Errorf("无效的 IP 地址: %s", ip)
	}

	var countryCode string
	err = db.Lookup(ipAddr).DecodePath(&countryCode, "country", "iso_code")
	if err != nil {
		slog.Debug(fmt.Sprintf("MaxMind 数据库查询失败: %s, err: %v", ip, err))
		return err
	}

	if countryCode != "" {
		info.CountryCode = strings.ToUpper(countryCode)
	}

	return nil
}

// FetchGeoIPInfo 使用 geoQueryAPI 获取 ip 和 国家代码
func FetchGeoIPInfo(httpClient *http.Client, geoQueryAPI string) (IPData, error) {
	req, err := http.NewRequest("GET", geoQueryAPI, nil)
	if err != nil {
		return IPData{}, err
	}
	for key, value := range apiCommonHeaders() {
		req.Header.Set(key, value)
	}
	// 特定适配
	if strings.Contains(geoQueryAPI, "checkip.info") {
		req.Header.Set("User-Agent", "PostmanRuntime/7.32.3")
		req.Header.Set("Accept", "*/*")
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return IPData{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return IPData{}, fmt.Errorf("status code: %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return IPData{}, err
	}

	// 在返回数据中查找 IP 和 国家代码,兼容不同格式的返回数据
	ip, countryCode := FindGeoIPStrings(bodyBytes)

	// 判断 IP 类型
	var ipv4, ipv6 string
	if net.ParseIP(ip) != nil {
		if strings.Contains(ip, ":") {
			ipv6 = ip
		} else {
			ipv4 = ip
		}
	}

	info := IPData{
		IPv4:        ipv4,
		IPv6:        ipv6,
		CountryCode: strings.ToUpper(countryCode),
	}
	info.CheckCDN()

	return info, nil
}

// FindGeoIPStrings 在返回数据中查找 IP 和 国家代码,兼容不同格式的返回数据
func FindGeoIPStrings(bodyBytes []byte) (ip string, countryCode string) {
	var data map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &data); err != nil {
		return "", ""
	}
	// 查询字段
	codeFields := []string{"countryCode", "country_code", "cc", "country"}
	ipFields := []string{"ip", "query"}

	// 通用解析函数
	extractField := func(obj map[string]interface{}, keys []string, validate func(string) bool) string {
		for _, key := range keys {
			if v, ok := obj[key]; ok {
				if s, ok := v.(string); ok && validate(s) {
					return s
				}
			}
		}
		return ""
	}

	// IP 和国家代码解析
	ip = extractField(data, ipFields, func(s string) bool { return s != "" })
	countryCode = extractField(data, codeFields, func(s string) bool {
		if len(s) == 2 {
			return true
		}
		if s == "United States" {
			countryCode = "US"
			return true
		}
		return false
	})

	// location 嵌套结构
	if locObj, ok := data["location"].(map[string]interface{}); ok {
		if ip == "" {
			ip = extractField(locObj, ipFields, func(s string) bool { return s != "" })
		}
		if countryCode == "" {
			countryCode = extractField(locObj, codeFields, func(s string) bool { return len(s) == 2 })
		}
	}

	// datacenter 嵌套结构
	if dcObj, ok := data["datacenter"].(map[string]interface{}); ok && countryCode == "" {
		countryCode = extractField(dcObj, codeFields, func(s string) bool { return len(s) == 2 })
	}

	// fallback: status=success 且 query 字段
	if ip == "" {
		if v, ok := data["status"]; ok && v == "success" {
			if q, ok := data["query"].(string); ok {
				ip = q
			}
		}
	}

	return ip, countryCode
}
