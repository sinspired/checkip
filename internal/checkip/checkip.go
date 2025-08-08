// internal/checkip/checkip.go
package checkip

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
	"time"

	"github.com/metacubex/mihomo/common/convert"
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
	"https://api.ipapi.is",
	"https://checkip.info/json",
	// "https://ipinfo.io/json",
	// "http://ip-api.com/json",
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

// 请求头，避免被 ban
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

// GetProxyCountryMixed 获取国家信息和出口ip地址，并判断是否为 Cloudflare 代理。
func GetProxyCountryMixed(httpClient *http.Client, db *maxminddb.Reader) (loc string, ip string, countryCode_tag string, err error) {
	// 使用带超时的 context
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	geoIPData, err := GetGeoIPDataWithContext(ctx, httpClient, db)
	if err != nil {
		return "", "", "", err
	}
	ip = geoIPData.IPv4
	if ip == "" {
		ip = geoIPData.IPv6
	}
	isCfRealyIP, Exitloc, cf_relay_loc := checkCfRelayIPWithContext(ctx, httpClient, &geoIPData)

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

// GetProxyCountryMixedWithContext 带 context 的版本
func GetProxyCountryMixedWithContext(ctx context.Context, httpClient *http.Client, db *maxminddb.Reader) (loc string, ip string, countryCode_tag string, err error) {
	geoIPData, err := GetGeoIPDataWithContext(ctx, httpClient, db)
	if err != nil {
		return "", "", "", err
	}
	ip = geoIPData.IPv4
	if ip == "" {
		ip = geoIPData.IPv6
	}
	isCfRealyIP, Exitloc, cf_relay_loc := checkCfRelayIPWithContext(ctx, httpClient, &geoIPData)

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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return GetGeoIPDataWithContext(ctx, httpClient, db)
}

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

// GetGeoIPDataWithContext 带 context 的版本
func GetGeoIPDataWithContext(ctx context.Context, httpClient *http.Client, db *maxminddb.Reader) (geoData GeoIPData, err error) {
	var ipData IPData
	// 1. 优先：使用 ipAPI 获取出口 IP，然后用本地 MaxMind 查询国家代码
	// 打乱 ipAPIs 顺序
	shuffledAPIs := shuffleStringSlice(ipAPIs)
	for _, url := range shuffledAPIs {
		select {
		case <-ctx.Done():
			return GeoIPData{}, ctx.Err()
		default:
		}

		ipData, err = getExitIPWithContext(ctx, httpClient, url)
		if err == nil && (ipData.IPv4 != "" || ipData.IPv6 != "") {
			slog.Debug(fmt.Sprintf("%s : IPv4=%s IPv6=%s", url, ipData.IPv4, ipData.IPv6))
			break
		}
		slog.Debug(fmt.Sprintf("从 ipAPI 获取出口 IP 失败: %s, err: %v", url, err))
	}

	// 用 MaxMind 查询 地理位置
	if ipData.IPv4 != "" || ipData.IPv6 != "" {
		geoData = GeoIPData{
			IPData: ipData,
		}

		ip := ipData.IPv4
		if ip == "" {
			ip = ipData.IPv6
		}

		countryCode, mmErr := GetMaxMindData(db, ip)
		if mmErr == nil && countryCode != "" {
			slog.Debug(fmt.Sprintf("MaxMind 数据库获取到 %s 的国家代码: %s", ip, countryCode))
			geoData.CountryCode = countryCode
			return geoData, nil
		}

		if mmErr != nil {
			slog.Debug(fmt.Sprintf("获取 MaxMind 数据库信息失败: %s, err: %v", ip, mmErr))
		} else {
			slog.Debug(fmt.Sprintf("MaxMind 数据库未能找到国家代码: %s", ip))
		}
	} else {
		slog.Debug("所有 ipAPI 均未能获取到有效的IP地址，准备使用 geoAPI 查询（有限额）")
	}

	// 2. 如果 ipAPI + MaxMind 失败，使用 geoAPI 获取地理位置
	// 打乱 geoAPIs 顺序
	shuffledGeoAPIs := shuffleStringSlice(geoAPIs)
	for _, url := range shuffledGeoAPIs {
		select {
		case <-ctx.Done():
			return GeoIPData{}, ctx.Err()
		default:
		}

		geoData, err = GetGeoIP(httpClient, url)
		if err == nil && geoData.CountryCode != "" {
			// 在subs-check环境中，不接受 CN 代码
			if geoData.CountryCode == "CN" && os.Getenv("SUBS-CHECK-CALL") != "" {
				continue
			}
			slog.Debug(fmt.Sprintf("%s : %s", url, geoData.CountryCode))
			return geoData, nil
		}
		slog.Debug(fmt.Sprintf("从 geoAPI 获取地理位置信息失败: %s, err: %v", url, err))
	}
	// 3. 全部失败，返回错误
	if ipData.IPv4 == "" && ipData.IPv6 == "" {
		return GeoIPData{}, errors.New("所有 ipAPI 均未能获取到有效的IP地址,疑似网络断开！")
	}

	return geoData, errors.New("未能通过 MaxMind 或 geoAPI 获取到地理位置信息")
}

func checkCfRelayIPWithContext(ctx context.Context, httpClient *http.Client, geoIPData *GeoIPData) (bool, string, string) {
	cf_relay_loc, cf_relay_ip := FetchCFProxyWithContext(ctx, httpClient)

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
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		slog.Debug(fmt.Sprintf("创建请求失败: %s, err: %v", url, err))
		return GeoIPData{}, err
	}

	// 设置请求头
	for key, value := range apiCommonHeaders() {
		req.Header.Set(key, value)
	}

	// 特定 API 请求头覆盖
	switch {
	case strings.Contains(url, "checkip.info"):
		req.Header.Set("User-Agent", "PostmanRuntime/7.32.3")
		req.Header.Set("Accept", "*/*")
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		slog.Debug(fmt.Sprintf("请求失败: %s, err: %v", url, err))
		return GeoIPData{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		slog.Debug(fmt.Sprintf("请求状态码错误: %s, status: %d", url, resp.StatusCode))
		return GeoIPData{}, fmt.Errorf("status code: %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		slog.Debug(fmt.Sprintf("读取响应失败: %s, err: %v", url, err))
		return GeoIPData{}, err
	}

	var data map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &data); err != nil {
		slog.Debug(fmt.Sprintf("解析 JSON 失败: %s, err: %v, body: %s", url, err, string(bodyBytes)))
		return GeoIPData{}, err
	}

	// 检查是否被限流或返回错误
	if status, ok := data["status"]; ok {
		if s, ok := status.(string); ok && s == "fail" {
			return GeoIPData{}, fmt.Errorf("API returned failure status")
		}
		if f, ok := status.(float64); ok && int(f) == 429 {
			return GeoIPData{}, fmt.Errorf("rate limited")
		}
	}
	if _, ok := data["error"]; ok {
		return GeoIPData{}, fmt.Errorf("API returned error")
	}

	// 字段优先级
	codeFields := []string{"countryCode", "country_code", "cc", "country"}
	ipFields := []string{"ip", "query"}

	var ip, countryCode string

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

	if ip == "" && countryCode == "" {
		return GeoIPData{}, nil
	}

	if countryCode == "" {
		slog.Debug(fmt.Sprintf("未能获取到2位国家代码，原始数据: %s", string(bodyBytes)))
	}

	// 判断 IP 类型
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
		CountryCode: strings.ToUpper(countryCode),
		IPData:      ipData,
	}

	if geoData.CountryCode == "CN" {
		slog.Debug(fmt.Sprintf("%s 获取到 CN 代码，请检查返回数据:\n %s\n", url, string(bodyBytes)))
	}

	return geoData, nil
}

// getExitIP 获取出口ip
func getExitIP(client *http.Client, url string) (IPData, error) {
	var ipData IPData

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		slog.Debug(fmt.Sprintf("getExitIP 创建请求失败: %s, err: %v", url, err))
		return IPData{}, err
	}

	// 设置请求头
	for key, value := range apiCommonHeaders() {
		req.Header.Set(key, value)
	}

	resp, err := client.Do(req)
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

// getExitIPWithContext 带 context 的版本
func getExitIPWithContext(ctx context.Context, client *http.Client, url string) (IPData, error) {
	var ipData IPData

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		slog.Debug(fmt.Sprintf("getExitIPWithContext 创建请求失败: %s, err: %v", url, err))
		return IPData{}, err
	}
	// 确认 req 创建成功后 设置请求头
	for key, value := range apiCommonHeaders() {
		req.Header.Set(key, value)
	}

	resp, err := client.Do(req)
	if err != nil {
		slog.Debug(fmt.Sprintf("getExitIPWithContext 请求失败: %s, err: %v", url, err))
		return IPData{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		slog.Debug(fmt.Sprintf("getExitIPWithContext 非200状态码: %s, code: %d", url, resp.StatusCode))
		return IPData{}, fmt.Errorf("status: %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		slog.Debug(fmt.Sprintf("getExitIPWithContext 读取响应体失败: %s, err: %v", url, err))
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
