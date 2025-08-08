// internal/checkip/checkip.go
package checkip

import (
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
	"time"

	"github.com/oschwald/maxminddb-golang/v2"
	"github.com/sinspired/checkip/internal/assets"
)

type IPData struct {
	IPv4  string
	IPv6  string
	IsCDN bool
}

type GeoIPData struct {
	CountryCode string
	IPData
}

var ipAPIs = []string{
	"http://checkip.amazonaws.com",
	"http://whatismyip.akamai.com",
	"https://checkip.global.api.aws",
	"https://4.ident.me/ip",
	"https://4.tnedi.me/ip",
	"http://ifconfig.me/ip",
	"https://6.ident.me/ip",
	"https://ipv6.seeip.org/ip",
	"https://api.ipapi.is/ip",
	"https://ipinfo.io/ip",
	"https://ip4.me/api/",
	"https://checkip.info/ip",
	"https://checkip.dns.he.net/",
	"https://freedns.afraid.org/dynamic/check.php",
	"https://httpbin.org/ip",
	"http://checkip.dyndns.com/",
}

var geoAPIs = []string{
	"https://4.ident.me/json",
	"https://4.tnedi.me/json",
	"https://ident.me/json",
	"https://tnedi.me/json",
	"https://api.seeip.org/geoip",
	"https://checkip.info/json",
	"http://ip-api.com/json",
	"https://api.ipapi.is",
	// "https://ipinfo.io/json",
}

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

// GetProxyCountryMixed 获取国家信息和出口ip地址，并判断是否为 Cloudflare 代理。
func GetProxyCountryMixed(httpClient *http.Client, db *maxminddb.Reader) (loc string, ip string, countryCode_tag string, err error) {
	geoIPData, err := GetGeoIPData(httpClient, db)
	if err != nil {
		return "", "", "", err
	}
	ip = geoIPData.IPv4
	if ip == "" {
		ip = geoIPData.IPv6
	}
	isCfRealyIP, Exitloc, cf_relay_loc := checkCfRelayIP(httpClient, &geoIPData)

	if isCfRealyIP {
		if cf_relay_loc == "" {
			countryCode_tag = Exitloc + "⁻¹"
		} else if Exitloc == cf_relay_loc {
			countryCode_tag = Exitloc + "¹⁺"
		} else {
			countryCode_tag = Exitloc + "¹" + "-" + cf_relay_loc + "⁰"
		}
	} else {
		countryCode_tag = Exitloc + "²"
	}
	return Exitloc, ip, countryCode_tag, nil
}

// GetGeoIPData 获取地理位置信息，优先使用 geoAPI 获取完整数据，失败则使用 ipAPI 获取 IP 后查询 MaxMind 数据库。
func GetGeoIPData(httpClient *http.Client, db *maxminddb.Reader) (geoData GeoIPData, err error) {
	// 1. 优先尝试从 geoAPI 获取完整数据
	for _, url := range geoAPIs {
		geoData, err = GetGeoIP(httpClient, url)
		if err == nil && geoData.CountryCode != "" {
			// 在测试环境中，即使返回 CN 也接受结果
			if geoData.CountryCode == "CN" && os.Getenv("TESTING") != "1" {
				continue
			}
			slog.Debug(fmt.Sprintf("%s : %s", url, geoData.CountryCode))
			return geoData, nil
		}
		slog.Debug(fmt.Sprintf("从 geoAPI 获取地理位置信息失败: %s, err: %v", url, err))
	}

	// 2. 如果 geoAPI 失败或没有结果，再使用 ipAPI 获取纯 IP，单独查询 GEO 信息
	var ipData IPData
	var foundIP bool
	for _, url := range ipAPIs {
		ipData, err = getExitIP(httpClient, url)
		if err == nil && (ipData.IPv4 != "" || ipData.IPv6 != "") {
			foundIP = true
			break // 成功获取 IP，跳出循环
		}
		slog.Debug(fmt.Sprintf("从 ipAPI 获取出口 IP 失败: %s, err: %v", url, err))
	}
	if !foundIP {
		return GeoIPData{}, errors.New("所有 ipAPI 均未能获取到有效的IP地址,疑似网络断开！")
	}

	geoData = GeoIPData{
		IPData: ipData,
	}

	// 3. 使用获取到的 IP 和本地 MaxMind 数据库查询国家代码
	ip := geoData.IPv4
	if ip == "" {
		ip = geoData.IPv6
	}

	countryCode, err := GetMaxMindData(db, ip)
	if err != nil {
		slog.Debug(fmt.Sprintf("获取 MaxMind 数据库信息失败: %s, err: %v", ip, err))
		return geoData, err
	}
	if countryCode == "" {
		slog.Debug(fmt.Sprintf("MaxMind 数据库未能找到国家代码: %s", ip))
		return geoData, errors.New("MaxMind 数据库未能找到国家代码")
	}

	geoData.CountryCode = countryCode

	return geoData, nil
}

func checkCfRelayIP(httpClient *http.Client, geoIPData *GeoIPData) (bool, string, string) {
	cf_relay_loc, cf_relay_ip := FetchCFProxy(httpClient)

	// 判断 geoIPData 是否为 CDN 并返回相应的结果
	if geoIPData.IsCDN {
		if cf_relay_loc == "" || cf_relay_ip == "" {
			return true, geoIPData.CountryCode, ""
		}

		// 判断IP类型
		var ipv4, ipv6 string
		if net.ParseIP(cf_relay_ip) != nil {
			if strings.Contains(cf_relay_ip, ":") {
				ipv6 = cf_relay_ip
			} else {
				ipv4 = cf_relay_ip
			}
		}

		// 判断是否为Cloudflare CDN
		IsCfCDN := checkCDN(&IPData{IPv4: ipv4, IPv6: ipv6})

		// 如果不为Cloudflare CDN，且符合条件，则返回
		if !IsCfCDN && geoIPData.CountryCode != "" && geoIPData.IPv4 != ipv4 && geoIPData.IsCDN {
			return true, geoIPData.CountryCode, cf_relay_loc
		}
	}

	return false, geoIPData.CountryCode, ""
}

func GetGeoIP(httpClient *http.Client, url string) (geoData GeoIPData, err error) {
	resp, err := httpClient.Get(url)
	if err != nil || resp.StatusCode != http.StatusOK {
		return GeoIPData{}, err
	}
	defer resp.Body.Close()
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return GeoIPData{}, err
	}

	var data map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &data); err != nil {
		return GeoIPData{}, err
	}

	// 检查429或error字段
	if status, ok := data["status"]; ok {
		switch v := status.(type) {
		case float64:
			if int(v) == 429 {
				return GeoIPData{}, nil
			}
		case string:
			if v == "429" {
				return GeoIPData{}, nil
			}
		}
	}
	if _, ok := data["error"]; ok {
		return GeoIPData{}, nil
	}

	codeFields := []string{"country_code", "countryCode", "cc", "country"}
	ipFields := []string{"ip", "query"}

	var ip string
	var countryCode string

	// 顶层优先
	for _, ipKey := range ipFields {
		if v, ok := data[ipKey]; ok {
			if s, ok := v.(string); ok && s != "" {
				ip = s
				break
			}
		}
	}
	for _, codeKey := range codeFields {
		if v, ok := data[codeKey]; ok {
			if s, ok := v.(string); ok && len(s) == 2 {
				countryCode = s
				break
			}
		}
	}

	// location 嵌套结构
	if countryCode == "" || ip == "" {
		if locObj, ok := data["location"].(map[string]interface{}); ok {
			for _, codeKey := range codeFields {
				if v, ok := locObj[codeKey]; ok {
					if s, ok := v.(string); ok && len(s) == 2 {
						countryCode = s
						break
					}
				}
			}
			if ip == "" {
				if v, ok := locObj["ip"]; ok {
					if s, ok := v.(string); ok && s != "" {
						ip = s
					}
				}
			}
		}
	}

	// datacenter 嵌套结构
	if countryCode == "" {
		if dcObj, ok := data["datacenter"].(map[string]interface{}); ok {
			for _, codeKey := range codeFields {
				if v, ok := dcObj[codeKey]; ok {
					if s, ok := v.(string); ok && len(s) == 2 {
						countryCode = s
						break
					}
				}
			}
		}
	}

	// 兼容 status=success 且 query 字段
	if ip == "" {
		if v, ok := data["status"]; ok && v == "success" {
			if q, ok := data["query"].(string); ok {
				ip = q
			}
		}
	}

	// 兼容 rate limit 或错误结构
	if ip == "" && countryCode == "" {
		return GeoIPData{}, nil
	}

	// 判断IP类型
	var ipv4, ipv6 string
	if net.ParseIP(ip) != nil {
		if strings.Contains(ip, ":") {
			ipv6 = ip
		} else {
			ipv4 = ip
		}
	}

	ipData := IPData{
		IPv4: ipv4,
		IPv6: ipv6,
	}
	ipData.IsCDN = checkCDN(&ipData)
	geoData = GeoIPData{
		CountryCode: countryCode,
		IPData:      ipData,
	}
	if countryCode == "CN" {
		slog.Info(fmt.Sprintf("%s 获取到 CN 代码，请检查返回数据:\n %s\n", url, string(bodyBytes)))
	}
	return geoData, nil
}

// getExitIP 获取出口ip
func getExitIP(client *http.Client, url string) (IPData, error) {
	var ipData IPData

	resp, err := client.Get(url)
	if err != nil {
		slog.Debug(fmt.Sprintf("getExitIP 请求失败: %s, err: %v", url, err))
		return IPData{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		slog.Debug(fmt.Sprintf("getExitIP 非200状态码: %s, code: %d", url, resp.StatusCode))
		return IPData{}, fmt.Errorf("status: %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		slog.Debug(fmt.Sprintf("getExitIP 读取响应体失败: %s, err: %v", url, err))
		return IPData{}, err
	}

	body := strings.TrimSpace(string(bodyBytes))
	contentType := resp.Header.Get("Content-Type")

	// 解析响应内容
	if strings.Contains(contentType, "application/json") {
		ipData.IPv4, ipData.IPv6 = getIPFromJSON(bodyBytes)
	} else {
		ipData.IPv4 = reIPv4.FindString(body)
		ipData.IPv6 = reIPv6.FindString(body)
	}

	if ipData.IPv4 == "" {
		if m := reCurrentIP.FindStringSubmatch(body); len(m) > 1 {
			ipData.IPv4 = m[1]
		}
	}

	// httpbin.org 特殊处理
	if m := reOriginIP.FindStringSubmatch(body); len(m) > 1 {
		if strings.Contains(m[1], ":") {
			ipData.IPv6 = m[1]
		} else {
			ipData.IPv4 = m[1]
		}
	}

	if net.ParseIP(ipData.IPv4) == nil {
		ipData.IPv4 = ""
	}
	if net.ParseIP(ipData.IPv6) == nil {
		ipData.IPv6 = ""
	}
	// 检测是否为 CF CDN IP
	if ipData.IPv4 != "" || ipData.IPv6 != "" {
		ipData.IsCDN = checkCDN(&ipData)
	}

	return ipData, nil
}

func GetMaxMindData(db *maxminddb.Reader, ip string) (countryCode string, err error) {
	if db == nil {
		return "", fmt.Errorf("MaxMind database is not initialized")
	}
	ipAddr, err := netip.ParseAddr(ip)
	if err != nil {
		return "", fmt.Errorf("invalid IP address: %s", ip)
	}
	err = db.Lookup(ipAddr).DecodePath(&countryCode, "country", "iso_code")
	return countryCode, err
}

// checkCDN 检查 IP 是否属于 Cloudflare CDN
func checkCDN(ipInfo *IPData) bool {

	cfCdnIPRanges := assets.GetCfCdnIPRanges()
	if cfCdnIPRanges == nil {
		slog.Debug("Cloudflare CDN IP ranges not loaded")
		return false
	}
	// 用 map简化类型和字段处理
	ipMap := map[string]string{"ipv4": ipInfo.IPv4, "ipv6": ipInfo.IPv6}
	for typ, ip := range ipMap {
		if ip == "" {
			continue
		}
		parsedIP := net.ParseIP(ip)
		if parsedIP == nil {
			continue
		}
		// IPv4/IPv6 类型判断
		if typ == "ipv4" && parsedIP.To4() == nil {
			continue
		}
		if typ == "ipv6" && (parsedIP.To16() == nil || parsedIP.To4() != nil) {
			continue
		}
		for _, ipnet := range cfCdnIPRanges[typ] {
			if ipnet != nil && ipnet.Contains(parsedIP) {
				slog.Debug(fmt.Sprintf("IP %s 属于 cloudflare CDN IP范围: %s", ip, ipnet.String()))
				return true
			}
		}
	}
	return false
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

// Checker 提供 IP 检查功能
type Checker struct {
	cfCdnRanges map[string][]*net.IPNet
	geoDB       *maxminddb.Reader
	httpClient  *http.Client
}

// NewChecker 创建一个新的 Checker 实例
func NewChecker(cfCdnRanges map[string][]*net.IPNet, geoDB *maxminddb.Reader) *Checker {
	return &Checker{
		cfCdnRanges: cfCdnRanges,
		geoDB:       geoDB,
		httpClient:  &http.Client{Timeout: 10 * time.Second},
	}
}

// CheckResult 表示检查结果
type CheckResult struct {
	IP          string `json:"ip"`
	CountryCode string `json:"country_code"`
	IsCDN       bool   `json:"is_cdn"`
	Location    string `json:"location,omitempty"`
	Tag         string `json:"tag,omitempty"`
}

// Check 检查指定的 IP 地址
func (c *Checker) Check(ip string) (*CheckResult, error) {
	// 解析 IP 地址
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return nil, fmt.Errorf("invalid IP address: %s", ip)
	}

	// 检查是否为 CDN
	isCDN := false
	if c.cfCdnRanges != nil {
		ipData := &IPData{}
		if parsedIP.To4() != nil {
			ipData.IPv4 = ip
		} else {
			ipData.IPv6 = ip
		}
		isCDN = checkCDN(ipData)
	}

	// 获取地理位置信息
	var countryCode string
	if c.geoDB != nil {
		countryCode, _ = GetMaxMindData(c.geoDB, ip)
	}

	// 获取代理信息（仅用于当前 IP，不用于指定 IP）
	loc, _, tag, _ := GetProxyCountryMixed(c.httpClient, c.geoDB)

	result := &CheckResult{
		IP:          ip,
		CountryCode: countryCode,
		IsCDN:       isCDN,
		Location:    loc,
		Tag:         tag,
	}

	return result, nil
}

// GetCurrentIPInfo 获取当前 IP 的地理位置信息
func (c *Checker) GetCurrentIPInfo() (*CheckResult, error) {
	geoData, err := GetGeoIPData(c.httpClient, c.geoDB)
	if err != nil {
		return nil, fmt.Errorf("failed to get current IP info: %w", err)
	}

	// 确定 IP 地址
	ip := geoData.IPv4
	if ip == "" {
		ip = geoData.IPv6
	}
	if ip == "" {
		return nil, fmt.Errorf("no valid IP address found")
	}

	// 获取代理信息
	loc, _, tag, _ := GetProxyCountryMixed(c.httpClient, c.geoDB)

	result := &CheckResult{
		IP:          ip,
		CountryCode: geoData.CountryCode,
		IsCDN:       geoData.IsCDN,
		Location:    loc,
		Tag:         tag,
	}

	return result, nil
}

// GetCurrentIP 仅获取当前 IP 地址
func (c *Checker) GetCurrentIP() (string, error) {
	geoData, err := GetGeoIPData(c.httpClient, c.geoDB)
	if err != nil {
		return "", fmt.Errorf("failed to get current IP: %w", err)
	}

	ip := geoData.IPv4
	if ip == "" {
		ip = geoData.IPv6
	}
	if ip == "" {
		return "", fmt.Errorf("no valid IP address found")
	}

	return ip, nil
}
