package config

import (
	"bufio"
	"errors"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	AppEnv                  string
	OpenRouterAPIKey        string
	OpenRouterPrimaryModel  string
	OpenRouterFallbackModel string
	OpenRouterCheapModel    string
	OpenRouterReceiptModel  string
	DatabaseURL             string
	JWTSecret               string
	Port                    string
	AllowedOrigins          []string
	MaxRequestBodyBytes     int64
}

func FromEnv() (Config, error) {
	cfg := Config{
		AppEnv:                  envOrDefault("APP_ENV", "development"),
		OpenRouterAPIKey:        strings.TrimSpace(os.Getenv("OPENROUTER_API_KEY")),
		OpenRouterPrimaryModel:  envOrDefault("OPENROUTER_PRIMARY_MODEL", "openai/gpt-oss-120b:free"),
		OpenRouterFallbackModel: envOrDefault("OPENROUTER_FALLBACK_MODEL", "qwen/qwen3-next-80b-a3b-instruct:free"),
		OpenRouterCheapModel:    envOrDefault("OPENROUTER_CHEAP_MODEL", "openai/gpt-oss-20b:free"),
		OpenRouterReceiptModel:  envOrDefault("OPENROUTER_RECEIPT_MODEL", "nvidia/nemotron-nano-12b-v2-vl:free"),
		DatabaseURL:             strings.TrimSpace(os.Getenv("DATABASE_URL")),
		JWTSecret:               strings.TrimSpace(os.Getenv("JWT_SECRET")),
		Port:                    envOrDefault("PORT", "8080"),
		AllowedOrigins:          splitCSV(envOrDefault("ALLOWED_ORIGINS", "*")),
		MaxRequestBodyBytes:     envInt64OrDefault("MAX_REQUEST_BODY_BYTES", 10<<20),
	}

	if cfg.OpenRouterAPIKey == "" {
		return Config{}, errors.New("OPENROUTER_API_KEY is required")
	}
	if cfg.DatabaseURL == "" {
		return Config{}, errors.New("DATABASE_URL is required")
	}
	if cfg.JWTSecret == "" {
		return Config{}, errors.New("JWT_SECRET is required")
	}
	if cfg.IsProduction() && allowsAllOrigins(cfg.AllowedOrigins) {
		return Config{}, errors.New("ALLOWED_ORIGINS must be set to your frontend origin in production")
	}
	if cfg.MaxRequestBodyBytes <= 0 {
		return Config{}, errors.New("MAX_REQUEST_BODY_BYTES must be greater than 0")
	}

	return cfg, nil
}

func (cfg Config) IsProduction() bool {
	return strings.EqualFold(cfg.AppEnv, "production")
}

func LoadDotEnv(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}

		key = strings.TrimSpace(key)
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		if key != "" {
			_ = os.Setenv(key, value)
		}
	}

	return scanner.Err()
}

func envOrDefault(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func envInt64OrDefault(key string, fallback int64) int64 {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			values = append(values, part)
		}
	}
	return values
}

func allowsAllOrigins(origins []string) bool {
	for _, origin := range origins {
		if origin == "*" {
			return true
		}
	}
	return len(origins) == 0
}
