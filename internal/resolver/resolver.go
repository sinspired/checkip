// internal/checkip/checkip.go
package resolver

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/oschwald/maxminddb-golang/v2"
	"github.com/sinspired/checkip/pkg/ipinfo"
)

// NewResolver 创建一个新的 Resolver 实例
func NewResolver(cfCdnRanges map[string][]*net.IPNet, geoDB *maxminddb.Reader) *Resolver {
	cli, _ := ipinfo.New(
		ipinfo.WithHttpClient(&http.Client{Timeout: 10 * time.Second}),
	)
	return &Resolver{
		cli:        cli,
		geoDB:      geoDB,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// 填充 ResolveResult 公共逻辑
func fillResult(ip string, isCDN bool, loc, tag string, data *ipinfo.IPData) *ResolveResult {
	return &ResolveResult{
		Tag:           tag,
		IsCDN:         isCDN,
		IP:            ip,
		CountryCode:   data.CountryCode,
		CountryName:   data.CountryName,
		ContinentCode: data.ContinentCode,
		City:          data.City,
		LocationInfo: LocationInfo{
			Location:  loc,
			TimeZone:  data.TimeZone,
			Latitude:  data.Latitude,
			Longitude: data.Longitude,
		},
		RegionInfo: RegionInfo{
			Region:     data.Region,
			RegionCode: data.RegionCode,
			PostalCode: data.PostalCode,
		},
	}
}

// Resolve 检查指定的 IP 地址
func (r *Resolver) Resolve(ip string) (*ResolveResult, error) {
	ipData := ipinfo.CreateIPDataFromIP(ip)

	// 检查是否为 CDN
	isCDN := r.cli.CheckCDN(ipData)

	// 获取地理位置信息
	if r.geoDB != nil {
		if _, err := r.cli.LookupGeoIPDataWithMMDB(ipData); err != nil {
			return nil, err
		}
	}

	// 获取代理信息（仅用于当前 IP，不用于指定 IP）
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	loc, _, tag, _ := r.cli.GetAnalyzed(ctx, "", "")

	return fillResult(ip, isCDN, loc, tag, ipData), nil
}

// GetCurrentIPInfo 获取当前 IP 的地理位置信息
func (r *Resolver) GetCurrentIPInfo() (*ResolveResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	geoData, err := r.cli.GetGeoIPData(ctx)
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
	loc, _, tag, _ := r.cli.GetAnalyzed(ctx, "", "")

	return fillResult(ip, geoData.IsCDN, loc, tag, &geoData), nil
}

// GetCurrentIP 仅获取当前 IP 地址
func (r *Resolver) GetCurrentIP() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	geoData, err := r.cli.GetGeoIPData(ctx)
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
