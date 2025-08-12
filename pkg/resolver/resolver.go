// checkip.go
package resolver

import (
	"net"

	"github.com/oschwald/maxminddb-golang/v2"
	"github.com/sinspired/checkip/internal/resolver"
)

// Resolver 提供IP检查功能
type Resolver struct {
	resolver *resolver.Resolver
}

// NewResolver 创建一个新的解析器实例
func NewResolver(cfCdnRanges map[string][]*net.IPNet, geoDB *maxminddb.Reader) *Resolver {
	return &Resolver{
		resolver: resolver.NewResolver(cfCdnRanges, geoDB),
	}
}

// Resolve 检查指定IP的信息
func (c *Resolver) Resolve(ip string) (*resolver.ResolveResult, error) {
	return c.resolver.Resolve(ip)
}

// GetCurrentIPInfo 获取当前IP的完整信息
func (c *Resolver) GetCurrentIPInfo() (*resolver.ResolveResult, error) {
	return c.resolver.GetCurrentIPInfo()
}

// GetCurrentIP 获取当前IP地址
func (c *Resolver) GetCurrentIP() (string, error) {
	return c.resolver.GetCurrentIP()
}
