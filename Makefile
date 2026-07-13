.PHONY: run build test clean

run:
	go run cmd/messenger/main.go

build:
	go build -o bin/messenger cmd/messenger/main.go

test:
	go test ./...

clean:
	rm -rf bin/

deps:
	go mod download
	go mod tidy