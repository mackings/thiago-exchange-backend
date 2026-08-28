.PHONY: run build test up down

run:
	go run ./cmd/api

build:
	go build -o bin/api ./cmd/api

test:
	go test ./...

up:
	docker compose up -d

down:
	docker compose down
