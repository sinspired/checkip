package ipinfo

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/sinspired/checkip/internal/config"
)

// CheckCloudflare 检测当前客户端是否可以访问 Cloudflare CDN
func (c *Client) CheckCloudflare() (bool, string, string) {
	cfRelayLoc, cfRelayIP := c.GetCFTrace()

	if cfRelayLoc != "" && cfRelayIP != "" {
		slog.Debug(fmt.Sprintf("Cloudflare CDN 检测成功: loc=%s, ip=%s", cfRelayLoc, cfRelayIP))
		return true, cfRelayLoc, cfRelayIP
	}

	ok, err := c.checkCFEndpoint("https://cloudflare.com", 200)
	if err == nil && ok {
		slog.Debug("Cloudflare 可达，但未获取到 loc/ip")
		return true, "", ""
	}

	return false, "", ""
}

// GetCFTrace 获取 Cloudflare Trace 的 loc 和 ip,并设置 10s 超时
func (c *Client) GetCFTrace() (string, string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return c.FetchCFTraceFirstConcurrent(ctx, cancel)
}

// FetchCFTraceFirstConcurrent 并发处理 FetchCFCDNTrace
func (c *Client) FetchCFTraceFirstConcurrent(ctx context.Context, cancel context.CancelFunc) (string, string) {
	type result struct {
		loc string
		ip  string
	}

	// 乱序 + 截取前5, 减轻网络负载
	apis := shuffle(config.CF_CDN_APIS)
	if len(apis) > 5 {
		apis = apis[:5]
	}

	resultChan := make(chan result, 1)
	var once sync.Once
	var wg sync.WaitGroup

	retries := 2

	for _, baseURL := range apis {
		wg.Add(1)
		go func(url string) {
			defer wg.Done()
			for range retries {
				select {
				case <-ctx.Done():
					return
				default:
				}
				loc, ip := c.FetchCFTrace(ctx, url)
				if loc != "" && ip != "" {
					once.Do(func() {
						resultChan <- result{loc, ip}
						cancel()
					})
					return
				}
			}
		}(baseURL)
	}

	go func() {
		wg.Wait()
		close(resultChan)
	}()

	select {
	case r := <-resultChan:
		return r.loc, r.ip
	case <-ctx.Done():
		return "", ""
	}
}

// FetchCFTrace 从cloudflare 的cdn-cgi/trace API获取CDN节点位置
func (c *Client) FetchCFTrace(ctx context.Context, baseURL string) (string, string) {
	url := fmt.Sprintf("%s/cdn-cgi/trace", baseURL)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", ""
	}

	for key, value := range cfCommonHeaders() {
		req.Header.Set(key, value)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		return "", ""
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", ""
	}

	var loc, ip string
	for _, line := range strings.Split(string(body), "\n") {
		if strings.HasPrefix(line, "loc=") {
			loc = strings.TrimPrefix(line, "loc=")
		}
		if strings.HasPrefix(line, "ip=") {
			ip = strings.TrimPrefix(line, "ip=")
		}
	}
	return loc, ip
}

// checkCFEndpoint 检查指定的 Cloudflare 端点是否可达，并返回是否成功和错误信息
func (c *Client) checkCFEndpoint(url string, expectedStatus int) (bool, error) {
	req, err := http.NewRequest("HEAD", url, nil)
	if err != nil {
		return false, err
	}

	for key, value := range cfCommonHeaders() {
		req.Header.Set(key, value)
	}

	transport := c.httpClient.Transport
	if transport == nil {
		transport = &http.Transport{}
	}
	if t, ok := transport.(*http.Transport); ok {
		sni := req.URL.Hostname()
		t.TLSClientConfig = &tls.Config{
			InsecureSkipVerify: true,
			ServerName:         sni,
		}
		c.httpClient.Transport = t
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		errStr := err.Error()

		// 检测是否为典型的 Cloudflare 拒绝自身加速请求的错误
		//  strings.Contains(errStr, "EOF") ||
		// 	strings.Contains(errStr, "tls:") ||
		// 	strings.Contains(errStr, "ws closed: 1005") ||
		// 	strings.Contains(errStr, "connection reset") ||

		if strings.Contains(errStr, "EOF") ||
			strings.Contains(errStr, "tls:") ||
			strings.Contains(errStr, "connection reset") {
			slog.Debug("Cloudflare 连接异常，但可能可以访问，暂时返回 true", "error", errStr)
			return true, nil
		}
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != expectedStatus {
		if resp.StatusCode == 403 {
			slog.Debug("放行状态码", "code", resp.StatusCode)
			return true, nil
		} else {
			slog.Warn("cloudflare.com 返回非预期状态码", "code", resp.StatusCode)
		}
		return false, nil
	}
	return true, nil
}
