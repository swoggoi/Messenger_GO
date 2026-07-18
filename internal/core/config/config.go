package config

import (
	"os"
	"strconv"
)

type Config struct {
	Port          string
	DBSource      string
	JWTSecret     string
	RedisAddr     string
	RedisPassword string
	RedisDB       int
	KafkaBrokers  []string
	KafkaTopic    string
}

func NewConfigMust() *Config {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	dbSource := os.Getenv("DB_SOURCE")
	if dbSource == "" {
		dbSource = "postgres://postgres:postgres@localhost:5432/messenger?sslmode=disable"
	}

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		jwtSecret = "super-secret-key"
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
		kafkaBrokers = []string{kb}
	}

	kafkaTopic := os.Getenv("KAFKA_TOPIC")
	if kafkaTopic == "" {
		kafkaTopic = "messenger-messages"
	}

	return &Config{
		Port:          port,
		DBSource:      dbSource,
		JWTSecret:     jwtSecret,
		RedisAddr:     redisAddr,
		RedisPassword: os.Getenv("REDIS_PASSWORD"),
		RedisDB:       redisDB,
		KafkaBrokers:  kafkaBrokers,
		KafkaTopic:    kafkaTopic,
	}
}
