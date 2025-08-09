package ipinfo

import (
	"net/http"

	"github.com/oschwald/maxminddb-golang/v2"
)

type IPData struct {
	IPv4        string
	IPv6        string
	IsCDN       bool
	CountryCode string
}

type Client struct {
	httpClient *http.Client
	mmdb       *maxminddb.Reader

	ipAPIs  []string
	geoAPIs []string
}

type Option func(*Client) error