package ipinfo

import (
	"context"
	"crypto/tls"
	// "crypto/tls"
	"net/http"
	"regexp"
	"testing"
	"time"

	"github.com/sinspired/checkip/internal/data"
)

var test_IP_APIS = []string{
	"https://ip.122911.xyz/api/ipinfo",
	"https://qifu-api.baidubce.com/ip/local/geo/v1/district",
	"https://r.inews.qq.com/api/ip2city",
	"https://g3.letv.com/r?format=1",
	"https://cdid.c-ctrip.com/model-poc2/h",
	"https://whois.pconline.com.cn/ipJson.jsp",
	"https://api.live.bilibili.com/xlive/web-room/v1/index/getIpInfo",
	"https://6.ipw.cn/",                  // IPv4使用了 CFCDN, IPv6 位置准确
	"https://api6.ipify.org?format=json", // IPv4使用了 CFCDN, IPv6 位置准确
}

var test_GEOIP_APIS = []string{
	"https://ip.122911.xyz/api/ipinfo",
	"https://ident.me/json",
	"https://tnedi.me/json",
	"https://api.seeip.org/geoip",
}

func TestGetGeoIPData(t *testing.T) {
	// 设置测试环境变量
	// os.Setenv("SUBS-CHECK-CALL", "true")
	// defer os.Unsetenv("SUBS-CHECK-CALL")

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	db, err := data.OpenMaxMindDB("")
	if err != nil {
		t.Fatalf("打开 MaxMind 数据库失败: %v", err)
	}
	defer db.Close()

	cli, err := New(
		WithHttpClient(client),
		WithDBReader(db),
		WithIPAPIs(test_IP_APIS...),
		WithGeoAPIs(),
	)
	if err != nil {
		t.Error("客户端初始化失败")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	geoData, err := cli.GetGeoIPData(ctx)
	if err != nil {
		t.Errorf("获取 GeoIP 数据失败: %v", err)
	} else {
		t.Logf("国家代码: %s, IPv4: %s, IPv6: %s, IsCDN: %v", geoData.CountryCode, geoData.IPv4, geoData.IPv6, geoData.IsCDN)
		if geoData.CountryCode == "" || (geoData.IPv4 == "" && geoData.IPv6 == "") {
			t.Error("获取的 GeoIP 信息不完整")
		}
	}
}

func TestFetchExitIPandLookupGeoDB(t *testing.T) {
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
	db, err := data.OpenMaxMindDB("")
	if err != nil {
		t.Fatalf("打开 MaxMind 数据库失败: %v", err)
	}
	defer db.Close()

	cli, err := New(
		WithHttpClient(client),
		WithDBReader(db),
		WithIPAPIs(test_IP_APIS...),
		WithGeoAPIs(test_GEOIP_APIS...),
	)
	if err != nil {
		t.Fatalf("初始化客户端失败: %v", err)
	}
	defer cli.Close()

	total := len(cli.ipAPIs)
	success := 0

	for _, url := range cli.ipAPIs {
		ipData, err := cli.FetchExitIP(url)
		if err != nil {
			t.Logf("获取 %s 时出错: %v", url, err)
		}
		ip := ipData.IPv4
		if ip == "" {
			ip = ipData.IPv6
		}

		// 查询地理位置并更新
		cli.LookupGeoIPDataWithMMDB(&ipData)
		t.Logf("URL: %s, IPv4: %s, IPv6: %s, Country: %s", url, ipData.IPv4, ipData.IPv6, ipData.CountryCode)

		if ip != "" {
			success++
		} else if err == nil {
			t.Errorf("%s 未能获取有效IP", url)
		}
	}

	// 判断是否超过一半成功
	if success*2 <= total {
		t.Fatalf("仅成功获取 %d/%d 个 IP，未超过一半", success, total)
	} else {
		t.Logf("成功获取 %d/%d 个 IP，测试通过", success, total)
	}
}

func TestFetchExitIP(t *testing.T) {
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
	db, err := data.OpenMaxMindDB("")
	if err != nil {
		t.Fatalf("打开 MaxMind 数据库失败: %v", err)
	}
	defer db.Close()

	cli, err := New(
		WithHttpClient(client),
		WithDBReader(db),
		WithIPAPIs(test_IP_APIS...),
		WithGeoAPIs(test_GEOIP_APIS...),
	)
	if err != nil {
		t.Fatalf("初始化客户端失败: %v", err)
	}
	defer cli.Close()

	total := len(cli.ipAPIs)
	success := 0

	for _, url := range cli.ipAPIs {
		ipData, err := cli.FetchExitIP(url)
		if err != nil {
			t.Logf("获取 %s 时出错: %v", url, err)
		}
		ip := ipData.IPv4
		if ip == "" {
			ip = ipData.IPv6
		}

		t.Logf("URL: %s, IPInfo: %s", url, ip)

		if ip != "" {
			success++
		} else if err == nil {
			t.Errorf("%s 未能获取有效IP", url)
		}
	}

	// 判断是否超过一半成功
	if success*2 <= total {
		t.Fatalf("仅成功获取 %d/%d 个 IP，未超过一半", success, total)
	} else {
		t.Logf("成功获取 %d/%d 个 IP，测试通过", success, total)
	}
}

func TestLookupGeoIPDataWithMMDB(t *testing.T) {
	db, err := data.OpenMaxMindDB("")
	if err != nil {
		t.Fatalf("打开 MaxMind 数据库失败: %v", err)
	}
	defer db.Close()

	cli, err := New(
		WithDBReader(db),
	)
	if err != nil {
		t.Fatalf("初始化客户端失败: %v", err)
	}
	defer cli.Close()
	ip := "2a09:bac5:3988:263c::3cf:59"

	ipData := CreateIPDataFromIP(ip)

	_, err = cli.LookupGeoIPDataWithMMDB(ipData)
	if err != nil {
		t.Errorf("获取 MaxMind 数据失败: %v", err)
	} else {
		t.Logf("IP: %s, Country Code: %s, City: %s", ip, ipData.CountryCode, ipData.City)
		if ipData.CountryCode == "" {
			t.Error("未能获取有效国家代码")
		}
	}
}

func TestFetchGeoIPData(t *testing.T) {
	// 创建支持不安全 TLS 的客户端（仅用于测试）
	// tr := &http.Transport{
	// 	TLSClientConfig: &tls.Config{
	// 		InsecureSkipVerify: true,
	// 	},
	// }
	client := &http.Client{
		Timeout: 10 * time.Second,
		// Transport: tr,
	}

	successCount := 0
	failCount := 0

	db, err := data.OpenMaxMindDB("")
	if err != nil {
		t.Fatalf("打开 MaxMind 数据库失败: %v", err)
	}
	defer db.Close()

	cli, err := New(
		WithHttpClient(client),
		WithDBReader(db),
		WithIPAPIs(),
		WithGeoAPIs(test_GEOIP_APIS...),
	)
	if err != nil {
		t.Fatalf("初始化客户端失败: %v", err)
	}
	defer cli.Close()

	for _, url := range cli.geoAPIs {
		geo, err := cli.FetchGeoIPData(url)
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
	db, err := data.OpenMaxMindDB("")
	if err != nil {
		t.Fatalf("打开 MaxMind 数据库失败: %v", err)
	}
	defer db.Close()
	cli, err := New(
		WithDBReader(db),
	)
	if err != nil {
		t.Fatalf("初始化客户端失败: %v", err)
	}
	defer cli.Close()

	// 测试 Cloudflare IPv4
	ipInfo4 := &IPData{IPv4: "104.28.163.56"}
	cli.CheckCDN(ipInfo4)
	if !ipInfo4.IsCDN {
		t.Error("IPv4 CDN 检测失败")
	}

	// 非 CDN IPv4
	ipInfo4 = &IPData{IPv4: "45.65.122.98"}
	cli.CheckCDN(ipInfo4)
	if ipInfo4.IsCDN {
		t.Error("IPv4 非 CDN 错误识别为 CDN")
	}

	// 测试 Cloudflare IPv6
	ipInfo6 := &IPData{IPv6: "2a09:bac1:31e0:8::245:d4"}
	cli.CheckCDN(ipInfo6)
	t.Logf("IPv6: %s, IsCDN: %v", ipInfo6.IPv6, ipInfo6.IsCDN)
	if !ipInfo6.IsCDN {
		t.Error("IPv6 CDN 检测失败")
	}

	// 非 CDN IPv6
	ipInfo6 = &IPData{IPv6: "22a00:2381:2ebf:8c00:1:2:3:4"}
	cli.CheckCDN(ipInfo6)
	t.Logf("IPv6: %s, IsCDN: %v", ipInfo6.IPv6, ipInfo6.IsCDN)
	if ipInfo6.IsCDN {
		t.Error("IPv6 非 CDN 错误识别为 CDN")
	}
}

func TestGetIPFromJSON(t *testing.T) {
	jsonStr := `{"ip":"8.8.8.8","country_code":"US","ipv6":"2001:4860:4860::8888"}`
	ipv4, ipv6 := getIPFromJSON([]byte(jsonStr))
	t.Logf("IPv4: %s, IPv6: %s", ipv4, ipv6)
	if ipv4 != "8.8.8.8" && ipv6 != "2001:4860:4860::8888" {
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
