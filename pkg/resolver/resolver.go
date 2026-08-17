// checkip.go
package resolver

import (
	"net"

	"github.com/oschwald/maxminddb-golang/v2"
	"github.com/sinspired/checkip/internal/config"
	intresolver "github.com/sinspired/checkip/internal/resolver"
)

// Resolver 提供IP检查功能
type Resolver struct {
	resolver *intresolver.Resolver
}

// NewResolver 创建一个新的解析器实例
func NewResolver(cfCdnRanges map[string][]*net.IPNet, geoDB *maxminddb.Reader, cfg *config.Config) *Resolver {
	return &Resolver{
		// 传递 cfg 给底层的 NewResolver
		resolver: intresolver.NewResolver(cfCdnRanges, geoDB, cfg),
	}
}

// Resolve 检查指定IP的信息
func (c *Resolver) Resolve(ip string) (*intresolver.ResolveResult, error) {
	return c.resolver.Resolve(ip)
}

// GetCurrentIPInfo 获取当前IP的完整信息
func (c *Resolver) GetCurrentIPInfo() (*intresolver.ResolveResult, error) {
	return c.resolver.GetCurrentIPInfo()
}

// GetCurrentIP 获取当前IP地址
func (c *Resolver) GetCurrentIP() (string, error) {
	return c.resolver.GetCurrentIP()
}