.PHONY: run build seed migrate-up migrate-down sqlc-gen fmt vet tidy test

APP        := tracker
DB_DSN     ?= $(shell grep -E '^MIGRATION_DATABASE_URL=' .env 2>/dev/null | cut -d= -f2-)
MIGRATIONS := db/migrations

run:
	go run ./cmd/server

build:
	go build -o bin/$(APP) ./cmd/server

seed:
	go run ./cmd/seed

migrate-up:
	migrate -path $(MIGRATIONS) -database "$(DB_DSN)" up

migrate-down:
	migrate -path $(MIGRATIONS) -database "$(DB_DSN)" down 1

sqlc-gen:
	cd db && sqlc generate

fmt:
	go fmt ./...

vet:
	go vet ./...

tidy:
	go mod tidy

test:
	go test ./...
