package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestGatewayBearerRejectsBareAuthorizationToken(t *testing.T) {
	router := protectedRouter(GatewayBearer("secret"))

	resp := performAuthRequest(router, http.Header{"Authorization": []string{"secret"}}, "")

	assert.Equal(t, http.StatusUnauthorized, resp.Code)
}

func TestGatewayBearerAcceptsStrictBearerToken(t *testing.T) {
	router := protectedRouter(GatewayBearer("secret"))

	resp := performAuthRequest(router, http.Header{"Authorization": []string{"Bearer secret"}}, "")

	assert.Equal(t, http.StatusNoContent, resp.Code)
}

func TestInternalTokenAcceptsAuthorizationBearerToken(t *testing.T) {
	router := protectedRouter(InternalToken("secret"))

	resp := performAuthRequest(router, http.Header{"Authorization": []string{"Bearer secret"}}, "")

	assert.Equal(t, http.StatusNoContent, resp.Code)
}

func TestInternalTokenRejectsBareAuthorizationToken(t *testing.T) {
	router := protectedRouter(InternalToken("secret"))

	resp := performAuthRequest(router, http.Header{"Authorization": []string{"secret"}}, "")

	assert.Equal(t, http.StatusUnauthorized, resp.Code)
}

func TestInternalTokenRejectsLegacyHeader(t *testing.T) {
	router := protectedRouter(InternalToken("secret"))

	resp := performAuthRequest(router, http.Header{"X-Internal-Token": []string{"secret"}}, "")

	assert.Equal(t, http.StatusUnauthorized, resp.Code)
}

func TestCallbackTokenRecordsAuthMode(t *testing.T) {
	router := callbackRouter(CallbackToken("secret"))

	headerResp := performAuthRequest(router, http.Header{"X-Callback-Token": []string{"secret"}}, "")
	assert.Equal(t, http.StatusOK, headerResp.Code)
	assert.JSONEq(t, `{"auth_mode":"header"}`, headerResp.Body.String())

	queryResp := performAuthRequest(router, nil, "token=secret")
	assert.Equal(t, http.StatusOK, queryResp.Code)
	assert.JSONEq(t, `{"auth_mode":"query"}`, queryResp.Body.String())
}

func protectedRouter(middleware gin.HandlerFunc) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware)
	router.GET("/ok", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})
	return router
}

func callbackRouter(middleware gin.HandlerFunc) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware)
	router.GET("/ok", func(c *gin.Context) {
		authMode, _ := c.Get("auth_mode")
		c.JSON(http.StatusOK, gin.H{"auth_mode": authMode})
	})
	return router
}

func performAuthRequest(router *gin.Engine, headers http.Header, rawQuery string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/ok", nil)
	req.URL.RawQuery = rawQuery
	for key, values := range headers {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	return resp
}
