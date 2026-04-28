package config

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateAllowsMissingAuthTokensInDev(t *testing.T) {
	cfg := validConfig()
	cfg.AppEnv = "dev"
	cfg.InternalAPIToken = ""
	cfg.AICCSCallbackToken = ""
	cfg.GatewayAuthToken = ""

	require.NoError(t, cfg.Validate())
}

func TestValidateRequiresAuthTokensOutsideDev(t *testing.T) {
	cfg := validConfig()
	cfg.AppEnv = "prod"
	cfg.InternalAPIToken = ""
	cfg.AICCSCallbackToken = ""
	cfg.GatewayAuthToken = ""

	err := cfg.Validate()

	require.Error(t, err)
	message := err.Error()
	assert.True(t, strings.Contains(message, "INTERNAL_API_TOKEN"), message)
	assert.True(t, strings.Contains(message, "AICCS_CALLBACK_TOKEN"), message)
	assert.True(t, strings.Contains(message, "GATEWAY_AUTH_TOKEN"), message)
}

func validConfig() Config {
	return Config{
		AppEnv:                "prod",
		InternalAPIToken:      "internal-secret",
		AICCSCallbackToken:    "callback-secret",
		GatewayAuthToken:      "gateway-secret",
		AliyunAccessKeyID:     "aliyun-id",
		AliyunAccessKeySecret: "aliyun-secret",
		AICCSAppCode:          "app-code",
		CallerNumber:          "10086",
		PGHost:                "127.0.0.1",
		PGUser:                "postgres",
		PGDatabase:            "chatbot",
	}
}
