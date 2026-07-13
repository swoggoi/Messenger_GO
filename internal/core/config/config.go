package config

import "os"

type Config struct {
	Port      string
	DBSource  string
	JWTSecret string
}

func NewConfigMust() *Config {
	cfg := &Config{
		Port:      getEnv("PORT", "8080"),
		DBSource:  os.Getenv("DB_SOURCE"),
		JWTSecret: os.Getenv("JWT_SECRET"),
	}

	if cfg.DBSource == "" {
		panic("DB_SOURCE environment variable is required (set it in .env file)")
	}

	if cfg.JWTSecret == "" {
		panic("JWT_SECRET environment variable is required (set it in .env file)")
	}

	return cfg
}

func getEnv(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}
