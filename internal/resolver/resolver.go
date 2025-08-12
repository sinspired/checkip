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

// Resolve 检查指定的 IP 地址
func (r *Resolver) Resolve(ip string) (*ResolveResult, error) {
	ipData := ipinfo.CreateIPDataFromIP(ip)

	// 检查是否为 CDN
	isCDN := r.cli.CheckCDN(ipData)

	// 获取地理位置信息
	var countryCode string
	if r.geoDB != nil {
		// 使用 MaxMind 数据库获取国家代码
		_, err := r.cli.LookupGeoIPDataWithMMDB(ipData)
		if err != nil {
			return nil, err
		}
		countryCode = ipData.CountryCode
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// 获取代理信息（仅用于当前 IP，不用于指定 IP）
	loc, _, tag, _ := r.cli.GetAnalyzed(ctx)

	result := &ResolveResult{
		IP:          ip,
		CountryCode: countryCode,
		IsCDN:       isCDN,
		Location:    loc,
		Tag:         tag,
	}

	return result, nil
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
	loc, _, tag, _ := r.cli.GetAnalyzed(ctx)

	result := &ResolveResult{
		IP:          ip,
		CountryCode: geoData.CountryCode,
		IsCDN:       geoData.IsCDN,
		Location:    loc,
		Tag:         tag,
	}

	return result, nil
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
