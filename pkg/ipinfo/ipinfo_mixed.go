package ipinfo

import (
	"context"
	"net/http"
	"time"

	"github.com/oschwald/maxminddb-golang/v2"
)

// GetProxyCountryMixed 获取国家信息和出口ip地址，并判断是否为 Cloudflare 代理。
func GetMixed(httpClient *http.Client, db *maxminddb.Reader) (loc string, ip string, countryCode_tag string, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	geoIPData, err := GetIPInfoData(ctx, httpClient, db)
	if err != nil {
		return "", "", "", err
	}
	ip = geoIPData.IPv4
	if ip == "" {
		ip = geoIPData.IPv6
	}
	isCfRealyIP, Exitloc, cf_relay_loc := CheckCfRelayIP(ctx, httpClient, &geoIPData)

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
