.PHONY: run test dev-up dev-rst

run:
	go run ./cmd/server

build:
	go build -o bin/server ./cmd/server

test:
	go test ./internal/...

dev-up:
	docker compose up -d
	docker compose exec ollama ollama pull nomic-embed-text

dev-rst:
	docker compose down -v --remove-orphans