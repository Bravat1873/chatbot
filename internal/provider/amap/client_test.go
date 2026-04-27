package amap

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestResolvePlaceReturnsFirstPOI(t *testing.T) {
	var path string
	var keyword string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		keyword = r.URL.Query().Get("keywords")
		_, _ = w.Write([]byte(`{"status":"1","info":"OK","pois":[{"name":"小家公寓","address":"仑头村仑头路82号","adname":"海珠区"}]}`))
	}))
	defer server.Close()

	client := New(Config{
		APIKey:  "amap-key",
		BaseURL: server.URL,
		Timeout: time.Second,
	})
	result, err := client.ResolvePlace(context.Background(), "小家公寓")
	if err != nil {
		t.Fatalf("resolve place: %v", err)
	}

	if path != "/place/text" || keyword != "小家公寓" {
		t.Fatalf("unexpected request: path=%s keyword=%s", path, keyword)
	}
	if !result.Found || result.Best == nil {
		t.Fatalf("expected best result, got %#v", result)
	}
	if result.Best.Name != "小家公寓" || result.Best.Address != "仑头村仑头路82号" || result.Best.District != "海珠区" {
		t.Fatalf("unexpected candidate: %#v", result.Best)
	}
}
