package ipinfo

import (
	"context"
	"fmt"
	"log/slog"
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
func (c *Client) GetAnalyzed(ctx context.Context, cfLoc string, cfIP string) (loc, ip, countryCodeTag, ispTag string, err error) {
	ipData, err := c.GetGeoIPData(ctx)
	if err != nil {
		return "", "", "", "", fmt.Errorf("failed to get IP info data: %w", err)
	}

	ip = ipData.IPv4
	if ip == "" {
		ip = ipData.IPv6
	}

	// CN 不需要判断 CF Proxy
	if ipData.CountryCode == "CN" {
		return ipData.ContinentCode, ip, "Local ISP", "", nil
	}

	// 实际ip的isp标签
	slog.Debug("GetAnalyzed", "ip", ip, "isCDN", ipData.IsCDN, "countryCode", ipData.CountryCode, "cfLoc", cfLoc, "cfIP", cfIP)
	ispTag = c.GetISPInfo(ip)

	if !ipData.IsCDN {
		countryCodeTag = ipData.CountryCode + "²"
		return ipData.CountryCode, ip, countryCodeTag, ispTag, nil
	}

	cfProxyInfo := c.GetCfProxyInfo(&ipData, cfLoc, cfIP)

	switch {
	case cfProxyInfo.isCFProxy && cfProxyInfo.cfLoc != "":
		// 获取 CF 代理节点的 ISP 标签
		ispTagCF := c.GetISPInfo(cfProxyInfo.cfIP)
		if ispTagCF != "" {
			ispTag = ispTag + "¹" + ispTagCF + "⁰"
		} else if ispTag != "" {
			ispTag = ispTag + "¹" + "[未知]" + "⁰"
		}
		countryCodeTag = cfProxyInfo.exitLoc + "¹" + "-" + cfProxyInfo.cfLoc + "⁰"

	case cfProxyInfo.isCFProxy && cfProxyInfo.cfLoc == "":
		if !c.CheckCloudflareQuick() {
			countryCodeTag = cfProxyInfo.exitLoc + "⁻¹"
		} else {
			ispTagCF, countryCodeCF := c.GetISPInfoCFfallback()
			if ispTagCF == "" {
				ispTagCF = "[未知]"
			}
			if ispTag != "" {
				ispTag = ispTag + "¹" + ispTagCF + "⁰"
			}

			countryCodeTag = cfProxyInfo.exitLoc + "¹" + "-" + countryCodeCF + "⁰"
		}

	case cfProxyInfo.isCFProxy && cfProxyInfo.exitLoc == cfProxyInfo.cfLoc:
		countryCodeTag = cfProxyInfo.exitLoc + "¹⁺"

	case cfProxyInfo.isCFProxy:
		countryCodeTag = cfProxyInfo.exitLoc + "¹" + "-" + cfProxyInfo.cfLoc + "⁰"

	default:
		countryCodeTag = cfProxyInfo.exitLoc + "²"
	}

	return cfProxyInfo.exitLoc, ip, countryCodeTag, ispTag, nil
}

// GetCfProxyInfo 获取 /cdn-cgi/trace 获取的 CDN 节点位置
func (c *Client) GetCfProxyInfo(info *IPData, cfLoc string, cfIP string) (cfProxyInfo CFProxyInfo) {
	cfRelayLoc, cfRelayIP := cfLoc, cfIP
	if cfLoc == "" {
		cfRelayLoc, cfRelayIP = c.GetCFTrace()
	}

	cfProxyInfo.isCFProxy = info.IsCDN && (info.IPv4 != cfRelayIP || info.IPv6 != "")

	cfProxyInfo.exitLoc = info.CountryCode
	cfProxyInfo.cfLoc = cfRelayLoc
	cfProxyInfo.cfIP = cfRelayIP
	return cfProxyInfo
}
