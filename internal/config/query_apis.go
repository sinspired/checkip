package config

// 使用了 cloudflare CDN 的地址，用于拼接 /cdn-cgi/trace 获取 CDN 节点位置
var CF_CDN_APIS = []string{
	"https://4.ipw.cn",
	"https://www.cloudflare.com",
	"https://api.ipify.org",
	"https://ip.122911.xyz",
	"https://6.iplark.com",
	"https://ifconfig.co",
	"https://api.ip2location.io",
	"https://api.ip.sb/",
	"https://realip.cc",
	"https://ipapi.co",
	"https://free.freeipapi.com",
	"https://api.myip.com",
	"https://api.ipbase.com",
	"https://api.ipquery.io",
}

var IP_APIS = []string{
	"http://checkip.amazonaws.com",
	"https://checkip.global.api.aws",
	"https://check.torproject.org/api/ip",
	"http://whatismyip.akamai.com",

	"https://4.tnedi.me/ip",
	"https://6.ident.me/ip",
	"https://ipv4.seeip.org/ip",
	"https://ipv6.seeip.org/ip",
	"https://ip4.me/api/",
	"https://ip6.me/api/",
	"https://ipv4.my.ipinfo.app/api/ipDetails.php",
	"https://ipv6.my.ipinfo.app/api/ipDetails.php",
	"https://ipv6.wtfismyip.com/text", //名字很有趣
	"https://myip.wtf/json",           //名字很有趣

	"https://checkip.info/ip",
	"https://checkip.dns.he.net/",
	"https://httpbin.org/ip",
	"http://checkip.dyndns.com/",
	"https://api.vore.top/api/IPdata",
	"https://api.ipapi.is/ip",
	"http://ifconfig.me/ip",
	"https://ipinfo.io/ip",
	"https://freedns.afraid.org/dynamic/check.php",

	"https://test.ipw.cn/",               // IPv4使用了 CFCDN, IPv6 位置准确
	"https://6.ipw.cn/",                  // IPv4使用了 CFCDN, IPv6 位置准确
	"https://api6.ipify.org?format=json", // IPv4使用了 CFCDN, IPv6 位置准确

	// 国内大厂接口
	"https://qifu-api.baidubce.com/ip/local/geo/v1/district",
	"https://r.inews.qq.com/api/ip2city",
	"https://g3.letv.com/r?format=1",
	"https://cdid.c-ctrip.com/model-poc2/h",
	"https://whois.pconline.com.cn/ipJson.jsp",
	"https://api.live.bilibili.com/xlive/web-room/v1/index/getIpInfo",

	// 位置不准,ip倒是准的,访问不稳定
	// "https://geolocation-db.com/json/",
}

var GEOIP_APIS = []string{
	"https://4.ident.me/json",
	"https://4.tnedi.me/json",
	"https://ident.me/json",
	"https://tnedi.me/json",
	"https://a.ident.me/json",
	"https://api.seeip.org/geoip",
	"https://api.ipapi.is",
	"https://checkip.info/json",
	"https://ip-api.io/json",
	"https://ip-api.io/api/v1/ip",
	"http://ip-api.com/json",
	"https://ipwhois.app/json/",
	"https://ipapi.co/json",
	// "https://ipinfo.io/json", // 准确,免费速率限制
}
