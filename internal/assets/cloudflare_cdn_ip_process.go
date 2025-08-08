// internal/assets/cloudflare_cdn_ip_process.go
package assets

import (
	"bufio"
	"fmt"
	"log"
	"log/slog"
	"net"
	"os"
	"strings"
	"sync"
)

var (
	cfCdnIPRanges map[string][]*net.IPNet
	loadOnce      sync.Once
	loadError     error
)

func loadCfCdnIPRanges() {
	cfCdnIPRanges = make(map[string][]*net.IPNet)
	ipContents := map[string]string{
		"ipv4": embeddedIPv4,
		"ipv6": embeddedIPv6,
	}

	totalLoaded := 0
	for version, content := range ipContents {
		var ipNets []*net.IPNet
		scanner := bufio.NewScanner(strings.NewReader(content))
		lineCount := 0

		for scanner.Scan() {
			lineCount++
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}

			_, ipNet, err := net.ParseCIDR(line)
			if err != nil {
				log.Printf("Warning: Failed to parse CIDR %s on line %d: %v", line, lineCount, err)
				continue
			}
			ipNets = append(ipNets, ipNet)
			slog.Debug("Loaded Cloudflare CDN IP range",
				slog.String("version", version),
				slog.String("cidr", ipNet.String()))
		}

		if err := scanner.Err(); err != nil {
			slog.Debug("Error reading IP ranges",
				slog.String("version", version),
				slog.Any("error", err))
			loadError = err
			return
		}

		cfCdnIPRanges[version] = ipNets
		totalLoaded += len(ipNets)
		slog.Debug("Loaded Cloudflare CDN IP ranges",
			slog.Int("count", len(ipNets)),
			slog.String("version", version))
	}

	slog.Debug("Successfully loaded Cloudflare CDN IP ranges",
		slog.Int("total_loaded", totalLoaded))
}

func GetCfCdnIPRanges() map[string][]*net.IPNet {
	loadOnce.Do(loadCfCdnIPRanges)

	if loadError != nil {
		slog.Debug("Error loading CDN IP ranges", slog.Any("error", loadError))
		return nil
	}

	if cfCdnIPRanges == nil || (len(cfCdnIPRanges["ipv4"]) == 0 && len(cfCdnIPRanges["ipv6"]) == 0) {
		slog.Debug("Warning: No CDN IP ranges loaded")
		return nil
	}

	return cfCdnIPRanges
}

// LoadCloudflareCIDRs 加载 Cloudflare CIDR 范围
func LoadCloudflareCIDRs(path string) (map[string][]*net.IPNet, error) {
	// 如果路径为空，使用嵌入的数据
	if path == "" {
		cfCdnIPRanges := GetCfCdnIPRanges()
		if cfCdnIPRanges == nil {
			return nil, fmt.Errorf("failed to load embedded Cloudflare CIDRs")
		}
		return cfCdnIPRanges, nil
	}

	// 从文件加载
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open CIDR file: %w", err)
	}
	defer file.Close()

	cfCdnIPRanges := make(map[string][]*net.IPNet)
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		_, ipNet, err := net.ParseCIDR(line)
		if err != nil {
			slog.Debug("Failed to parse CIDR", slog.String("line", line), slog.Any("error", err))
			continue
		}

		// 判断是 IPv4 还是 IPv6
		version := "ipv4"
		if strings.Contains(line, ":") {
			version = "ipv6"
		}

		cfCdnIPRanges[version] = append(cfCdnIPRanges[version], ipNet)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading CIDR file: %w", err)
	}

	return cfCdnIPRanges, nil
}
