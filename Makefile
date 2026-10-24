.PHONY: run test dev-up

run:
	go run ./cmd/server

build:
	go build -o bin/server ./cmd/server

test:
	go test ./internal/...

dev-up:
	docker compose up -d
	docker compose exec ollama ollama pull nomic-embed-text
