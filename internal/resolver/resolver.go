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
	return &Resolver{
		cfCdnRanges: cfCdnRanges,
		geoDB:       geoDB,
		httpClient:  &http.Client{Timeout: 10 * time.Second},
	}
}

// Resolve 检查指定的 IP 地址
func (r *Resolver) Resolve(ip string) (*ResolveResult, error) {
	// 解析 IP 地址
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return nil, fmt.Errorf("invalid IP address: %s", ip)
	}

	ipData := &ipinfo.IPData{}
	// 检查是否为 CDN
	isCDN := false
	if r.cfCdnRanges != nil {
		if parsedIP.To4() != nil {
			ipData.IPv4 = ip
		} else {
			ipData.IPv6 = ip
		}
		isCDN = ipData.IsCDN
	}

	// 获取地理位置信息
	var countryCode string
	if r.geoDB != nil {
		err := ipData.GetMaxMindData(r.geoDB)
		if err !=nil {
			return nil, err
		}
		countryCode = ipData.CountryCode
	}

	// 获取代理信息（仅用于当前 IP，不用于指定 IP）
	loc, _, tag, _ := ipinfo.GetMixed(r.httpClient, r.geoDB)

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
	geoData, err := ipinfo.GetIPInfoData(ctx, r.httpClient, r.geoDB)
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
	loc, _, tag, _ := ipinfo.GetMixed(r.httpClient, r.geoDB)

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
	geoData, err := ipinfo.GetIPInfoData(ctx, r.httpClient, r.geoDB)
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
