package ipinfo

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"os"
	"strings"
	"time"

	"log/slog"

	"github.com/sinspired/checkip/internal/data"
)

// GetGeoIPData 获取出口 IP 地址和地理位置信息, ipAPI -> MaxMind -> geoAPI 兜底
func (c *Client) GetGeoIPData(resolveCtx context.Context) (info IPData, err error) {
	// 检测是否已经收到停止信号（取消/超时）
	stopped := false
	if resolveCtx.Err() != nil {
		stopped = true
	}

	// 1) ipAPI 获取 IP
	if len(c.ipAPIs) > 0 {
		shuffledIPAPIs := shuffle(c.ipAPIs)

		// 若已收到停止信号，则只尝试前三个；否则全部尝试
		if stopped && len(shuffledIPAPIs) > 3 {
			shuffledIPAPIs = shuffledIPAPIs[:3]
		}

		// 当运行过程中收到停止信号时，从“检测到停止”的那一刻起，最多再尝试 3 次
		ipAPIsAttemptsSinceStop := 0

		for _, url := range shuffledIPAPIs {
			// 运行中动态检测停止信号
			if !stopped {
				select {
				case <-resolveCtx.Done():
					stopped = true
					ipAPIsAttemptsSinceStop = 0
				default:
				}
			}
			// 若已停止且已尝试满 3 次，直接结束该循环
			if stopped && ipAPIsAttemptsSinceStop >= 3 {
				slog.Debug("收到停止信号后，ipAPI 最多只尝试三次，已达上限")
				break
			}

			temp, e := c.FetchExitIP(url)
			if e == nil && (temp.IPv4 != "" || temp.IPv6 != "") {
				slog.Debug(fmt.Sprintf("%s : IPv4=%s IPv6=%s", url, temp.IPv4, temp.IPv6))
				info = temp
				break
			}
			slog.Debug(fmt.Sprintf("从 ipAPI 获取出口 IP 失败: %s, err: %v", url, e))

			if stopped {
				ipAPIsAttemptsSinceStop++
			}
		}

		// 2) MaxMind
		if info.IPv4 != "" || info.IPv6 != "" {
			if _, mmErr := c.LookupGeoIPDataWithMMDB(&info); mmErr == nil && info.CountryCode != "" {
				ip := info.IPv4
				if ip == "" {
					ip = info.IPv6
				}
				slog.Debug(fmt.Sprintf("MaxMind 获取到 %s 的国家代码: %s", ip, info.CountryCode))
				if info.CountryCode != "CN" || os.Getenv("SUBS-CHECK-CALL") == "" {
					c.CheckCDN(&info)
					return info, nil
				}
			} else if mmErr != nil {
				slog.Debug(fmt.Sprintf("MaxMind 查询失败: %v", mmErr))
			} else {
				slog.Debug("MaxMind 未能找到国家代码")
			}
		} else {
			slog.Debug("所有 ipAPI 均未能获取到有效的IP地址，准备使用 geoAPI 查询（有限额）")
		}
	}

	// 3) geoAPI 兜底
	if len(c.geoAPIs) > 0 {
		shuffledGeoAPIs := shuffle(c.geoAPIs)

		// 若已经停止，则只尝试前三个；否则全部尝试
		if stopped && len(shuffledGeoAPIs) > 3 {
			shuffledGeoAPIs = shuffledGeoAPIs[:3]
		}
		geoAPIsAttemptsSinceStop := 0

		for _, url := range shuffledGeoAPIs {
			// 动态检测停止信号
			if !stopped {
				select {
				case <-resolveCtx.Done():
					stopped = true
					geoAPIsAttemptsSinceStop = 0
				default:
				}
			}
			// 若已停止且已尝试满 3 次，结束循环
			if stopped && geoAPIsAttemptsSinceStop >= 3 {
				slog.Debug("收到停止信号后，geoAPI 最多只尝试三次，已达上限")
				break
			}

			temp, geoErr := c.FetchGeoIPData(url)
			if geoErr == nil && temp.CountryCode != "" {
				// 在 subs-check 环境中，不接受 CN 代码
				if temp.CountryCode == "CN" && os.Getenv("SUBS-CHECK-CALL") != "" {
					if stopped {
						geoAPIsAttemptsSinceStop++
					}
					continue
				}
				slog.Debug(fmt.Sprintf("%s : %s", url, temp.CountryCode))
				c.CheckCDN(&temp)
				return temp, nil
			}
			slog.Debug(fmt.Sprintf("从 geoAPI 获取地理位置信息失败: %s, err: %v", url, geoErr))

			if stopped {
				geoAPIsAttemptsSinceStop++
			}
		}
	}

	// 4) 全部失败
	if info.IPv4 == "" && info.IPv6 == "" {
		return IPData{}, errors.New("所有 ipAPI 及 geoAPI 均未能获取到有效的IP地址,疑似网络断开")
	}
	c.CheckCDN(&info)
	return info, errors.New("未能通过 MaxMind 或 geoAPI 获取到地理位置信息")
}

// FetchExitIP 从指定的 URL 获取出口 IP 地址
func (c *Client) FetchExitIP(url string) (IPData, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 6000*time.Millisecond)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		slog.Debug(fmt.Sprintf("GetExitIP 创建请求失败: %s, err: %v", url, err))
		return IPData{}, err
	}
	// TODO: 需要继续优化以减少拒绝概率
	for k, v := range apiCommonHeaders() {
		req.Header.Set(k, v)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		slog.Debug(fmt.Sprintf("GetExitIP 请求失败: %s, err: %v", url, err))
		return IPData{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		slog.Debug(fmt.Sprintf("GetExitIP 非200状态码: %s, code: %d", url, resp.StatusCode))
		return IPData{}, fmt.Errorf("status: %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		slog.Debug(fmt.Sprintf("GetExitIP 读取响应体失败: %s, err: %v", url, err))
		return IPData{}, err
	}

	// 去除 UTF-8 BOM 并裁剪空白
	bodyBytes = bytes.TrimPrefix(bodyBytes, []byte("\xef\xbb\xbf"))
	body := strings.TrimSpace(string(bodyBytes))

	// 如果返回的字符串长度小于 IPv4 或 IPv6 的最大可能长度
	if (len(body) <= 15 && strings.Contains(body, ".")) || (len(body) <= 39 && strings.Contains(body, ":")) {
		if ip := net.ParseIP(body); ip != nil {
			info := IPData{}
			if ip.To4() != nil {
				info.IPv4 = ip.String()
			} else {
				info.IPv6 = ip.String()
			}
			c.CheckCDN(&info)
			return info, nil
		}
	}

	var ipv4, ipv6 string

	// 使用“字符级扫描”从任意文本/HTML 中提取 IP,性能最好
	ipv4, ipv6 = ExtractIPStrings(body)

	if ipv4 == "" && ipv6 == "" {
		// 如果没有找到 IP，尝试解析 json 和 正则匹配
		contentType := resp.Header.Get("Content-Type")
		if strings.Contains(contentType, "application/json") {
			ipv4, ipv6 = getIPFromJSON(bodyBytes)
		} else {
			ipv4 = reIPv4.FindString(body)
			ipv6 = reIPv6.FindString(body)
		}
	}

	// 校验合法性
	if net.ParseIP(ipv4) == nil {
		ipv4 = ""
	}
	if net.ParseIP(ipv6) == nil {
		ipv6 = ""
	}

	info := IPData{IPv4: ipv4, IPv6: ipv6}
	if ipv4 != "" || ipv6 != "" {
		c.CheckCDN(&info)
		return info, nil
	}

	slog.Debug(fmt.Sprintf("%s 未获取到ip, 返回数据: %q", url, body))
	return info, fmt.Errorf("%s 未获取到ip, 返回数据: %q", url, body)
}

// LookupGeoIPDataWithMMDB 使用 MaxMind 数据库查找地理位置信息
func (c *Client) LookupGeoIPDataWithMMDB(info *IPData) (string, error) {
	if c.mmdb == nil {
		return "", fmt.Errorf("MaxMind 数据库未初始化")
	}

	ip := info.IPv4
	if ip == "" {
		ip = info.IPv6
	}
	ipAddr, err := netip.ParseAddr(ip)
	if err != nil {
		return "", fmt.Errorf("无效的 IP 地址: %s", ip)
	}

	var rec struct {
		Country struct {
			ISOCode string            `maxminddb:"iso_code"`
			Names   map[string]string `maxminddb:"names"`
		} `maxminddb:"country"`
		Continent struct {
			Code string `maxminddb:"code"`
		} `maxminddb:"continent"`
		City struct {
			Names map[string]string `maxminddb:"names"`
		} `maxminddb:"city"`
		Subdivisions []struct {
			ISOCode string            `maxminddb:"iso_code"`
			Names   map[string]string `maxminddb:"names"`
		} `maxminddb:"subdivisions"`
		Postal struct {
			Code string `maxminddb:"code"`
		} `maxminddb:"postal"`
		Location struct {
			Latitude  float64 `maxminddb:"latitude"`
			Longitude float64 `maxminddb:"longitude"`
			TimeZone  string  `maxminddb:"time_zone"`
			// AccuracyRadius uint16  `maxminddb:"accuracy_radius"`
		} `maxminddb:"location"`
	}

	if err := c.mmdb.Lookup(ipAddr).Decode(&rec); err != nil {
		return "", err
	}

	info.CountryCode = strings.ToUpper(rec.Country.ISOCode)
	info.CountryName = rec.Country.Names["en"]
	info.ContinentCode = strings.ToUpper(rec.Continent.Code)
	info.City = rec.City.Names["en"]

	if len(rec.Subdivisions) > 0 {
		info.Region = rec.Subdivisions[0].Names["en"]
		info.RegionCode = strings.ToUpper(rec.Subdivisions[0].ISOCode)
	}

	info.PostalCode = rec.Postal.Code
	info.Latitude = rec.Location.Latitude
	info.Longitude = rec.Location.Longitude
	info.TimeZone = rec.Location.TimeZone
	// info.AccuracyRadius = rec.Location.AccuracyRadius

	return info.CountryCode, nil
}

// FetchGeoIPData 从指定的 URL 获取地理位置信息
func (c *Client) FetchGeoIPData(url string) (IPData, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 6000*time.Millisecond)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		slog.Debug(fmt.Sprintf("创建请求失败: %s, err: %v", url, err))
		return IPData{}, err
	}
	for k, v := range apiCommonHeaders() {
		req.Header.Set(k, v)
	}
	if strings.Contains(url, "checkip.info") {
		req.Header.Set("User-Agent", "PostmanRuntime/7.32.3")
		req.Header.Set("Accept", "*/*")
	}

	if strings.Contains(url, "122911.xyz") {
		req.Header.Set("User-Agent", "subs-check (https://github.com/beck-8/subs-check)")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		slog.Debug(fmt.Sprintf("请求失败: %s, err: %v", url, err))
		return IPData{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		slog.Debug(fmt.Sprintf("请求状态码错误: %s, status: %d", url, resp.StatusCode))
		return IPData{}, fmt.Errorf("status code: %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		slog.Debug(fmt.Sprintf("读取响应失败: %s, err: %v", url, err))
		return IPData{}, err
	}

	// 在返回数据中查找 IP 和 国家代码,兼容不同格式的返回数据
	ip, countryCode := ExtractGeoIPStrings(bodyBytes)

	var ipv4, ipv6 string
	if parsed := net.ParseIP(ip); parsed != nil {
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
	c.CheckCDN(&info)

	if info.CountryCode == "CN" {
		slog.Debug(fmt.Sprintf("%s 获取到 CN 代码，请检查返回数据:\n %s\n", url, string(bodyBytes)))
	}
	return info, nil
}

// CheckCDN 检查 IP 是否属于 Cloudflare CDN IP 范围
func (c *Client) CheckCDN(info *IPData) bool {
	cfCdnIPRanges := data.GetCfCdnIPRanges()
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
