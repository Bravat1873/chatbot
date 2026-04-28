// 配置层：从环境变量加载所有运行期参数，支持 .env 文件和默认值。
package config

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

// Config 聚合所有环境变量配置，字段名自描述。
type Config struct {
	AppEnv             string
	AppPort            int
	LogLevel           string
	InternalAPIToken   string
	AICCSCallbackToken string
	GatewayAuthToken   string
	LLMModel           string
	DashScopeAPIKey    string
	DashScopeBaseURL   string
	DashScopeTimeout   int
	AMapKey            string
	AMapBaseURL        string
	AMapCity           string
	AMapTimeout        int

	AliyunAccessKeyID     string
	AliyunAccessKeySecret string
	AICCSAppCode          string
	CallerNumber          string
	AICCSRegionID         string
	AICCSEndpoint         string
	SessionTimeoutSeconds int

	PGHost     string
	PGPort     int
	PGUser     string
	PGPassword string
	PGDatabase string
	PGSSLMode  string
}

// Load 从环境变量加载配置，并自动尝试读取 .env 文件。
func Load() (Config, error) {

	// godotenv 失败不阻塞，因为容器环境可能直接注入环境变量
	_ = godotenv.Load()

	cfg := Config{
		AppEnv:                getEnv("APP_ENV", "dev"),
		AppPort:               getEnvInt("APP_PORT", 8080),
		LogLevel:              getEnv("LOG_LEVEL", "info"),
		InternalAPIToken:      strings.TrimSpace(os.Getenv("INTERNAL_API_TOKEN")),
		AICCSCallbackToken:    strings.TrimSpace(os.Getenv("AICCS_CALLBACK_TOKEN")),
		GatewayAuthToken:      strings.TrimSpace(os.Getenv("GATEWAY_AUTH_TOKEN")),
		LLMModel:              getEnv("LLM_MODEL", "qwen3.5-flash"),
		DashScopeAPIKey:       strings.TrimSpace(os.Getenv("DASHSCOPE_API_KEY")),
		DashScopeBaseURL:      getEnv("DASHSCOPE_BASE_URL", "https://dashscope.aliyuncs.com/compatible-mode/v1"),
		DashScopeTimeout:      getEnvInt("DASHSCOPE_TIMEOUT_SECONDS", 3),
		AMapKey:               strings.TrimSpace(os.Getenv("AMAP_KEY")),
		AMapBaseURL:           getEnv("AMAP_BASE_URL", "https://restapi.amap.com/v3"),
		AMapCity:              getEnv("AMAP_CITY", ""),
		AMapTimeout:           getEnvInt("AMAP_TIMEOUT_SECONDS", 3),
		AliyunAccessKeyID:     strings.TrimSpace(os.Getenv("ALIYUN_ACCESS_KEY_ID")),
		AliyunAccessKeySecret: strings.TrimSpace(os.Getenv("ALIYUN_ACCESS_KEY_SECRET")),
		AICCSAppCode:          strings.TrimSpace(os.Getenv("AICCS_APP_CODE")),
		CallerNumber:          strings.TrimSpace(os.Getenv("CALLER_NUMBER")),
		AICCSRegionID:         getEnv("AICCS_REGION_ID", "cn-hangzhou"),
		AICCSEndpoint:         getEnv("AICCS_ENDPOINT", "aiccs.aliyuncs.com"),
		SessionTimeoutSeconds: getEnvInt("AICCS_SESSION_TIMEOUT", 1200),
		PGHost:                getEnv("PG_HOST", "127.0.0.1"),
		PGPort:                getEnvInt("PG_PORT", 5432),
		PGUser:                getEnv("PG_USER", "postgres"),
		PGPassword:            os.Getenv("PG_PASSWORD"),
		PGDatabase:            getEnv("PG_DATABASE", "postgres"),
		PGSSLMode:             getEnv("PG_SSLMODE", "disable"),
	}

	// 校验必填字段
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// Validate 检查必填配置项是否缺失。
func (c Config) Validate() error {
	missing := make([]string, 0)
	required := map[string]string{
		"ALIYUN_ACCESS_KEY_ID":     c.AliyunAccessKeyID,
		"ALIYUN_ACCESS_KEY_SECRET": c.AliyunAccessKeySecret,
		"AICCS_APP_CODE":           c.AICCSAppCode,
		"CALLER_NUMBER":            c.CallerNumber,
		"PG_HOST":                  c.PGHost,
		"PG_USER":                  c.PGUser,
		"PG_DATABASE":              c.PGDatabase,
	}
	if authTokensRequired(c.AppEnv) {
		required["INTERNAL_API_TOKEN"] = c.InternalAPIToken
		required["AICCS_CALLBACK_TOKEN"] = c.AICCSCallbackToken
		required["GATEWAY_AUTH_TOKEN"] = c.GatewayAuthToken
	}
	for key, value := range required {
		if strings.TrimSpace(value) == "" {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return fmt.Errorf("missing required env: %s", strings.Join(missing, ", "))
	}
	return nil
}

func authTokensRequired(appEnv string) bool {
	switch strings.ToLower(strings.TrimSpace(appEnv)) {
	case "", "dev", "development", "local", "test":
		return false
	default:
		return true
	}
}

// PostgresDSN 拼接 pgx 兼容的连接串。
func (c Config) PostgresDSN() string {
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=%s",
		c.PGUser,
		c.PGPassword,
		c.PGHost,
		c.PGPort,
		c.PGDatabase,
		c.PGSSLMode,
	)
}

// getEnv 读取环境变量，空值时回退到默认值。
func getEnv(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

// getEnvInt 读取整数类型环境变量，解析失败时回退到默认值。
func getEnvInt(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}
