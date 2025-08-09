package ipinfo

import (
	"net/http"
	"testing"

	"github.com/sinspired/checkip/internal/data"
)

func TestGetMixed(t *testing.T) {
	client := &http.Client{}
	db, err := data.OpenMaxMindDB()
	if err != nil {
		t.Fatalf("打开 MaxMind 数据库失败: %v", err)
	}
	defer db.Close()

	loc, ip, countryCode_tag, err := GetMixed(client, db)
	if err != nil {
		t.Errorf("获取代理国家信息失败: %v", err)
	} else {
		t.Logf("位置: %s, IP: %s, 标签: %s", loc, ip, countryCode_tag)
		if loc == "" || ip == "" || countryCode_tag == "" {
			t.Error("获取的国家信息或IP地址不完整")
		}
	}
}