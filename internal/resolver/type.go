package resolver

import (
	"net"
	"net/http"

	"github.com/oschwald/maxminddb-golang/v2"
)

// Resolver 提供 IP 检查功能
type Resolver struct {
	cfCdnRanges map[string][]*net.IPNet
	geoDB       *maxminddb.Reader
	httpClient  *http.Client
}

// ResolveResult 表示检查结果
type ResolveResult struct {
	IP          string `json:"ip"`
	CountryCode string `json:"country_code"`
	IsCDN       bool   `json:"is_cdn"`
	Location    string `json:"location,omitempty"`
	Tag         string `json:"tag,omitempty"`
}