package ipinfo

import (
	"context"
	"crypto/tls"
	"net/http"
	"os"
	"regexp"
	"testing"
	"time"

	"github.com/sinspired/checkip/internal/config"
	"github.com/sinspired/checkip/internal/data"
)

func TestGetIPInfoData(t *testing.T) {
	// 设置测试环境变量
	// os.Setenv("SUBS-CHECK-CALL", "true")
	// defer os.Unsetenv("SUBS-CHECK-CALL")

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	db, err := data.OpenMaxMindDB()
	if err != nil {
		t.Fatalf("打开 MaxMind 数据库失败: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	geoData, err := GetIPInfoData(ctx, client, db)
	if err != nil {
		t.Errorf("获取 GeoIP 数据失败: %v", err)
	} else {
		t.Logf("国家代码: %s, IPv4: %s, IPv6: %s, IsCDN: %v", geoData.CountryCode, geoData.IPv4, geoData.IPv6, geoData.IsCDN)
		if geoData.CountryCode == "" || (geoData.IPv4 == "" && geoData.IPv6 == "") {
			t.Error("获取的 GeoIP 信息不完整")
		}
	}
}

func TestFetchGeoIPInfo(t *testing.T) {
	// 创建支持不安全 TLS 的客户端（仅用于测试）
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}
	client := &http.Client{
		Timeout:   10 * time.Second,
		Transport: tr,
	}

	successCount := 0
	failCount := 0
	// testAPIs := []string{
	// 	"https://checkip.info/json",
	// 	"http://ip-api.com/json", // 改回 HTTP 版本
	// }
	// for _, url := range testAPIs {
	// 	geo, err := GetGeoIP(client, url)
	// 	if err != nil {
	// 		t.Logf("[FAIL] URL: %s, err: %v", url, err)
	// 		failCount++
	// 		continue
	// 	}
	// 	t.Logf("[OK] URL: %s, Country: %s, IPv4: %s, IPv6: %s, IsCDN: %v", url, geo.CountryCode, geo.IPv4, geo.IPv6, geo.IsCDN)
	// 	if geo.CountryCode == "" || (geo.IPv4 == "" && geo.IPv6 == "") {
	// 		t.Logf("[WARN] %s 获取 GeoIP 不完整", url)
	// 		failCount++
	// 		continue
	// 	}
	// 	successCount++
	// }
	for _, url := range config.GEOIP_APIS {
		geo, err := FetchGeoIPInfo(client, url)
		if err != nil {
			t.Logf("[FAIL] URL: %s, err: %v", url, err)
			failCount++
			continue
		}
		t.Logf("[OK] URL: %s, Country: %s, IPv4: %s, IPv6: %s, IsCDN: %v", url, geo.CountryCode, geo.IPv4, geo.IPv6, geo.IsCDN)
		if geo.CountryCode == "" || (geo.IPv4 == "" && geo.IPv6 == "") {
			t.Logf("[WARN] %s 获取 GeoIP 不完整", url)
			failCount++
			continue
		}
		successCount++
	}
	if successCount == 0 {
		t.Error("所有 GeoIP API 均获取失败，请检查网络或 API 列表")
	} else {
		t.Logf("GeoIP API 测试成功 %d 个，失败 %d 个", successCount, failCount)
	}
}

func TestCheckCDN(t *testing.T) {
	// 测试 Cloudflare IPv4
	ipInfo4 := &IPData{IPv4: "104.28.163.56"}
	ipInfo4.CheckCDN()
	isCDN4 := ipInfo4.IsCDN
	t.Logf("IPv4 IsCDN: %v", isCDN4)
	if !isCDN4 {
		t.Error("IPv4 CDN 检测失败")
	}
	// 一个非cf cdn 的 IPv4 地址
	ipInfo4 = &IPData{IPv4: "45.65.122.98"}
	ipInfo4.CheckCDN()
	isCDN4 = ipInfo4.IsCDN
	t.Logf("IPv4 IsCDN: %v", isCDN4)
	if isCDN4 {
		t.Error("IPv4 CDN 检测失败")
	}
	// 测试 Cloudflare IPv6（如有）
	ipInfo6 := &IPData{IPv6: "2606:4700:3037::ac43:bd3a"}
	ipInfo6.CheckCDN()
	isCDN6 := ipInfo4.IsCDN
	t.Logf("IPv6 IsCDN: %v", isCDN6)
}
func TestGetIPFromJSON(t *testing.T) {
	jsonStr := `{"ip":"8.8.8.8","country_code":"US"}`
	ipv4, ipv6 := getIPFromJSON([]byte(jsonStr))
	t.Logf("IPv4: %s, IPv6: %s", ipv4, ipv6)
	if ipv4 != "8.8.8.8" {
		t.Error("getIPFromJSON 解析失败")
	}
}

func TestGetIP_HTML(t *testing.T) {
	// 模拟 HTML 响应
	body := `<html><body>Current IP Address: 8.8.8.8</body></html>`
	ipv4Re := `Current IP Address: ([\d\.]+)`
	ip := ""
	if matches := regexp.MustCompile(ipv4Re).FindStringSubmatch(body); len(matches) > 1 {
		ip = matches[1]
	}
	t.Logf("HTML解析IP: %s", ip)
	if ip != "8.8.8.8" {
		t.Error("HTML IP 解析失败")
	}
}

func TestGetExitIP(t *testing.T) {
	client := &http.Client{}

	for _, url := range config.IP_APIS {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		ipData := &IPData{}
		err := ipData.GetExitIP(ctx, client, url)
		t.Logf("URL: %s, IPInfo: %+v", url, ipData)
		if err == nil && ipData.IPv4 == "" && ipData.IPv6 == "" {
			t.Error("未能获取有效IP")
		}
	}
}

func TestGetMaxMindData(t *testing.T) {
	// 设置测试环境变量
	os.Setenv("TESTING", "1")
	defer os.Unsetenv("TESTING")

	db, err := data.OpenMaxMindDB()
	if err != nil {
		t.Fatalf("打开 MaxMind 数据库失败: %v", err)
	}
	defer db.Close()

	ip := "193.124.46.41"
	ipData := &IPData{IPv4: ip}
	err = ipData.GetMaxMindData(db)
	if err != nil {
		t.Errorf("获取 MaxMind 数据失败: %v", err)
	} else {
		t.Logf("IP: %s, Country Code: %s", ip, ipData.CountryCode)
		if ipData.CountryCode == "" {
			t.Error("未能获取有效国家代码")
		}
	}
}
