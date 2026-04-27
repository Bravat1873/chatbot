package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHealthzReturnsOK(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/healthz", Healthz(nil))

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusOK, resp.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &body))
	assert.Equal(t, "OK", body["code"])
	assert.Equal(t, "success", body["message"])
	data := body["data"].(map[string]any)
	assert.Equal(t, "ok", data["status"])
}

func TestHealthzReturnsServiceUnavailableWhenCheckFails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/healthz", Healthz(func(context.Context) error { return errors.New("db down") }))

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusServiceUnavailable, resp.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &body))
	assert.Equal(t, "SERVICE_UNAVAILABLE", body["code"])
	assert.Equal(t, "service unavailable", body["message"])
}
