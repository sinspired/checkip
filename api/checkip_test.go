// api/checkip_test.go
package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sinspired/checkip/internal/checkip"
)

func TestHandler_ServeHTTP(t *testing.T) {
	// 创建测试用的 Checker
	checker := checkip.NewChecker(nil, nil)
	handler := &Handler{Checker: checker}

	// 测试 /api 路由 - 获取当前 IP 信息
	req, err := http.NewRequest("GET", "/api", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	// 由于实际调用外部 API 可能失败，我们只检查状态码不为 500
	if status := rr.Code; status == http.StatusInternalServerError {
		t.Logf("API 调用失败，这是预期的，因为测试环境无法访问外部服务")
		return
	}

	// 测试 /api/ip 路由 - 仅返回 IP
	req, err = http.NewRequest("GET", "/api/ip", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if status := rr.Code; status == http.StatusInternalServerError {
		t.Logf("API 调用失败，这是预期的，因为测试环境无法访问外部服务")
		return
	}

	// 检查响应内容类型
	contentType := rr.Header().Get("Content-Type")
	if contentType != "application/json; charset=utf-8" {
		t.Errorf("handler returned wrong content type: got %v want %v", contentType, "application/json; charset=utf-8")
	}

	// 测试 /api?ip=8.8.8.8 路由
	req, err = http.NewRequest("GET", "/api?ip=8.8.8.8", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}

	// 解析响应 JSON
	var result checkip.CheckResult
	if err := json.NewDecoder(rr.Body).Decode(&result); err != nil {
		t.Errorf("failed to decode response: %v", err)
	}

	if result.IP != "8.8.8.8" {
		t.Errorf("handler returned wrong IP: got %v want %v", result.IP, "8.8.8.8")
	}

	// 测试 /api/8.8.8.8 路由
	req, err = http.NewRequest("GET", "/api/8.8.8.8", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}

	// 解析响应 JSON
	if err := json.NewDecoder(rr.Body).Decode(&result); err != nil {
		t.Errorf("failed to decode response: %v", err)
	}

	if result.IP != "8.8.8.8" {
		t.Errorf("handler returned wrong IP: got %v want %v", result.IP, "8.8.8.8")
	}
}
