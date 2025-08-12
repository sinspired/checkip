package resolver

import (
	"net/http"

	"github.com/oschwald/maxminddb-golang/v2"
	"github.com/sinspired/checkip/pkg/ipinfo"
)

// Resolver 提供 IP 检查功能
type Resolver struct {
	cli        *ipinfo.Client
	httpClient *http.Client
	geoDB      *maxminddb.Reader
}

// ResolveResult 表示检查结果
type ResolveResult struct {
	IP          string `json:"ip"`
	CountryCode string `json:"country_code"`
	IsCDN       bool   `json:"is_cdn"`
	Location    string `json:"location,omitempty"`
	Tag         string `json:"tag,omitempty"`
}
