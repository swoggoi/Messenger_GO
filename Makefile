APP_NAME := messenger
CMD_PATH := ./cmd/messenger
MIGRATIONS_PATH := ./migrations
DOCKER_IMAGE := $(APP_NAME)-app
DOCKER_COMPOSE := docker compose

.PHONY: all run dev build test test-cover clean deps fmt lint migrate-up migrate-down migrate-force migrate-go-up migrate-go-down migrate-go-status migrate-go-force swagger docker-build docker-up docker-down docker-logs docker-clean help

all: help

help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[32m%-20s\033[0m %s\n", $$1, $$2}'

run:
	go run $(CMD_PATH)

dev:
	@if ! command -v air &> /dev/null; then go install github.com/air-verse/air@latest; fi
	air

build:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o bin/$(APP_NAME) $(CMD_PATH)

test:
	go test ./... -v -race

test-cover:
	go test ./... -coverprofile=coverage.out
	go tool cover -html=coverage.out -o coverage.html

clean:
	rm -rf bin/ coverage.out coverage.html

deps:
	go mod download
	go mod tidy

fmt:
	gofmt -s -w .

lint:
	@if ! command -v golangci-lint &> /dev/null; then go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest; fi
	golangci-lint run ./...

migrate-up:
	migrate -path $(MIGRATIONS_PATH) -database "$(DB_SOURCE)" up

migrate-down:
	migrate -path $(MIGRATIONS_PATH) -database "$(DB_SOURCE)" down 1

migrate-force:
	migrate -path $(MIGRATIONS_PATH) -database "$(DB_SOURCE)" force $(v)

# Воспроизводимый путь без установленного CLI migrate (используется start.bat).
migrate-go-up:
	go run ./cmd/migrate -path $(MIGRATIONS_PATH) up

migrate-go-down:
	go run ./cmd/migrate -path $(MIGRATIONS_PATH) down 1

migrate-go-status:
	go run ./cmd/migrate -path $(MIGRATIONS_PATH) status

migrate-go-force:
	go run ./cmd/migrate -path $(MIGRATIONS_PATH) force $(v)

swagger:
	swag init -g $(CMD_PATH)/main.go -o ./docs

docker-build:
	docker build -t $(DOCKER_IMAGE):latest .

docker-up:
	$(DOCKER_COMPOSE) up -d --build

docker-down:
	$(DOCKER_COMPOSE) down

docker-logs:
	$(DOCKER_COMPOSE) logs -f app

docker-clean:
	$(DOCKER_COMPOSE) down -v --rmi all