.PHONY: run test dev-up dev-rst build proto

run:
	go run ./cmd/server

build:
	go build -o bin/server ./cmd/server

test:
	go test -race ./...

dev-up:
	docker compose up -d
	docker compose exec ollama ollama pull nomic-embed-text

dev-rst:
	docker compose down -v --remove-orphans

proto:
	protoc -I proto \
		--go_out=gen --go_opt=paths=source_relative \
		--go-grpc_out=gen --go-grpc_opt=paths=source_relative \
		proto/embeddings/v1/*.proto
