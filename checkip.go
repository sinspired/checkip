// checkip.go
package checkip

import (
	"net"
	"net/http"

	"github.com/oschwald/maxminddb-golang/v2"
	"github.com/sinspired/checkip/internal/assets"
	internalcheckip "github.com/sinspired/checkip/internal/checkip"
)

// Checker 提供IP检查功能
type Checker struct {
	checker *internalcheckip.Checker
}

// NewChecker 创建一个新的检查器实例
func NewChecker(cfCdnRanges map[string][]*net.IPNet, geoDB *maxminddb.Reader) *Checker {
	return &Checker{
		checker: internalcheckip.NewChecker(cfCdnRanges, geoDB),
	}
}

// GetProxyCountryMixed 获取国家信息和出口ip地址，并判断是否为 Cloudflare 代理
func (c *Checker) GetProxyCountryMixed(httpClient *http.Client, db *maxminddb.Reader) (loc string, ip string, countryCode_tag string, err error) {
	return internalcheckip.GetProxyCountryMixed(httpClient, db)
}

// Check 检查指定IP的信息
func (c *Checker) Check(ip string) (*internalcheckip.CheckResult, error) {
	return c.checker.Check(ip)
}

// GetCurrentIPInfo 获取当前IP的完整信息
func (c *Checker) GetCurrentIPInfo() (*internalcheckip.CheckResult, error) {
	return c.checker.GetCurrentIPInfo()
}

// GetCurrentIP 获取当前IP地址
func (c *Checker) GetCurrentIP() (string, error) {
	return c.checker.GetCurrentIP()
}

// LoadCloudflareCIDRs 加载Cloudflare CDN IP范围
func LoadCloudflareCIDRs(path string) (map[string][]*net.IPNet, error) {
	return assets.LoadCloudflareCIDRs(path)
}

// OpenGeoDB 打开MaxMind地理数据库
func OpenGeoDB(path string) (*maxminddb.Reader, error) {
	return assets.OpenGeoDB(path)
}
