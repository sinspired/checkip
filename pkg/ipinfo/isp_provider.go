package ipinfo

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// ============================================================
// 渠道实现：ipapi.is
// https://api.ipapi.is/?q=<ip>&key=<apikey>
// 免费额度：注册后每天 1000 次
// ============================================================

type ipapiIsResponse struct {
	IP       string `json:"ip"`
	IsMobile bool   `json:"is_mobile"`
	Company  struct {
		Type string `json:"type"` // hosting, isp, business, education, government, banking
	} `json:"company"`
	ASN struct {
		Type    string `json:"type"` // 同上
		Country string `json:"country"`
	} `json:"asn"`
	Location struct {
		CountryCode string `json:"country_code"`
	} `json:"location"`
}

func (c *Client) fetchIPAPIIs(ctx context.Context, apiKey string, ip string) (*ISPInfo, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("ipapi.is: apikey 未配置")
	}

	baseURL, _ := url.Parse("https://api.ipapi.is")
	q := baseURL.Query()
	q.Set("key", apiKey)
	if ip != "" && ip != "me" {
		q.Set("q", ip)
	}
	baseURL.RawQuery = q.Encode()

	body, err := c.doGet(ctx, baseURL.String())
	if err != nil {
		return nil, fmt.Errorf("ipapi.is: %w", err)
	}

	var data ipapiIsResponse
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("ipapi.is: 解析响应失败: %w", err)
	}

	in := classifyInput{
		IP:          data.IP,
		CompanyType: data.Company.Type,
		ASNType:     data.ASN.Type,
		IsMobile:    data.IsMobile,
		LocationCC:  data.Location.CountryCode,
		ASNCC:       data.ASN.Country,
	}
	return classify("ipapi.is", in), nil
}

// ============================================================
// 渠道实现：proxycheck.io
// https://proxycheck.io/v3/<ip>?key=<apikey>
// 免费额度：每天 1000 次（另有约 5 倍的突发令牌可用）
//
// 注意：proxycheck.io 的 IP 必须显式写在 URL 路径里，不支持"留空查自己"，
// 所以这里会先用 ipify 拿一次出口 IP 再发起查询（多一次请求，但 ipify 无速率限制问题）。
// ============================================================

// proxyCheckResult 对应响应里以 IP 为 key 的那个对象。
type proxyCheckResult struct {
	Network struct {
		Type string `json:"type"` // Residential / Business / Wireless / Hosting / null
	} `json:"network"`
	Location struct {
		CountryCode string `json:"country_code"`
	} `json:"location"`
	Detections struct {
		Hosting bool `json:"hosting"`
	} `json:"detections"`
}

func (c *Client) fetchProxyCheck(ctx context.Context, apiKey string, ip string) (*ISPInfo, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("proxycheck.io: apikey 未配置")
	}

	var err error
	if ip == "" {
		ip, err = c.getOwnPublicIP(ctx)
		if err != nil {
			return nil, fmt.Errorf("proxycheck.io: %w", err)
		}
	}
	reqURL := fmt.Sprintf("https://proxycheck.io/v3/%s?key=%s&asn=1", ip, apiKey)
	body, err := c.doGet(ctx, reqURL)
	if err != nil {
		return nil, fmt.Errorf("proxycheck.io: %w", err)
	}

	// 响应格式是 {"status":"ok","<ip>":{...},"query_time":N}，
	// 查询的 IP 会作为 key 出现在顶层，key 名不固定，所以先解析成通用 map。
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("proxycheck.io: 解析响应失败: %w", err)
	}
	if status, ok := raw["status"]; ok {
		var s string
		_ = json.Unmarshal(status, &s)
		if s != "ok" {
			return nil, fmt.Errorf("proxycheck.io: status=%s", s)
		}
	}

	var result *proxyCheckResult
	for key, val := range raw {
		if key == "status" || key == "query_time" {
			continue
		}
		var r proxyCheckResult
		if err := json.Unmarshal(val, &r); err == nil {
			result = &r
			break
		}
	}
	if result == nil {
		return nil, fmt.Errorf("proxycheck.io: 响应中未找到 IP 结果")
	}

	// network.type 是 proxycheck.io 自己已经综合判定好的结果（不是 company/asn 两个独立字段），
	// 这里统一"复制成两份"塞进 CompanyType/ASNType，这样就能直接复用 classify() 里
	// 现成的机房/住宅/商宽判定分支，不用再单独写一套映射逻辑。
	var in classifyInput
	in.IP = ip
	in.LocationCC = result.Location.CountryCode
	switch strings.ToLower(result.Network.Type) {
	case "residential":
		in.CompanyType, in.ASNType = "isp", "isp" // 触发"双 ISP → 住宅"分支
	case "business":
		in.CompanyType, in.ASNType = "business", "business"
	case "wireless":
		in.IsMobile = true
	case "hosting":
		in.CompanyType, in.ASNType = "hosting", "hosting"
	default:
		// network.type 为空/null 时，退而使用 detections.hosting 兜底
		if result.Detections.Hosting {
			in.CompanyType, in.ASNType = "hosting", "hosting"
		}
	}

	return classify("proxycheck.io", in), nil
}

// ============================================================
// 渠道实现：iplocate.io
// https://iplocate.io/api/lookup?apikey=<apikey>   （不带 ip 查自己）
// 免费额度：每天 1000 次，免费版和付费版字段完全一致
// ============================================================

type ipLocateResponse struct {
	IP          string `json:"ip"`
	CountryCode string `json:"country_code"`
	ASN         struct {
		Type        string `json:"type"`
		CountryCode string `json:"country_code"`
	} `json:"asn"`
	Company struct {
		Type string `json:"type"`
	} `json:"company"`
}

func (c *Client) fetchIPLocate(ctx context.Context, apiKey, ip string) (*ISPInfo, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("iplocate.io: apikey 未配置")
	}

	// 构造请求 URL
	var reqURL string
	if ip != "" {
		reqURL = fmt.Sprintf("https://iplocate.io/api/lookup/%s?apikey=%s", ip, apiKey)
	} else {
		// 查自己出口 IP，优先返回cloudflare proxyIP
		reqURL = fmt.Sprintf("https://iplocate.io/api/lookup?apikey=%s", apiKey)
	}

	// 发起请求
	body, err := c.doGet(ctx, reqURL)
	if err != nil {
		return nil, fmt.Errorf("iplocate.io: 请求失败: %w", err)
	}

	// 解析响应
	var data ipLocateResponse
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("iplocate.io: 解析响应失败: %w", err)
	}

	// 如果响应数据缺失，直接返回错误，避免 classify 出现空值
	if data.Company.Type == "" && data.ASN.Type == "" && data.CountryCode == "" {
		return nil, fmt.Errorf("iplocate.io: 响应数据为空或无效")
	}

	// 分类处理
	in := classifyInput{
		IP:          data.IP,
		CompanyType: data.Company.Type,
		ASNType:     data.ASN.Type,
		LocationCC:  data.CountryCode,
		ASNCC:       data.ASN.CountryCode,
	}
	return classify("iplocate.io", in), nil
}

// ============================================================
// 渠道实现：ipdata.co
// https://api.ipdata.co?api-key=<apikey>   （不带 ip 查自己）
// 免费额度：每天 1500 次（或每月 45000 次）
//
// 注意：ipdata.co 只有 asn.type 这一个分类字段（没有独立的 company.type），
// 取值范围也和其他渠道不完全一致：hosting / isp / unknown / edu / gov / mil / business。
// ============================================================

type ipDataResponse struct {
	IP 		string `json:"ip"`
	CountryCode string `json:"country_code"`
	ASN         struct {
		Type string `json:"type"`
	} `json:"asn"`
}

func (c *Client) fetchIPData(ctx context.Context, apiKey string, ip string) (*ISPInfo, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("ipdata.co: apikey 未配置")
	}

	var reqURL string
	if ip != "" {
		reqURL = fmt.Sprintf("https://api.ipdata.co/%s?api-key=%s", ip, apiKey)
	} else {
		reqURL = fmt.Sprintf("https://api.ipdata.co?api-key=%s", apiKey)
	}
	body, err := c.doGet(ctx, reqURL)
	if err != nil {
		return nil, fmt.Errorf("ipdata.co: %w", err)
	}

	var data ipDataResponse
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("ipdata.co: 解析响应失败: %w", err)
	}

	normalized := normalizeIPDataType(data.ASN.Type)

	// 只有一个字段可用，把它同时当作 CompanyType 和 ASNType 塞进去，
	// 这样 hosting/business/education/government/military 这些分支能直接复用；
	// 唯独 "isp" 会因为双字段都是 isp 而被判成"住宅"，这与 ipdata 文档里
	// "isp = 归属 ISP 网段"的定义是一致的。
	in := classifyInput{
		IP:          data.IP,
		CompanyType: normalized,
		ASNType:     normalized,
		LocationCC:  data.CountryCode,
	}
	return classify("ipdata.co", in), nil
}

func normalizeIPDataType(t string) string {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "edu":
		return "education"
	case "gov":
		return "government"
	case "mil":
		return "military"
	case "unknown", "":
		return ""
	default:
		return strings.ToLower(t) // hosting / isp / business 原样透传
	}
}
