package ipinfo

import (
	"net"
	"net/netip"
	"testing"
)

var sampleText = `
    Here are some IPs: 192.168.1.1, fe80::1ff:fe23:4567:890a, 10.0.0.2, ::1, 172.16.254.1
    And some noise: abc.def.ghi.jkl, not-an-ip, 1234:5678:9abc:def0:1234:5678:9abc:def0
`

func extractIPRegex(text string) (ipv4s, ipv6s string) {
    ipv4s = reIPv4.FindString(text)
    ipv6s = reIPv6.FindString(text)
    return
}

func extractIPScan(text string) (ipv4s, ipv6s []string) {
    isIPChar := func(b byte) bool {
        return (b >= '0' && b <= '9') ||
            (b >= 'a' && b <= 'f') ||
            (b >= 'A' && b <= 'F') ||
            b == '.' || b == ':'
    }
    bs := []byte(text)
    for i := 0; i < len(bs); {
        if isIPChar(bs[i]) {
            j := i + 1
            for j < len(bs) && isIPChar(bs[j]) {
                j++
            }
            token := string(bs[i:j])
            if ip := net.ParseIP(token); ip != nil {
                if ip.To4() != nil {
                    ipv4s = append(ipv4s, ip.String())
                } else {
                    ipv6s = append(ipv6s, ip.String())
                }
            }
            i = j
        } else {
            i++
        }
    }
    return
}

func extractIPScanFast(text string) (ipv4, ipv6 string) {
    isHex := func(b byte) bool { return (b >= '0' && b <= '9') || (b|0x20 >= 'a' && b|0x20 <= 'f') }
    isDigit := func(b byte) bool { return b >= '0' && b <= '9' }

    n := len(text)
    for i := 0; i < n && (ipv4 == "" || ipv6 == ""); {
        c := text[i]
        // 只从可能是 IP 的起始字符开始（数字/冒号/点）
        if !(isDigit(c) || c == ':' || c == '.') {
            i++
            continue
        }
        j := i
        dots, colons := 0, 0
        // valid := true

        for j < n {
            ch := text[j]
            if ch == '.' {
                dots++
            } else if ch == ':' {
                colons++
            } else if !(isHex(ch)) {
                break
            }
            j++
        }

        // 只处理包含至少一个分隔符的 token
        if dots > 0 && colons == 0 {
            // IPv4 快速筛：最多 3 个点、长度不超过 15、仅数字和点
            if dots <= 3 && (j-i) <= 15 {
                token := text[i:j] // 零拷贝子串
                if addr, err := netip.ParseAddr(token); err == nil && addr.Is4() && ipv4 == "" {
                    ipv4 = addr.String()
                }
            }
        } else if colons > 0 && dots == 0 {
            // IPv6 快速筛：长度不超过 39（典型上限）
            if (j - i) <= 39 {
                token := text[i:j]
                if addr, err := netip.ParseAddr(token); err == nil && addr.Is6() && ipv6 == "" {
                    ipv6 = addr.String()
                }
            }
        }

        // 跳过当前 token；+1 跳过分隔符，防止卡在同一位置
        if j == i {
            i++
        } else {
            i = j + 1
        }
    }
    return
}

func BenchmarkRegex(b *testing.B) {
    for i := 0; i < b.N; i++ {
        extractIPRegex(sampleText)
    }
}

func BenchmarkScan(b *testing.B) {
    for i := 0; i < b.N; i++ {
        extractIPScan(sampleText)
    }
}

func BenchmarkScanFast(b *testing.B) {
    for i := 0; i < b.N; i++ {
        extractIPScanFast(sampleText)
    }
}
