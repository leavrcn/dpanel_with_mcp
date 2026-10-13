package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newTestClient 起一个 httptest server 模拟 DPanel 面板并返回指向它的 Client。
// login 直接返回 token，业务路径由 handler 处理。
func newTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/dpanel/api/common/user/login", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"data":{"token":"test-jwt"}}`))
	})
	mux.HandleFunc("/dpanel/api/test/path", handler)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return New(srv.URL, "u", "p", true, false, 10)
}

// TestPostJSONEnvelope JSON 信封：code=0/200 时返回 data。
func TestPostJSONEnvelope(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":200,"data":{"list":[1,2,3]}}`))
	})
	data, err := c.Post(context.Background(), "/test/path", nil)
	if err != nil {
		t.Fatalf("JSON 信封应成功: %v", err)
	}
	m, ok := data.(map[string]any)
	if !ok {
		t.Fatalf("应返回 map，实际 %T", data)
	}
	if list, ok := m["list"].([]any); !ok || len(list) != 3 {
		t.Errorf("data.list 应有 3 项，实际 %v", m["list"])
	}
}

// TestPostTextStream 非 JSON 200 响应（导出/下载类 attachment 流）：
// 必须以 TextPayload 透传（Content + ContentType），不报「响应解析失败」。
func TestPostTextStream(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("line1\nline2\n"))
	})
	data, err := c.Post(context.Background(), "/test/path", nil)
	if err != nil {
		t.Fatalf("text/plain 流应透传而非报错: %v", err)
	}
	tp, ok := data.(TextPayload)
	if !ok {
		t.Fatalf("应返回 TextPayload，实际 %T: %v", data, data)
	}
	if tp.Content != "line1\nline2\n" {
		t.Errorf("Content 应为原文，实际 %q", tp.Content)
	}
	if !strings.HasPrefix(tp.ContentType, "text/plain") {
		t.Errorf("ContentType 应透传，实际 %q", tp.ContentType)
	}
}

// TestPostBinaryStream 二进制流（zip/octet-stream）同样透传。
func TestPostBinaryStream(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		_, _ = w.Write([]byte{0x50, 0x4b, 0x03, 0x04, 0xff, 0xfe})
	})
	data, err := c.Post(context.Background(), "/test/path", nil)
	if err != nil {
		t.Fatalf("二进制流应透传而非报错: %v", err)
	}
	tp, ok := data.(TextPayload)
	if !ok {
		t.Fatalf("应返回 TextPayload，实际 %T", data)
	}
	if !strings.Contains(tp.ContentType, "zip") {
		t.Errorf("ContentType 应为 zip，实际 %q", tp.ContentType)
	}
}

// TestPostHTMLFallback 200 + HTML（SPA 兜底）必须显式报错，不能当流透传。
func TestPostHTMLFallback(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte("<html><body>index</body></html>"))
	})
	_, err := c.Post(context.Background(), "/test/path", nil)
	if err == nil {
		t.Fatal("HTML 响应应报错（endpoint not found 诊断）")
	}
	if !strings.Contains(err.Error(), "endpoint not found") {
		t.Errorf("错误应含 endpoint not found 诊断，实际 %v", err)
	}
}

// TestPostErrorEnvelope 业务错误信封（code!=0/200）：提取 message/error 字段。
func TestPostErrorEnvelope(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":500,"message":"fd not found"}`))
	})
	_, err := c.Post(context.Background(), "/test/path", nil)
	if err == nil {
		t.Fatal("业务错误应报错")
	}
	if !strings.Contains(err.Error(), "fd not found") {
		t.Errorf("错误应透传 message，实际 %v", err)
	}
}

// TestPostNoContentType 无 Content-Type 的 JSON 体（Go server 嗅探为 text/plain）：
// 按文本透传（黑名单判定下不误伤——上游 JSON 端点都显式设 CT）。
func TestPostNoContentType(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		// 不设 Content-Type：net/http 会嗅探为 text/plain; charset=utf-8
		_, _ = w.Write([]byte(`{"code":200,"data":"ok"}`))
	})
	data, err := c.Post(context.Background(), "/test/path", nil)
	if err != nil {
		t.Fatalf("无 CT 的 JSON 体不应报错: %v", err)
	}
	// 当前实现按文本透传（非 JSON CT）；确认拿到的是 TextPayload 而非解析失败
	if _, ok := data.(TextPayload); !ok {
		// 若未来改为先尝试 JSON 解析，这里返回 map 也算通过
		var m map[string]any
		if b, err := json.Marshal(data); err == nil && json.Unmarshal(b, &m) == nil {
			return
		}
		t.Errorf("应为 TextPayload 或可解析 JSON，实际 %T: %v", data, data)
	}
}
