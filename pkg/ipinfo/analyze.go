package ipinfo

import (
	"context"
	"fmt"
	"time"
)

// GetAnalyzed 获取出口 IP 地址和地理位置信息并分析 CDN 信息, 收到 ctx 取消信号时，会加速进行获取;
// countryCode_tag examples:
//
// - BadCFNode: HK⁻¹
//
// - CFNodeWithSameCountry: HK¹⁺
//
// - CFNodeWithDifferentCountry: HK¹-US⁰
//
// - NodeWithoutCF: HK²
//
// - 前两位字母是实际浏览网站识别的位置, -US⁰为使用CF CDN服务的网站识别的位置, 比如GPT, X等
func (c *Client) GetAnalyzed(ctx context.Context, cfLoc string, cfIP string) (loc string, ip string, countryCode_tag string, err error) {
	ipData, err := c.GetGeoIPData(ctx)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to get IP info data: %w", err)
	}

	ip = ipData.IPv4
	if ip == "" {
		ip = ipData.IPv6
	}
	cfProxyInfo := c.GetCfProxyInfo(&ipData, cfLoc, cfIP)
	if cfProxyInfo.isCFProxy {
		if cfProxyInfo.cfLoc == "" {
			countryCode_tag = cfProxyInfo.exitLoc + "⁻¹"
		} else if cfProxyInfo.exitLoc == cfProxyInfo.cfLoc {
			countryCode_tag = cfProxyInfo.exitLoc + "¹⁺"
		} else {
			countryCode_tag = cfProxyInfo.exitLoc + "¹" + "-" + cfProxyInfo.cfLoc + "⁰"
		}
	} else {
		countryCode_tag = cfProxyInfo.exitLoc + "²"
	}
	return cfProxyInfo.exitLoc, ip, countryCode_tag, nil
}

// GetCfProxyInfo 获取 /cdn-cgi/trace 获取的 CDN 节点位置
func (c *Client) GetCfProxyInfo(info *IPData, cfLoc string, cfIP string) (cfProxyInfo CFProxyInfo) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cfRelayLoc, cfRelayIP := cfLoc, cfIP
	if cfLoc == "" {
		cfRelayLoc, cfRelayIP = c.FetchCFTraceFirstConcurrent(ctx, cancel)
	}

	cfProxyInfo.isCFProxy = info.IsCDN && (info.IPv4 != cfRelayIP || info.IPv6 != "")

	cfProxyInfo.exitLoc = info.CountryCode
	cfProxyInfo.cfLoc = cfRelayLoc
	return cfProxyInfo
}
