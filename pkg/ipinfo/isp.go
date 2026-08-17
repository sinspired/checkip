package ipinfo

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
)

// ============================================================
// 通用类型定义（所有渠道共用）
// ============================================================

type ISPType string

const (
	ISPHosting     ISPType = "hosting"     // 机房
	ISPResidential ISPType = "residential" // 住宅
	ISPMobile      ISPType = "mobile"      // 移动网络
	ISPBusiness    ISPType = "business"    // 商宽
	ISPEducation   ISPType = "education"   // 教育网
	ISPGovernment  ISPType = "government"  // 政府网络
	ISPBanking     ISPType = "banking"     // 银行/金融网络
	ISPMilitary    ISPType = "military"    // 军事网络（目前仅 ipdata.co 会返回）
	ISPUnknown     ISPType = "unknown"     // 有数据但无法归类 —— 与"该渠道没有返回数据"区分开
	ISPOther       ISPType = "other"       // 保留字段，兼容旧逻辑
)

type ISPInfo struct {
	IP          string  `json:"ip"`           // IP 地址
	Type        ISPType `json:"type"`         // 分类结果
	Details     string  `json:"details"`      // 中文描述
	IsNative    bool    `json:"is_native"`    // 是否原生（IP 注册地与 ASN 注册地一致）
	IsDualISP   bool    `json:"is_dual_isp"`  // 是否双 ISP（住宅判定的核心信号）
	Source      string  `json:"source"`       // 本次数据来自哪个渠道，便于排查问题
	CountryCode string  `json:"country_code"` // IP 地理位置国家代码（ISO 3166-1 alpha-2，大写），取不到则为空
}

type ISPConfig struct {
	// ISPCheck 是否启用 ISP 检查
	ISPCheck bool

	// ISPTimeout 是 ISP 检查的超时时间，单位为秒
	ISPTimeout time.Duration

	// 以下四个渠道的 apikey 均为可选：留空则该渠道自动跳过，不参与轮询。
	// 建议至少配置 2 个渠道，通过轮询F叠加每日免费额度，并在某一渠道
	// 请求失败（超额 / 网络错误）时自动切换到下一个渠道。

	// ISPCheckAPIKeyIPAPI ipapi.is 的 apikey（https://ipapi.is）
	// 免费额度：注册后每天 1000 次
	ISPCheckAPIKeyIPAPI string
	// ISPCheckAPIKeyProxyCheck proxycheck.io 的 apikey（https://proxycheck.io）
	// 免费额度：每天 1000 次（另有约 5 倍的突发令牌可用）
	ISPCheckAPIKeyProxyCheck string

	// ISPCheckAPIKeyIPLocate iplocate.io 的 apikey（https://iplocate.io）
	// 免费额度：每天 1000 次，免费版与付费版字段完全一致
	ISPCheckAPIKeyIPLocate string

	// ISPCheckAPIKeyIPData ipdata.co 的 apikey（https://ipdata.co）
	// 免费额度：每天 1500 次（或每月 45000 次）
	ISPCheckAPIKeyIPData string
}

// classifyInput 是从任意渠道的原始响应中提取出的"标准化信号"。
// 不同渠道字段名、粒度都不一样，但只要能被归约成这几个字段，
// 就可以复用同一套分类逻辑（classify），不用每个渠道各写一遍 switch。
type classifyInput struct {
	IP          string // IP 地址
	CompanyType string // 组织/公司维度的类型，取不到就留空
	ASNType     string // ASN/网络维度的类型，取不到就留空
	IsMobile    bool   // 是否有明确的移动网络标记
	LocationCC  string // IP 地理位置国家代码（用于原生/广播判定）
	ASNCC       string // ASN 注册地国家代码（用于原生/广播判定）
}

// classify 是所有渠道共用的统一判定逻辑。
//
// 优先级说明（相较最早版本做了调整）：
// 越"明确的组织属性"信号优先级越高，越"笼统的网络类型"信号放在最后兜底。
// 原因：is_mobile / government / banking 这类信号通常是数据源经过专门识别得出的，
// 误判率低；而 hosting vs isp 这种笼统分类本身就是用来"猜"住宅/机房的模糊信号，
// 不应该抢在明确信号之前命中。
func classify(source string, in classifyInput) *ISPInfo {
	info := &ISPInfo{Source: source}

	if in.LocationCC != "" {
		info.CountryCode = strings.ToUpper(strings.TrimSpace(in.LocationCC))
	}

	companyType := strings.ToLower(strings.TrimSpace(in.CompanyType))
	asnType := strings.ToLower(strings.TrimSpace(in.ASNType))

	// 完全没有可用信号 —— 交给调用方决定是否要重试下一个渠道
	// 注意：即便这里提前返回，上面已经填充的 CountryCode 依然保留，
	// 因为"查不出 ISP 类型"和"查不出国家"是两件独立的事。
	if companyType == "" && asnType == "" && !in.IsMobile {
		return info
	}

	// 原生 IP：IP 地理位置国家 与 ASN 注册地国家 一致
	if in.LocationCC != "" && in.ASNCC != "" {
		info.IsNative = strings.EqualFold(in.LocationCC, in.ASNCC)
	}

	// 双 ISP：company 和 asn 两个维度都判定为 isp，是"住宅"的核心信号
	info.IsDualISP = companyType == "isp" && asnType == "isp"

	switch {
	// 1. 移动网络 —— 最强信号，优先级最高
	case in.IsMobile || companyType == "mobile" || asnType == "mobile":
		info.Type = ISPMobile
		info.Details = "移动"

	// 2. 政府网络
	case companyType == "government" || asnType == "government":
		info.Type = ISPGovernment
		info.Details = "政府"

	// 3. 银行/金融网络
	case companyType == "banking" || asnType == "banking":
		info.Type = ISPBanking
		info.Details = "银行"

	// 4. 军事网络（目前仅 ipdata.co 的 mil 分类会命中）
	case companyType == "military" || asnType == "military":
		info.Type = ISPMilitary
		info.Details = "军事"

	// 5. 教育网
	case companyType == "education" || asnType == "education":
		info.Type = ISPEducation
		info.Details = "教育"

	// 6. 商宽
	case companyType == "business" || asnType == "business":
		info.Type = ISPBusiness
		info.Details = "商宽"

	// 7. 住宅拨号 VPS（伪装成住宅的机房出口，company=hosting 但 asn=isp）
	case companyType == "hosting" && asnType == "isp":
		info.Type = ISPResidential
		info.Details = "住宅拨号"

	// 8. 住宅（双 ISP）
	case info.IsDualISP:
		info.Type = ISPResidential
		info.Details = "住宅"

	// 9. 机房（兜底的 hosting 判定，放在最后）
	case companyType == "hosting" || asnType == "hosting":
		info.Type = ISPHosting
		info.Details = "机房"

	// 10. 有数据但无法归类 —— 明确标注为"未知"，而不是想当然地归为机房
	default:
		info.Type = ISPUnknown
		info.Details = "未知"
	}

	return info
}

// ============================================================
// 多渠道轮询调度
// ============================================================

// ispProvider 描述一个 ISP 检测渠道。
type ispProvider struct {
	Name string
	// Fallback=true 的渠道不参与轮询起点的随机选择，
	// 只有在所有非兜底渠道都失败之后才会被尝试。
	// 目前只有 proxycheck.io 是 Fallback：它不支持"留空查自己"，
	// 每次调用都要多打一次 checkip.amazonaws.com，成本比其他渠道高。
	Fallback bool
	Enabled  func() bool
	Fetch    func(ctx context.Context) (*ISPInfo, error)
}

// rrCounter 用于在多个已启用的渠道之间做轮询，
// 让每个渠道分摊到大致相等的调用量，从而把各家的每日免费额度叠加起来用。
var rrCounter atomic.Uint64

// buildProviders 接收配置，返回一个闭包函数
func (c *Client) buildProviders(ip string) []ispProvider {
	cfg := c.ispCfg // 直接读取 Client 内部的配置

	return []ispProvider{
		{
			Name:    "ipapi.is",
			Enabled: func() bool { return cfg.ISPCheckAPIKeyIPAPI != "" },
			Fetch: func(ctx context.Context) (*ISPInfo, error) {
				return c.fetchIPAPIIs(ctx, cfg.ISPCheckAPIKeyIPAPI, ip)
			},
		},
		{
			Name:    "ipdata.co",
			Enabled: func() bool { return cfg.ISPCheckAPIKeyIPData != "" },
			Fetch: func(ctx context.Context) (*ISPInfo, error) {
				return c.fetchIPData(ctx, cfg.ISPCheckAPIKeyIPData, ip)
			},
		},
		{
			Name:     "proxycheck.io",
			Fallback: true,
			Enabled:  func() bool { return cfg.ISPCheckAPIKeyProxyCheck != "" },
			Fetch: func(ctx context.Context) (*ISPInfo, error) {
				return c.fetchProxyCheck(ctx, cfg.ISPCheckAPIKeyProxyCheck, ip)
			},
		},
	}
}

// buildProvidersCF 接收配置，返回闭包函数
func (c *Client) buildProvidersCF(ip string) []ispProvider {
	cfg := c.ispCfg

	return []ispProvider{
		{
			Name:    "iplocate.io",
			Enabled: func() bool { return cfg.ISPCheckAPIKeyIPLocate != "" },
			Fetch: func(ctx context.Context) (*ISPInfo, error) {
				return c.fetchIPLocate(ctx, cfg.ISPCheckAPIKeyIPLocate, ip)
			},
		},
	}
}

// 遍历渠道，返回第一个完整分类结果；
// 如果只有国家代码则记录下来，供兜底使用。
func (c *Client) tryProviders(ctx context.Context, providers []ispProvider, bestPartial **ISPInfo) *ISPInfo {
	for _, p := range providers {
		info, err := p.Fetch(ctx)
		if err != nil || info == nil {
			continue
		}
		if info.Details != "" {
			return info
		}
		if info.CountryCode != "" && *bestPartial == nil {
			*bestPartial = info
		}
	}
	return nil
}

// 把渠道拆分成主力和兜底
func splitProviders(all []ispProvider) (primary []ispProvider, fallback []ispProvider) {
	primary = make([]ispProvider, 0, len(all))
	fallback = make([]ispProvider, 0, len(all))
	for _, p := range all {
		if !p.Enabled() {
			continue
		}
		if p.Fallback {
			fallback = append(fallback, p)
		} else {
			primary = append(primary, p)
		}
	}
	return
}

// 按 rrCounter 做轮询偏移
func rotateProviders(primary []ispProvider) []ispProvider {
	start := int(rrCounter.Add(1) % uint64(len(primary)))
	rotated := make([]ispProvider, len(primary))
	for i := range primary {
		rotated[i] = primary[(start+i)%len(primary)]
	}
	return rotated
}

// queryISPDetail 是内部核心遍历逻辑：按"主力轮询 + 兜底"的顺序依次尝试各渠道。
//
// 返回值的选取规则：
//  1. 优先返回第一个成功归类出 ISP 类型的结果（Details 非空）——这是最理想的情况；
//  2. 如果所有渠道都没能归类出 ISP 类型，但其中至少有一个渠道返回了国家代码，
//     就把"只有国家代码"的这个结果留作兜底返回，而不是直接判定为失败——
//     对于只想要国家代码的调用方（比如 updateProxyName）来说，这依然是有效数据；
//  3. 只有当所有渠道都请求失败、或者压根没有任何可用数据时，才返回 nil。
func (c *Client) queryISPDetail(ip string) *ISPInfo {
	if !c.ispCfg.ISPCheck {
		return nil
	}

	slog.Debug("queryISPDetail", "ip", ip, "ispCheck", c.ispCfg.ISPCheck, "ispTimeout", c.ispCfg.ISPTimeout, "ispAPIKeyIPAPI", c.ispCfg.ISPCheckAPIKeyIPAPI, "ispAPIKeyProxyCheck", c.ispCfg.ISPCheckAPIKeyProxyCheck, "ispAPIKeyIPLocate", c.ispCfg.ISPCheckAPIKeyIPLocate, "ispAPIKeyIPData", c.ispCfg.ISPCheckAPIKeyIPData)
	all := c.buildProviders(ip)
	cf := c.buildProvidersCF(ip)

	primary, fallback := splitProviders(all)
	if len(primary) == 0 && len(fallback) == 0 {
		return nil
	}

	if c.ispCfg.ISPTimeout <= 0 {
		c.ispCfg.ISPTimeout = 5 * time.Second
	}
	
	ctx, cancel := context.WithTimeout(context.Background(), c.ispCfg.ISPTimeout)
	defer cancel()

	var bestPartial *ISPInfo

	if ip != "" {
		if info := c.tryProviders(ctx, cf, &bestPartial); info != nil {
			return info
		}
	}

	if len(primary) > 0 {
		rotated := rotateProviders(primary)
		if info := c.tryProviders(ctx, rotated, &bestPartial); info != nil {
			return info
		}
	}

	if info := c.tryProviders(ctx, fallback, &bestPartial); info != nil {
		return info
	}

	return bestPartial
}

// GetISPInfo 是外部调用入口（保持原有签名，兼容现有调用点）：
// 返回格式化好的 "[原生|机房]" 标签文本；没能归类出 ISP 类型时返回空字符串
// —— 即便某个渠道只查到了国家代码，这里也不会用它拼标签，避免出现
// "[原生]"这种看不出具体类型、容易让人误解的标签。
func (c *Client) GetISPInfo(ip string) string {
	slog.Debug("GetISPInfo", "ip", ip, "ispCheck", c.ispCfg.ISPCheck, "ispTimeout", c.ispCfg.ISPTimeout, "ispAPIKeyIPAPI", c.ispCfg.ISPCheckAPIKeyIPAPI, "ispAPIKeyProxyCheck", c.ispCfg.ISPCheckAPIKeyProxyCheck, "ispAPIKeyIPLocate", c.ispCfg.ISPCheckAPIKeyIPLocate, "ispAPIKeyIPData", c.ispCfg.ISPCheckAPIKeyIPData)
	if !c.ispCfg.ISPCheck {
		return ""
	}
	info := c.queryISPDetail(ip)
	if info == nil || info.Details == "" {
		return ""
	}
	return info.formatISPInfo()
}

// GetISPInfoCFfallback 只使用 cf 渠道，且 ip 为空时调用。
// 返回格式化好的标签文本；如果没有完整分类则返回空字符串。
func (c *Client) GetISPInfoCFfallback() (ispTag, countryCode string) {
	if !c.ispCfg.ISPCheck {
		return "", ""
	}
	if c.ispCfg.ISPTimeout <= 0 {
		c.ispCfg.ISPTimeout = 5 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), c.ispCfg.ISPTimeout)
	defer cancel()

	cf := c.buildProvidersCF("") // 内部直接使用 c
	var bestPartial *ISPInfo

	info := c.tryProviders(ctx, cf, &bestPartial)

	// 修复：如果 info 为 nil，直接返回，避免 info.CountryCode 触发 panic
	if info == nil {
		return "", ""
	}

	if info.Details == "" {
		return "", info.CountryCode
	}

	return info.formatISPInfo(), info.CountryCode
}

// GetISPDetail 返回完整的 *IPInfo（含 CountryCode），供 updateProxyName 这类
// 需要国家代码兜底的场景使用。没有任何渠道启用、或全部请求失败时返回 nil。
//
// 典型用法：GetProxyCountry 没拿到国家代码时，调用本函数，
// 用 info.CountryCode（非空时）作为节点标签里国家代码的兜底来源——
// 前提是该渠道配置了 apikey，数据准确度通常比免费 GeoIP 库更高。
func (c *Client) GetISPDetail(ip string) *ISPInfo {
	return c.queryISPDetail(ip)
}

func (info *ISPInfo) formatISPInfo() string {
	parts := make([]string, 0, 2)
	if info.IsNative {
		parts = append(parts, "原生")
	} else {
		parts = append(parts, "广播")
	}
	if info.Details != "" {
		parts = append(parts, info.Details)
	}
	return "[" + strings.Join(parts, "|") + "]"
}

// ============================================================
// 公共小工具
// ============================================================

func (c *Client) doGet(ctx context.Context, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "subs-check-pro/isp")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}
	return body, nil
}

// getOwnPublicIP 用于 proxycheck.io 这类"必须把 IP 显式写在 URL 路径里"的渠道。
// ipapi.is / iplocate.io / ipdata.co 都支持"不传 ip 参数即查询请求方自己的出口 IP"，
// 但 proxycheck.io 的路径参数是必填的，所以需要先拿到自己的公网 IP。
//
// 用 checkip.amazonaws.com：响应体就是纯文本 IP，没有多余字段需要解析，
// 背靠 AWS 的基础设施，可用性比大多数小型第三方 IP 查询服务更有保障。
func (c *Client) getOwnPublicIP(ctx context.Context) (string, error) {
	body, err := c.doGet(ctx, "https://checkip.amazonaws.com")
	if err != nil {
		return "", fmt.Errorf("获取出口 IP 失败: %w", err)
	}
	ip := strings.TrimSpace(string(body))
	if ip == "" {
		return "", fmt.Errorf("获取出口 IP 失败: 空响应")
	}
	return ip, nil
}
