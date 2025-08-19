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
	IP            string `json:"ip"`
	CountryCode   string `json:"country_code"`
	CountryName   string `json:"country_name"`
	ContinentCode string `json:"continent_code"`
	City          string `json:"city"`

	RegionInfo   RegionInfo   `json:"region_info"`
	LocationInfo LocationInfo `json:"location_info"`

	IsCDN bool   `json:"is_cdn"`
	Tag   string `json:"tag,omitempty"`
}

type RegionInfo struct {
	Region     string `json:"region"`
	RegionCode string `json:"region_code"`
	PostalCode string `json:"postal_code"`
}

type LocationInfo struct {
	Location string `json:"location,omitempty"`
	TimeZone string `json:"time_zone"`

	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}
