package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	AppPort                   string
	AppEnv                    string
	AppLogLevel               string
	PostgresDSN               string
	RedisAddr                 string
	FeishuAppID               string
	FeishuAppSecret           string
	FeishuVerificationToken   string
	FeishuEncryptKey          string
	FeishuDocBaseURL          string
	LLMEnabled                bool
	LLMBaseURL                string
	LLMAPIKey                 string
	LLMModel                  string
	LLMTimeoutMS              int
	LLMMaxTokens              int
	LLMFallbackEnabled        bool
	AutoClassifyTop1Threshold float64
	AutoClassifyGapThreshold  float64
	SoftDeleteRetentionDays   int
	AutoMigrate               bool
}

func Load() (Config, error) {
	cfg := Config{
		AppPort:                 getEnv("APP_PORT", "8080"),
		AppEnv:                  getEnv("APP_ENV", "development"),
		AppLogLevel:             getEnv("APP_LOG_LEVEL", "info"),
		PostgresDSN:             os.Getenv("POSTGRES_DSN"),
		RedisAddr:               getEnv("REDIS_ADDR", "127.0.0.1:6379"),
		FeishuAppID:             os.Getenv("FEISHU_APP_ID"),
		FeishuAppSecret:         os.Getenv("FEISHU_APP_SECRET"),
		FeishuVerificationToken: os.Getenv("FEISHU_VERIFICATION_TOKEN"),
		FeishuEncryptKey:        os.Getenv("FEISHU_ENCRYPT_KEY"),
		FeishuDocBaseURL:        getEnv("FEISHU_DOC_BASE_URL", "https://www.feishu.cn/docx"),
		LLMBaseURL:              os.Getenv("LLM_BASE_URL"),
		LLMAPIKey:               os.Getenv("LLM_API_KEY"),
		LLMModel:                os.Getenv("LLM_MODEL"),
	}

	if cfg.PostgresDSN == "" {
		return cfg, fmt.Errorf("POSTGRES_DSN is required")
	}

	var err error
	cfg.LLMEnabled, err = getEnvBool("LLM_ENABLED", false)
	if err != nil {
		return cfg, err
	}
	cfg.LLMTimeoutMS, err = getEnvInt("LLM_TIMEOUT_MS", 8000)
	if err != nil {
		return cfg, err
	}
	cfg.LLMMaxTokens, err = getEnvInt("LLM_MAX_TOKENS", 1200)
	if err != nil {
		return cfg, err
	}
	cfg.LLMFallbackEnabled, err = getEnvBool("LLM_FALLBACK_ENABLED", true)
	if err != nil {
		return cfg, err
	}
	cfg.AutoClassifyTop1Threshold, err = getEnvFloat("AUTO_CLASSIFY_TOP1_THRESHOLD", 0.85)
	if err != nil {
		return cfg, err
	}
	cfg.AutoClassifyGapThreshold, err = getEnvFloat("AUTO_CLASSIFY_GAP_THRESHOLD", 0.15)
	if err != nil {
		return cfg, err
	}
	cfg.SoftDeleteRetentionDays, err = getEnvInt("SOFT_DELETE_RETENTION_DAYS", 30)
	if err != nil {
		return cfg, err
	}
	cfg.AutoMigrate, err = getEnvBool("AUTO_MIGRATE", true)
	if err != nil {
		return cfg, err
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	return strconv.Atoi(v)
}

func getEnvFloat(key string, fallback float64) (float64, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	return strconv.ParseFloat(v, 64)
}

func getEnvBool(key string, fallback bool) (bool, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	return strconv.ParseBool(v)
}
