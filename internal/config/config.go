package config

import (
	"os"
	"strconv"
	"strings"
)

type Config struct {
	// 9Router
	NineRouterBaseURL string
	NineRouterModel   string
	NineRouterAPIKey  string

	// Redis
	RedisURL string

	// SearXNG
	SearXNGBaseURL string

	// App
	Port     int
	Env      string
	LogLevel string
	DataDir  string

	// Bot behavior
	SelfRespon     bool
	AllowedNumbers []string // ["*"] = semua nomor
}

func Load() *Config {
	allowed := getAllowedNumbers("ALLOWED_NUMBERS", "*")

	return &Config{
	NineRouterBaseURL: getEnv("NINE_ROUTER_BASE_URL", "http://127.0.0.1:20128/v1"),
	NineRouterModel:   getEnv("NINE_ROUTER_MODEL", "kr/claude-sonnet-4.5"),
	NineRouterAPIKey:  getEnv("NINE_ROUTER_API_KEY", "sk-9router-key"),

		RedisURL: getEnv("REDIS_URL", "redis://127.0.0.1:6379"),

		SearXNGBaseURL: getEnv("SEARXNG_BASE_URL", "http://127.0.0.1:4000"),

		Port:     getEnvInt("PORT", 3000),
		Env:      getEnv("NODE_ENV", "development"),
		LogLevel: getEnv("LOG_LEVEL", "info"),
		DataDir:  getEnv("DATA_DIR", "."),

		SelfRespon:     getEnvBool("SELF_RESPON", false),
		AllowedNumbers: allowed,
	}
}

func getEnvBool(key string, fallback bool) bool {
	val := os.Getenv(key)
	if val == "" {
		return fallback
	}
	return val == "true" || val == "1" || val == "yes"
}

func getAllowedNumbers(key, fallback string) []string {
	val := os.Getenv(key)
	if val == "" {
		val = fallback
	}
	if val == "*" {
		return []string{"*"}
	}
	parts := strings.Split(val, ",")
	result := make([]string, len(parts))
	for i, p := range parts {
		result[i] = strings.TrimSpace(p)
	}
	return result
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if val := os.Getenv(key); val != "" {
		if n, err := strconv.Atoi(val); err == nil {
			return n
		}
	}
	return fallback
}
