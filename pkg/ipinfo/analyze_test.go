package ipinfo

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/sinspired/checkip/internal/data"
)

func TestGetAnalyzed(t *testing.T) {
	client := &http.Client{}
	db, err := data.OpenMaxMindDB()
	if err != nil {
		t.Fatalf("打开 MaxMind 数据库失败: %v", err)
	}
	defer db.Close()

	cli, err := New(
		WithHttpClient(client),
		WithDBReader(db),
		WithIPAPIs(),
		WithGeoAPIs(),
	)
	if err != nil {
		t.Error("客户端初始化失败")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	loc, ip, countryCode_tag, err := cli.GetAnalyzed(ctx, "", "")
	if err != nil {
		t.Errorf("获取代理国家信息失败: %v", err)
	} else {
		t.Logf("位置: %s, IP: %s, 标签: %s", loc, ip, countryCode_tag)
		if loc == "" || ip == "" || countryCode_tag == "" {
			t.Error("获取的国家信息或IP地址不完整")
		}
	}
}
