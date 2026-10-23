.PHONY: run test

run:
	go run ./cmd/server

build:
	go build -o bin/server ./cmd/server

test:
	go test ./internal/...