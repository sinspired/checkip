package ipinfo

import (
	"fmt"
	"net/http"
	"slices"
	"time"

	"github.com/oschwald/maxminddb-golang/v2"
	"github.com/sinspired/checkip/internal/data"
)

// IPData 存储 IP 地址、CDN、国家代码
type IPData struct {
	IPv4          string
	IPv6          string
	IsCDN         bool
	CountryCode   string // 国家代码（ISO）
	CountryName   string // 国家英文全称
	ContinentCode string
	City          string
	Region        string // 第一层行政区名称（省/州）
	RegionCode    string // 第一层行政区 ISO 代码
	PostalCode    string

	TimeZone  string
	Latitude  float64
	Longitude float64
}

// CFProxyInfo 存储 cloudflare CDN信息
type CFProxyInfo struct {
	isCFProxy bool   // 是否 Cloudflare 代理
	exitLoc   string // 出口 IP 的地理位置
	cfLoc     string // Cloudflare 代理 IP 的地理位置
}

// IP 信息检测客户端
type Client struct {
	httpClient *http.Client      // 指定 http 客户端
	mmdb       *maxminddb.Reader // 指定 MaxMind Geo 数据库

	ipAPIs  []string // 指定当前客户端获取出口 IP 的API
	geoAPIs []string // 指定当前客户端获取出口 GeoIP 的API

	// internal
	dbPath  string // 自定义数据库路径
	ownMMDB bool
}

// 客户端设置
type Option func(*Client) error

const defaultHTTPTimeout = 10 * time.Second

// 默认 IPAPIs
var defaultIPAPIs = []string{
	"https://check.torproject.org/api/ip",
	"https://qifu-api.baidubce.com/ip/local/geo/v1/district",
	"https://r.inews.qq.com/api/ip2city",
	"https://g3.letv.com/r?format=1",
	"https://cdid.c-ctrip.com/model-poc2/h",
	"https://whois.pconline.com.cn/ipJson.jsp",
	"https://api.live.bilibili.com/xlive/web-room/v1/index/getIpInfo",
	"https://6.ipw.cn/",                  // IPv4使用了 CFCDN, IPv6 位置准确
	"https://api6.ipify.org?format=json", // IPv4使用了 CFCDN, IPv6 位置准确
}

// 默认 GeoAPIs
var defaultGeoAPIs = []string{
	"https://ident.me/json",
	"https://tnedi.me/json",
	"https://api.seeip.org/geoip",
}

// 指定 http 客户端, 默认为  &http.Client{Timeout: 10 * time.Second}
func WithHttpClient(hc *http.Client) Option {
	return func(c *Client) error {
		if hc == nil {
			return fmt.Errorf("http client is nil")
		}
		c.httpClient = hc
		return nil
	}
}

// 指定 MaxMind 格式的数据库路径,默认为内置数据库
func WithDBPath(path string) Option {
	return func(c *Client) error {
		if path == "" {
			return fmt.Errorf("mmdb path is empty")
		}
		c.dbPath = path
		// 延迟到 New 打开，避免后续选项覆盖导致泄露
		return nil
	}
}

// 指定 MaxMind 数据库阅读器,默认为内置阅读器
func WithDBReader(db *maxminddb.Reader) Option {
	return func(c *Client) error {
		if db == nil {
			return fmt.Errorf("mmdb reader is nil")
		}
		c.mmdb = db
		c.ownMMDB = false
		c.dbPath = ""
		return nil
	}
}

// 指定当前客户端获取出口 API,默认为内置 API
func WithIPAPIs(apis ...string) Option {
	return func(c *Client) error {
		c.ipAPIs = slices.Clone(apis)
		return nil
	}
}

// 指定当前客户端获取 Geo 信息的API,默认为内置 API
func WithGeoAPIs(apis ...string) Option {
	return func(c *Client) error {
		c.geoAPIs = slices.Clone(apis)
		return nil
	}
}

// 创建新的 ipinfo 客户端
func New(opts ...Option) (*Client, error) {
	c := &Client{}

	for _, opt := range opts {
		if err := opt(c); err != nil {
			return nil, err
		}
	}

	// httpClient 默认
	if c.httpClient == nil {
		c.httpClient = &http.Client{Timeout: defaultHTTPTimeout}
	}

	// 初始化 mmdb
	if c.mmdb == nil {
		var db *maxminddb.Reader
		var err error
		if c.dbPath != "" {
			db, err = maxminddb.Open(c.dbPath)
			if err != nil {
				return nil, fmt.Errorf("open maxmind db: %w", err)
			}
		} else {
			db, err = data.OpenMaxMindDB("")
			if err != nil {
				return nil, fmt.Errorf("open default maxmind db: %w", err)
			}
		}
		c.mmdb = db
		c.ownMMDB = true
	}

	// API 列表兜底
	if len(c.ipAPIs) == 0 && len(c.geoAPIs) == 0{
		c.ipAPIs = slices.Clone(defaultIPAPIs)
		c.geoAPIs = slices.Clone(defaultGeoAPIs)
	}

	if c.mmdb == nil {
		return nil, fmt.Errorf("mmdb not initialized")
	}

	return c, nil
}

// Close 清理资源
func (c *Client) Close() error {
	if c == nil || c.mmdb == nil || !c.ownMMDB {
		return nil
	}
	err := c.mmdb.Close()

	c.mmdb = nil
	c.ownMMDB = false
	return err
}
