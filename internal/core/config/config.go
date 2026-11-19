package config

import (
	"errors"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Port           string
	DBSource       string
	JWTSecret      string
	RedisAddr      string
	RedisPassword  string
	RedisDB        int
	KafkaBrokers   []string
	KafkaTopic     string
	AllowedOrigins []string
}

func NewConfig() (*Config, error) {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	dbSource := os.Getenv("DB_SOURCE")
	if dbSource == "" {
		dbSource = "postgres://messenger:messenger_pass@localhost:5432/messenger_db?sslmode=disable"
	}

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" || strings.Contains(jwtSecret, "REPLACE_WITH") {
		return nil, errors.New("JWT_SECRET must be set to a private random value (see .env.example)")
	}

	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}

	redisDB := 0
	if dbStr := os.Getenv("REDIS_DB"); dbStr != "" {
		redisDB, _ = strconv.Atoi(dbStr)
	}

	kafkaBrokers := []string{"localhost:9092"}
	if kb := os.Getenv("KAFKA_BROKERS"); kb != "" {
		for _, broker := range strings.Split(kb, ",") {
			if broker = strings.TrimSpace(broker); broker != "" {
				kafkaBrokers = []string{broker}
				break
			}
		}
	}

	kafkaTopic := os.Getenv("KAFKA_TOPIC")
	if kafkaTopic == "" {
		kafkaTopic = "messenger-messages"
	}

	allowedOrigins := []string{}
	if raw := os.Getenv("CORS_ALLOWED_ORIGINS"); raw != "" {
		for _, origin := range strings.Split(raw, ",") {
			if origin = strings.TrimSpace(origin); origin != "" {
				allowedOrigins = append(allowedOrigins, origin)
			}
		}
	}

	return &Config{
		Port:           port,
		DBSource:       dbSource,
		JWTSecret:      jwtSecret,
		RedisAddr:      redisAddr,
		RedisPassword:  os.Getenv("REDIS_PASSWORD"),
		RedisDB:        redisDB,
		KafkaBrokers:   kafkaBrokers,
		KafkaTopic:     kafkaTopic,
		AllowedOrigins: allowedOrigins,
	}, nil
}

func NewConfigMust() *Config {
	cfg, err := NewConfig()
	if err != nil {
		panic(err)
	}
	return cfg
}
