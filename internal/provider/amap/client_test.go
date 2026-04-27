package amap

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolvePlaceMergesInputTipsPOIAndVerifiesCandidates(t *testing.T) {
	seen := make(map[string]int)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen[r.URL.Path]++
		switch r.URL.Path {
		case "/assistant/inputtips":
			if r.URL.Query().Get("citylimit") != "true" || r.URL.Query().Get("datatype") != "all" {
				t.Fatalf("unexpected inputtips query: %s", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`{"status":"1","info":"OK","tips":[{"name":"小家公寓","address":"仑头村仑头路82号","district":"海珠区","location":"113.1,23.1"}]}`))
		case "/place/text":
			if r.URL.Query().Get("keywords") != "小家公寓" || r.URL.Query().Get("citylimit") != "true" {
				t.Fatalf("unexpected poi query: %s", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`{"status":"1","info":"OK","pois":[{"name":"天河体育中心","address":"体育西路","adname":"天河区","cityname":"广州市","location":"113.2,23.2"}]}`))
		case "/geocode/geo":
			_, _ = w.Write([]byte(`{"status":"1","info":"OK","geocodes":[{"formatted_address":"广东省广州市海珠区仑头村仑头路82号小家公寓","level":"兴趣点","location":"113.1,23.1"}]}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := New(Config{
		APIKey:  "amap-key",
		BaseURL: server.URL,
		City:    "广州",
		Timeout: time.Second,
	})
	result, err := client.ResolvePlace(context.Background(), "小家公寓")
	require.NoError(t, err)

	assert.Equal(t, 1, seen["/assistant/inputtips"])
	assert.Equal(t, 1, seen["/place/text"])
	assert.Equal(t, 2, seen["/geocode/geo"])
	require.True(t, result.Found)
	require.NotNil(t, result.Best)
	assert.Equal(t, "小家公寓", result.Best.Name)
	assert.Equal(t, "input_tips", result.Best.Source)
	assert.True(t, result.Best.PrecisionOK)
	assert.Len(t, result.Candidates, 2)
	assert.Len(t, result.Tips, 1)
	assert.Len(t, result.POIs, 1)
}

func TestResolvePlaceReturnsUnconfiguredError(t *testing.T) {
	client := New(Config{})
	result, err := client.ResolvePlace(context.Background(), "小家公寓")
	require.NoError(t, err)
	assert.Equal(t, "未配置 AMAP_KEY", result.Error)
}

func TestResolvePlaceCapsCandidateVerification(t *testing.T) {
	geocodeCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/assistant/inputtips":
			_, _ = w.Write([]byte(`{"status":"1","info":"OK","tips":[` +
				`{"name":"地点1","address":"地址1号","district":"海珠区"},` +
				`{"name":"地点2","address":"地址2号","district":"海珠区"},` +
				`{"name":"地点3","address":"地址3号","district":"海珠区"},` +
				`{"name":"地点4","address":"地址4号","district":"海珠区"},` +
				`{"name":"地点5","address":"地址5号","district":"海珠区"}` +
				`]}`))
		case "/place/text":
			_, _ = w.Write([]byte(`{"status":"1","info":"OK","pois":[` +
				`{"name":"地点6","address":"地址6号","adname":"海珠区"},` +
				`{"name":"地点7","address":"地址7号","adname":"海珠区"},` +
				`{"name":"地点8","address":"地址8号","adname":"海珠区"}` +
				`]}`))
		case "/geocode/geo":
			geocodeCalls++
			_, _ = w.Write([]byte(`{"status":"1","info":"OK","geocodes":[{"formatted_address":"广东省广州市海珠区地址1号地点1","level":"兴趣点"}]}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := New(Config{
		APIKey:  "amap-key",
		BaseURL: server.URL,
		Timeout: time.Second,
	})
	result, err := client.ResolvePlace(context.Background(), "地点")
	require.NoError(t, err)

	assert.True(t, result.Found)
	assert.Equal(t, defaultVerifyCandidateLimit, geocodeCalls)
}

func TestResolvePlaceHonorsTotalTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		_, _ = w.Write([]byte(`{"status":"1","info":"OK"}`))
	}))
	defer server.Close()

	client := New(Config{
		APIKey:  "amap-key",
		BaseURL: server.URL,
		Timeout: 20 * time.Millisecond,
	})
	startedAt := time.Now()
	_, err := client.ResolvePlace(context.Background(), "小家公寓")
	elapsed := time.Since(startedAt)
	require.Error(t, err, "expected timeout error")
	assert.LessOrEqual(t, elapsed, 150*time.Millisecond, "resolve should honor total timeout")
}
